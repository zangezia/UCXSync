package monitor

import (
	"context"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
	"github.com/zangezia/UCXSync/pkg/models"
)

// Service monitors system performance
type Service struct {
	updateInterval      time.Duration
	cpuSmoothingSamples int
	maxDiskMBps         float64
	networkSpeedBps     int64

	mu             sync.RWMutex
	cpuReadings    []float64
	lastNetTime    time.Time
	lastNetBytes   uint64
	targetDiskPath string
}

// New creates a new monitoring service
func New(updateInterval time.Duration, cpuSamples int, maxDiskMBps float64, networkSpeedBps int64) *Service {
	return &Service{
		updateInterval:      updateInterval,
		cpuSmoothingSamples: cpuSamples,
		maxDiskMBps:         maxDiskMBps,
		networkSpeedBps:     networkSpeedBps,
		cpuReadings:         make([]float64, 0, cpuSamples),
	}
}

// SetTargetDisk sets the disk to monitor
func (s *Service) SetTargetDisk(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.targetDiskPath = path
}

// Start begins monitoring
func (s *Service) Start(ctx context.Context) <-chan models.PerformanceMetrics {
	metricsChan := make(chan models.PerformanceMetrics, 10)

	go func() {
		defer close(metricsChan)

		ticker := time.NewTicker(s.updateInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				metrics := s.collectMetrics()
				select {
				case metricsChan <- metrics:
				default:
					// Channel full, skip this update
				}
			}
		}
	}()

	return metricsChan
}

func (s *Service) collectMetrics() models.PerformanceMetrics {
	metrics := models.PerformanceMetrics{}

	// CPU with smoothing
	cpuPercents, err := cpu.Percent(0, false)
	if err == nil && len(cpuPercents) > 0 {
		s.mu.Lock()
		s.cpuReadings = append(s.cpuReadings, cpuPercents[0])
		if len(s.cpuReadings) > s.cpuSmoothingSamples {
			s.cpuReadings = s.cpuReadings[1:]
		}

		// Calculate average
		var sum float64
		for _, v := range s.cpuReadings {
			sum += v
		}
		metrics.CPUPercent = sum / float64(len(s.cpuReadings))
		s.mu.Unlock()
	}

	// Memory
	memInfo, err := mem.VirtualMemory()
	if err == nil {
		metrics.MemoryUsedBytes = memInfo.Used
		metrics.MemoryTotalBytes = memInfo.Total
		metrics.MemoryPercent = memInfo.UsedPercent
	}

	// Disk I/O
	s.mu.RLock()
	diskPath := s.targetDiskPath
	s.mu.RUnlock()

	if diskPath != "" {
		// Get disk I/O stats
		ioCounters, err := disk.IOCounters()
		if err == nil {
			// Sum all disk I/O (simplified - in real app would filter by partition)
			var readBytes, writeBytes uint64
			for _, counter := range ioCounters {
				readBytes += counter.ReadBytes
				writeBytes += counter.WriteBytes
			}

			totalBytes := float64(readBytes + writeBytes)
			metrics.DiskBytesPerSec = totalBytes
			metrics.DiskMBps = totalBytes / 1024.0 / 1024.0
			metrics.DiskPercent = (metrics.DiskMBps / s.maxDiskMBps) * 100.0
			if metrics.DiskPercent > 100 {
				metrics.DiskPercent = 100
			}
		}

		// Free disk space
		usage, err := disk.Usage(diskPath)
		if err == nil {
			metrics.FreeDiskBytes = usage.Free
			metrics.FreeDiskGB = float64(usage.Free) / 1024.0 / 1024.0 / 1024.0
		}
	}

	// Network
	netStats, err := net.IOCounters(false)
	if err == nil && len(netStats) > 0 {
		stat := netStats[0]
		currentBytes := stat.BytesSent + stat.BytesRecv
		now := time.Now()

		s.mu.Lock()
		if !s.lastNetTime.IsZero() {
			elapsed := now.Sub(s.lastNetTime).Seconds()
			if elapsed > 0 {
				bytesDiff := float64(currentBytes - s.lastNetBytes)
				metrics.NetworkBytesPerSec = bytesDiff / elapsed
				metrics.NetworkMBps = metrics.NetworkBytesPerSec / 1024.0 / 1024.0

				// Calculate percentage of 1 Gbps
				bps := metrics.NetworkBytesPerSec * 8 // bytes to bits
				metrics.NetworkPercent = (bps / float64(s.networkSpeedBps)) * 100.0
				if metrics.NetworkPercent > 100 {
					metrics.NetworkPercent = 100
				}
			}
		}
		s.lastNetBytes = currentBytes
		s.lastNetTime = now
		s.mu.Unlock()
	}

	return metrics
}

// GetMetrics returns current metrics (one-time snapshot)
func (s *Service) GetMetrics() models.PerformanceMetrics {
	return s.collectMetrics()
}
