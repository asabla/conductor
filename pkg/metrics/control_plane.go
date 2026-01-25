package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// ControlPlaneMetrics holds all metrics for the control plane.
type ControlPlaneMetrics struct {
	// Agent metrics
	AgentsTotal *prometheus.GaugeVec

	// Run metrics
	RunsTotal     *prometheus.CounterVec
	RunsActive    prometheus.Gauge
	RunDuration   *prometheus.HistogramVec
	QueueDepth    *prometheus.GaugeVec
	QueueWaitTime *prometheus.HistogramVec

	// API metrics
	APIRequestDuration *prometheus.HistogramVec
	APIRequestsTotal   *prometheus.CounterVec

	// WebSocket metrics
	WebSocketConnections   prometheus.Gauge
	WebSocketMessagesTotal *prometheus.CounterVec

	// gRPC metrics
	GRPCRequestDuration *prometheus.HistogramVec
	GRPCRequestsTotal   *prometheus.CounterVec
	GRPCStreamDuration  *prometheus.HistogramVec
	GRPCStreamsActive   *prometheus.GaugeVec

	// Database metrics
	DBQueryDuration     *prometheus.HistogramVec
	DBQueriesTotal      *prometheus.CounterVec
	DBConnectionsActive prometheus.Gauge
	DBConnectionsIdle   prometheus.Gauge

	// Scheduler metrics
	SchedulerDecisions *prometheus.CounterVec
	SchedulerLatency   prometheus.Histogram
}

// newControlPlaneMetrics creates and registers all control plane metrics.
func newControlPlaneMetrics(registry *prometheus.Registry) *ControlPlaneMetrics {
	m := &ControlPlaneMetrics{
		// Agent metrics
		AgentsTotal: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "conductor",
				Subsystem: "control_plane",
				Name:      "agents_total",
				Help:      "Total number of agents by status.",
			},
			[]string{"status"},
		),

		// Run metrics
		RunsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "conductor",
				Subsystem: "control_plane",
				Name:      "runs_total",
				Help:      "Total number of test runs by status.",
			},
			[]string{"status"},
		),

		RunsActive: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "conductor",
				Subsystem: "control_plane",
				Name:      "runs_active",
				Help:      "Number of currently running tests.",
			},
		),

		RunDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "conductor",
				Subsystem: "control_plane",
				Name:      "run_duration_seconds",
				Help:      "Duration of test runs in seconds.",
				Buckets:   []float64{1, 5, 10, 30, 60, 120, 300, 600, 1800, 3600},
			},
			[]string{"status", "service"},
		),

		QueueDepth: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "conductor",
				Subsystem: "control_plane",
				Name:      "queue_depth",
				Help:      "Number of pending runs in the queue.",
			},
			[]string{"priority"},
		),

		QueueWaitTime: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "conductor",
				Subsystem: "control_plane",
				Name:      "queue_wait_seconds",
				Help:      "Time spent waiting in the queue before execution.",
				Buckets:   []float64{0.1, 0.5, 1, 5, 10, 30, 60, 120, 300},
			},
			[]string{"priority"},
		),

		// API metrics
		APIRequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "conductor",
				Subsystem: "http",
				Name:      "request_duration_seconds",
				Help:      "HTTP API request latency in seconds.",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"method", "path", "status"},
		),

		APIRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "conductor",
				Subsystem: "http",
				Name:      "requests_total",
				Help:      "Total number of HTTP API requests.",
			},
			[]string{"method", "path", "status"},
		),

		// WebSocket metrics
		WebSocketConnections: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "conductor",
				Subsystem: "websocket",
				Name:      "connections_active",
				Help:      "Number of active WebSocket connections.",
			},
		),

		WebSocketMessagesTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "conductor",
				Subsystem: "websocket",
				Name:      "messages_total",
				Help:      "Total number of WebSocket messages.",
			},
			[]string{"direction", "type"},
		),

		// gRPC metrics
		GRPCRequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "conductor",
				Subsystem: "grpc",
				Name:      "request_duration_seconds",
				Help:      "gRPC unary request latency in seconds.",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"method", "status"},
		),

		GRPCRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "conductor",
				Subsystem: "grpc",
				Name:      "requests_total",
				Help:      "Total number of gRPC unary requests.",
			},
			[]string{"method", "status"},
		),

		GRPCStreamDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "conductor",
				Subsystem: "grpc",
				Name:      "stream_duration_seconds",
				Help:      "gRPC stream duration in seconds.",
				Buckets:   []float64{1, 5, 10, 30, 60, 300, 600, 1800, 3600},
			},
			[]string{"method", "status"},
		),

		GRPCStreamsActive: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "conductor",
				Subsystem: "grpc",
				Name:      "streams_active",
				Help:      "Number of active gRPC streams.",
			},
			[]string{"method"},
		),

		// Database metrics
		DBQueryDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "conductor",
				Subsystem: "database",
				Name:      "query_duration_seconds",
				Help:      "Database query latency in seconds.",
				Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
			},
			[]string{"operation", "table"},
		),

		DBQueriesTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "conductor",
				Subsystem: "database",
				Name:      "queries_total",
				Help:      "Total number of database queries.",
			},
			[]string{"operation", "table", "status"},
		),

		DBConnectionsActive: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "conductor",
				Subsystem: "database",
				Name:      "connections_active",
				Help:      "Number of active database connections.",
			},
		),

		DBConnectionsIdle: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "conductor",
				Subsystem: "database",
				Name:      "connections_idle",
				Help:      "Number of idle database connections.",
			},
		),

		// Scheduler metrics
		SchedulerDecisions: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "conductor",
				Subsystem: "scheduler",
				Name:      "decisions_total",
				Help:      "Total number of scheduling decisions.",
			},
			[]string{"decision"},
		),

		SchedulerLatency: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: "conductor",
				Subsystem: "scheduler",
				Name:      "decision_duration_seconds",
				Help:      "Time taken to make scheduling decisions.",
				Buckets:   []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.025, 0.05, 0.1},
			},
		),
	}

	// Register all metrics
	registry.MustRegister(
		m.AgentsTotal,
		m.RunsTotal,
		m.RunsActive,
		m.RunDuration,
		m.QueueDepth,
		m.QueueWaitTime,
		m.APIRequestDuration,
		m.APIRequestsTotal,
		m.WebSocketConnections,
		m.WebSocketMessagesTotal,
		m.GRPCRequestDuration,
		m.GRPCRequestsTotal,
		m.GRPCStreamDuration,
		m.GRPCStreamsActive,
		m.DBQueryDuration,
		m.DBQueriesTotal,
		m.DBConnectionsActive,
		m.DBConnectionsIdle,
		m.SchedulerDecisions,
		m.SchedulerLatency,
	)

	return m
}

