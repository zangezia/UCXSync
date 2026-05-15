package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
	"github.com/zangezia/UCXSync/internal/config"
	"github.com/zangezia/UCXSync/internal/ead"
	"github.com/zangezia/UCXSync/internal/monitor"
	"github.com/zangezia/UCXSync/internal/network"
	"github.com/zangezia/UCXSync/internal/report"
	"github.com/zangezia/UCXSync/internal/state"
	syncService "github.com/zangezia/UCXSync/internal/sync"
	"github.com/zangezia/UCXSync/pkg/models"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins in development
	},
}

const (
	defaultDataMountPoint = "/ucdata"
)

// Server represents the web server
type Server struct {
	cfg         *config.Config
	syncService *syncService.Service
	monService  *monitor.Service
	netService  *network.Service
	serviceName string
	stateStore  *state.Store
	webRoot     string
	httpClient  *http.Client

	mountSharesFunc          func() error
	checkSharesAvailability  func() []syncService.UnavailableShare
	checkNetworkRequirements func() error
	nowFunc                  func() time.Time
	setHostTimeFunc          func(time.Time) error
	syncHardwareClockFunc    func() error
	findProjectsFunc         func(context.Context) ([]models.ProjectInfo, error)
	getDestinationsFunc      func() []models.DestinationInfo
	getStatusFunc            func() models.SyncStatus
	ensureDestinationFunc    func(string) error
	checkDiskSpaceFunc       func(string) (syncService.DiskSpaceCheckResult, error)

	mu      sync.RWMutex
	clients map[*websocket.Conn]bool
}

func getServiceName() string {
	if serviceName := strings.TrimSpace(os.Getenv("UCXSYNC_SERVICE_NAME")); serviceName != "" {
		return serviceName
	}

	return "ucxsync"
}

// getWebRoot determines the web assets directory
func getWebRoot() string {
	// Try current directory first
	if _, err := os.Stat("web"); err == nil {
		return "web"
	}

	// Try installed location
	if _, err := os.Stat("/opt/ucxsync/web"); err == nil {
		return "/opt/ucxsync/web"
	}

	// Try executable directory
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		webPath := filepath.Join(exeDir, "web")
		if _, err := os.Stat(webPath); err == nil {
			return webPath
		}
	}

	// Default to current directory
	return "web"
}

// NewServer creates a new web server
func NewServer(cfg *config.Config) (*Server, error) {
	store, err := state.New(cfg.Database.Path, getServiceName())
	if err != nil {
		return nil, err
	}

	svc := syncService.New(
		cfg.Nodes,
		cfg.Shares,
		cfg.Network.MountRoot,
	)
	svc.SetServiceLoopInterval(cfg.Sync.ServiceLoopInterval)
	svc.SetDiskSpaceThresholds(cfg.Sync.MinFreeDiskSpace, cfg.Sync.DiskSpaceSafetyMargin)
	if err := svc.SetStateStore(store); err != nil {
		store.Close()
		return nil, err
	}
	svc.SetCopiedFileProcessor(ead.NewProcessor(store))

	monService := monitor.New(
		cfg.Monitoring.PerformanceUpdateInterval,
		cfg.Monitoring.CPUSmoothingSamples,
		cfg.Monitoring.MaxDiskThroughputMBps,
		cfg.Monitoring.NetworkSpeedBps,
	)

	netService := network.New(
		cfg.Nodes,
		cfg.Shares,
		cfg.Credentials.Username,
		cfg.Credentials.Password,
	)
	netService.SetBaseMountDir(cfg.Network.MountRoot)
	netService.SetMountOptions(cfg.Network.MountOptions)

	server := &Server{
		cfg:         cfg,
		syncService: svc,
		monService:  monService,
		netService:  netService,
		serviceName: getServiceName(),
		stateStore:  store,
		webRoot:     getWebRoot(),
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		clients: make(map[*websocket.Conn]bool),
	}

	server.mountSharesFunc = netService.MountAll
	server.checkSharesAvailability = svc.CheckSharesAvailability
	server.checkNetworkRequirements = network.CheckRequirements
	server.nowFunc = time.Now
	server.setHostTimeFunc = setSystemClock
	server.syncHardwareClockFunc = syncHardwareClock
	server.findProjectsFunc = svc.FindProjects
	server.getDestinationsFunc = server.getAvailableDestinations
	server.getStatusFunc = svc.GetStatus
	server.ensureDestinationFunc = svc.EnsureDestinationReady
	server.checkDiskSpaceFunc = svc.CheckDiskSpace

	return server, nil
}

// Start starts the web server
func (s *Server) Start(ctx context.Context) error {
	// Start performance monitoring
	metricsChan := s.monService.Start(ctx)
	go s.broadcastMetrics(ctx, metricsChan)

	// Setup routes
	mux := http.NewServeMux()

	// Static files
	staticPath := filepath.Join(s.webRoot, "static")
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(staticPath))))

	// API endpoints
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/projects", s.handleGetProjects)
	mux.HandleFunc("/api/destinations", s.handleGetDestinations)
	mux.HandleFunc("/api/devices", s.handleGetDevices)
	mux.HandleFunc("/api/devices/mount", s.handleMountDevice)
	mux.HandleFunc("/api/shares/mount", s.handleMountShares)
	mux.HandleFunc("/api/shares/check", s.handleCheckShares)
	mux.HandleFunc("/api/service/restart", s.handleRestartService)
	mux.HandleFunc("/api/host/time", s.handleHostTime)
	mux.HandleFunc("/api/host/time/sync", s.handleSyncHostTime)
	mux.HandleFunc("/api/host/shutdown", s.handleHostShutdown)
	mux.HandleFunc("/api/status", s.handleGetStatus)
	mux.HandleFunc("/api/project-stats", s.handleGetProjectStats)
	mux.HandleFunc("/api/project/report", s.handleDownloadProjectReport)
	mux.HandleFunc("/api/project/clear-history", s.handleClearProjectHistory)
	mux.HandleFunc("/api/database/projects", s.handleDatabaseProjects)
	mux.HandleFunc("/api/database/project", s.handleDatabaseProject)
	mux.HandleFunc("/api/metrics", s.handleGetMetrics)
	mux.HandleFunc("/api/preflight", s.handleGetPreflight)
	mux.HandleFunc("/api/sync/start", s.handleStartSync)
	mux.HandleFunc("/api/sync/stop", s.handleStopSync)
	mux.HandleFunc("/api/dashboard/project-stats", s.handleDashboardProjectStats)
	mux.HandleFunc("/api/dashboard/project/report", s.handleDownloadProjectReport)
	mux.HandleFunc("/api/dashboard/config", s.handleDashboardConfig)
	mux.HandleFunc("/api/dashboard/overview", s.handleDashboardOverview)
	mux.HandleFunc("/api/dashboard/preflight", s.handleDashboardPreflight)
	mux.HandleFunc("/api/dashboard/projects", s.handleDashboardProjects)
	mux.HandleFunc("/api/dashboard/destinations", s.handleDashboardDestinations)
	mux.HandleFunc("/api/dashboard/sync/start", s.handleDashboardStartSync)
	mux.HandleFunc("/api/dashboard/sync/stop", s.handleDashboardStopSync)
	mux.HandleFunc("/api/dashboard/shares/mount", s.handleDashboardMountShares)
	mux.HandleFunc("/api/dashboard/service/restart", s.handleDashboardRestartService)
	mux.HandleFunc("/ws", s.handleWebSocket)

	addr := fmt.Sprintf("%s:%d", s.cfg.Web.Host, s.cfg.Web.Port)
	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	// Start server in goroutine
	go func() {
		log.Info().Str("address", addr).Msg("Starting web server")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error().Err(err).Msg("Web server error")
		}
	}()

	go s.autoRemountShares(ctx)

	// Wait for context cancellation
	<-ctx.Done()

	// Graceful shutdown
	log.Info().Msg("Shutting down web server...")
	defer func() {
		if s.stateStore != nil {
			if err := s.stateStore.Close(); err != nil {
				log.Error().Err(err).Msg("Failed to close SQLite state store")
			}
		}
	}()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Stop sync
	s.syncService.Stop()

	// Unmount shares
	if err := s.netService.UnmountAll(); err != nil {
		log.Error().Err(err).Msg("Failed to unmount shares")
	}

	return server.Shutdown(shutdownCtx)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	indexPath := filepath.Join(s.webRoot, "templates", "index.html")
	http.ServeFile(w, r, indexPath)
}

