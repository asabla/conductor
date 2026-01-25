package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// AgentMetrics holds all metrics for agents.
type AgentMetrics struct {
	// Work execution metrics
	WorkDuration *prometheus.HistogramVec
	WorkTotal    *prometheus.CounterVec
	WorkActive   prometheus.Gauge

	// Test execution metrics
	TestDuration *prometheus.HistogramVec
	TestsTotal   *prometheus.CounterVec
	TestsActive  prometheus.Gauge

	// Resource metrics
	CPUUsage    prometheus.Gauge
	MemoryUsage prometheus.Gauge
	DiskUsage   prometheus.Gauge
	MemoryBytes *prometheus.GaugeVec
	DiskBytes   *prometheus.GaugeVec

	// Connection metrics
	ConnectionState   *prometheus.GaugeVec
	ReconnectTotal    prometheus.Counter
	HeartbeatLatency  prometheus.Histogram
	HeartbeatsTotal   prometheus.Counter
	HeartbeatFailures prometheus.Counter

	// Repository metrics
	RepoCloneDuration prometheus.Histogram
	RepoClonesTotal   *prometheus.CounterVec

	// Executor metrics
	ExecutorErrors *prometheus.CounterVec
}

// newAgentMetrics creates and registers all agent metrics.
func newAgentMetrics(registry *prometheus.Registry) *AgentMetrics {
	m := &AgentMetrics{
		// Work execution metrics
		WorkDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "conductor",
				Subsystem: "agent",
				Name:      "work_duration_seconds",
				Help:      "Duration of test execution in seconds.",
				Buckets:   []float64{1, 5, 10, 30, 60, 120, 300, 600, 1800, 3600, 7200},
			},
			[]string{"status", "execution_type"},
		),

		WorkTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "conductor",
				Subsystem: "agent",
				Name:      "work_total",
				Help:      "Total number of work items executed.",
			},
			[]string{"status", "execution_type"},
		),

		WorkActive: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "conductor",
				Subsystem: "agent",
				Name:      "work_active",
				Help:      "Number of currently active work items.",
			},
		),

		// Test execution metrics
		TestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "conductor",
				Subsystem: "agent",
				Name:      "test_duration_seconds",
				Help:      "Duration of individual test execution in seconds.",
				Buckets:   []float64{0.1, 0.5, 1, 5, 10, 30, 60, 120, 300, 600},
			},
			[]string{"status"},
		),

		TestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "conductor",
				Subsystem: "agent",
				Name:      "tests_total",
				Help:      "Total number of individual tests executed.",
			},
			[]string{"status"},
		),

		TestsActive: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "conductor",
				Subsystem: "agent",
				Name:      "tests_active",
				Help:      "Number of currently running tests.",
			},
		),

		// Resource metrics
		CPUUsage: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "conductor",
				Subsystem: "agent",
				Name:      "cpu_usage_percent",
				Help:      "Current CPU usage as a percentage (0-100).",
			},
		),

		MemoryUsage: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "conductor",
				Subsystem: "agent",
				Name:      "memory_usage_percent",
				Help:      "Current memory usage as a percentage (0-100).",
			},
		),

		DiskUsage: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "conductor",
				Subsystem: "agent",
				Name:      "disk_usage_percent",
				Help:      "Current disk usage as a percentage (0-100).",
			},
		),

		MemoryBytes: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "conductor",
				Subsystem: "agent",
				Name:      "memory_bytes",
				Help:      "Memory in bytes.",
			},
			[]string{"type"}, // used, available, total
		),

		DiskBytes: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "conductor",
				Subsystem: "agent",
				Name:      "disk_bytes",
				Help:      "Disk space in bytes.",
			},
			[]string{"type"}, // used, available, total
		),

		// Connection metrics
		ConnectionState: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "conductor",
				Subsystem: "agent",
				Name:      "connection_state",
				Help:      "Current connection state (1=connected, 0=disconnected).",
			},
			[]string{"state"},
		),

		ReconnectTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: "conductor",
				Subsystem: "agent",
				Name:      "reconnects_total",
				Help:      "Total number of reconnection attempts.",
			},
		),

		HeartbeatLatency: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: "conductor",
				Subsystem: "agent",
				Name:      "heartbeat_latency_seconds",
				Help:      "Latency of heartbeat round trips.",
				Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
			},
		),

		HeartbeatsTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: "conductor",
				Subsystem: "agent",
				Name:      "heartbeats_total",
				Help:      "Total number of heartbeats sent.",
			},
		),

		HeartbeatFailures: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: "conductor",
				Subsystem: "agent",
				Name:      "heartbeat_failures_total",
				Help:      "Total number of failed heartbeats.",
			},
		),

		// Repository metrics
		RepoCloneDuration: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: "conductor",
				Subsystem: "agent",
				Name:      "repo_clone_duration_seconds",
				Help:      "Duration of repository clone operations.",
				Buckets:   []float64{1, 5, 10, 30, 60, 120, 300},
			},
		),

		RepoClonesTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "conductor",
				Subsystem: "agent",
				Name:      "repo_clones_total",
				Help:      "Total number of repository clone operations.",
			},
			[]string{"status"}, // success, failure, cached
		),

		// Executor metrics
		ExecutorErrors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "conductor",
				Subsystem: "agent",
				Name:      "executor_errors_total",
				Help:      "Total number of executor errors.",
			},
			[]string{"executor_type", "error_type"},
		),
	}

	// Register all metrics
	registry.MustRegister(
		m.WorkDuration,
		m.WorkTotal,
		m.WorkActive,
		m.TestDuration,
		m.TestsTotal,
		m.TestsActive,
		m.CPUUsage,
		m.MemoryUsage,
		m.DiskUsage,
		m.MemoryBytes,
		m.DiskBytes,
		m.ConnectionState,
		m.ReconnectTotal,
		m.HeartbeatLatency,
		m.HeartbeatsTotal,
		m.HeartbeatFailures,
		m.RepoCloneDuration,
		m.RepoClonesTotal,
		m.ExecutorErrors,
	)

	return m
}