// RecordAPIRequest records an HTTP API request.
func (m *ControlPlaneMetrics) RecordAPIRequest(method, path, status string, durationSeconds float64) {
	m.APIRequestDuration.WithLabelValues(method, path, status).Observe(durationSeconds)
	m.APIRequestsTotal.WithLabelValues(method, path, status).Inc()
}

// RecordGRPCRequest records a gRPC unary request.
func (m *ControlPlaneMetrics) RecordGRPCRequest(method, status string, durationSeconds float64) {
	m.GRPCRequestDuration.WithLabelValues(method, status).Observe(durationSeconds)
	m.GRPCRequestsTotal.WithLabelValues(method, status).Inc()
}

// RecordGRPCStream records a gRPC stream completion.
func (m *ControlPlaneMetrics) RecordGRPCStream(method, status string, durationSeconds float64) {
	m.GRPCStreamDuration.WithLabelValues(method, status).Observe(durationSeconds)
}

// RecordRunComplete records a completed test run.
func (m *ControlPlaneMetrics) RecordRunComplete(status, service string, durationSeconds float64) {
	m.RunsTotal.WithLabelValues(status).Inc()
	m.RunDuration.WithLabelValues(status, service).Observe(durationSeconds)
}

// RecordDBQuery records a database query.
func (m *ControlPlaneMetrics) RecordDBQuery(operation, table, status string, durationSeconds float64) {
	m.DBQueryDuration.WithLabelValues(operation, table).Observe(durationSeconds)
	m.DBQueriesTotal.WithLabelValues(operation, table, status).Inc()
}

// SetAgentCount sets the count of agents by status.
func (m *ControlPlaneMetrics) SetAgentCount(status string, count float64) {
	m.AgentsTotal.WithLabelValues(status).Set(count)
}

// SetActiveRuns sets the count of active runs.
func (m *ControlPlaneMetrics) SetActiveRuns(count float64) {
	m.RunsActive.Set(count)
}

// SetQueueDepth sets the queue depth for a priority level.
func (m *ControlPlaneMetrics) SetQueueDepth(priority string, count float64) {
	m.QueueDepth.WithLabelValues(priority).Set(count)
}

// SetWebSocketConnections sets the count of active WebSocket connections.
func (m *ControlPlaneMetrics) SetWebSocketConnections(count float64) {
	m.WebSocketConnections.Set(count)
}

// IncrementGRPCStream increments the active stream count.
func (m *ControlPlaneMetrics) IncrementGRPCStream(method string) {
	m.GRPCStreamsActive.WithLabelValues(method).Inc()
}

// DecrementGRPCStream decrements the active stream count.
func (m *ControlPlaneMetrics) DecrementGRPCStream(method string) {
	m.GRPCStreamsActive.WithLabelValues(method).Dec()
}

// SetDBConnections sets the database connection counts.
func (m *ControlPlaneMetrics) SetDBConnections(active, idle float64) {
	m.DBConnectionsActive.Set(active)
	m.DBConnectionsIdle.Set(idle)
}

// RecordSchedulerDecision records a scheduling decision.
func (m *ControlPlaneMetrics) RecordSchedulerDecision(decision string, durationSeconds float64) {
	m.SchedulerDecisions.WithLabelValues(decision).Inc()
	m.SchedulerLatency.Observe(durationSeconds)
}
