package agent

import (
	"os"
	"runtime"
	"sync"
	"syscall"
	"time"

	conductorv1 "github.com/conductor/conductor/api/gen/conductor/v1"
	"github.com/rs/zerolog"
)

// Monitor tracks system resource usage.
type Monitor struct {
	config *Config
	logger zerolog.Logger
	mu     sync.RWMutex

	// Current usage
	cpuPercent  float64
	memoryBytes int64
	memoryTotal int64
	diskBytes   int64
	diskTotal   int64
	lastUpdate  time.Time

	// CPU tracking
	prevIdleTime  uint64
	prevTotalTime uint64
}

// NewMonitor creates a new resource monitor.
func NewMonitor(cfg *Config, logger zerolog.Logger) *Monitor {
	m := &Monitor{
		config: cfg,
		logger: logger.With().Str("component", "monitor").Logger(),
	}

	// Initial update
	m.Update()

	return m
}

// Update refreshes the resource usage metrics.
func (m *Monitor) Update() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Update CPU usage
	m.updateCPU()

	// Update memory usage
	m.updateMemory()

	// Update disk usage
	m.updateDisk()

	m.lastUpdate = time.Now()
}

// GetCPUUsage returns the current CPU usage percentage.
func (m *Monitor) GetCPUUsage() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cpuPercent
}

// GetMemoryUsage returns current memory usage in bytes.
func (m *Monitor) GetMemoryUsage() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.memoryBytes
}

// GetDiskUsage returns current disk usage in bytes.
func (m *Monitor) GetDiskUsage() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.diskBytes
}

// GetUsage returns the current resource usage as a proto message.
func (m *Monitor) GetUsage() *conductorv1.ResourceUsage {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return &conductorv1.ResourceUsage{
		CpuPercent:       m.cpuPercent,
		MemoryBytes:      m.memoryBytes,
		MemoryTotalBytes: m.memoryTotal,
		DiskBytes:        m.diskBytes,
		DiskTotalBytes:   m.diskTotal,
	}
}

// GetResources returns the available system resources.
func (m *Monitor) GetResources() *conductorv1.Resources {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return &conductorv1.Resources{
		CpuCores:    int32(runtime.NumCPU()),
		MemoryBytes: m.memoryTotal,
		DiskBytes:   m.diskTotal,
	}
}

// CanAcceptWork checks if the system has enough resources to accept more work.
func (m *Monitor) CanAcceptWork() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Check CPU
	if m.cpuPercent >= m.config.CPUThreshold {
		m.logger.Debug().
			Float64("cpu_percent", m.cpuPercent).
			Float64("threshold", m.config.CPUThreshold).
			Msg("CPU threshold exceeded")
		return false
	}

	// Check memory
	if m.memoryTotal > 0 {
		memPercent := float64(m.memoryBytes) / float64(m.memoryTotal) * 100
		if memPercent >= m.config.MemoryThreshold {
			m.logger.Debug().
				Float64("memory_percent", memPercent).
				Float64("threshold", m.config.MemoryThreshold).
				Msg("Memory threshold exceeded")
			return false
		}
	}

	// Check disk
	if m.diskTotal > 0 {
		diskPercent := float64(m.diskBytes) / float64(m.diskTotal) * 100
		if diskPercent >= m.config.DiskThreshold {
			m.logger.Debug().
				Float64("disk_percent", diskPercent).
				Float64("threshold", m.config.DiskThreshold).
				Msg("Disk threshold exceeded")
			return false
		}
	}

	return true
}

// updateCPU updates CPU usage metrics.
func (m *Monitor) updateCPU() {
	// Read /proc/stat on Linux
	if runtime.GOOS == "linux" {
		m.updateCPULinux()
		return
	}

	// For other platforms, use a simplified approach
	// In production, you might want to use a cross-platform library
	m.cpuPercent = 0
}

