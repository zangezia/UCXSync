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
	DataType      string `json:"data_type"`       // Lvl01X, Lvl02X, etc.
	CaptureNumber string `json:"capture_number"`  // 00001, 00002, etc.
	IsTest        bool   `json:"is_test"`         // true if test capture
	ProjectName   string `json:"project_name"`
	SessionID     string `json:"session_id"`
}

// SyncStatus holds overall synchronization status
type SyncStatus struct {
	IsRunning              bool        `json:"is_running"`
	Project                string      `json:"project"`
	Destination            string      `json:"destination"`
	CompletedCaptures      int         `json:"completed_captures"`
	CompletedTestCaptures  int         `json:"completed_test_captures"`
	LastCaptureNumber      string      `json:"last_capture_number"`
	LastTestCaptureNumber  string      `json:"last_test_capture_number"`
	ActiveTasks            []SyncTask  `json:"active_tasks"`
}

// PerformanceMetrics holds system performance data
type PerformanceMetrics struct {
	CPUPercent         float64 `json:"cpu_percent"`
	MemoryUsedBytes    uint64  `json:"memory_used_bytes"`
	MemoryTotalBytes   uint64  `json:"memory_total_bytes"`
	MemoryPercent      float64 `json:"memory_percent"`
	DiskBytesPerSec    float64 `json:"disk_bytes_per_sec"`
	DiskMBps           float64 `json:"disk_mbps"`
	DiskPercent        float64 `json:"disk_percent"`
	NetworkBytesPerSec float64 `json:"network_bytes_per_sec"`
	NetworkMBps        float64 `json:"network_mbps"`
	NetworkPercent     float64 `json:"network_percent"`
	FreeDiskBytes      uint64  `json:"free_disk_bytes"`
	FreeDiskGB         float64 `json:"free_disk_gb"`
}

// ProjectInfo holds information about an available project
type ProjectInfo struct {
	Name   string `json:"name"`
	Source string `json:"source"` // First node/share where found
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
