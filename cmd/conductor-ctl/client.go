package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client wraps HTTP client for API operations
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewClient creates a new API client
func NewClient(server, token string) *Client {
	// Ensure server has protocol prefix
	if !strings.HasPrefix(server, "http://") && !strings.HasPrefix(server, "https://") {
		server = "http://" + server
	}

	return &Client{
		baseURL: strings.TrimSuffix(server, "/"),
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// request makes an HTTP request to the API
func (c *Client) request(ctx context.Context, method, path string, body interface{}, result interface{}) error {
	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		var errResp struct {
			Message string `json:"message"`
			Error   string `json:"error"`
			Code    int    `json:"code"`
		}
		if err := json.Unmarshal(respBody, &errResp); err == nil {
			if errResp.Message != "" {
				return fmt.Errorf("API error (%d): %s", resp.StatusCode, errResp.Message)
			}
			if errResp.Error != "" {
				return fmt.Errorf("API error (%d): %s", resp.StatusCode, errResp.Error)
			}
		}
		return fmt.Errorf("API error (%d): %s", resp.StatusCode, string(respBody))
	}

	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}
	}

	return nil
}

// Agent represents an agent in the system
type Agent struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Status        string            `json:"status"`
	Version       string            `json:"version"`
	NetworkZones  []string          `json:"network_zones"`
	Labels        map[string]string `json:"labels"`
	OS            string            `json:"os"`
	Arch          string            `json:"arch"`
	Hostname      string            `json:"hostname"`
	IPAddress     string            `json:"ip_address"`
	MaxParallel   int               `json:"max_parallel"`
	ActiveRunCnt  int               `json:"active_run_count"`
	LastHeartbeat string            `json:"last_heartbeat"`
	RegisteredAt  string            `json:"registered_at"`
	CurrentRuns   []string          `json:"current_runs"`
	Capabilities  *AgentCaps        `json:"capabilities"`
	ResourceUsage *ResourceUsage    `json:"resource_usage"`
}

// AgentCaps represents agent capabilities
type AgentCaps struct {
	Runtimes        []string `json:"runtimes"`
	DockerAvailable bool     `json:"docker_available"`
	CPUCores        int      `json:"cpu_cores"`
	MemoryBytes     int64    `json:"memory_bytes"`
	DiskBytes       int64    `json:"disk_bytes"`
}

// ResourceUsage represents resource utilization
type ResourceUsage struct {
	CPUPercent       float64 `json:"cpu_percent"`
	MemoryBytes      int64   `json:"memory_bytes"`
	MemoryTotalBytes int64   `json:"memory_total_bytes"`
	DiskBytes        int64   `json:"disk_bytes"`
	DiskTotalBytes   int64   `json:"disk_total_bytes"`
}

// ListAgentsResponse is the response from listing agents
type ListAgentsResponse struct {
	Agents     []Agent             `json:"agents"`
	Pagination *PaginationResponse `json:"pagination"`
}

// PaginationResponse contains pagination metadata
type PaginationResponse struct {
	NextPageToken string `json:"next_page_token"`
	TotalCount    int64  `json:"total_count"`
	HasMore       bool   `json:"has_more"`
}

