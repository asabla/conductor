// Package metrics provides Prometheus metrics for the Conductor platform.
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds all Prometheus metrics for the Conductor platform.
type Metrics struct {
	registry *prometheus.Registry

	// Control plane metrics
	ControlPlane *ControlPlaneMetrics

	// Agent metrics
	Agent *AgentMetrics
}

// NewMetrics creates a new Metrics instance with all metrics registered.
func NewMetrics() *Metrics {
	registry := prometheus.NewRegistry()

	// Register default Go and process collectors
	registry.MustRegister(prometheus.NewGoCollector())
	registry.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))

	m := &Metrics{
		registry:     registry,
		ControlPlane: newControlPlaneMetrics(registry),
		Agent:        newAgentMetrics(registry),
	}

	return m
}

// NewControlPlaneMetrics creates metrics only for the control plane.
func NewControlPlaneMetrics() *Metrics {
	registry := prometheus.NewRegistry()

	// Register default Go and process collectors
	registry.MustRegister(prometheus.NewGoCollector())
	registry.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))

	m := &Metrics{
		registry:     registry,
		ControlPlane: newControlPlaneMetrics(registry),
	}

	return m
}

// NewAgentMetrics creates metrics only for agents.
func NewAgentMetrics() *Metrics {
	registry := prometheus.NewRegistry()

	// Register default Go and process collectors
	registry.MustRegister(prometheus.NewGoCollector())
	registry.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))

	m := &Metrics{
		registry: registry,
		Agent:    newAgentMetrics(registry),
	}

	return m
}

// Handler returns an HTTP handler for the /metrics endpoint.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(
		m.registry,
		promhttp.HandlerOpts{
			EnableOpenMetrics:   true,
			MaxRequestsInFlight: 10,
		},
	)
}

// Registry returns the underlying Prometheus registry.
func (m *Metrics) Registry() *prometheus.Registry {
	return m.registry
}
