// Package git provides git provider integration for syncing test definitions.
package git

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultGitHubBaseURL is the default GitHub API base URL.
	DefaultGitHubBaseURL = "https://api.github.com"
	// DefaultUserAgent is the default user agent string.
	DefaultUserAgent = "Conductor/1.0"
	// DefaultHTTPTimeout is the default HTTP client timeout.
	DefaultHTTPTimeout = 30 * time.Second
	// MaxRetries is the maximum number of retries for transient failures.
	MaxRetries = 3
	// RetryBaseDelay is the base delay for exponential backoff.
	RetryBaseDelay = 1 * time.Second
)

// GitHubProvider implements the Provider interface for GitHub.
type GitHubProvider struct {
	client    *http.Client
	baseURL   string
	token     string
	userAgent string
	logger    *slog.Logger

	// Rate limiting
	rateLimitMu        sync.RWMutex
	rateLimitRemaining int
	rateLimitReset     time.Time
}

// NewGitHubProvider creates a new GitHub provider.
func NewGitHubProvider(cfg Config) (*GitHubProvider, error) {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = DefaultGitHubBaseURL
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	return &GitHubProvider{
		client:             &http.Client{Timeout: DefaultHTTPTimeout},
		baseURL:            baseURL,
		token:              cfg.Token,
		userAgent:          DefaultUserAgent,
		logger:             slog.Default().With("component", "github_provider"),
		rateLimitRemaining: -1, // Unknown initially
	}, nil
}

// NewGitHubProviderWithLogger creates a new GitHub provider with a custom logger.
func NewGitHubProviderWithLogger(cfg Config, logger *slog.Logger) (*GitHubProvider, error) {
	p, err := NewGitHubProvider(cfg)
	if err != nil {
		return nil, err
	}
	if logger != nil {
		p.logger = logger.With("component", "github_provider")
	}
	return p, nil
}

// GetRepository retrieves repository information.
func (g *GitHubProvider) GetRepository(ctx context.Context, owner, repo string) (*Repository, error) {
	url := fmt.Sprintf("%s/repos/%s/%s", g.baseURL, owner, repo)

	var result githubRepo
	if err := g.doRequestWithRetry(ctx, "GET", url, nil, &result); err != nil {
		return nil, fmt.Errorf("failed to get repository: %w", err)
	}

	return &Repository{
		ID:            result.ID,
		Name:          result.Name,
		FullName:      result.FullName,
		Description:   result.Description,
		DefaultBranch: result.DefaultBranch,
		Private:       result.Private,
		HTMLURL:       result.HTMLURL,
		CloneURL:      result.CloneURL,
		SSHURL:        result.SSHURL,
	}, nil
}

