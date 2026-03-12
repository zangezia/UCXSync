package monitor

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
	"github.com/zangezia/UCXSync/pkg/models"
)

type netSnapshot struct {
	bytes uint64
	at    time.Time
}

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
	lastInterface  map[string]netSnapshot
	lastDiskTime   time.Time
	lastDiskBytes  uint64
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
		lastInterface:       make(map[string]netSnapshot),
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

	// CPU temperature
	if temperatures, err := host.SensorsTemperatures(); err == nil {
		if temp, ok := selectCPUTemperature(temperatures); ok {
			metrics.CPUTemperatureCelsius = temp
			metrics.CPUTemperatureAvailable = true
		}
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

			currentDiskBytes := readBytes + writeBytes
			now := time.Now()

			s.mu.Lock()
			if !s.lastDiskTime.IsZero() {
				elapsed := now.Sub(s.lastDiskTime).Seconds()
				if elapsed > 0 {
					bytesDiff := float64(currentDiskBytes - s.lastDiskBytes)
					metrics.DiskBytesPerSec = bytesDiff / elapsed
					metrics.DiskMBps = metrics.DiskBytesPerSec / 1024.0 / 1024.0
					metrics.DiskPercent = (metrics.DiskMBps / s.maxDiskMBps) * 100.0
					if metrics.DiskPercent > 100 {
						metrics.DiskPercent = 100
					}
				}
			}
			s.lastDiskBytes = currentDiskBytes
			s.lastDiskTime = now
			s.mu.Unlock()
		}

		// Free disk space
		usage, err := disk.Usage(diskPath)
		if err == nil {
			metrics.FreeDiskBytes = usage.Free
			metrics.FreeDiskGB = float64(usage.Free) / 1024.0 / 1024.0 / 1024.0
		}
	}

	// Network (aggregate + per-interface)
	netStats, err := net.IOCounters(true)
	if err == nil && len(netStats) > 0 {
		now := time.Now()
		currentBytes := uint64(0)
		seenInterfaces := make(map[string]struct{})
		interfaceMetrics := make([]models.NetworkInterfaceMetrics, 0, len(netStats))

		s.mu.Lock()
		for _, stat := range netStats {
			if !shouldMonitorInterface(stat.Name) {
				continue
			}

			seenInterfaces[stat.Name] = struct{}{}
			ifaceBytes := stat.BytesSent + stat.BytesRecv
			currentBytes += ifaceBytes

			ifaceMetric := models.NetworkInterfaceMetrics{Name: stat.Name}
			if prev, ok := s.lastInterface[stat.Name]; ok {
				elapsed := now.Sub(prev.at).Seconds()
				if elapsed > 0 && ifaceBytes >= prev.bytes {
					bytesDiff := float64(ifaceBytes - prev.bytes)
					ifaceMetric.BytesPerSec = bytesDiff / elapsed
					ifaceMetric.MBps = ifaceMetric.BytesPerSec / 1024.0 / 1024.0
					ifaceMetric.Percent = networkPercent(ifaceMetric.BytesPerSec, s.networkSpeedBps)
				}
			}
			s.lastInterface[stat.Name] = netSnapshot{bytes: ifaceBytes, at: now}
			interfaceMetrics = append(interfaceMetrics, ifaceMetric)
		}

		for name := range s.lastInterface {
			if _, ok := seenInterfaces[name]; !ok {
				delete(s.lastInterface, name)
			}
		}

		if !s.lastNetTime.IsZero() {
			elapsed := now.Sub(s.lastNetTime).Seconds()
			if elapsed > 0 && currentBytes >= s.lastNetBytes {
				bytesDiff := float64(currentBytes - s.lastNetBytes)
				metrics.NetworkBytesPerSec = bytesDiff / elapsed
				metrics.NetworkMBps = metrics.NetworkBytesPerSec / 1024.0 / 1024.0
				metrics.NetworkPercent = networkPercent(metrics.NetworkBytesPerSec, s.networkSpeedBps)
			}
		}

		s.lastNetBytes = currentBytes
		s.lastNetTime = now
		s.mu.Unlock()

		sort.Slice(interfaceMetrics, func(i, j int) bool {
			leftScore := preferredInterfaceScore(interfaceMetrics[i].Name)
			rightScore := preferredInterfaceScore(interfaceMetrics[j].Name)
			if leftScore != rightScore {
				return leftScore < rightScore
			}
			if interfaceMetrics[i].BytesPerSec != interfaceMetrics[j].BytesPerSec {
				return interfaceMetrics[i].BytesPerSec > interfaceMetrics[j].BytesPerSec
			}
			return interfaceMetrics[i].Name < interfaceMetrics[j].Name
		})

		metrics.NetworkInterfaces = interfaceMetrics
	}

	return metrics
}

// GetMetrics returns current metrics (one-time snapshot)
func (s *Service) GetMetrics() models.PerformanceMetrics {
	return s.collectMetrics()
}

func shouldMonitorInterface(name string) bool {
	lower := strings.ToLower(name)
	if lower == "" || lower == "lo" {
		return false
	}

	excludedPrefixes := []string{"docker", "br-", "veth", "virbr", "tun", "tap", "zt", "tailscale", "wg"}
	for _, prefix := range excludedPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return false
		}
	}

	return true
}

func networkPercent(bytesPerSec float64, networkSpeedBps int64) float64 {
	if networkSpeedBps <= 0 {
		return 0
	}

	percent := ((bytesPerSec * 8) / float64(networkSpeedBps)) * 100.0
	if percent > 100 {
		return 100
	}
	if percent < 0 {
		return 0
	}
	return percent
}

func preferredInterfaceScore(name string) int {
	switch strings.ToLower(name) {
	case "end0":
		return 0
	case "end1":
		return 1
	default:
		return 10
	}
}

func selectCPUTemperature(temperatures []host.TemperatureStat) (float64, bool) {
	keywords := []string{"cpu", "package", "core", "soc", "tctl", "tdie"}
	bestPreferred := 0.0
	hasPreferred := false
	bestFallback := 0.0
	hasFallback := false

	for _, sensor := range temperatures {
		if sensor.Temperature <= 0 {
			continue
		}

		name := strings.ToLower(sensor.SensorKey)
		if sensor.Temperature > bestFallback {
			bestFallback = sensor.Temperature
			hasFallback = true
		}

		for _, keyword := range keywords {
			if strings.Contains(name, keyword) {
				if !hasPreferred || sensor.Temperature > bestPreferred {
					bestPreferred = sensor.Temperature
					hasPreferred = true
				}
				break
			}
		}
	}

	if hasPreferred {
		return bestPreferred, true
	}
	if hasFallback {
		return bestFallback, true
	}

	return 0, false
}
