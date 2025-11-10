package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
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
		fsType := fields[2]

		// Skip if already processed
		if seen[mountPoint] {
			continue
		}
		seen[mountPoint] = true

		// Determine if this is a USB device or regular disk
		var destType string
		var label string
		isDefault := false

		// USB devices (typically /dev/sd* mounted on /media or /mnt)
		if strings.HasPrefix(device, "/dev/sd") && 
		   (strings.HasPrefix(mountPoint, "/media/") || 
		    strings.HasPrefix(mountPoint, "/mnt/") && !strings.HasPrefix(mountPoint, "/mnt/ucx")) {
			destType = "usb"
			label = fmt.Sprintf("USB: %s", filepath.Base(mountPoint))
		} else if fsType == "ext4" || fsType == "xfs" || fsType == "btrfs" {
			// Regular disk partitions
			if mountPoint == "/" {
				destType = "disk"
				label = "System Root (/)"
				isDefault = true
			} else if strings.HasPrefix(mountPoint, "/home") {
				destType = "disk"
				label = fmt.Sprintf("Home: %s", mountPoint)
			} else if strings.HasPrefix(mountPoint, "/data") || strings.HasPrefix(mountPoint, "/storage") {
				destType = "disk"
				label = fmt.Sprintf("Data: %s", mountPoint)
			} else {
				continue // Skip other system mounts
			}
		} else {
			continue // Skip network mounts, tmpfs, etc.
		}

		// Get disk space info
		var stat syscall.Statfs_t
		if err := syscall.Statfs(mountPoint, &stat); err != nil {
			continue
		}

		totalGB := float64(stat.Blocks*uint64(stat.Bsize)) / 1024 / 1024 / 1024
		freeGB := float64(stat.Bavail*uint64(stat.Bsize)) / 1024 / 1024 / 1024

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
