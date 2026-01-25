package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewMetrics(t *testing.T) {
	m := NewMetrics()

	if m == nil {
		t.Fatal("NewMetrics() returned nil")
	}

	if m.registry == nil {
		t.Error("registry should not be nil")
	}

	if m.ControlPlane == nil {
		t.Error("ControlPlane metrics should not be nil")
	}

	if m.Agent == nil {
		t.Error("Agent metrics should not be nil")
	}
}

func TestNewControlPlaneMetrics(t *testing.T) {
	m := NewControlPlaneMetrics()

	if m == nil {
		t.Fatal("NewControlPlaneMetrics() returned nil")
	}

	if m.ControlPlane == nil {
		t.Error("ControlPlane metrics should not be nil")
	}

	if m.Agent != nil {
		t.Error("Agent metrics should be nil for control plane only")
	}
}

func TestNewAgentMetrics(t *testing.T) {
	m := NewAgentMetrics()

	if m == nil {
		t.Fatal("NewAgentMetrics() returned nil")
	}

	if m.Agent == nil {
		t.Error("Agent metrics should not be nil")
	}

	if m.ControlPlane != nil {
		t.Error("ControlPlane metrics should be nil for agent only")
	}
}

func TestMetricsHandler(t *testing.T) {
	m := NewMetrics()

	handler := m.Handler()
	if handler == nil {
		t.Fatal("Handler() returned nil")
	}

	// Test that the handler serves metrics
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	body := w.Body.String()

	// Check for Go runtime metrics (always present)
	if !strings.Contains(body, "go_") {
		t.Error("expected Go runtime metrics in response")
	}

	// Check for process metrics (always present)
	if !strings.Contains(body, "process_") {
		t.Error("expected process metrics in response")
	}
}

func TestControlPlaneMetricsRecording(t *testing.T) {
	m := NewControlPlaneMetrics()

	// Test RecordAPIRequest
	m.ControlPlane.RecordAPIRequest("GET", "/api/runs", "200", 0.5)

	// Test RecordGRPCRequest
	m.ControlPlane.RecordGRPCRequest("CreateRun", "ok", 0.1)

	// Test RecordGRPCStream
	m.ControlPlane.RecordGRPCStream("WorkStream", "ok", 10.5)

	// Test RecordRunComplete
	m.ControlPlane.RecordRunComplete("passed", "test-service", 60.0)

	// Test RecordDBQuery
	m.ControlPlane.RecordDBQuery("SELECT", "runs", "ok", 0.01)

	// Test SetAgentCount
	m.ControlPlane.SetAgentCount("online", 5)
	m.ControlPlane.SetAgentCount("offline", 2)

	// Test SetActiveRuns
	m.ControlPlane.SetActiveRuns(10)

	// Test SetQueueDepth
	m.ControlPlane.SetQueueDepth("high", 5)
	m.ControlPlane.SetQueueDepth("normal", 20)

	// Test SetWebSocketConnections
	m.ControlPlane.SetWebSocketConnections(15)

	// Test IncrementGRPCStream and DecrementGRPCStream
	m.ControlPlane.IncrementGRPCStream("WorkStream")
	m.ControlPlane.DecrementGRPCStream("WorkStream")

	// Test SetDBConnections
	m.ControlPlane.SetDBConnections(10, 5)

	// Test RecordSchedulerDecision
	m.ControlPlane.RecordSchedulerDecision("assigned", 0.001)

	// Verify metrics are exposed
	handler := m.Handler()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	body := w.Body.String()

	// Check for our custom metrics
	expectedMetrics := []string{
		"conductor_http_request_duration_seconds",
		"conductor_http_requests_total",
		"conductor_grpc_request_duration_seconds",
		"conductor_grpc_requests_total",
		"conductor_control_plane_agents_total",
		"conductor_control_plane_runs_active",
		"conductor_control_plane_queue_depth",
	}

	for _, metric := range expectedMetrics {
		if !strings.Contains(body, metric) {
			t.Errorf("expected metric %s in response", metric)
		}
	}
}

func TestAgentMetricsRecording(t *testing.T) {
	m := NewAgentMetrics()

	// Test RecordWorkComplete
	m.Agent.RecordWorkComplete("passed", "subprocess", 120.0)

	// Test SetCPUUsage, SetMemoryUsage, SetDiskUsage
	m.Agent.SetCPUUsage(50.5)
	m.Agent.SetMemoryUsage(60.2)
	m.Agent.SetDiskUsage(30.0)

	// Test SetConnected and SetDisconnected
	m.Agent.SetConnected()
	m.Agent.SetDisconnected()

	// Test RecordHeartbeat with latency
	m.Agent.RecordHeartbeat(0.05)
	m.Agent.RecordHeartbeat(0.1)
	m.Agent.RecordHeartbeatFailure()

	// Test RecordRepoClone
	m.Agent.RecordRepoClone("success", 5.5)
	m.Agent.RecordRepoClone("failure", 2.0)

	// Test SetActiveWork
	m.Agent.SetActiveWork(3)

	// Test RecordTestComplete
	m.Agent.RecordTestComplete("passed", 1.5)

	// Test SetMemoryBytes and SetDiskBytes
	m.Agent.SetMemoryBytes(1024*1024*512, 1024*1024*1024, 1024*1024*1024+1024*1024*512)
	m.Agent.SetDiskBytes(1024*1024*1024*50, 1024*1024*1024*100, 1024*1024*1024*150)

	// Test RecordExecutorError
	m.Agent.RecordExecutorError("container", "image_pull")

	// Verify metrics are exposed
	handler := m.Handler()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	body := w.Body.String()

	// Check for our custom metrics
	expectedMetrics := []string{
		"conductor_agent_work_duration_seconds",
		"conductor_agent_work_total",
		"conductor_agent_cpu_usage_percent",
		"conductor_agent_memory_usage_percent",
		"conductor_agent_disk_usage_percent",
	}

	for _, metric := range expectedMetrics {
		if !strings.Contains(body, metric) {
			t.Errorf("expected metric %s in response", metric)
		}
	}
}

func TestMetricsRegistry(t *testing.T) {
	m := NewMetrics()

	registry := m.Registry()
	if registry == nil {
		t.Error("Registry() should not return nil")
	}

	// Verify we can gather metrics from the registry
	families, err := registry.Gather()
	if err != nil {
		t.Errorf("failed to gather metrics: %v", err)
	}

	if len(families) == 0 {
		t.Error("expected at least some metric families")
	}
}