// GetFile retrieves file content from a repository.
func (g *GitHubProvider) GetFile(ctx context.Context, owner, repo, path, ref string) ([]byte, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/contents/%s", g.baseURL, owner, repo, path)
	if ref != "" {
		url += "?ref=" + ref
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	g.setHeaders(req)
	req.Header.Set("Accept", "application/vnd.github.v3.raw")

	resp, err := g.doWithRateLimitAndRetry(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("file not found: %s", path)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error (status %d): %s", resp.StatusCode, string(body))
	}

	return io.ReadAll(resp.Body)
}

// ListFiles lists files in a directory.
func (g *GitHubProvider) ListFiles(ctx context.Context, owner, repo, path, ref string) ([]FileInfo, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/contents/%s", g.baseURL, owner, repo, path)
	if ref != "" {
		url += "?ref=" + ref
	}

	var entries []githubContent
	if err := g.doRequestWithRetry(ctx, "GET", url, nil, &entries); err != nil {
		if strings.Contains(err.Error(), "404") {
			return nil, fmt.Errorf("directory not found: %s", path)
		}
		return nil, fmt.Errorf("failed to list files: %w", err)
	}

	files := make([]FileInfo, len(entries))
	for i, e := range entries {
		files[i] = FileInfo{
			Name: e.Name,
			Path: e.Path,
			Type: e.Type,
			Size: e.Size,
			SHA:  e.SHA,
		}
	}

	return files, nil
}

// GetBranch retrieves branch information.
func (g *GitHubProvider) GetBranch(ctx context.Context, owner, repo, branch string) (*Branch, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/branches/%s", g.baseURL, owner, repo, branch)

	var result githubBranch
	if err := g.doRequestWithRetry(ctx, "GET", url, nil, &result); err != nil {
		return nil, fmt.Errorf("failed to get branch: %w", err)
	}

	return &Branch{
		Name:      result.Name,
		SHA:       result.Commit.SHA,
		Protected: result.Protected,
	}, nil
}

// GetDefaultBranch returns the default branch for a repository.
func (g *GitHubProvider) GetDefaultBranch(ctx context.Context, owner, repo string) (string, error) {
	repoInfo, err := g.GetRepository(ctx, owner, repo)
	if err != nil {
		return "", err
	}
	return repoInfo.DefaultBranch, nil
}

// CreateCommitStatus creates a commit status.
func (g *GitHubProvider) CreateCommitStatus(ctx context.Context, owner, repo, sha string, status CommitStatus) error {
	url := fmt.Sprintf("%s/repos/%s/%s/statuses/%s", g.baseURL, owner, repo, sha)

	payload := githubStatusRequest{
		State:       status.State,
		Context:     status.Context,
		Description: status.Description,
		TargetURL:   status.TargetURL,
	}

	if err := g.doRequestWithRetry(ctx, "POST", url, payload, nil); err != nil {
		return fmt.Errorf("failed to create commit status: %w", err)
	}

	g.logger.Debug("created commit status",
		"owner", owner,
		"repo", repo,
		"sha", sha,
		"state", status.State,
		"context", status.Context,
	)

	return nil
}

// CreateCheckRun creates a new check run.
func (g *GitHubProvider) CreateCheckRun(ctx context.Context, owner, repo, sha, name, status string, conclusion *string) (*CheckRun, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/check-runs", g.baseURL, owner, repo)

	payload := githubCheckRunRequest{
		Name:    name,
		HeadSHA: sha,
		Status:  status,
	}
	if conclusion != nil {
		payload.Conclusion = *conclusion
	}
	if status == "in_progress" {
		now := time.Now().UTC().Format(time.RFC3339)
		payload.StartedAt = now
	}

	var result githubCheckRun
	if err := g.doRequestWithRetry(ctx, "POST", url, payload, &result); err != nil {
		return nil, fmt.Errorf("failed to create check run: %w", err)
	}

	g.logger.Debug("created check run",
		"owner", owner,
		"repo", repo,
		"sha", sha,
		"name", name,
		"check_run_id", result.ID,
	)

	return &CheckRun{
		ID:         result.ID,
		Name:       result.Name,
		HeadSHA:    result.HeadSHA,
		Status:     result.Status,
		Conclusion: result.Conclusion,
		HTMLURL:    result.HTMLURL,
	}, nil
}

// UpdateCheckRun updates an existing check run.
func (g *GitHubProvider) UpdateCheckRun(ctx context.Context, owner, repo string, checkRunID int64, status string, conclusion *string, output *CheckRunOutput) error {
	url := fmt.Sprintf("%s/repos/%s/%s/check-runs/%d", g.baseURL, owner, repo, checkRunID)

	payload := githubCheckRunUpdateRequest{
		Status: status,
	}
	if conclusion != nil {
		payload.Conclusion = *conclusion
		now := time.Now().UTC().Format(time.RFC3339)
		payload.CompletedAt = now
	}
	if output != nil {
		payload.Output = &githubCheckRunOutput{
			Title:   output.Title,
			Summary: output.Summary,
			Text:    output.Text,
		}
	}

	if err := g.doRequestWithRetry(ctx, "PATCH", url, payload, nil); err != nil {
		return fmt.Errorf("failed to update check run: %w", err)
	}

	g.logger.Debug("updated check run",
		"owner", owner,
		"repo", repo,
		"check_run_id", checkRunID,
		"status", status,
	)

	return nil
}

// GetPullRequest retrieves pull request details.
func (g *GitHubProvider) GetPullRequest(ctx context.Context, owner, repo string, number int) (*PullRequest, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/pulls/%d", g.baseURL, owner, repo, number)

	var ghPR githubPullRequest
	if err := g.doRequestWithRetry(ctx, "GET", url, nil, &ghPR); err != nil {
		if strings.Contains(err.Error(), "404") {
			return nil, fmt.Errorf("pull request not found: %d", number)
		}
		return nil, fmt.Errorf("failed to get pull request: %w", err)
	}

	pr := &PullRequest{
		Number:    ghPR.Number,
		Title:     ghPR.Title,
		Body:      ghPR.Body,
		State:     ghPR.State,
		HeadRef:   ghPR.Head.Ref,
		HeadSHA:   ghPR.Head.SHA,
		BaseRef:   ghPR.Base.Ref,
		BaseSHA:   ghPR.Base.SHA,
		Author:    ghPR.User.Login,
		CreatedAt: ghPR.CreatedAt,
		UpdatedAt: ghPR.UpdatedAt,
	}

	if ghPR.MergedAt != nil {
		pr.MergedAt = ghPR.MergedAt
	}
	if ghPR.MergeCommitSHA != "" {
		pr.MergeCommit = ghPR.MergeCommitSHA
	}

	return pr, nil
}

// CreateComment posts a comment on a pull request.
func (g *GitHubProvider) CreateComment(ctx context.Context, owner, repo string, prNumber int, body string) error {
	url := fmt.Sprintf("%s/repos/%s/%s/issues/%d/comments", g.baseURL, owner, repo, prNumber)

	payload := githubCommentRequest{Body: body}

	if err := g.doRequestWithRetry(ctx, "POST", url, payload, nil); err != nil {
		return fmt.Errorf("failed to create comment: %w", err)
	}

	g.logger.Debug("created PR comment",
		"owner", owner,
		"repo", repo,
		"pr_number", prNumber,
	)

	return nil
}

// CreatePRComment is an alias for CreateComment.
func (g *GitHubProvider) CreatePRComment(ctx context.Context, owner, repo string, number int, body string) error {
	return g.CreateComment(ctx, owner, repo, number, body)
}

// setHeaders sets common headers for GitHub API requests.
func (g *GitHubProvider) setHeaders(req *http.Request) {
	req.Header.Set("User-Agent", g.userAgent)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if g.token != "" {
		req.Header.Set("Authorization", "Bearer "+g.token)
	}
}

// doRequestWithRetry performs an HTTP request with retry logic.
func (g *GitHubProvider) doRequestWithRetry(ctx context.Context, method, url string, body interface{}, result interface{}) error {
	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(jsonBody)
	}

	var lastErr error
	for attempt := 0; attempt < MaxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			delay := RetryBaseDelay * time.Duration(1<<(attempt-1))
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}

			// Reset body reader for retry
			if body != nil {
				jsonBody, _ := json.Marshal(body)
				reqBody = bytes.NewReader(jsonBody)
			}
		}

		req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		g.setHeaders(req)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := g.doWithRateLimitAndRetry(ctx, req)
		if err != nil {
			lastErr = err
			if isRetryableError(err) {
				g.logger.Debug("retrying request due to error",
					"attempt", attempt+1,
					"error", err,
				)
				continue
			}
			return err
		}
		defer resp.Body.Close()

		// Check for rate limiting
		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
			if g.handleRateLimitResponse(resp) {
				lastErr = fmt.Errorf("rate limited")
				continue
			}
		}

		// Check for server errors (5xx) - retryable
		if resp.StatusCode >= 500 {
			respBody, _ := io.ReadAll(resp.Body)
			lastErr = fmt.Errorf("server error (status %d): %s", resp.StatusCode, string(respBody))
			g.logger.Debug("retrying request due to server error",
				"attempt", attempt+1,
				"status", resp.StatusCode,
			)
			continue
		}

		// Check for client errors (4xx) - not retryable
		if resp.StatusCode >= 400 {
			respBody, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("GitHub API error (status %d): %s", resp.StatusCode, string(respBody))
		}

		// Success - parse response if needed
		if result != nil && resp.StatusCode != http.StatusNoContent {
			if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
				return fmt.Errorf("failed to decode response: %w", err)
			}
		}

		return nil
	}

	if lastErr != nil {
		return fmt.Errorf("max retries exceeded: %w", lastErr)
	}
	return fmt.Errorf("max retries exceeded")
}