// updateCPULinux reads CPU stats from /proc/stat.
func (m *Monitor) updateCPULinux() {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		m.logger.Debug().Err(err).Msg("Failed to read /proc/stat")
		return
	}

	var user, nice, system, idle, iowait, irq, softirq, steal uint64
	_, err = parseFirstLine(string(data), &user, &nice, &system, &idle, &iowait, &irq, &softirq, &steal)
	if err != nil {
		m.logger.Debug().Err(err).Msg("Failed to parse /proc/stat")
		return
	}

	idleTime := idle + iowait
	totalTime := user + nice + system + idle + iowait + irq + softirq + steal

	if m.prevTotalTime > 0 {
		idleDelta := idleTime - m.prevIdleTime
		totalDelta := totalTime - m.prevTotalTime

		if totalDelta > 0 {
			m.cpuPercent = (1.0 - float64(idleDelta)/float64(totalDelta)) * 100
		}
	}

	m.prevIdleTime = idleTime
	m.prevTotalTime = totalTime
}

// parseFirstLine parses the first cpu line from /proc/stat.
func parseFirstLine(data string, values ...*uint64) (int, error) {
	var n int
	var line string

	// Find first line
	for i, c := range data {
		if c == '\n' {
			line = data[:i]
			break
		}
	}

	if line == "" {
		line = data
	}

	// Skip "cpu " prefix
	if len(line) > 4 && line[:4] == "cpu " {
		line = line[4:]
	}

	// Parse values
	var val uint64
	for _, v := range values {
		val = 0
		for len(line) > 0 && line[0] == ' ' {
			line = line[1:]
		}
		for len(line) > 0 && line[0] >= '0' && line[0] <= '9' {
			val = val*10 + uint64(line[0]-'0')
			line = line[1:]
		}
		*v = val
		n++
	}

	return n, nil
}

// updateMemory updates memory usage metrics.
func (m *Monitor) updateMemory() {
	if runtime.GOOS == "linux" {
		m.updateMemoryLinux()
		return
	}

	// For other platforms, use runtime.MemStats as an approximation
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	m.memoryBytes = int64(memStats.Alloc)
	m.memoryTotal = int64(memStats.Sys)
}

// updateMemoryLinux reads memory stats from /proc/meminfo.
func (m *Monitor) updateMemoryLinux() {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		m.logger.Debug().Err(err).Msg("Failed to read /proc/meminfo")
		return
	}

	var total, free, available, buffers, cached int64
	lines := splitLines(string(data))

	for _, line := range lines {
		switch {
		case hasPrefix(line, "MemTotal:"):
			total = parseMemValue(line)
		case hasPrefix(line, "MemFree:"):
			free = parseMemValue(line)
		case hasPrefix(line, "MemAvailable:"):
			available = parseMemValue(line)
		case hasPrefix(line, "Buffers:"):
			buffers = parseMemValue(line)
		case hasPrefix(line, "Cached:"):
			cached = parseMemValue(line)
		}
	}

	m.memoryTotal = total * 1024 // Convert from KB to bytes

	// Calculate used memory
	if available > 0 {
		m.memoryBytes = (total - available) * 1024
	} else {
		m.memoryBytes = (total - free - buffers - cached) * 1024
	}
}

// updateDisk updates disk usage metrics.
func (m *Monitor) updateDisk() {
	// Get disk usage for the workspace directory
	path := m.config.WorkspaceDir
	if path == "" {
		path = "/"
	}

	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		m.logger.Debug().Err(err).Str("path", path).Msg("Failed to get disk stats")
		return
	}

	m.diskTotal = int64(stat.Blocks) * int64(stat.Bsize)
	m.diskBytes = int64(stat.Blocks-stat.Bfree) * int64(stat.Bsize)
}

// splitLines splits a string into lines.
func splitLines(s string) []string {
	var lines []string
	var start int

	for i, c := range s {
		if c == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}

	if start < len(s) {
		lines = append(lines, s[start:])
	}

	return lines
}

// hasPrefix checks if s starts with prefix.
func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

// parseMemValue extracts the numeric value from a meminfo line.
func parseMemValue(line string) int64 {
	var value int64

	// Skip to first digit
	i := 0
	for i < len(line) && (line[i] < '0' || line[i] > '9') {
		i++
	}

	// Parse number
	for i < len(line) && line[i] >= '0' && line[i] <= '9' {
		value = value*10 + int64(line[i]-'0')
		i++
	}

	return value
}