// RecordWorkComplete records a completed work item.
func (m *AgentMetrics) RecordWorkComplete(status, executionType string, durationSeconds float64) {
	m.WorkDuration.WithLabelValues(status, executionType).Observe(durationSeconds)
	m.WorkTotal.WithLabelValues(status, executionType).Inc()
}

// RecordTestComplete records a completed test.
func (m *AgentMetrics) RecordTestComplete(status string, durationSeconds float64) {
	m.TestDuration.WithLabelValues(status).Observe(durationSeconds)
	m.TestsTotal.WithLabelValues(status).Inc()
}

// SetActiveWork sets the count of active work items.
func (m *AgentMetrics) SetActiveWork(count float64) {
	m.WorkActive.Set(count)
}

// SetActiveTests sets the count of active tests.
func (m *AgentMetrics) SetActiveTests(count float64) {
	m.TestsActive.Set(count)
}

// SetCPUUsage sets the current CPU usage percentage.
func (m *AgentMetrics) SetCPUUsage(percent float64) {
	m.CPUUsage.Set(percent)
}

// SetMemoryUsage sets the current memory usage percentage.
func (m *AgentMetrics) SetMemoryUsage(percent float64) {
	m.MemoryUsage.Set(percent)
}

// SetDiskUsage sets the current disk usage percentage.
func (m *AgentMetrics) SetDiskUsage(percent float64) {
	m.DiskUsage.Set(percent)
}

// SetMemoryBytes sets the memory metrics in bytes.
func (m *AgentMetrics) SetMemoryBytes(used, available, total uint64) {
	m.MemoryBytes.WithLabelValues("used").Set(float64(used))
	m.MemoryBytes.WithLabelValues("available").Set(float64(available))
	m.MemoryBytes.WithLabelValues("total").Set(float64(total))
}

// SetDiskBytes sets the disk metrics in bytes.
func (m *AgentMetrics) SetDiskBytes(used, available, total uint64) {
	m.DiskBytes.WithLabelValues("used").Set(float64(used))
	m.DiskBytes.WithLabelValues("available").Set(float64(available))
	m.DiskBytes.WithLabelValues("total").Set(float64(total))
}

// SetConnected sets the connection state to connected.
func (m *AgentMetrics) SetConnected() {
	m.ConnectionState.WithLabelValues("connected").Set(1)
	m.ConnectionState.WithLabelValues("disconnected").Set(0)
}

// SetDisconnected sets the connection state to disconnected.
func (m *AgentMetrics) SetDisconnected() {
	m.ConnectionState.WithLabelValues("connected").Set(0)
	m.ConnectionState.WithLabelValues("disconnected").Set(1)
}

// RecordReconnect records a reconnection attempt.
func (m *AgentMetrics) RecordReconnect() {
	m.ReconnectTotal.Inc()
}

// RecordHeartbeat records a successful heartbeat with latency.
func (m *AgentMetrics) RecordHeartbeat(latencySeconds float64) {
	m.HeartbeatsTotal.Inc()
	m.HeartbeatLatency.Observe(latencySeconds)
}

// RecordHeartbeatFailure records a failed heartbeat.
func (m *AgentMetrics) RecordHeartbeatFailure() {
	m.HeartbeatFailures.Inc()
}

// RecordRepoClone records a repository clone operation.
func (m *AgentMetrics) RecordRepoClone(status string, durationSeconds float64) {
	m.RepoClonesTotal.WithLabelValues(status).Inc()
	m.RepoCloneDuration.Observe(durationSeconds)
}

// RecordExecutorError records an executor error.
func (m *AgentMetrics) RecordExecutorError(executorType, errorType string) {
	m.ExecutorErrors.WithLabelValues(executorType, errorType).Inc()
}