// doWithRateLimitAndRetry performs a request with rate limit awareness.
func (g *GitHubProvider) doWithRateLimitAndRetry(ctx context.Context, req *http.Request) (*http.Response, error) {
	// Check if we need to wait for rate limit reset
	g.rateLimitMu.RLock()
	remaining := g.rateLimitRemaining
	resetTime := g.rateLimitReset
	g.rateLimitMu.RUnlock()

	if remaining == 0 && time.Now().Before(resetTime) {
		waitDuration := time.Until(resetTime)
		g.logger.Info("rate limit exceeded, waiting for reset",
			"wait_duration", waitDuration,
			"reset_time", resetTime,
		)

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(waitDuration):
		}
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}

	// Update rate limit info from response headers
	g.updateRateLimitFromResponse(resp)

	return resp, nil
}

// updateRateLimitFromResponse updates rate limit tracking from response headers.
func (g *GitHubProvider) updateRateLimitFromResponse(resp *http.Response) {
	g.rateLimitMu.Lock()
	defer g.rateLimitMu.Unlock()

	if remaining := resp.Header.Get("X-RateLimit-Remaining"); remaining != "" {
		if n, err := strconv.Atoi(remaining); err == nil {
			g.rateLimitRemaining = n
		}
	}

	if reset := resp.Header.Get("X-RateLimit-Reset"); reset != "" {
		if ts, err := strconv.ParseInt(reset, 10, 64); err == nil {
			g.rateLimitReset = time.Unix(ts, 0)
		}
	}
}