func (s *Server) handleGetProjects(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	projects, err := s.findProjects(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Failed to find projects")
		http.Error(w, "Failed to find projects", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(projects)
}

func (s *Server) handleGetDestinations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	destinations := s.getAvailableDestinations()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(destinations)
}

func (s *Server) handleGetStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	status := s.currentSyncStatus()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (s *Server) handleGetPreflight(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	project := strings.TrimSpace(r.URL.Query().Get("project"))
	destination := strings.TrimSpace(r.URL.Query().Get("destination"))

	preflight := s.buildPreflightStatus(r.Context(), project, destination)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(preflight)
}

func (s *Server) handleDashboardPreflight(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !s.dashboardEnabled() {
		http.Error(w, "Dashboard mode is not configured", http.StatusNotFound)
		return
	}

	project := strings.TrimSpace(r.URL.Query().Get("project"))
	destination := strings.TrimSpace(r.URL.Query().Get("destination"))
	preflight := s.buildDashboardPreflightStatus(r.Context(), project, destination)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(preflight)
}

func (s *Server) handleGetProjectStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	project := r.URL.Query().Get("project")
	if project == "" {
		http.Error(w, "project parameter required", http.StatusBadRequest)
		return
	}

	type projectStats struct {
		Project               string `json:"project"`
		CompletedCaptures     int    `json:"completed_captures"`
		CompletedTestCaptures int    `json:"completed_test_captures"`
		LastCaptureNumber     string `json:"last_capture_number"`
		LastTestCaptureNumber string `json:"last_test_capture_number"`
	}

	stats := projectStats{Project: project}
	if s.stateStore != nil {
		if ps, err := s.stateStore.LoadProjectStatus(project); err == nil {
			stats.CompletedCaptures = ps.CompletedCaptures
			stats.CompletedTestCaptures = ps.CompletedTestCaptures
			stats.LastCaptureNumber = ps.LastCaptureNumber
			stats.LastTestCaptureNumber = ps.LastTestCaptureNumber
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (s *Server) handleDashboardProjectStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	project := r.URL.Query().Get("project")
	if project == "" {
		http.Error(w, "project parameter required", http.StatusBadRequest)
		return
	}

	// Use local stateStore if available (dashboard aggregator has its own DB)
	type projectStats struct {
		Project               string `json:"project"`
		CompletedCaptures     int    `json:"completed_captures"`
		CompletedTestCaptures int    `json:"completed_test_captures"`
		LastCaptureNumber     string `json:"last_capture_number"`
		LastTestCaptureNumber string `json:"last_test_capture_number"`
	}

	stats := projectStats{Project: project}
	if s.stateStore != nil {
		if ps, err := s.stateStore.LoadProjectStatus(project); err == nil {
			stats.CompletedCaptures = ps.CompletedCaptures
			stats.CompletedTestCaptures = ps.CompletedTestCaptures
			stats.LastCaptureNumber = ps.LastCaptureNumber
			stats.LastTestCaptureNumber = ps.LastTestCaptureNumber
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (s *Server) handleDownloadProjectReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	project := strings.TrimSpace(r.URL.Query().Get("project"))
	destination := strings.TrimSpace(r.URL.Query().Get("destination"))
	if project == "" || destination == "" {
		http.Error(w, "project and destination parameters required", http.StatusBadRequest)
		return
	}

	filename, err := safeReportFilename(project)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	destinationPath, ok := s.allowedReportDestination(destination)
	if !ok {
		http.Error(w, "destination is not available", http.StatusNotFound)
		return
	}

	reportPath := report.DefaultPath(destinationPath, project)
	if filepath.Base(reportPath) != filename || !isPathWithin(destinationPath, reportPath) {
		http.Error(w, "invalid report path", http.StatusBadRequest)
		return
	}

	file, err := os.Open(reportPath)
	if os.IsNotExist(err) {
		http.Error(w, "report file not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Error().Err(err).Str("project", project).Str("path", reportPath).Msg("Failed to open project report")
		http.Error(w, "failed to open report file", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil || info.IsDir() {
		http.Error(w, "report file not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, quoteHeaderFilename(filename)))
	http.ServeContent(w, r, filename, info.ModTime(), file)
}

func (s *Server) handleGetMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	metrics := s.monService.GetMetrics()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

func (s *Server) handleStartSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Project         string `json:"project"`
		Destination     string `json:"destination"`
		MaxParallelism  int    `json:"max_parallelism"`
		ForceFullResync bool   `json:"force_full_resync"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.Project == "" || req.Destination == "" {
		http.Error(w, "Project and destination are required", http.StatusBadRequest)
		return
	}

	if req.MaxParallelism <= 0 {
		req.MaxParallelism = s.cfg.Sync.MaxParallelism
	}

	// Set target disk for monitoring
	s.monService.SetTargetDisk(req.Destination)

	// Check that all shares are mounted and accessible
	if unavailable := s.getUnavailableShares(); len(unavailable) > 0 {
		var missing []string
		for _, u := range unavailable {
			missing = append(missing, fmt.Sprintf("%s/%s (%s)", u.Node, u.Share, u.Path))
		}
		msg := fmt.Sprintf("Cannot start sync: %d share(s) unavailable: %s", len(missing), strings.Join(missing, "; "))
		log.Warn().Strs("unavailable", missing).Msg("Shares not available, sync blocked")
		http.Error(w, msg, http.StatusServiceUnavailable)
		return
	}

	// Start sync
	ctx := context.Background()
	if err := s.syncService.Start(ctx, req.Project, req.Destination, req.MaxParallelism, req.ForceFullResync); err != nil {
		log.Error().Err(err).Msg("Failed to start sync")
		http.Error(w, fmt.Sprintf("Failed to start sync: %v", err), http.StatusInternalServerError)
		return
	}

	// Broadcast log message
	s.broadcast(models.WSMessage{
		Type: "log",
		Payload: models.LogMessage{
			Timestamp: time.Now(),
			Level:     "info",
			Message:   fmt.Sprintf("Started synchronization: project=%s, destination=%s, full_resync=%t", req.Project, req.Destination, req.ForceFullResync),
		},
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "started"})
}

func (s *Server) handleStopSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.syncService.Stop()

	// Broadcast log message
	s.broadcast(models.WSMessage{
		Type: "log",
		Payload: models.LogMessage{
			Timestamp: time.Now(),
			Level:     "info",
			Message:   "Synchronization stopped",
		},
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "stopped"})
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error().Err(err).Msg("WebSocket upgrade failed")
		return
	}

	s.mu.Lock()
	s.clients[conn] = true
	s.mu.Unlock()

	log.Info().Str("remote", r.RemoteAddr).Msg("WebSocket client connected")

	// Send initial status
	status := s.currentSyncStatus()
	s.sendToClient(conn, models.WSMessage{
		Type:    "status",
		Payload: status,
	})

	// Send initial metrics
	metrics := s.monService.GetMetrics()
	s.sendToClient(conn, models.WSMessage{
		Type:    "metrics",
		Payload: metrics,
	})

	// Keep connection alive and handle disconnection
	go func() {
		defer func() {
			s.mu.Lock()
			delete(s.clients, conn)
			s.mu.Unlock()
			conn.Close()
			log.Info().Str("remote", r.RemoteAddr).Msg("WebSocket client disconnected")
		}()

		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				break
			}
		}
	}()
}

func (s *Server) broadcast(msg models.WSMessage) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for client := range s.clients {
		s.sendToClient(client, msg)
	}
}

func (s *Server) sendToClient(conn *websocket.Conn, msg models.WSMessage) {
	if err := conn.WriteJSON(msg); err != nil {
		log.Error().Err(err).Msg("Failed to send WebSocket message")
	}
}

func (s *Server) broadcastMetrics(ctx context.Context, metricsChan <-chan models.PerformanceMetrics) {
	ticker := time.NewTicker(s.cfg.Monitoring.UIUpdateInterval)
	defer ticker.Stop()

	var lastMetrics models.PerformanceMetrics

	for {
		select {
		case <-ctx.Done():
			return
		case metrics, ok := <-metricsChan:
			if !ok {
				return
			}
			lastMetrics = metrics
		case <-ticker.C:
			// Broadcast status
			status := s.currentSyncStatus()
			s.broadcast(models.WSMessage{
				Type:    "status",
				Payload: status,
			})

			// Broadcast metrics
			s.broadcast(models.WSMessage{
				Type:    "metrics",
				Payload: lastMetrics,
			})
		}
	}
}

// getAvailableDestinations scans for available storage destinations
func (s *Server) getAvailableDestinations() []models.DestinationInfo {
	var destinations []models.DestinationInfo

	// Read mount points from /proc/mounts
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		log.Error().Err(err).Msg("Failed to read /proc/mounts")
		return destinations
	}

	lines := strings.Split(string(data), "\n")
	seen := make(map[string]bool)

	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		device := fields[0]
		mountPoint := fields[1]
		// fsType := fields[2] // Not needed anymore

		// Skip if already processed
		if seen[mountPoint] {
			continue
		}
		seen[mountPoint] = true

		// Determine if this is a USB device or regular disk
		var destType string
		var label string
		isDefault := false

		// Skip system mounts - we only want external storage
		// Skip: /, /boot, /home, /var, /tmp, /snap, etc.
		if mountPoint == "/" ||
			strings.HasPrefix(mountPoint, "/boot") ||
			strings.HasPrefix(mountPoint, "/home") ||
			strings.HasPrefix(mountPoint, "/var") ||
			strings.HasPrefix(mountPoint, "/tmp") ||
			strings.HasPrefix(mountPoint, "/snap") ||
			strings.HasPrefix(mountPoint, "/sys") ||
			strings.HasPrefix(mountPoint, "/proc") ||
			strings.HasPrefix(mountPoint, "/dev") ||
			strings.HasPrefix(mountPoint, "/run") {
			continue
		}

		// Skip UCX network mounts
		if strings.HasPrefix(mountPoint, s.cfg.Network.MountRoot) {
			continue
		}

		// Only allow external storage: /media/* or the configured default data mount.
		if mountPoint != defaultDataMountPoint && !strings.HasPrefix(mountPoint, "/media/") {
			continue
		}

		// USB/external storage devices
		if strings.HasPrefix(device, "/dev/sd") || strings.HasPrefix(device, "/dev/nvme") {
			destType = "usb"

			// Check if it's the default USB-SSD mount.
			if mountPoint == defaultDataMountPoint {
				label = "USB-SSD Storage (default)"
				isDefault = true
			} else {
				label = fmt.Sprintf("External: %s", filepath.Base(mountPoint))
			}
		} else {
			continue // Skip non-disk devices
		}

		// Get disk space info
		freeGB, totalGB, err := getDiskSpace(mountPoint)
		if err != nil {
			continue
		}

		// Only include mounts with reasonable space (> 1GB total)
		if totalGB < 1 {
			continue
		}

		destinations = append(destinations, models.DestinationInfo{
			Path:        mountPoint,
			Label:       label,
			Type:        destType,
			FreeSpaceGB: freeGB,
			TotalGB:     totalGB,
			IsDefault:   isDefault,
		})
	}

	// Sort: USB first, then disk, then by path
	sort.Slice(destinations, func(i, j int) bool {
		if destinations[i].Type != destinations[j].Type {
			return destinations[i].Type == "usb"
		}
		return destinations[i].Path < destinations[j].Path
	})

	return destinations
}

// handleGetDevices returns list of all block devices
func (s *Server) handleGetDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	devices, err := s.getBlockDevices()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get block devices")
		http.Error(w, "Failed to get devices", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(devices)
}

// handleMountDevice handles mount/unmount requests
func (s *Server) handleMountDevice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req models.MountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.DevicePath == "" || req.Action == "" {
		http.Error(w, "Device path and action are required", http.StatusBadRequest)
		return
	}

	if req.Action == "unmount" {
		status := s.syncService.GetStatus()
		if status.IsRunning && isManagedDataDestination(status.Destination) {
			s.syncService.Stop()
			s.broadcast(models.WSMessage{
				Type: "log",
				Payload: models.LogMessage{
					Timestamp: time.Now(),
					Level:     "warn",
					Message:   "Синхронизация остановлена перед размонтированием диска назначения",
				},
			})
		}
	}

	var err error
	switch req.Action {
	case "mount":
		err = s.mountDevice(req.DevicePath)
	case "unmount":
		err = s.unmountDevice(req.DevicePath)
	default:
		http.Error(w, "Invalid action. Use 'mount' or 'unmount'", http.StatusBadRequest)
		return
	}

	if err != nil {
		log.Error().Err(err).Str("device", req.DevicePath).Str("action", req.Action).Msg("Device operation failed")
		http.Error(w, fmt.Sprintf("Failed to %s device: %v", req.Action, err), http.StatusInternalServerError)
		return
	}

	// Broadcast log message
	s.broadcast(models.WSMessage{
		Type: "log",
		Payload: models.LogMessage{
			Timestamp: time.Now(),
			Level:     "info",
			Message:   fmt.Sprintf("Device %s: %s", req.Action, req.DevicePath),
		},
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "success",
		"action": req.Action,
		"device": req.DevicePath,
	})
}

func (s *Server) handleCheckShares(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	unavailable := s.getUnavailableShares()

	type shareStatus struct {
		Node  string `json:"node"`
		Share string `json:"share"`
		Path  string `json:"path"`
	}
	type result struct {
		OK          bool          `json:"ok"`
		Unavailable []shareStatus `json:"unavailable"`
	}

	res := result{OK: len(unavailable) == 0, Unavailable: []shareStatus{}}
	for _, u := range unavailable {
		res.Unavailable = append(res.Unavailable, shareStatus{Node: u.Node, Share: u.Share, Path: u.Path})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

func (s *Server) handleMountShares(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := s.requireNetworkRequirements(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := s.mountAllShares(); err != nil {
		log.Error().Err(err).Msg("Failed to mount network shares on demand")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.broadcast(models.WSMessage{
		Type: "log",
		Payload: models.LogMessage{
			Timestamp: time.Now(),
			Level:     "info",
			Message:   "Повторная попытка монтирования шар выполнена",
		},
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "mounted"})
}

func (s *Server) handleClearProjectHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		Project string `json:"project"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Project) == "" {
		http.Error(w, "project field required", http.StatusBadRequest)
		return
	}

	if s.syncService.IsProjectRunning(body.Project) {
		http.Error(w, "cannot clear history while sync is running", http.StatusConflict)
		return
	}

	if s.stateStore == nil {
		http.Error(w, "state store not available", http.StatusServiceUnavailable)
		return
	}

	if err := s.stateStore.ClearProjectHistory(body.Project); err != nil {
		log.Error().Err(err).Str("project", body.Project).Msg("Failed to clear project history")
		http.Error(w, fmt.Sprintf("failed to clear history: %v", err), http.StatusInternalServerError)
		return
	}

	log.Info().Str("project", body.Project).Msg("Project history cleared")
	s.broadcast(models.WSMessage{
		Type: "log",
		Payload: models.LogMessage{
			Timestamp: time.Now(),
			Level:     "warn",
			Message:   fmt.Sprintf("История проекта '%s' очищена", body.Project),
		},
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "cleared", "project": body.Project})
}

func (s *Server) handleDatabaseProjects(w http.ResponseWriter, r *http.Request) {
	if s.stateStore == nil {
		http.Error(w, "state store not available", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		projects, err := s.stateStore.ListProjectDatabaseSummaries()
		if err != nil {
			log.Error().Err(err).Msg("Failed to list database projects")
			http.Error(w, fmt.Sprintf("failed to list database projects: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(projects)
	case http.MethodDelete:
		if status := s.currentSyncStatus(); status.IsRunning {
			http.Error(w, "cannot clear database while sync is running", http.StatusConflict)
			return
		}

		if err := s.stateStore.ClearDatabase(); err != nil {
			statusCode := http.StatusInternalServerError
			if strings.Contains(err.Error(), "sync is running") {
				statusCode = http.StatusConflict
			}
			log.Error().Err(err).Msg("Failed to clear project database")
			http.Error(w, fmt.Sprintf("failed to clear database: %v", err), statusCode)
			return
		}

		log.Warn().Msg("Project database cleared")
		s.broadcast(models.WSMessage{
			Type: "log",
			Payload: models.LogMessage{
				Timestamp: time.Now(),
				Level:     "warn",
				Message:   "Project database cleared",
			},
		})

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "cleared"})
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleDatabaseProject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.stateStore == nil {
		http.Error(w, "state store not available", http.StatusServiceUnavailable)
		return
	}

	var body struct {
		Project string `json:"project"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Project) == "" {
		http.Error(w, "project field required", http.StatusBadRequest)
		return
	}
	body.Project = strings.TrimSpace(body.Project)

	if s.syncService != nil && s.syncService.IsProjectRunning(body.Project) {
		http.Error(w, "cannot delete project while sync is running", http.StatusConflict)
		return
	}

	if err := s.stateStore.DeleteProject(body.Project); err != nil {
		statusCode := http.StatusInternalServerError
		if strings.Contains(err.Error(), "sync is running") {
			statusCode = http.StatusConflict
		}
		log.Error().Err(err).Str("project", body.Project).Msg("Failed to delete project from database")
		http.Error(w, fmt.Sprintf("failed to delete project: %v", err), statusCode)
		return
	}

	log.Warn().Str("project", body.Project).Msg("Project deleted from database")
	s.broadcast(models.WSMessage{
		Type: "log",
		Payload: models.LogMessage{
			Timestamp: time.Now(),
			Level:     "warn",
			Message:   fmt.Sprintf("Project '%s' deleted from database", body.Project),
		},
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted", "project": body.Project})
}

func (s *Server) handleRestartService(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cmd := exec.Command("sh", "-c", "sleep 1; systemctl restart \"$1\"", "sh", s.serviceName)
	if err := cmd.Start(); err != nil {
		log.Error().Err(err).Msg("Failed to schedule service restart")
		http.Error(w, fmt.Sprintf("failed to restart service: %v", err), http.StatusInternalServerError)
		return
	}

	s.broadcast(models.WSMessage{
		Type: "log",
		Payload: models.LogMessage{
			Timestamp: time.Now(),
			Level:     "warn",
			Message:   fmt.Sprintf("Запрошен перезапуск службы %s", s.serviceName),
		},
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"status": "restarting"})
}

func (s *Server) handleHostTime(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	now := s.hostNow()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"unix_ms":    now.UnixMilli(),
		"iso":        now.UTC().Format(time.RFC3339),
		"local":      now.Format(time.RFC3339),
		"time_zone":  now.Format("MST"),
		"utc_offset": now.Format("-07:00"),
	})
}

func (s *Server) handleSyncHostTime(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ClientUnixMS int64 `json:"client_unix_ms"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	target := time.UnixMilli(req.ClientUnixMS).UTC()
	if err := validateHostTimeTarget(target); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	before := s.hostNow()
	if err := s.setHostTime(target); err != nil {
		log.Error().Err(err).Time("target", target).Msg("Failed to set host time")
		http.Error(w, fmt.Sprintf("failed to set host time: %v", err), http.StatusInternalServerError)
		return
	}

	hwclockSynced := true
	var hwclockError string
	if err := s.syncHardwareClock(); err != nil {
		hwclockSynced = false
		hwclockError = err.Error()
		log.Warn().Err(err).Msg("Failed to sync hardware clock after host time correction")
	}

	sharesMounted := true
	var mountError string
	if err := s.mountAllShares(); err != nil {
		sharesMounted = false
		mountError = err.Error()
		log.Warn().Err(err).Msg("Failed to remount shares after host time correction")
	}

	after := s.hostNow()
	drift := target.Sub(before)
	s.broadcast(models.WSMessage{
		Type: "log",
		Payload: models.LogMessage{
			Timestamp: after,
			Level:     "warn",
			Message:   fmt.Sprintf("Host time synchronized from browser device, drift corrected: %s", drift.Round(time.Second)),
		},
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":         "time_synced",
		"before_unix_ms": before.UnixMilli(),
		"after_unix_ms":  after.UnixMilli(),
		"target_unix_ms": target.UnixMilli(),
		"drift_ms":       drift.Milliseconds(),
		"hwclock_synced": hwclockSynced,
		"hwclock_error":  hwclockError,
		"shares_mounted": sharesMounted,
		"mount_error":    mountError,
	})
}

func (s *Server) handleHostShutdown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cmd := exec.Command("sh", "-c", "sleep 2; shutdown -h now")
	if err := cmd.Start(); err != nil {
		log.Error().Err(err).Msg("Failed to schedule host shutdown")
		http.Error(w, fmt.Sprintf("failed to shutdown host: %v", err), http.StatusInternalServerError)
		return
	}

	s.broadcast(models.WSMessage{
		Type: "log",
		Payload: models.LogMessage{
			Timestamp: time.Now(),
			Level:     "warn",
			Message:   "Запрошено выключение хоста",
		},
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"status": "shutting_down"})
}

func (s *Server) handleDashboardConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.dashboardConfig())
}

func (s *Server) currentSyncStatus() models.SyncStatus {
	if s.getStatusFunc != nil {
		return s.getStatusFunc()
	}
	if s.syncService == nil {
		return models.SyncStatus{}
	}
	return s.syncService.GetStatus()
}

func (s *Server) hostNow() time.Time {
	if s.nowFunc != nil {
		return s.nowFunc()
	}
	return time.Now()
}

func (s *Server) setHostTime(target time.Time) error {
	if s.setHostTimeFunc != nil {
		return s.setHostTimeFunc(target)
	}
	return setSystemClock(target)
}

func (s *Server) syncHardwareClock() error {
	if s.syncHardwareClockFunc != nil {
		return s.syncHardwareClockFunc()
	}
	return syncHardwareClock()
}

func validateHostTimeTarget(target time.Time) error {
	if target.IsZero() {
		return fmt.Errorf("client_unix_ms is required")
	}

	year := target.UTC().Year()
	if year < 2024 || year > 2035 {
		return fmt.Errorf("client time is outside the allowed range")
	}

	return nil
}

func setSystemClock(target time.Time) error {
	formatted := target.UTC().Format("2006-01-02 15:04:05")
	cmd := exec.Command("date", "-u", "-s", formatted)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}

func syncHardwareClock() error {
	cmd := exec.Command("hwclock", "--systohc")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}

func (s *Server) findProjects(ctx context.Context) ([]models.ProjectInfo, error) {
	if s.findProjectsFunc != nil {
		return s.findProjectsFunc(ctx)
	}
	if s.syncService == nil {
		return nil, fmt.Errorf("sync service is not configured")
	}
	return s.syncService.FindProjects(ctx)
}

func (s *Server) availableDestinations() []models.DestinationInfo {
	if s.getDestinationsFunc != nil {
		return s.getDestinationsFunc()
	}
	return s.getAvailableDestinations()
}

func (s *Server) allowedReportDestination(destination string) (string, bool) {
	destination = filepath.Clean(strings.TrimSpace(destination))
	if destination == "" || destination == "." {
		return "", false
	}

	for _, candidate := range s.availableDestinations() {
		candidatePath := filepath.Clean(strings.TrimSpace(candidate.Path))
		if candidatePath != "" && candidatePath == destination {
			return candidatePath, true
		}
	}

	return "", false
}

func (s *Server) requireNetworkRequirements() error {
	if s.checkNetworkRequirements != nil {
		return s.checkNetworkRequirements()
	}
	return network.CheckRequirements()
}

func (s *Server) mountAllShares() error {
	if s.mountSharesFunc != nil {
		return s.mountSharesFunc()
	}
	if s.netService == nil {
		return fmt.Errorf("network service is not configured")
	}
	return s.netService.MountAll()
}

func (s *Server) getUnavailableShares() []syncService.UnavailableShare {
	if s.checkSharesAvailability != nil {
		return s.checkSharesAvailability()
	}
	if s.syncService == nil {
		return nil
	}
	return s.syncService.CheckSharesAvailability()
}

func (s *Server) ensureDestinationReady(destination string) error {
	if s.ensureDestinationFunc != nil {
		return s.ensureDestinationFunc(destination)
	}
	if s.syncService == nil {
		return fmt.Errorf("sync service is not configured")
	}
	return s.syncService.EnsureDestinationReady(destination)
}

func (s *Server) checkDestinationDiskSpace(destination string) (syncService.DiskSpaceCheckResult, error) {
	if s.checkDiskSpaceFunc != nil {
		return s.checkDiskSpaceFunc(destination)
	}
	if s.syncService == nil {
		return syncService.DiskSpaceCheckResult{}, fmt.Errorf("sync service is not configured")
	}
	return s.syncService.CheckDiskSpace(destination)
}

func (s *Server) attemptShareRemount() {
	unavailable := s.getUnavailableShares()
	if len(unavailable) == 0 {
		return
	}

	if err := s.requireNetworkRequirements(); err != nil {
		log.Warn().Err(err).Msg("Skipping share remount: requirements not met")
		return
	}

	if err := s.mountAllShares(); err != nil {
		log.Warn().Err(err).Msg("Share remount attempt failed")
	}
}

func (s *Server) autoRemountShares(ctx context.Context) {
	interval := 10 * time.Second
	if s.cfg != nil && s.cfg.Sync.ServiceLoopInterval > 0 {
		interval = s.cfg.Sync.ServiceLoopInterval
	}

	s.attemptShareRemount()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.attemptShareRemount()
		}
	}
}

func (s *Server) buildPreflightStatus(ctx context.Context, project, destination string) models.PreflightStatus {
	const gib = float64(1024 * 1024 * 1024)

	status := s.currentSyncStatus()
	preflight := models.PreflightStatus{
		Ready:               true,
		IsRunning:           status.IsRunning,
		SelectedProject:     project,
		SelectedDestination: destination,
		ActiveProject:       status.Project,
		ActiveDestination:   status.Destination,
		Checks:              make([]models.PreflightCheck, 0, 5),
	}

	appendCheck := func(key, label, checkStatus, message string) {
		preflight.Checks = append(preflight.Checks, models.PreflightCheck{
			Key:     key,
			Label:   label,
			Status:  checkStatus,
			Message: message,
		})
		if checkStatus != "ready" {
			preflight.Ready = false
		}
	}

	if status.IsRunning {
		message := "Синхронизация уже выполняется"
		if status.Project != "" || status.Destination != "" {
			message = fmt.Sprintf("Синхронизация уже выполняется: %s → %s", status.Project, status.Destination)
		}
		appendCheck("sync", "Состояние службы", "blocked", message)
	} else {
		appendCheck("sync", "Состояние службы", "ready", "Служба готова к новому запуску")
	}

	projects, err := s.findProjects(ctx)
	if err != nil {
		appendCheck("project", "Проект", "blocked", "Не удалось получить список проектов")
	} else {
		preflight.AvailableProjects = len(projects)
		projectFound := false
		for _, candidate := range projects {
			if candidate.Name == project {
				projectFound = true
				break
			}
		}

		switch {
		case len(projects) == 0:
			appendCheck("project", "Проект", "blocked", "Доступные проекты не найдены")
		case project == "":
			appendCheck("project", "Проект", "blocked", "Выберите проект для синхронизации")
		case !projectFound:
			appendCheck("project", "Проект", "blocked", "Выбранный проект сейчас недоступен")
		default:
			appendCheck("project", "Проект", "ready", fmt.Sprintf("Проект '%s' выбран", project))
		}
	}

	destinations := s.availableDestinations()
	preflight.AvailableDestinations = len(destinations)
	destinationFound := false
	for _, candidate := range destinations {
		if candidate.Path == destination {
			destinationFound = true
			preflight.FreeSpaceGB = candidate.FreeSpaceGB
			break
		}
	}

	switch {
	case len(destinations) == 0:
		appendCheck("destination", "Диск назначения", "blocked", "Доступные накопители не найдены")
	case destination == "":
		appendCheck("destination", "Диск назначения", "blocked", "Выберите накопитель назначения")
	case !destinationFound:
		appendCheck("destination", "Диск назначения", "blocked", "Выбранный накопитель сейчас недоступен")
	default:
		appendCheck("destination", "Диск назначения", "ready", fmt.Sprintf("Накопитель '%s' доступен", destination))
	}

	unavailableShares := s.getUnavailableShares()
	if len(unavailableShares) > 0 {
		preflight.UnavailableShares = make([]models.PreflightUnavailableShare, 0, len(unavailableShares))
		for _, share := range unavailableShares {
			preflight.UnavailableShares = append(preflight.UnavailableShares, models.PreflightUnavailableShare{
				Node:  share.Node,
				Share: share.Share,
				Path:  share.Path,
			})
		}
		appendCheck("shares", "Сетевые шары", "blocked", fmt.Sprintf("Недоступно сетевых шар: %d", len(unavailableShares)))
	} else {
		appendCheck("shares", "Сетевые шары", "ready", "Все сетевые шары доступны")
	}

	if destination == "" || !destinationFound {
		appendCheck("disk", "Свободное место", "blocked", "Невозможно проверить место без выбранного накопителя")
		return preflight
	}

	diskCheck, err := s.checkDestinationDiskSpace(destination)
	if err != nil {
		appendCheck("disk", "Свободное место", "blocked", "Не удалось проверить свободное место")
		return preflight
	}

	preflight.FreeSpaceGB = float64(diskCheck.FreeBytes) / gib
	preflight.RequiredFreeSpaceGB = float64(diskCheck.RequiredFreeBytes) / gib

	if !diskCheck.OK {
		appendCheck(
			"disk",
			"Свободное место",
			"blocked",
			fmt.Sprintf("Свободно %.1f ГБ, требуется минимум %.1f ГБ", preflight.FreeSpaceGB, preflight.RequiredFreeSpaceGB),
		)
	} else {
		appendCheck(
			"disk",
			"Свободное место",
			"ready",
			fmt.Sprintf("Свободно %.1f ГБ, минимальный запас %.1f ГБ", preflight.FreeSpaceGB, preflight.RequiredFreeSpaceGB),
		)
	}

	return preflight
}

func (s *Server) buildDashboardPreflightStatus(ctx context.Context, project, destination string) models.DashboardPreflightStatus {
	preflight := models.DashboardPreflightStatus{
		Ready:               true,
		SelectedProject:     project,
		SelectedDestination: destination,
		Checks:              make([]models.PreflightCheck, 0, 7),
		Instances:           make([]models.DashboardPreflightInstanceStatus, len(s.cfg.Web.Dashboard.Instances)),
	}

	if !s.dashboardEnabled() {
		preflight.Ready = false
		preflight.Checks = append(preflight.Checks, models.PreflightCheck{
			Key:     "instances",
			Label:   "Инстансы",
			Status:  "blocked",
			Message: "Shared dashboard не настроен",
		})
		return preflight
	}

	query := url.Values{}
	if project != "" {
		query.Set("project", project)
	}
	if destination != "" {
		query.Set("destination", destination)
	}
	apiPath := "/api/preflight"
	if encoded := query.Encode(); encoded != "" {
		apiPath += "?" + encoded
	}

	var wg sync.WaitGroup
	for i, instance := range s.cfg.Web.Dashboard.Instances {
		wg.Add(1)
		go func(index int, inst config.DashboardInstance) {
			defer wg.Done()

			instanceStatus := models.DashboardPreflightInstanceStatus{
				ID:   inst.ID,
				Name: inst.Name,
				URL:  inst.URL,
			}

			var payload models.PreflightStatus
			if _, err := s.proxyJSON(ctx, http.MethodGet, inst.URL, apiPath, nil, &payload); err != nil {
				instanceStatus.Available = false
				instanceStatus.Error = err.Error()
				instanceStatus.Ready = false
			} else {
				instanceStatus.Available = true
				instanceStatus.Ready = payload.Ready
				instanceStatus.IsRunning = payload.IsRunning
				instanceStatus.AvailableProjects = payload.AvailableProjects
				instanceStatus.AvailableDestinations = payload.AvailableDestinations
				instanceStatus.FreeSpaceGB = payload.FreeSpaceGB
				instanceStatus.RequiredFreeSpaceGB = payload.RequiredFreeSpaceGB
				instanceStatus.UnavailableShares = payload.UnavailableShares
				instanceStatus.Checks = payload.Checks
			}

			preflight.Instances[index] = instanceStatus
		}(i, instance)
	}

	wg.Wait()
	preflight.Checks = aggregateDashboardPreflightChecks(preflight.Instances)
	for _, check := range preflight.Checks {
		if check.Status != "ready" {
			preflight.Ready = false
			break
		}
	}

	return preflight
}

func aggregateDashboardPreflightChecks(instances []models.DashboardPreflightInstanceStatus) []models.PreflightCheck {
	checks := make([]models.PreflightCheck, 0, 7)
	appendCheck := func(key, label, status, message string) {
		checks = append(checks, models.PreflightCheck{
			Key:     key,
			Label:   label,
			Status:  status,
			Message: message,
		})
	}

	if len(instances) == 0 {
		appendCheck("instances", "Инстансы", "blocked", "Нет настроенных инстансов для общего дашборда")
		return checks
	}

	unavailableMessages := make([]string, 0)
	availableCount := 0
	for _, instance := range instances {
		if instance.Available {
			availableCount++
			continue
		}
		message := instance.Error
		if strings.TrimSpace(message) == "" {
			message = "инстанс недоступен"
		}
		unavailableMessages = append(unavailableMessages, fmt.Sprintf("%s: %s", instance.Name, message))
	}

	if len(unavailableMessages) == 0 {
		appendCheck("instances", "Инстансы", "ready", fmt.Sprintf("Все инстансы на связи (%d/%d)", availableCount, len(instances)))
	} else {
		appendCheck("instances", "Инстансы", "blocked", strings.Join(unavailableMessages, "; "))
	}

	definitions := []struct {
		key          string
		label        string
		readyMessage string
	}{
		{key: "sync", label: "Состояние служб", readyMessage: "Все инстансы готовы к новому запуску"},
		{key: "project", label: "Проект", readyMessage: "Проект выбран и найден на всех инстансах"},
		{key: "destination", label: "Диск назначения", readyMessage: "Накопитель назначения доступен на всех инстансах"},
		{key: "shares", label: "Сетевые шары", readyMessage: "Все сетевые шары доступны на всех инстансах"},
		{key: "disk", label: "Свободное место", readyMessage: "Свободного места достаточно на всех инстансах"},
	}

	for _, definition := range definitions {
		blockers := make([]string, 0)
		readyCount := 0
		for _, instance := range instances {
			if !instance.Available {
				blockers = append(blockers, fmt.Sprintf("%s: инстанс недоступен", instance.Name))
				continue
			}

			check, ok := findPreflightCheckByKey(instance.Checks, definition.key)
			if !ok {
				blockers = append(blockers, fmt.Sprintf("%s: нет данных проверки", instance.Name))
				continue
			}

			if check.Status == "ready" {
				readyCount++
				continue
			}

			message := strings.TrimSpace(check.Message)
			if message == "" {
				message = "есть блокер"
			}
			blockers = append(blockers, fmt.Sprintf("%s: %s", instance.Name, message))
		}

		if len(blockers) == 0 && readyCount == len(instances) {
			appendCheck(definition.key, definition.label, "ready", definition.readyMessage)
		} else {
			appendCheck(definition.key, definition.label, "blocked", strings.Join(blockers, "; "))
		}
	}

	return checks
}

func findPreflightCheckByKey(checks []models.PreflightCheck, key string) (models.PreflightCheck, bool) {
	for _, check := range checks {
		if check.Key == key {
			return check, true
		}
	}

	return models.PreflightCheck{}, false
}

func (s *Server) handleDashboardOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !s.dashboardEnabled() {
		http.Error(w, "Dashboard mode is not configured", http.StatusNotFound)
		return
	}

	overview := models.DashboardOverview{
		Config:      s.dashboardConfig(),
		HostMetrics: s.monService.GetMetrics(),
		Instances:   s.collectDashboardStates(r.Context()),
	}

	overview.Summary = s.buildDashboardSummary(overview.Instances)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(overview)
}

func (s *Server) handleDashboardProjects(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !s.dashboardEnabled() {
		http.Error(w, "Dashboard mode is not configured", http.StatusNotFound)
		return
	}

	type projectAccumulator struct {
		name    string
		sources map[string]struct{}
	}

	projectsByName := make(map[string]*projectAccumulator)
	for _, instance := range s.cfg.Web.Dashboard.Instances {
		var remoteProjects []models.ProjectInfo
		if _, err := s.proxyJSON(r.Context(), http.MethodGet, instance.URL, "/api/projects", nil, &remoteProjects); err != nil {
			continue
		}

		for _, project := range remoteProjects {
			acc, ok := projectsByName[project.Name]
			if !ok {
				acc = &projectAccumulator{name: project.Name, sources: make(map[string]struct{})}
				projectsByName[project.Name] = acc
			}
			sourceLabel := instance.Name
			if project.Source != "" {
				sourceLabel = fmt.Sprintf("%s (%s)", instance.Name, project.Source)
			}
			acc.sources[sourceLabel] = struct{}{}
		}
	}

	projects := make([]models.ProjectInfo, 0, len(projectsByName))
	for _, acc := range projectsByName {
		sources := make([]string, 0, len(acc.sources))
		for source := range acc.sources {
			sources = append(sources, source)
		}
		sort.Strings(sources)
		projects = append(projects, models.ProjectInfo{Name: acc.name, Source: strings.Join(sources, ", ")})
	}

	sort.Slice(projects, func(i, j int) bool { return projects[i].Name < projects[j].Name })

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(projects)
}

func (s *Server) handleDashboardDestinations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !s.dashboardEnabled() {
		http.Error(w, "Dashboard mode is not configured", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.getAvailableDestinations())
}

func (s *Server) handleDashboardStartSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !s.dashboardEnabled() {
		http.Error(w, "Dashboard mode is not configured", http.StatusNotFound)
		return
	}

	var req struct {
		Project         string   `json:"project"`
		Destination     string   `json:"destination"`
		MaxParallelism  int      `json:"max_parallelism"`
		ForceFullResync bool     `json:"force_full_resync"`
		Targets         []string `json:"targets"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.Project == "" || req.Destination == "" {
		http.Error(w, "Project and destination are required", http.StatusBadRequest)
		return
	}

	body, err := json.Marshal(map[string]interface{}{
		"project":           req.Project,
		"destination":       req.Destination,
		"max_parallelism":   req.MaxParallelism,
		"force_full_resync": req.ForceFullResync,
	})
	if err != nil {
		http.Error(w, "Failed to build request", http.StatusInternalServerError)
		return
	}

	s.respondDashboardAction(w, r, "sync/start", http.MethodPost, "/api/sync/start", body, req.Targets)
}

func (s *Server) handleDashboardStopSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !s.dashboardEnabled() {
		http.Error(w, "Dashboard mode is not configured", http.StatusNotFound)
		return
	}

	var req struct {
		Targets []string `json:"targets"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	s.respondDashboardAction(w, r, "sync/stop", http.MethodPost, "/api/sync/stop", nil, req.Targets)
}

func (s *Server) handleDashboardMountShares(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !s.dashboardEnabled() {
		http.Error(w, "Dashboard mode is not configured", http.StatusNotFound)
		return
	}

	var req struct {
		Targets []string `json:"targets"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	s.respondDashboardAction(w, r, "shares/mount", http.MethodPost, "/api/shares/mount", nil, req.Targets)
}

func (s *Server) handleDashboardRestartService(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !s.dashboardEnabled() {
		http.Error(w, "Dashboard mode is not configured", http.StatusNotFound)
		return
	}

	var req struct {
		Targets []string `json:"targets"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	s.respondDashboardAction(w, r, "service/restart", http.MethodPost, "/api/service/restart", nil, req.Targets)
}

func (s *Server) dashboardEnabled() bool {
	return len(s.cfg.Web.Dashboard.Instances) > 0
}

func (s *Server) dashboardConfig() models.DashboardConfig {
	instances := make([]models.DashboardInstanceConfig, 0, len(s.cfg.Web.Dashboard.Instances))
	for _, instance := range s.cfg.Web.Dashboard.Instances {
		instances = append(instances, models.DashboardInstanceConfig{
			ID:   instance.ID,
			Name: instance.Name,
			URL:  instance.URL,
		})
	}

	return models.DashboardConfig{
		Enabled:   len(instances) > 0,
		Instances: instances,
	}
}

func (s *Server) collectDashboardStates(ctx context.Context) []models.DashboardInstanceState {
	states := make([]models.DashboardInstanceState, len(s.cfg.Web.Dashboard.Instances))
	var wg sync.WaitGroup

	for i, instance := range s.cfg.Web.Dashboard.Instances {
		wg.Add(1)
		go func(index int, inst config.DashboardInstance) {
			defer wg.Done()

			state := models.DashboardInstanceState{
				ID:   inst.ID,
				Name: inst.Name,
				URL:  inst.URL,
			}

			var status models.SyncStatus
			if _, err := s.proxyJSON(ctx, http.MethodGet, inst.URL, "/api/status", nil, &status); err != nil {
				state.Available = false
				state.Error = err.Error()
			} else {
				state.Available = true
				state.Status = status
			}

			states[index] = state
		}(i, instance)
	}

	wg.Wait()
	return states
}

func (s *Server) buildDashboardSummary(states []models.DashboardInstanceState) models.DashboardSummary {
	summary := models.DashboardSummary{
		ConfiguredInstances: len(states),
	}
	completedValues := make([]int, 0, len(states))
	testValues := make([]int, 0, len(states))
	lastCaptureValues := make([]string, 0, len(states))
	lastTestCaptureValues := make([]string, 0, len(states))

	for _, state := range states {
		if state.Available {
			summary.AvailableInstances++
			completedValues = append(completedValues, state.Status.CompletedCaptures)
			testValues = append(testValues, state.Status.CompletedTestCaptures)
			if strings.TrimSpace(state.Status.LastCaptureNumber) != "" {
				lastCaptureValues = append(lastCaptureValues, state.Status.LastCaptureNumber)
			}
			if strings.TrimSpace(state.Status.LastTestCaptureNumber) != "" {
				lastTestCaptureValues = append(lastTestCaptureValues, state.Status.LastTestCaptureNumber)
			}
		}
		if state.Status.IsRunning {
			summary.RunningInstances++
		}
		summary.TotalActiveFileOps += state.Status.ActiveFileOperations
		summary.TotalMaxParallelism += state.Status.MaxParallelism
		summary.TotalActiveTasks += len(state.Status.ActiveTasks)
	}

	summary.TotalCompletedCaptures = minIntSlice(completedValues)
	summary.TotalCompletedTest = minIntSlice(testValues)
	summary.LastCaptureNumber = maxCaptureNumber(append(lastCaptureValues, lastTestCaptureValues...))
	summary.LastTestCaptureNumber = maxCaptureNumber(lastTestCaptureValues)

	project := dashboardProjectName(states)
	if s.stateStore != nil && project != "" {
		persisted, err := s.stateStore.LoadProjectStatus(project)
		if err == nil {
			summary.TotalCompletedCaptures = persisted.CompletedCaptures
			summary.TotalCompletedTest = persisted.CompletedTestCaptures
			summary.LastCaptureNumber = maxCaptureNumber([]string{persisted.LastCaptureNumber, persisted.LastTestCaptureNumber})
			summary.LastTestCaptureNumber = persisted.LastTestCaptureNumber
		}
	}

	return summary
}

func dashboardProjectName(states []models.DashboardInstanceState) string {
	counts := make(map[string]int)
	bestProject := ""
	bestCount := 0

	for _, state := range states {
		project := strings.TrimSpace(state.Status.Project)
		if project == "" {
			continue
		}
		counts[project]++
		if counts[project] > bestCount {
			bestCount = counts[project]
			bestProject = project
		}
	}

	return bestProject
}

func maxCaptureNumber(values []string) string {
	maxValue := ""
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if maxValue == "" || value > maxValue {
			maxValue = value
		}
	}
	return maxValue
}

func minIntSlice(values []int) int {
	if len(values) == 0 {
		return 0
	}

	min := values[0]
	for _, value := range values[1:] {
		if value < min {
			min = value
		}
	}

	return min
}

func (s *Server) respondDashboardAction(w http.ResponseWriter, r *http.Request, action, method, apiPath string, body []byte, targets []string) {
	results, err := s.dispatchDashboardAction(r.Context(), method, apiPath, body, targets)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(models.DashboardActionResponse{
		Action:  action,
		Results: results,
	})
}

func (s *Server) dispatchDashboardAction(ctx context.Context, method, apiPath string, body []byte, targetIDs []string) ([]models.DashboardActionResult, error) {
	targets, err := s.selectDashboardTargets(targetIDs)
	if err != nil {
		return nil, err
	}

	results := make([]models.DashboardActionResult, len(targets))
	var wg sync.WaitGroup

	for i, instance := range targets {
		wg.Add(1)
		go func(index int, inst config.DashboardInstance) {
			defer wg.Done()

			result := models.DashboardActionResult{ID: inst.ID, Name: inst.Name}
			statusCode, err := s.proxyJSON(ctx, method, inst.URL, apiPath, body, nil)
			result.StatusCode = statusCode
			if err != nil {
				result.Success = false
				result.Error = err.Error()
			} else {
				result.Success = true
			}

			results[index] = result
		}(i, instance)
	}

	wg.Wait()
	return results, nil
}

func (s *Server) selectDashboardTargets(targetIDs []string) ([]config.DashboardInstance, error) {
	if !s.dashboardEnabled() {
		return nil, fmt.Errorf("dashboard mode is not configured")
	}

	if len(targetIDs) == 0 {
		return append([]config.DashboardInstance(nil), s.cfg.Web.Dashboard.Instances...), nil
	}

	lookup := make(map[string]config.DashboardInstance, len(s.cfg.Web.Dashboard.Instances))
	for _, instance := range s.cfg.Web.Dashboard.Instances {
		lookup[instance.ID] = instance
	}

	selected := make([]config.DashboardInstance, 0, len(targetIDs))
	for _, id := range targetIDs {
		instance, ok := lookup[id]
		if !ok {
			return nil, fmt.Errorf("unknown dashboard target: %s", id)
		}
		selected = append(selected, instance)
	}

	return selected, nil
}

func (s *Server) proxyJSON(ctx context.Context, method, baseURL, apiPath string, body []byte, out interface{}) (int, error) {
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(baseURL, "/")+apiPath, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}

	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		trimmed := strings.TrimSpace(string(message))
		if trimmed == "" {
			trimmed = resp.Status
		}
		return resp.StatusCode, fmt.Errorf(trimmed)
	}

	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return resp.StatusCode, err
		}
	}

	return resp.StatusCode, nil
}

// getBlockDevices returns list of all block devices using lsblk
func (s *Server) getBlockDevices() ([]models.BlockDeviceInfo, error) {
	var devices []models.BlockDeviceInfo
	type lsblkDevice struct {
		Name       string        `json:"name"`
		Size       string        `json:"size"`
		FSType     string        `json:"fstype"`
		Label      string        `json:"label"`
		MountPoint string        `json:"mountpoint"`
		Type       string        `json:"type"`
		RM         interface{}   `json:"rm"`
		Model      string        `json:"model"`
		Children   []lsblkDevice `json:"children"`
	}

	// Use lsblk to get block device information
	cmd := exec.Command("lsblk", "-J", "-o", "NAME,SIZE,FSTYPE,LABEL,MOUNTPOINT,TYPE,RM,MODEL")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run lsblk: %w", err)
	}

	var lsblkOutput struct {
		BlockDevices []lsblkDevice `json:"blockdevices"`
	}

	if err := json.Unmarshal(output, &lsblkOutput); err != nil {
		return nil, fmt.Errorf("failed to parse lsblk output: %w", err)
	}

	parseRemovable := func(raw interface{}) bool {
		switch v := raw.(type) {
		case bool:
			return v
		case string:
			return v == "1" || strings.EqualFold(v, "true")
		case float64:
			return v != 0
		default:
			return false
		}
	}

	var walkDevices func([]lsblkDevice)
	walkDevices = func(entries []lsblkDevice) {
		for _, dev := range entries {
			// Skip if no filesystem
			if dev.FSType == "" {
				walkDevices(dev.Children)
				continue
			}

			// Allow partitions and whole-disk filesystems.
			if dev.Type != "part" && dev.Type != "disk" {
				walkDevices(dev.Children)
				continue
			}

			// Skip system partitions (mounted on /, /boot, /home, etc.)
			if dev.MountPoint == "/" ||
				strings.HasPrefix(dev.MountPoint, "/boot") ||
				strings.HasPrefix(dev.MountPoint, "/home") ||
				strings.HasPrefix(dev.MountPoint, "/var") ||
				strings.HasPrefix(dev.MountPoint, "/snap") {
				walkDevices(dev.Children)
				continue
			}

			// Skip UCX network mounts
			if strings.HasPrefix(dev.MountPoint, s.cfg.Network.MountRoot) {
				walkDevices(dev.Children)
				continue
			}

			devicePath := "/dev/" + dev.Name
			isRemovable := parseRemovable(dev.RM)
			isMounted := dev.MountPoint != ""

			// Get size in bytes for sorting
			sizeBytes := parseSizeToBytes(dev.Size)

			label := dev.Label
			if label == "" {
				if isRemovable {
					label = fmt.Sprintf("Removable: %s", dev.Name)
				} else {
					label = fmt.Sprintf("Disk: %s", dev.Name)
				}
			}

			// Add model info if available
			if dev.Model != "" {
				label = fmt.Sprintf("%s (%s)", label, strings.TrimSpace(dev.Model))
			}

			devices = append(devices, models.BlockDeviceInfo{
				DevicePath:  devicePath,
				DeviceName:  dev.Name,
				Label:       label,
				Size:        dev.Size,
				SizeBytes:   sizeBytes,
				FSType:      dev.FSType,
				MountPoint:  dev.MountPoint,
				IsMounted:   isMounted,
				IsRemovable: isRemovable,
				Model:       strings.TrimSpace(dev.Model),
			})

			walkDevices(dev.Children)
		}
	}

	walkDevices(lsblkOutput.BlockDevices)

	// Sort: removable first, then by size (largest first)
	sort.Slice(devices, func(i, j int) bool {
		if devices[i].IsRemovable != devices[j].IsRemovable {
			return devices[i].IsRemovable
		}
		return devices[i].SizeBytes > devices[j].SizeBytes
	})

	return devices, nil
}

