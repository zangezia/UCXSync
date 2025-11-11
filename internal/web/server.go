package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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
	"github.com/zangezia/UCXSync/internal/monitor"
	"github.com/zangezia/UCXSync/internal/network"
	syncService "github.com/zangezia/UCXSync/internal/sync"
	"github.com/zangezia/UCXSync/pkg/models"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins in development
	},
}

// Server represents the web server
type Server struct {
	cfg         *config.Config
	syncService *syncService.Service
	monService  *monitor.Service
	netService  *network.Service
	webRoot     string

	mu      sync.RWMutex
	clients map[*websocket.Conn]bool
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
func NewServer(cfg *config.Config) *Server {
	svc := syncService.New(
		cfg.Nodes,
		cfg.Shares,
		"/mnt/ucx", // TODO: Get from config
	)

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

	return &Server{
		cfg:         cfg,
		syncService: svc,
		monService:  monService,
		netService:  netService,
		webRoot:     getWebRoot(),
		clients:     make(map[*websocket.Conn]bool),
	}
}

// Start starts the web server
func (s *Server) Start(ctx context.Context) error {
	// Check network requirements
	if err := network.CheckRequirements(); err != nil {
		log.Warn().Err(err).Msg("Network requirements check failed")
	}

	// Mount network shares
	if err := s.netService.MountAll(); err != nil {
		log.Error().Err(err).Msg("Failed to mount network shares")
	}

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
	mux.HandleFunc("/api/status", s.handleGetStatus)
	mux.HandleFunc("/api/sync/start", s.handleStartSync)
	mux.HandleFunc("/api/sync/stop", s.handleStopSync)
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

	// Wait for context cancellation
	<-ctx.Done()

	// Graceful shutdown
	log.Info().Msg("Shutting down web server...")

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

	projects, err := s.syncService.FindProjects(ctx)
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

	status := s.syncService.GetStatus()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (s *Server) handleStartSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Project        string `json:"project"`
		Destination    string `json:"destination"`
		MaxParallelism int    `json:"max_parallelism"`
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

	// Start sync
	ctx := context.Background()
	if err := s.syncService.Start(ctx, req.Project, req.Destination, req.MaxParallelism); err != nil {
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
			Message:   fmt.Sprintf("Started synchronization: project=%s, destination=%s", req.Project, req.Destination),
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
	status := s.syncService.GetStatus()
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
			status := s.syncService.GetStatus()
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
		if strings.HasPrefix(mountPoint, "/mnt/ucx") {
			continue
		}

		// Only allow external storage: /mnt/* and /media/*
		if !strings.HasPrefix(mountPoint, "/mnt/") && !strings.HasPrefix(mountPoint, "/media/") {
			continue
		}

		// USB/external storage devices
		if strings.HasPrefix(device, "/dev/sd") || strings.HasPrefix(device, "/dev/nvme") {
			destType = "usb"
			
			// Check if it's /mnt/storage (our default USB-SSD mount)
			if mountPoint == "/mnt/storage" {
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

// getBlockDevices returns list of all block devices using lsblk
func (s *Server) getBlockDevices() ([]models.BlockDeviceInfo, error) {
	var devices []models.BlockDeviceInfo

	// Use lsblk to get block device information
	cmd := exec.Command("lsblk", "-J", "-o", "NAME,SIZE,FSTYPE,LABEL,MOUNTPOINT,TYPE,RM,MODEL")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run lsblk: %w", err)
	}

	var lsblkOutput struct {
		BlockDevices []struct {
			Name       string `json:"name"`
			Size       string `json:"size"`
			FSType     string `json:"fstype"`
			Label      string `json:"label"`
			MountPoint string `json:"mountpoint"`
			Type       string `json:"type"`
			RM         string `json:"rm"` // Removable: "0" or "1"
			Model      string `json:"model"`
		} `json:"blockdevices"`
	}

	if err := json.Unmarshal(output, &lsblkOutput); err != nil {
		return nil, fmt.Errorf("failed to parse lsblk output: %w", err)
	}

	for _, dev := range lsblkOutput.BlockDevices {
		// Skip if no filesystem
		if dev.FSType == "" {
			continue
		}

		// Skip if device type is not "part" (partition)
		if dev.Type != "part" {
			continue
		}

		// Skip system partitions (mounted on /, /boot, /home, etc.)
		if dev.MountPoint == "/" ||
			strings.HasPrefix(dev.MountPoint, "/boot") ||
			strings.HasPrefix(dev.MountPoint, "/home") ||
			strings.HasPrefix(dev.MountPoint, "/var") ||
			strings.HasPrefix(dev.MountPoint, "/snap") {
			continue
		}

		// Skip UCX network mounts
		if strings.HasPrefix(dev.MountPoint, "/mnt/ucx") {
			continue
		}

		devicePath := "/dev/" + dev.Name
		isRemovable := dev.RM == "1"
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
	}

	// Sort: removable first, then by size (largest first)
	sort.Slice(devices, func(i, j int) bool {
		if devices[i].IsRemovable != devices[j].IsRemovable {
			return devices[i].IsRemovable
		}
		return devices[i].SizeBytes > devices[j].SizeBytes
	})

	return devices, nil
}

// mountDevice mounts a device to /mnt/storage
func (s *Server) mountDevice(devicePath string) error {
	mountPoint := "/mnt/storage"

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
	mountPoint := "/mnt/storage"

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

