package git

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
)

// StatusState represents the state of a commit status.
type StatusState string

const (
	StatusStatePending StatusState = "pending"
	StatusStateRunning StatusState = "running"
	StatusStateSuccess StatusState = "success"
	StatusStateFailure StatusState = "failure"
	StatusStateError   StatusState = "error"
)

// StatusReporter reports test run status to git providers.
type StatusReporter struct {
	providers map[string]Provider
	logger    *slog.Logger
	baseURL   string // Base URL for constructing target URLs
	mu        sync.RWMutex
}

// StatusReporterConfig holds configuration for the status reporter.
type StatusReporterConfig struct {
	BaseURL string
	Logger  *slog.Logger
}

// NewStatusReporter creates a new status reporter.
func NewStatusReporter(cfg StatusReporterConfig) *StatusReporter {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &StatusReporter{
		providers: make(map[string]Provider),
		logger:    logger.With("component", "status_reporter"),
		baseURL:   strings.TrimSuffix(cfg.BaseURL, "/"),
	}
}

// RegisterProvider registers a provider for a specific git host.
func (s *StatusReporter) RegisterProvider(host string, provider Provider) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.providers[strings.ToLower(host)] = provider
}

// ReportPending reports that a test run is pending.
func (s *StatusReporter) ReportPending(ctx context.Context, gitURL, sha, runID, description string) error {
	return s.report(ctx, gitURL, sha, runID, StatusStatePending, description)
}

// ReportRunning reports that a test run is in progress.
func (s *StatusReporter) ReportRunning(ctx context.Context, gitURL, sha, runID, description string) error {
	return s.report(ctx, gitURL, sha, runID, StatusStateRunning, description)
}

// ReportSuccess reports that a test run succeeded.
func (s *StatusReporter) ReportSuccess(ctx context.Context, gitURL, sha, runID, summary string) error {
	return s.report(ctx, gitURL, sha, runID, StatusStateSuccess, summary)
}

// ReportFailure reports that a test run failed.
func (s *StatusReporter) ReportFailure(ctx context.Context, gitURL, sha, runID, summary string) error {
	return s.report(ctx, gitURL, sha, runID, StatusStateFailure, summary)
}

// ReportError reports that a test run encountered an error.
func (s *StatusReporter) ReportError(ctx context.Context, gitURL, sha, runID, summary string) error {
	return s.report(ctx, gitURL, sha, runID, StatusStateError, summary)
}

// report sends a status update to the appropriate provider.
func (s *StatusReporter) report(ctx context.Context, gitURL, sha, runID string, state StatusState, description string) error {
	owner, repo, err := parseRepositoryURL(gitURL)
	if err != nil {
		return fmt.Errorf("failed to parse git URL: %w", err)
	}

	provider, err := s.getProviderForURL(gitURL)
	if err != nil {
		return fmt.Errorf("no provider for URL: %w", err)
	}

	// Map internal state to provider state
	providerState := mapState(state)

	// Truncate description if too long (GitHub has 140 char limit)
	if len(description) > 140 {
		description = description[:137] + "..."
	}

	status := CommitStatus{
		State:       providerState,
		Context:     "conductor",
		Description: description,
		TargetURL:   s.buildTargetURL(runID),
	}

	s.logger.Info("reporting status",
		"owner", owner,
		"repo", repo,
		"sha", sha,
		"state", providerState,
		"run_id", runID,
	)

	if err := provider.CreateCommitStatus(ctx, owner, repo, sha, status); err != nil {
		s.logger.Error("failed to report status",
			"error", err,
			"owner", owner,
			"repo", repo,
			"sha", sha,
		)
		return err
	}

	return nil
}

// ReportWithCheckRun creates/updates a GitHub check run (more detailed than commit status).
func (s *StatusReporter) ReportWithCheckRun(ctx context.Context, gitURL, sha, runID, name string, state StatusState, summary, details string) error {
	owner, repo, err := parseRepositoryURL(gitURL)
	if err != nil {
		return fmt.Errorf("failed to parse git URL: %w", err)
	}

	provider, err := s.getProviderForURL(gitURL)
	if err != nil {
		return fmt.Errorf("no provider for URL: %w", err)
	}

	// Check if this is a GitHub provider with check run support
	ghProvider, ok := provider.(*GitHubProvider)
	if !ok {
		// Fall back to commit status for non-GitHub providers
		return s.report(ctx, gitURL, sha, runID, state, summary)
	}

	// Map state to check run status/conclusion
	status, conclusion := mapToCheckRun(state)

	var conclusionPtr *string
	if conclusion != "" {
		conclusionPtr = &conclusion
	}

	// Create or update check run
	checkRun, err := ghProvider.CreateCheckRun(ctx, owner, repo, sha, name, status, conclusionPtr)
	if err != nil {
		return fmt.Errorf("failed to create check run: %w", err)
	}

	// If we have details, update the check run with output
	if details != "" && checkRun != nil {
		output := &CheckRunOutput{
			Title:   summary,
			Summary: summary,
			Text:    details,
		}
		if err := ghProvider.UpdateCheckRun(ctx, owner, repo, checkRun.ID, status, conclusionPtr, output); err != nil {
			s.logger.Warn("failed to update check run with output",
				"error", err,
				"check_run_id", checkRun.ID,
			)
		}
	}

	return nil
}