// mountDevice mounts a device to /ucdata
func (s *Server) mountDevice(devicePath string) error {
	mountPoint := defaultDataMountPoint

	// Check if something is already mounted
	if isMounted, _ := isPathMounted(mountPoint); isMounted {
		return fmt.Errorf("something is already mounted at %s", mountPoint)
	}

	// Create mount point if it doesn't exist
	if err := os.MkdirAll(mountPoint, 0755); err != nil {
		return fmt.Errorf("failed to create mount point: %w", err)
	}

	// Mount the device
	cmd := exec.Command("mount", devicePath, mountPoint)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("mount failed: %s: %w", string(output), err)
	}

	// Set permissions
	if err := os.Chmod(mountPoint, 0755); err != nil {
		log.Warn().Err(err).Msg("Failed to set permissions on mount point")
	}

	log.Info().Str("device", devicePath).Str("mount_point", mountPoint).Msg("Device mounted successfully")
	return nil
}

// unmountDevice unmounts a device
func (s *Server) unmountDevice(devicePath string) error {
	mountPoint := defaultDataMountPoint

	// Check if the device is actually mounted at this location
	mounted, err := isDeviceMountedAt(devicePath, mountPoint)
	if err != nil {
		return fmt.Errorf("failed to check mount status: %w", err)
	}

	if !mounted {
		return fmt.Errorf("device %s is not mounted at %s", devicePath, mountPoint)
	}

	// Unmount
	cmd := exec.Command("umount", mountPoint)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("unmount failed: %s: %w", string(output), err)
	}

	log.Info().Str("device", devicePath).Str("mount_point", mountPoint).Msg("Device unmounted successfully")
	return nil
}