// ListAgents lists all agents
func (c *Client) ListAgents(ctx context.Context, status string, zone string, limit int) (*ListAgentsResponse, error) {
	path := "/api/v1/agents"
	params := url.Values{}
	if status != "" {
		params.Add("statuses", status)
	}
	if zone != "" {
		params.Add("network_zone", zone)
	}
	if limit > 0 {
		params.Add("pagination.page_size", fmt.Sprintf("%d", limit))
	}
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	var resp ListAgentsResponse
	if err := c.request(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetAgent retrieves a specific agent
func (c *Client) GetAgent(ctx context.Context, agentID string, includeRuns bool) (*Agent, []AgentRun, error) {
	path := fmt.Sprintf("/api/v1/agents/%s", agentID)
	if includeRuns {
		path += "?include_current_runs=true"
	}

	var resp struct {
		Agent       Agent      `json:"agent"`
		CurrentRuns []AgentRun `json:"current_runs"`
	}
	if err := c.request(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, nil, err
	}
	return &resp.Agent, resp.CurrentRuns, nil
}

// AgentRun represents a run being executed by an agent
type AgentRun struct {
	RunID           string `json:"run_id"`
	ServiceName     string `json:"service_name"`
	StartedAt       string `json:"started_at"`
	ProgressPercent int    `json:"progress_percent"`
}

// DrainAgent puts an agent into draining mode
func (c *Client) DrainAgent(ctx context.Context, agentID string, reason string, cancelActive bool) (*Agent, int, error) {
	path := fmt.Sprintf("/api/v1/agents/%s/drain", agentID)
	body := map[string]interface{}{
		"reason":        reason,
		"cancel_active": cancelActive,
	}

	var resp struct {
		Agent         Agent `json:"agent"`
		CancelledRuns int   `json:"cancelled_runs"`
	}
	if err := c.request(ctx, http.MethodPost, path, body, &resp); err != nil {
		return nil, 0, err
	}
	return &resp.Agent, resp.CancelledRuns, nil
}

// UndrainAgent removes an agent from draining mode
func (c *Client) UndrainAgent(ctx context.Context, agentID string) (*Agent, error) {
	path := fmt.Sprintf("/api/v1/agents/%s/undrain", agentID)

	var resp struct {
		Agent Agent `json:"agent"`
	}
	if err := c.request(ctx, http.MethodPost, path, map[string]interface{}{}, &resp); err != nil {
		return nil, err
	}
	return &resp.Agent, nil
}

// Run represents a test run
type Run struct {
	ID            string            `json:"id"`
	ServiceID     string            `json:"service_id"`
	ServiceName   string            `json:"service_name"`
	Status        string            `json:"status"`
	GitRef        *GitRef           `json:"git_ref"`
	Trigger       *RunTrigger       `json:"trigger"`
	ExecutionType string            `json:"execution_type"`
	AgentID       string            `json:"agent_id"`
	CreatedAt     string            `json:"created_at"`
	StartedAt     string            `json:"started_at"`
	FinishedAt    string            `json:"finished_at"`
	Summary       *RunSummary       `json:"summary"`
	ErrorMessage  string            `json:"error_message"`
	Labels        map[string]string `json:"labels"`
	Priority      int               `json:"priority"`
	Timeout       *Duration         `json:"timeout"`
	RetryOfRunID  string            `json:"retry_of_run_id"`
	RetryCount    int               `json:"retry_count"`
}

// GitRef represents a git reference
type GitRef struct {
	RepositoryURL     string `json:"repository_url"`
	Branch            string `json:"branch"`
	CommitSHA         string `json:"commit_sha"`
	CommitSHAShort    string `json:"commit_sha_short"`
	PullRequestNumber int64  `json:"pull_request_number"`
	Tag               string `json:"tag"`
}

// RunTrigger describes what initiated a run
type RunTrigger struct {
	Type              string `json:"type"`
	User              string `json:"user"`
	CIJobID           string `json:"ci_job_id"`
	CIPipelineURL     string `json:"ci_pipeline_url"`
	WebhookDeliveryID string `json:"webhook_delivery_id"`
}

// RunSummary provides aggregate test statistics
type RunSummary struct {
	Total    int       `json:"total"`
	Passed   int       `json:"passed"`
	Failed   int       `json:"failed"`
	Skipped  int       `json:"skipped"`
	Errored  int       `json:"errored"`
	Duration *Duration `json:"duration"`
}

// Duration represents a time duration
type Duration struct {
	Seconds int64 `json:"seconds"`
	Nanos   int32 `json:"nanos"`
}

// ListRunsResponse is the response from listing runs
type ListRunsResponse struct {
	Runs       []Run               `json:"runs"`
	Pagination *PaginationResponse `json:"pagination"`
}

// ListRuns lists test runs with optional filters
func (c *Client) ListRuns(ctx context.Context, serviceID, status string, limit int) (*ListRunsResponse, error) {
	path := "/api/v1/runs"
	params := url.Values{}
	if serviceID != "" {
		params.Add("service_id", serviceID)
	}
	if status != "" {
		params.Add("statuses", status)
	}
	if limit > 0 {
		params.Add("pagination.page_size", fmt.Sprintf("%d", limit))
	}
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	var resp ListRunsResponse
	if err := c.request(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetRun retrieves a specific run
func (c *Client) GetRun(ctx context.Context, runID string, includeResults, includeArtifacts bool) (*Run, []TestResult, []Artifact, error) {
	path := fmt.Sprintf("/api/v1/runs/%s", runID)
	params := url.Values{}
	if includeResults {
		params.Add("include_results", "true")
	}
	if includeArtifacts {
		params.Add("include_artifacts", "true")
	}
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	var resp struct {
		Run       Run          `json:"run"`
		Results   []TestResult `json:"results"`
		Artifacts []Artifact   `json:"artifacts"`
	}
	if err := c.request(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, nil, nil, err
	}
	return &resp.Run, resp.Results, resp.Artifacts, nil
}

// TestResult represents the outcome of a single test
type TestResult struct {
	ID           string            `json:"id"`
	RunID        string            `json:"run_id"`
	TestID       string            `json:"test_id"`
	TestName     string            `json:"test_name"`
	TestPath     string            `json:"test_path"`
	Status       string            `json:"status"`
	Duration     *Duration         `json:"duration"`
	ErrorMessage string            `json:"error_message"`
	StackTrace   string            `json:"stack_trace"`
	RetryAttempt int               `json:"retry_attempt"`
	Timestamp    string            `json:"timestamp"`
	Metadata     map[string]string `json:"metadata"`
	SystemOut    string            `json:"system_out"`
	SystemErr    string            `json:"system_err"`
}

// Artifact represents a file produced during test execution
type Artifact struct {
	ID          string `json:"id"`
	RunID       string `json:"run_id"`
	TestID      string `json:"test_id"`
	Name        string `json:"name"`
	Path        string `json:"path"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	URL         string `json:"url"`
	Checksum    string `json:"checksum"`
	CreatedAt   string `json:"created_at"`
	ExpiresAt   string `json:"expires_at"`
}

// CreateRunRequest specifies parameters for creating a new run
type CreateRunRequest struct {
	ServiceID     string            `json:"service_id"`
	GitRef        *GitRef           `json:"git_ref,omitempty"`
	TestIDs       []string          `json:"test_ids,omitempty"`
	Tags          []string          `json:"tags,omitempty"`
	Environment   map[string]string `json:"environment,omitempty"`
	Priority      int               `json:"priority,omitempty"`
	ExecutionType string            `json:"execution_type,omitempty"`
	Timeout       *Duration         `json:"timeout,omitempty"`
	Trigger       *RunTrigger       `json:"trigger,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
}

// CreateRun creates a new test run
func (c *Client) CreateRun(ctx context.Context, req *CreateRunRequest) (*Run, error) {
	var resp struct {
		Run Run `json:"run"`
	}
	if err := c.request(ctx, http.MethodPost, "/api/v1/runs", req, &resp); err != nil {
		return nil, err
	}
	return &resp.Run, nil
}

// CancelRun cancels a pending or running test run
func (c *Client) CancelRun(ctx context.Context, runID, reason string) (*Run, error) {
	path := fmt.Sprintf("/api/v1/runs/%s/cancel", runID)
	body := map[string]interface{}{
		"reason": reason,
	}

	var resp struct {
		Run Run `json:"run"`
	}
	if err := c.request(ctx, http.MethodPost, path, body, &resp); err != nil {
		return nil, err
	}
	return &resp.Run, nil
}

// RetryRun creates a new run from a previous run
func (c *Client) RetryRun(ctx context.Context, runID string, failedOnly bool, envOverride map[string]string) (*Run, string, error) {
	path := fmt.Sprintf("/api/v1/runs/%s/retry", runID)
	body := map[string]interface{}{
		"failed_only": failedOnly,
	}
	if len(envOverride) > 0 {
		body["environment_override"] = envOverride
	}

	var resp struct {
		Run           Run    `json:"run"`
		OriginalRunID string `json:"original_run_id"`
	}
	if err := c.request(ctx, http.MethodPost, path, body, &resp); err != nil {
		return nil, "", err
	}
	return &resp.Run, resp.OriginalRunID, nil
}

// LogEntry represents a single log entry
type LogEntry struct {
	Sequence  int64  `json:"sequence"`
	Timestamp string `json:"timestamp"`
	Stream    string `json:"stream"`
	Message   string `json:"message"`
	TestID    string `json:"test_id"`
}

// GetRunLogs retrieves logs for a completed run
func (c *Client) GetRunLogs(ctx context.Context, runID string, stream string, testID string, limit int) ([]LogEntry, error) {
	path := fmt.Sprintf("/api/v1/runs/%s/logs", runID)
	params := url.Values{}
	if stream != "" {
		params.Add("stream", stream)
	}
	if testID != "" {
		params.Add("test_id", testID)
	}
	if limit > 0 {
		params.Add("pagination.page_size", fmt.Sprintf("%d", limit))
	}
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	var resp struct {
		Entries []LogEntry `json:"entries"`
	}
	if err := c.request(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return resp.Entries, nil
}

// Service represents a registered service
type Service struct {
	ID                   string            `json:"id"`
	Name                 string            `json:"name"`
	GitURL               string            `json:"git_url"`
	DefaultBranch        string            `json:"default_branch"`
	NetworkZones         []string          `json:"network_zones"`
	Owner                string            `json:"owner"`
	Contact              *Contact          `json:"contact"`
	DefaultExecutionType string            `json:"default_execution_type"`
	DefaultContainerImg  string            `json:"default_container_image"`
	DefaultTimeout       *Duration         `json:"default_timeout"`
	ConfigPath           string            `json:"config_path"`
	Labels               map[string]string `json:"labels"`
	Active               bool              `json:"active"`
	CreatedAt            string            `json:"created_at"`
	UpdatedAt            string            `json:"updated_at"`
	LastSyncedAt         string            `json:"last_synced_at"`
	TestCount            int               `json:"test_count"`
}

// Contact information for service owners
type Contact struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Slack string `json:"slack"`
}

// TestDefinition describes a test that can be executed
type TestDefinition struct {
	ID                   string            `json:"id"`
	ServiceID            string            `json:"service_id"`
	Name                 string            `json:"name"`
	Path                 string            `json:"path"`
	Type                 string            `json:"type"`
	Command              string            `json:"command"`
	ResultFormat         string            `json:"result_format"`
	Timeout              *Duration         `json:"timeout"`
	Tags                 []string          `json:"tags"`
	Environment          map[string]string `json:"environment"`
	ArtifactPaths        []string          `json:"artifact_paths"`
	Enabled              bool              `json:"enabled"`
	RetryCount           int               `json:"retry_count"`
	RequiredRuntimes     []string          `json:"required_runtimes"`
	RequiredNetworkZones []string          `json:"required_network_zones"`
	CreatedAt            string            `json:"created_at"`
	UpdatedAt            string            `json:"updated_at"`
	EstimatedDuration    *Duration         `json:"estimated_duration"`
	FlakinessRate        float64           `json:"flakiness_rate"`
}

// ListServicesResponse is the response from listing services
type ListServicesResponse struct {
	Services   []Service           `json:"services"`
	Pagination *PaginationResponse `json:"pagination"`
}

// ListServices lists all services
func (c *Client) ListServices(ctx context.Context, owner, zone, query string, limit int) (*ListServicesResponse, error) {
	path := "/api/v1/services"
	params := url.Values{}
	if owner != "" {
		params.Add("owner", owner)
	}
	if zone != "" {
		params.Add("network_zone", zone)
	}
	if query != "" {
		params.Add("query", query)
	}
	if limit > 0 {
		params.Add("pagination.page_size", fmt.Sprintf("%d", limit))
	}
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	var resp ListServicesResponse
	if err := c.request(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetService retrieves a specific service
func (c *Client) GetService(ctx context.Context, serviceID string, includeTests, includeRuns bool) (*Service, []TestDefinition, []RecentRun, error) {
	path := fmt.Sprintf("/api/v1/services/%s", serviceID)
	params := url.Values{}
	if includeTests {
		params.Add("include_tests", "true")
	}
	if includeRuns {
		params.Add("include_recent_runs", "true")
	}
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	var resp struct {
		Service    Service          `json:"service"`
		Tests      []TestDefinition `json:"tests"`
		RecentRuns []RecentRun      `json:"recent_runs"`
	}
	if err := c.request(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, nil, nil, err
	}
	return &resp.Service, resp.Tests, resp.RecentRuns, nil
}

// RecentRun is a summary of a recent test run
type RecentRun struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	Branch    string    `json:"branch"`
	CommitSHA string    `json:"commit_sha"`
	CreatedAt string    `json:"created_at"`
	Duration  *Duration `json:"duration"`
	Passed    int       `json:"passed"`
	Failed    int       `json:"failed"`
	Total     int       `json:"total"`
}

// CreateServiceRequest specifies parameters for creating a service
type CreateServiceRequest struct {
	Name                 string            `json:"name"`
	GitURL               string            `json:"git_url"`
	DefaultBranch        string            `json:"default_branch,omitempty"`
	NetworkZones         []string          `json:"network_zones,omitempty"`
	Owner                string            `json:"owner,omitempty"`
	Contact              *Contact          `json:"contact,omitempty"`
	DefaultExecutionType string            `json:"default_execution_type,omitempty"`
	DefaultContainerImg  string            `json:"default_container_image,omitempty"`
	DefaultTimeout       *Duration         `json:"default_timeout,omitempty"`
	ConfigPath           string            `json:"config_path,omitempty"`
	Labels               map[string]string `json:"labels,omitempty"`
}

// CreateService creates a new service
func (c *Client) CreateService(ctx context.Context, req *CreateServiceRequest) (*Service, error) {
	var resp struct {
		Service Service `json:"service"`
	}
	if err := c.request(ctx, http.MethodPost, "/api/v1/services", req, &resp); err != nil {
		return nil, err
	}
	return &resp.Service, nil
}

// SyncService triggers test discovery from the repository
func (c *Client) SyncService(ctx context.Context, serviceID, branch string, deleteMissing bool) (*SyncResult, error) {
	path := fmt.Sprintf("/api/v1/services/%s/sync", serviceID)
	body := map[string]interface{}{
		"delete_missing": deleteMissing,
	}
	if branch != "" {
		body["branch"] = branch
	}

	var resp SyncResult
	if err := c.request(ctx, http.MethodPost, path, body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// SyncResult contains the results of a sync operation
type SyncResult struct {
	TestsAdded   int      `json:"tests_added"`
	TestsUpdated int      `json:"tests_updated"`
	TestsRemoved int      `json:"tests_removed"`
	Errors       []string `json:"errors"`
	SyncedAt     string   `json:"synced_at"`
}