// getProviderForURL returns the appropriate provider for a git URL.
func (s *StatusReporter) getProviderForURL(gitURL string) (Provider, error) {
	host := GetProviderFromURL(gitURL)

	s.mu.RLock()
	provider, ok := s.providers[strings.ToLower(host)]
	s.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("no provider registered for host: %s", host)
	}

	return provider, nil
}

// buildTargetURL constructs the URL to the test run details page.
func (s *StatusReporter) buildTargetURL(runID string) string {
	if s.baseURL == "" {
		return ""
	}
	return fmt.Sprintf("%s/runs/%s", s.baseURL, runID)
}

// GetProviderFromURL determines the git provider from a repository URL.
func GetProviderFromURL(gitURL string) string {
	gitURL = strings.ToLower(gitURL)

	// Handle various URL formats
	if strings.Contains(gitURL, "github.com") {
		return "github"
	}
	if strings.Contains(gitURL, "gitlab.com") || strings.Contains(gitURL, "gitlab") {
		return "gitlab"
	}
	if strings.Contains(gitURL, "bitbucket.org") || strings.Contains(gitURL, "bitbucket") {
		return "bitbucket"
	}

	// Try to parse as URL and check host
	if u, err := url.Parse(gitURL); err == nil && u.Host != "" {
		host := u.Host
		if strings.Contains(host, "github") {
			return "github"
		}
		if strings.Contains(host, "gitlab") {
			return "gitlab"
		}
		if strings.Contains(host, "bitbucket") {
			return "bitbucket"
		}
	}

	// Default to github for unknown
	return "github"
}

// GetProviderType returns the provider type constant from a URL.
func GetProviderType(gitURL string) string {
	return GetProviderFromURL(gitURL)
}

// mapState maps internal status state to provider-specific state.
func mapState(state StatusState) string {
	switch state {
	case StatusStatePending:
		return "pending"
	case StatusStateRunning:
		// GitHub uses "pending" for in-progress via status API
		// Use check runs for more granular status
		return "pending"
	case StatusStateSuccess:
		return "success"
	case StatusStateFailure:
		return "failure"
	case StatusStateError:
		return "error"
	default:
		return "pending"
	}
}

// mapToCheckRun maps internal status to GitHub check run status and conclusion.
func mapToCheckRun(state StatusState) (status, conclusion string) {
	switch state {
	case StatusStatePending:
		return "queued", ""
	case StatusStateRunning:
		return "in_progress", ""
	case StatusStateSuccess:
		return "completed", "success"
	case StatusStateFailure:
		return "completed", "failure"
	case StatusStateError:
		return "completed", "failure"
	default:
		return "queued", ""
	}
}

// ParseOwnerRepo extracts owner and repo from a git URL.
// This is a convenience wrapper around parseRepositoryURL for external use.
func ParseOwnerRepo(gitURL string) (owner, repo string, err error) {
	return parseRepositoryURL(gitURL)
}

// ProviderRegistry manages multiple git providers.
type ProviderRegistry struct {
	providers map[string]Provider
	mu        sync.RWMutex
}

// NewProviderRegistry creates a new provider registry.
func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		providers: make(map[string]Provider),
	}
}

// Register registers a provider.
func (r *ProviderRegistry) Register(name string, provider Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[strings.ToLower(name)] = provider
}

// Get retrieves a provider by name.
func (r *ProviderRegistry) Get(name string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[strings.ToLower(name)]
	return p, ok
}

// GetForURL retrieves a provider based on the git URL.
func (r *ProviderRegistry) GetForURL(gitURL string) (Provider, error) {
	providerName := GetProviderFromURL(gitURL)
	r.mu.RLock()
	p, ok := r.providers[providerName]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("no provider registered for: %s", providerName)
	}
	return p, nil
}

// NewProviderFromConfig creates a provider based on configuration.
func NewProviderFromConfig(cfg Config) (Provider, error) {
	switch strings.ToLower(cfg.Provider) {
	case "github", "":
		return NewGitHubProvider(cfg)
	case "gitlab":
		return NewGitLabProvider(cfg)
	case "bitbucket":
		return NewBitbucketProvider(cfg)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", cfg.Provider)
	}
}