// isPathMounted checks if a path is currently mounted
func isPathMounted(path string) (bool, error) {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return false, err
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == path {
			return true, nil
		}
	}

	return false, nil
}

// isDeviceMountedAt checks if a specific device is mounted at a specific path
func isDeviceMountedAt(devicePath, mountPath string) (bool, error) {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return false, err
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == devicePath && fields[1] == mountPath {
			return true, nil
		}
	}

	return false, nil
}

// parseSizeToBytes converts human-readable size to bytes
func parseSizeToBytes(size string) uint64 {
	size = strings.TrimSpace(size)
	if size == "" {
		return 0
	}

	var multiplier uint64 = 1
	size = strings.ToUpper(size)

	if strings.HasSuffix(size, "T") {
		multiplier = 1024 * 1024 * 1024 * 1024
		size = strings.TrimSuffix(size, "T")
	} else if strings.HasSuffix(size, "G") {
		multiplier = 1024 * 1024 * 1024
		size = strings.TrimSuffix(size, "G")
	} else if strings.HasSuffix(size, "M") {
		multiplier = 1024 * 1024
		size = strings.TrimSuffix(size, "M")
	} else if strings.HasSuffix(size, "K") {
		multiplier = 1024
		size = strings.TrimSuffix(size, "K")
	}

	var value float64
	fmt.Sscanf(size, "%f", &value)

	return uint64(value * float64(multiplier))
}

func isManagedDataDestination(destination string) bool {
	clean := filepath.ToSlash(filepath.Clean(destination))
	return clean == defaultDataMountPoint || strings.HasPrefix(clean, defaultDataMountPoint+"/")
}

func safeReportFilename(project string) (string, error) {
	project = strings.TrimSpace(project)
	if project == "" {
		return "", fmt.Errorf("project parameter required")
	}
	if strings.ContainsAny(project, `/\`) || project == "." || project == ".." {
		return "", fmt.Errorf("invalid project name")
	}
	return fmt.Sprintf("%s-ead-report.json", project), nil
}

func isPathWithin(root, path string) bool {
	root = filepath.Clean(root)
	path = filepath.Clean(path)
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != "." && rel != "" && !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel)
}

func quoteHeaderFilename(filename string) string {
	filename = strings.ReplaceAll(filename, `\`, "_")
	filename = strings.ReplaceAll(filename, `"`, "_")
	return filename
}
