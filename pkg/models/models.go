package models

import "time"

// SyncTask represents an active synchronization task
type SyncTask struct {
	Node         string    `json:"node"`
	Share        string    `json:"share"`
	Status       string    `json:"status"`
	LastActivity time.Time `json:"last_activity"`
	TotalFiles   int       `json:"total_files"`
	CopiedFiles  int       `json:"copied_files"`
	FailedFiles  int       `json:"failed_files"`
	TotalBytes   int64     `json:"total_bytes"`
	CopiedBytes  int64     `json:"copied_bytes"`
	Progress     float64   `json:"progress"`
}

// CaptureInfo holds information about a capture file
type CaptureInfo struct {
	DataType      string `json:"data_type"`      // Lvl0X (unverified) or Lvl00 (verified)
	CaptureNumber string `json:"capture_number"` // 00001, 00002, etc.
	IsTest        bool   `json:"is_test"`        // true if test capture (has "T-" marker)
	ProjectName   string `json:"project_name"`   // e.g., Arh2k_mezen_200725
	SensorCode    string `json:"sensor_code"`    // e.g., 06-00, 00-00, 00-01, etc.
	SessionID     string `json:"session_id"`     // Unique session GUID
	IsVerified    bool   `json:"is_verified"`    // true if Lvl00, false if Lvl0X
}

// SyncStatus holds overall synchronization status
type SyncStatus struct {
	IsRunning             bool       `json:"is_running"`
	Project               string     `json:"project"`
	Destination           string     `json:"destination"`
	MaxParallelism        int        `json:"max_parallelism"`        // Configured limit
	ActiveFileOperations  int        `json:"active_file_operations"` // Current active file copies
	CompletedCaptures     int        `json:"completed_captures"`
	CompletedTestCaptures int        `json:"completed_test_captures"`
	LastCaptureNumber     string     `json:"last_capture_number"`
	LastTestCaptureNumber string     `json:"last_test_capture_number"`
	ActiveTasks           []SyncTask `json:"active_tasks"`
}

// PersistedCaptureStatus holds per-project persisted capture counters and progress.
type PersistedCaptureStatus struct {
	CompletedCaptures     int    `json:"completed_captures"`
	CompletedTestCaptures int    `json:"completed_test_captures"`
	LastCaptureNumber     string `json:"last_capture_number"`
	LastTestCaptureNumber string `json:"last_test_capture_number"`
	RawCount              int    `json:"raw_count"`
	HasXML                bool   `json:"has_xml"`
	HasDAT                bool   `json:"has_dat"`
}

// NetworkInterfaceMetrics holds throughput data for a specific network interface.
type NetworkInterfaceMetrics struct {
	Name        string  `json:"name"`
	BytesPerSec float64 `json:"bytes_per_sec"`
	MBps        float64 `json:"mbps"`
	Percent     float64 `json:"percent"`
}

// PerformanceMetrics holds system performance data
type PerformanceMetrics struct {
	CPUPercent              float64                   `json:"cpu_percent"`
	CPUTemperatureCelsius   float64                   `json:"cpu_temperature_celsius"`
	CPUTemperatureAvailable bool                      `json:"cpu_temperature_available"`
	MemoryUsedBytes         uint64                    `json:"memory_used_bytes"`
	MemoryTotalBytes        uint64                    `json:"memory_total_bytes"`
	MemoryPercent           float64                   `json:"memory_percent"`
	DiskBytesPerSec         float64                   `json:"disk_bytes_per_sec"`
	DiskMBps                float64                   `json:"disk_mbps"`
	DiskPercent             float64                   `json:"disk_percent"`
	NetworkBytesPerSec      float64                   `json:"network_bytes_per_sec"`
	NetworkMBps             float64                   `json:"network_mbps"`
	NetworkPercent          float64                   `json:"network_percent"`
	NetworkInterfaces       []NetworkInterfaceMetrics `json:"network_interfaces"`
	FreeDiskBytes           uint64                    `json:"free_disk_bytes"`
	FreeDiskGB              float64                   `json:"free_disk_gb"`
}

// ProjectInfo holds information about an available project
type ProjectInfo struct {
	Name   string `json:"name"`
	Source string `json:"source"` // First node/share where found
}

// ProjectDatabaseSummary describes one project persisted in the local SQLite DB.
type ProjectDatabaseSummary struct {
	Name                  string `json:"name"`
	Source                string `json:"source"`
	LastSeenAt            string `json:"last_seen_at"`
	Captures              int    `json:"captures"`
	CompletedCaptures     int    `json:"completed_captures"`
	CompletedTestCaptures int    `json:"completed_test_captures"`
	CopiedFiles           int    `json:"copied_files"`
	EADRecords            int    `json:"ead_records"`
	EADProcessingRecords  int    `json:"ead_processing_records"`
	LastCaptureNumber     string `json:"last_capture_number"`
	LastTestCaptureNumber string `json:"last_test_capture_number"`
}

// DestinationInfo holds information about available destination paths
type DestinationInfo struct {
	Path        string  `json:"path"`
	Label       string  `json:"label"`
	Type        string  `json:"type"` // "usb", "disk", "network"
	FreeSpaceGB float64 `json:"free_space_gb"`
	TotalGB     float64 `json:"total_gb"`
	IsDefault   bool    `json:"is_default"`
}

// PreflightCheck describes one readiness condition for starting synchronization.
type PreflightCheck struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

// PreflightUnavailableShare describes one inaccessible UCX share.
type PreflightUnavailableShare struct {
	Node  string `json:"node"`
	Share string `json:"share"`
	Path  string `json:"path"`
}