// handleRateLimitResponse handles a rate limit response.
func (g *GitHubProvider) handleRateLimitResponse(resp *http.Response) bool {
	g.updateRateLimitFromResponse(resp)

	retryAfter := resp.Header.Get("Retry-After")
	if retryAfter != "" {
		if seconds, err := strconv.Atoi(retryAfter); err == nil {
			g.rateLimitMu.Lock()
			g.rateLimitRemaining = 0
			g.rateLimitReset = time.Now().Add(time.Duration(seconds) * time.Second)
			g.rateLimitMu.Unlock()
		}
	}

	return true
}

// isRetryableError determines if an error is retryable.
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	retryablePatterns := []string{
		"connection reset",
		"connection refused",
		"timeout",
		"temporary failure",
		"EOF",
		"no such host",
	}

	for _, pattern := range retryablePatterns {
		if strings.Contains(strings.ToLower(errStr), pattern) {
			return true
		}
	}

	return false
}

// GitHub API response/request structures

type githubContent struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	SHA         string `json:"sha"`
	Size        int64  `json:"size"`
	URL         string `json:"url"`
	HTMLURL     string `json:"html_url"`
	GitURL      string `json:"git_url"`
	DownloadURL string `json:"download_url"`
	Type        string `json:"type"`
	Content     string `json:"content"`
	Encoding    string `json:"encoding"`
}

type githubRepo struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	DefaultBranch string `json:"default_branch"`
	Private       bool   `json:"private"`
	Description   string `json:"description"`
	HTMLURL       string `json:"html_url"`
	CloneURL      string `json:"clone_url"`
	SSHURL        string `json:"ssh_url"`
}

type githubBranch struct {
	Name      string `json:"name"`
	Protected bool   `json:"protected"`
	Commit    struct {
		SHA string `json:"sha"`
		URL string `json:"url"`
	} `json:"commit"`
}

type githubStatusRequest struct {
	State       string `json:"state"`
	TargetURL   string `json:"target_url,omitempty"`
	Description string `json:"description,omitempty"`
	Context     string `json:"context"`
}

type githubCommentRequest struct {
	Body string `json:"body"`
}

type githubPullRequest struct {
	Number         int                  `json:"number"`
	State          string               `json:"state"`
	Title          string               `json:"title"`
	Body           string               `json:"body"`
	CreatedAt      time.Time            `json:"created_at"`
	UpdatedAt      time.Time            `json:"updated_at"`
	MergedAt       *time.Time           `json:"merged_at"`
	MergeCommitSHA string               `json:"merge_commit_sha"`
	Head           githubPullRequestRef `json:"head"`
	Base           githubPullRequestRef `json:"base"`
	User           githubUser           `json:"user"`
}

type githubPullRequestRef struct {
	Ref  string `json:"ref"`
	SHA  string `json:"sha"`
	Repo struct {
		Name     string `json:"name"`
		FullName string `json:"full_name"`
	} `json:"repo"`
}

type githubUser struct {
	Login string `json:"login"`
	ID    int64  `json:"id"`
}

type githubCheckRunRequest struct {
	Name       string `json:"name"`
	HeadSHA    string `json:"head_sha"`
	Status     string `json:"status,omitempty"`
	Conclusion string `json:"conclusion,omitempty"`
	StartedAt  string `json:"started_at,omitempty"`
}

type githubCheckRunUpdateRequest struct {
	Status      string                `json:"status,omitempty"`
	Conclusion  string                `json:"conclusion,omitempty"`
	CompletedAt string                `json:"completed_at,omitempty"`
	Output      *githubCheckRunOutput `json:"output,omitempty"`
}

type githubCheckRunOutput struct {
	Title   string `json:"title"`
	Summary string `json:"summary"`
	Text    string `json:"text,omitempty"`
}

type githubCheckRun struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	HeadSHA    string `json:"head_sha"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	HTMLURL    string `json:"html_url"`
}

// Additional types for expanded GitHub functionality

// Repository contains GitHub repository information.
type Repository struct {
	ID            int64
	Name          string
	FullName      string
	Description   string
	DefaultBranch string
	Private       bool
	HTMLURL       string
	CloneURL      string
	SSHURL        string
}

// Branch contains GitHub branch information.
type Branch struct {
	Name      string
	SHA       string
	Protected bool
}

// CheckRun represents a GitHub check run.
type CheckRun struct {
	ID         int64
	Name       string
	HeadSHA    string
	Status     string
	Conclusion string
	HTMLURL    string
}

// CheckRunOutput contains output for a check run.
type CheckRunOutput struct {
	Title   string
	Summary string
	Text    string
}

// decodeBase64Content decodes base64-encoded file content from GitHub.
func decodeBase64Content(content string) ([]byte, error) {
	// GitHub API returns content with newlines, need to remove them
	content = strings.ReplaceAll(content, "\n", "")
	return base64.StdEncoding.DecodeString(content)
}