// PreflightStatus is the payload used by the start-readiness panel.
type PreflightStatus struct {
	Ready                 bool                        `json:"ready"`
	IsRunning             bool                        `json:"is_running"`
	SelectedProject       string                      `json:"selected_project"`
	SelectedDestination   string                      `json:"selected_destination"`
	ActiveProject         string                      `json:"active_project,omitempty"`
	ActiveDestination     string                      `json:"active_destination,omitempty"`
	AvailableProjects     int                         `json:"available_projects"`
	AvailableDestinations int                         `json:"available_destinations"`
	FreeSpaceGB           float64                     `json:"free_space_gb,omitempty"`
	RequiredFreeSpaceGB   float64                     `json:"required_free_space_gb,omitempty"`
	UnavailableShares     []PreflightUnavailableShare `json:"unavailable_shares,omitempty"`
	Checks                []PreflightCheck            `json:"checks"`
}

// DashboardPreflightInstanceStatus contains readiness details for one dashboard instance.
type DashboardPreflightInstanceStatus struct {
	ID                    string                      `json:"id"`
	Name                  string                      `json:"name"`
	URL                   string                      `json:"url"`
	Available             bool                        `json:"available"`
	Error                 string                      `json:"error,omitempty"`
	Ready                 bool                        `json:"ready"`
	IsRunning             bool                        `json:"is_running"`
	AvailableProjects     int                         `json:"available_projects"`
	AvailableDestinations int                         `json:"available_destinations"`
	FreeSpaceGB           float64                     `json:"free_space_gb,omitempty"`
	RequiredFreeSpaceGB   float64                     `json:"required_free_space_gb,omitempty"`
	UnavailableShares     []PreflightUnavailableShare `json:"unavailable_shares,omitempty"`
	Checks                []PreflightCheck            `json:"checks,omitempty"`
}

// DashboardPreflightStatus contains the aggregate readiness state across dashboard instances.
type DashboardPreflightStatus struct {
	Ready               bool                               `json:"ready"`
	SelectedProject     string                             `json:"selected_project"`
	SelectedDestination string                             `json:"selected_destination"`
	Checks              []PreflightCheck                   `json:"checks"`
	Instances           []DashboardPreflightInstanceStatus `json:"instances"`
}

// BlockDeviceInfo holds information about a block device
type BlockDeviceInfo struct {
	DevicePath  string `json:"device_path"`  // e.g., /dev/sdb1
	DeviceName  string `json:"device_name"`  // e.g., sdb1
	Label       string `json:"label"`        // Filesystem label
	Size        string `json:"size"`         // Human readable size
	SizeBytes   uint64 `json:"size_bytes"`   // Size in bytes
	FSType      string `json:"fstype"`       // Filesystem type (ext4, exfat, ntfs, etc)
	MountPoint  string `json:"mount_point"`  // Where mounted, empty if not mounted
	IsMounted   bool   `json:"is_mounted"`   // Mount status
	IsRemovable bool   `json:"is_removable"` // USB/removable device
	Model       string `json:"model"`        // Device model name
}

// MountRequest represents a mount/unmount request
type MountRequest struct {
	DevicePath string `json:"device_path"` // e.g., /dev/sdb1
	Action     string `json:"action"`      // "mount" or "unmount"
}

// LogMessage represents a log entry
type LogMessage struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
}

// WSMessage represents a WebSocket message
type WSMessage struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

// DashboardInstanceConfig describes one instance connected to the shared dashboard.
type DashboardInstanceConfig struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

// DashboardConfig describes the shared dashboard mode.
type DashboardConfig struct {
	Enabled   bool                      `json:"enabled"`
	Instances []DashboardInstanceConfig `json:"instances"`
}

// DashboardInstanceState contains the last known state of one UCXSync instance.
type DashboardInstanceState struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	URL       string     `json:"url"`
	Available bool       `json:"available"`
	Error     string     `json:"error,omitempty"`
	Status    SyncStatus `json:"status"`
}

// DashboardSummary contains aggregate counters across all configured instances.
type DashboardSummary struct {
	ConfiguredInstances    int    `json:"configured_instances"`
	AvailableInstances     int    `json:"available_instances"`
	RunningInstances       int    `json:"running_instances"`
	TotalCompletedCaptures int    `json:"total_completed_captures"`
	TotalCompletedTest     int    `json:"total_completed_test_captures"`
	LastCaptureNumber      string `json:"last_capture_number"`
	LastTestCaptureNumber  string `json:"last_test_capture_number"`
	TotalActiveFileOps     int    `json:"total_active_file_operations"`
	TotalMaxParallelism    int    `json:"total_max_parallelism"`
	TotalActiveTasks       int    `json:"total_active_tasks"`
}

// DashboardOverview is the main payload used by the shared dashboard UI.
type DashboardOverview struct {
	Config      DashboardConfig          `json:"config"`
	HostMetrics PerformanceMetrics       `json:"host_metrics"`
	Summary     DashboardSummary         `json:"summary"`
	Instances   []DashboardInstanceState `json:"instances"`
}

// DashboardActionResult contains the result of one proxied action.
type DashboardActionResult struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Success    bool   `json:"success"`
	StatusCode int    `json:"status_code,omitempty"`
	Error      string `json:"error,omitempty"`
}

// DashboardActionResponse contains results for an action fan-out across instances.
type DashboardActionResponse struct {
	Action  string                  `json:"action"`
	Results []DashboardActionResult `json:"results"`
}
