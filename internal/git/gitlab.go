package git

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultGitLabBaseURL is the default GitLab API base URL.
	DefaultGitLabBaseURL = "https://gitlab.com/api/v4"
)

// GitLabProvider implements the Provider interface for GitLab.
type GitLabProvider struct {
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

// NewGitLabProvider creates a new GitLab provider.
func NewGitLabProvider(cfg Config) (*GitLabProvider, error) {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = DefaultGitLabBaseURL
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	return &GitLabProvider{
		client:             &http.Client{Timeout: DefaultHTTPTimeout},
		baseURL:            baseURL,
		token:              cfg.Token,
		userAgent:          DefaultUserAgent,
		logger:             slog.Default().With("component", "gitlab_provider"),
		rateLimitRemaining: -1,
	}, nil
}

// NewGitLabProviderWithLogger creates a new GitLab provider with a custom logger.
func NewGitLabProviderWithLogger(cfg Config, logger *slog.Logger) (*GitLabProvider, error) {
	p, err := NewGitLabProvider(cfg)
	if err != nil {
		return nil, err
	}
	if logger != nil {
		p.logger = logger.With("component", "gitlab_provider")
	}
	return p, nil
}

// projectPath encodes the project path for GitLab API.
func (g *GitLabProvider) projectPath(owner, repo string) string {
	return url.PathEscape(owner + "/" + repo)
}

// GetRepository retrieves repository information.
func (g *GitLabProvider) GetRepository(ctx context.Context, owner, repo string) (*Repository, error) {
	projectPath := g.projectPath(owner, repo)
	apiURL := fmt.Sprintf("%s/projects/%s", g.baseURL, projectPath)

	var result gitlabProject
	if err := g.doRequestWithRetry(ctx, "GET", apiURL, nil, &result); err != nil {
		return nil, fmt.Errorf("failed to get repository: %w", err)
	}

	return &Repository{
		ID:            result.ID,
		Name:          result.Name,
		FullName:      result.PathWithNamespace,
		Description:   result.Description,
		DefaultBranch: result.DefaultBranch,
		Private:       result.Visibility == "private",
		HTMLURL:       result.WebURL,
		CloneURL:      result.HTTPURLToRepo,
		SSHURL:        result.SSHURLToRepo,
	}, nil
}

// GetFile retrieves file content from a repository.
func (g *GitLabProvider) GetFile(ctx context.Context, owner, repo, path, ref string) ([]byte, error) {
	projectPath := g.projectPath(owner, repo)
	encodedPath := url.PathEscape(path)
	apiURL := fmt.Sprintf("%s/projects/%s/repository/files/%s/raw", g.baseURL, projectPath, encodedPath)
	if ref != "" {
		apiURL += "?ref=" + url.QueryEscape(ref)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	g.setHeaders(req)

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
		return nil, fmt.Errorf("GitLab API error (status %d): %s", resp.StatusCode, string(body))
	}

	return io.ReadAll(resp.Body)
}

// ListFiles lists files in a directory.
func (g *GitLabProvider) ListFiles(ctx context.Context, owner, repo, path, ref string) ([]FileInfo, error) {
	projectPath := g.projectPath(owner, repo)
	apiURL := fmt.Sprintf("%s/projects/%s/repository/tree", g.baseURL, projectPath)

	params := url.Values{}
	if path != "" && path != "." {
		params.Set("path", path)
	}
	if ref != "" {
		params.Set("ref", ref)
	}
	if len(params) > 0 {
		apiURL += "?" + params.Encode()
	}

	var entries []gitlabTreeItem
	if err := g.doRequestWithRetry(ctx, "GET", apiURL, nil, &entries); err != nil {
		if strings.Contains(err.Error(), "404") {
			return nil, fmt.Errorf("directory not found: %s", path)
		}
		return nil, fmt.Errorf("failed to list files: %w", err)
	}

	files := make([]FileInfo, len(entries))
	for i, e := range entries {
		fileType := "file"
		if e.Type == "tree" {
			fileType = "dir"
		}
		files[i] = FileInfo{
			Name: e.Name,
			Path: e.Path,
			Type: fileType,
			SHA:  e.ID,
		}
	}

	return files, nil
}

// GetBranch retrieves branch information.
func (g *GitLabProvider) GetBranch(ctx context.Context, owner, repo, branch string) (*Branch, error) {
	projectPath := g.projectPath(owner, repo)
	apiURL := fmt.Sprintf("%s/projects/%s/repository/branches/%s", g.baseURL, projectPath, url.PathEscape(branch))

	var result gitlabBranch
	if err := g.doRequestWithRetry(ctx, "GET", apiURL, nil, &result); err != nil {
		return nil, fmt.Errorf("failed to get branch: %w", err)
	}

	return &Branch{
		Name:      result.Name,
		SHA:       result.Commit.ID,
		Protected: result.Protected,
	}, nil
}

// GetDefaultBranch returns the default branch for a repository.
func (g *GitLabProvider) GetDefaultBranch(ctx context.Context, owner, repo string) (string, error) {
	repoInfo, err := g.GetRepository(ctx, owner, repo)
	if err != nil {
		return "", err
	}
	return repoInfo.DefaultBranch, nil
}

// CreateCommitStatus creates a commit status (pipeline status in GitLab).
func (g *GitLabProvider) CreateCommitStatus(ctx context.Context, owner, repo, sha string, status CommitStatus) error {
	projectPath := g.projectPath(owner, repo)
	apiURL := fmt.Sprintf("%s/projects/%s/statuses/%s", g.baseURL, projectPath, sha)

	// Map GitHub status states to GitLab states
	gitlabState := mapStatusState(status.State)

	payload := gitlabStatusRequest{
		State:       gitlabState,
		Context:     status.Context,
		Description: status.Description,
		TargetURL:   status.TargetURL,
	}

	if err := g.doRequestWithRetry(ctx, "POST", apiURL, payload, nil); err != nil {
		return fmt.Errorf("failed to create commit status: %w", err)
	}

	g.logger.Debug("created commit status",
		"owner", owner,
		"repo", repo,
		"sha", sha,
		"state", gitlabState,
		"context", status.Context,
	)

	return nil
}

// GetPullRequest retrieves merge request details (GitLab's equivalent of PR).
func (g *GitLabProvider) GetPullRequest(ctx context.Context, owner, repo string, number int) (*PullRequest, error) {
	projectPath := g.projectPath(owner, repo)
	apiURL := fmt.Sprintf("%s/projects/%s/merge_requests/%d", g.baseURL, projectPath, number)

	var glMR gitlabMergeRequest
	if err := g.doRequestWithRetry(ctx, "GET", apiURL, nil, &glMR); err != nil {
		if strings.Contains(err.Error(), "404") {
			return nil, fmt.Errorf("merge request not found: %d", number)
		}
		return nil, fmt.Errorf("failed to get merge request: %w", err)
	}

	state := glMR.State
	if state == "merged" {
		state = "closed"
	}

	pr := &PullRequest{
		Number:    glMR.IID,
		Title:     glMR.Title,
		Body:      glMR.Description,
		State:     state,
		HeadRef:   glMR.SourceBranch,
		HeadSHA:   glMR.SHA,
		BaseRef:   glMR.TargetBranch,
		Author:    glMR.Author.Username,
		CreatedAt: glMR.CreatedAt,
		UpdatedAt: glMR.UpdatedAt,
	}

	if glMR.MergedAt != nil {
		pr.MergedAt = glMR.MergedAt
	}
	if glMR.MergeCommitSHA != "" {
		pr.MergeCommit = glMR.MergeCommitSHA
	}

	return pr, nil
}

// CreateComment posts a comment on a merge request.
func (g *GitLabProvider) CreateComment(ctx context.Context, owner, repo string, mrNumber int, body string) error {
	projectPath := g.projectPath(owner, repo)
	apiURL := fmt.Sprintf("%s/projects/%s/merge_requests/%d/notes", g.baseURL, projectPath, mrNumber)

	payload := gitlabNoteRequest{Body: body}

	if err := g.doRequestWithRetry(ctx, "POST", apiURL, payload, nil); err != nil {
		return fmt.Errorf("failed to create comment: %w", err)
	}

	g.logger.Debug("created MR comment",
		"owner", owner,
		"repo", repo,
		"mr_number", mrNumber,
	)

	return nil
}

// setHeaders sets common headers for GitLab API requests.
func (g *GitLabProvider) setHeaders(req *http.Request) {
	req.Header.Set("User-Agent", g.userAgent)
	req.Header.Set("Content-Type", "application/json")
	if g.token != "" {
		req.Header.Set("PRIVATE-TOKEN", g.token)
	}
}

// doRequestWithRetry performs an HTTP request with retry logic.
func (g *GitLabProvider) doRequestWithRetry(ctx context.Context, method, apiURL string, body interface{}, result interface{}) error {
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
			delay := RetryBaseDelay * time.Duration(1<<(attempt-1))
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}

			if body != nil {
				jsonBody, _ := json.Marshal(body)
				reqBody = bytes.NewReader(jsonBody)
			}
		}

		req, err := http.NewRequestWithContext(ctx, method, apiURL, reqBody)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		g.setHeaders(req)

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
		if resp.StatusCode == http.StatusTooManyRequests {
			if g.handleRateLimitResponse(resp) {
				lastErr = fmt.Errorf("rate limited")
				continue
			}
		}

		// Check for server errors (5xx) - retryable
		if resp.StatusCode >= 500 {
			respBody, _ := io.ReadAll(resp.Body)
			lastErr = fmt.Errorf("server error (status %d): %s", resp.StatusCode, string(respBody))
			continue
		}

		// Check for client errors (4xx) - not retryable
		if resp.StatusCode >= 400 {
			respBody, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("GitLab API error (status %d): %s", resp.StatusCode, string(respBody))
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
func (g *GitLabProvider) doWithRateLimitAndRetry(ctx context.Context, req *http.Request) (*http.Response, error) {
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

	g.updateRateLimitFromResponse(resp)

	return resp, nil
}

// updateRateLimitFromResponse updates rate limit tracking from response headers.
func (g *GitLabProvider) updateRateLimitFromResponse(resp *http.Response) {
	g.rateLimitMu.Lock()
	defer g.rateLimitMu.Unlock()

	if remaining := resp.Header.Get("RateLimit-Remaining"); remaining != "" {
		if n, err := strconv.Atoi(remaining); err == nil {
			g.rateLimitRemaining = n
		}
	}

	if reset := resp.Header.Get("RateLimit-Reset"); reset != "" {
		if ts, err := strconv.ParseInt(reset, 10, 64); err == nil {
			g.rateLimitReset = time.Unix(ts, 0)
		}
	}
}

// handleRateLimitResponse handles a rate limit response.
func (g *GitLabProvider) handleRateLimitResponse(resp *http.Response) bool {
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

// mapStatusState maps GitHub status states to GitLab states.
func mapStatusState(state string) string {
	switch state {
	case "pending":
		return "pending"
	case "success":
		return "success"
	case "failure":
		return "failed"
	case "error":
		return "failed"
	default:
		return "pending"
	}
}

// GitLab API response structures

type gitlabProject struct {
	ID                int64  `json:"id"`
	Name              string `json:"name"`
	Path              string `json:"path"`
	PathWithNamespace string `json:"path_with_namespace"`
	Description       string `json:"description"`
	DefaultBranch     string `json:"default_branch"`
	Visibility        string `json:"visibility"`
	WebURL            string `json:"web_url"`
	HTTPURLToRepo     string `json:"http_url_to_repo"`
	SSHURLToRepo      string `json:"ssh_url_to_repo"`
}

type gitlabTreeItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"` // "blob" or "tree"
	Path string `json:"path"`
	Mode string `json:"mode"`
}

type gitlabBranch struct {
	Name      string `json:"name"`
	Protected bool   `json:"protected"`
	Commit    struct {
		ID            string    `json:"id"`
		ShortID       string    `json:"short_id"`
		Title         string    `json:"title"`
		AuthorName    string    `json:"author_name"`
		AuthorEmail   string    `json:"author_email"`
		CommittedDate time.Time `json:"committed_date"`
	} `json:"commit"`
}

type gitlabStatusRequest struct {
	State       string `json:"state"`
	Context     string `json:"name"`
	Description string `json:"description,omitempty"`
	TargetURL   string `json:"target_url,omitempty"`
}

type gitlabMergeRequest struct {
	ID             int64      `json:"id"`
	IID            int        `json:"iid"`
	Title          string     `json:"title"`
	Description    string     `json:"description"`
	State          string     `json:"state"`
	SourceBranch   string     `json:"source_branch"`
	TargetBranch   string     `json:"target_branch"`
	SHA            string     `json:"sha"`
	MergeCommitSHA string     `json:"merge_commit_sha"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	MergedAt       *time.Time `json:"merged_at"`
	Author         struct {
		ID       int64  `json:"id"`
		Username string `json:"username"`
		Name     string `json:"name"`
	} `json:"author"`
}

type gitlabNoteRequest struct {
	Body string `json:"body"`
}

// GitLab webhook event types

// GitLabPushEvent represents a GitLab push webhook event.
type GitLabPushEvent struct {
	ObjectKind   string `json:"object_kind"`
	Before       string `json:"before"`
	After        string `json:"after"`
	Ref          string `json:"ref"`
	CheckoutSHA  string `json:"checkout_sha"`
	UserID       int64  `json:"user_id"`
	UserName     string `json:"user_name"`
	UserUsername string `json:"user_username"`
	UserEmail    string `json:"user_email"`
	Project      struct {
		ID                int64  `json:"id"`
		Name              string `json:"name"`
		PathWithNamespace string `json:"path_with_namespace"`
		DefaultBranch     string `json:"default_branch"`
		WebURL            string `json:"web_url"`
		GitHTTPURL        string `json:"git_http_url"`
		GitSSHURL         string `json:"git_ssh_url"`
	} `json:"project"`
	Commits []struct {
		ID        string    `json:"id"`
		Message   string    `json:"message"`
		Timestamp time.Time `json:"timestamp"`
		Author    struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"author"`
		Added    []string `json:"added"`
		Modified []string `json:"modified"`
		Removed  []string `json:"removed"`
	} `json:"commits"`
	TotalCommitsCount int `json:"total_commits_count"`
}

// GitLabMergeRequestEvent represents a GitLab merge request webhook event.
type GitLabMergeRequestEvent struct {
	ObjectKind string `json:"object_kind"`
	User       struct {
		ID       int64  `json:"id"`
		Name     string `json:"name"`
		Username string `json:"username"`
		Email    string `json:"email"`
	} `json:"user"`
	ObjectAttributes struct {
		ID           int64  `json:"id"`
		IID          int    `json:"iid"`
		Title        string `json:"title"`
		Description  string `json:"description"`
		State        string `json:"state"`
		Action       string `json:"action"`
		SourceBranch string `json:"source_branch"`
		TargetBranch string `json:"target_branch"`
		LastCommit   struct {
			ID      string `json:"id"`
			Message string `json:"message"`
		} `json:"last_commit"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
	} `json:"object_attributes"`
	Project struct {
		ID                int64  `json:"id"`
		Name              string `json:"name"`
		PathWithNamespace string `json:"path_with_namespace"`
		DefaultBranch     string `json:"default_branch"`
		WebURL            string `json:"web_url"`
		GitHTTPURL        string `json:"git_http_url"`
	} `json:"project"`
}

// GitLabPipelineEvent represents a GitLab pipeline webhook event.
type GitLabPipelineEvent struct {
	ObjectKind       string `json:"object_kind"`
	ObjectAttributes struct {
		ID     int64  `json:"id"`
		Ref    string `json:"ref"`
		SHA    string `json:"sha"`
		Status string `json:"status"`
		Source string `json:"source"`
	} `json:"object_attributes"`
	User struct {
		ID       int64  `json:"id"`
		Name     string `json:"name"`
		Username string `json:"username"`
	} `json:"user"`
	Project struct {
		ID                int64  `json:"id"`
		Name              string `json:"name"`
		PathWithNamespace string `json:"path_with_namespace"`
		DefaultBranch     string `json:"default_branch"`
		WebURL            string `json:"web_url"`
	} `json:"project"`
}

// ValidateGitLabToken validates a GitLab webhook token.
func ValidateGitLabToken(providedToken, expectedToken string) bool {
	if expectedToken == "" {
		return true
	}
	return providedToken == expectedToken
}

// ParseGitLabEvent parses a GitLab webhook event.
func ParseGitLabEvent(eventType string, payload []byte) (interface{}, error) {
	switch eventType {
	case "Push Hook":
		var event GitLabPushEvent
		if err := json.Unmarshal(payload, &event); err != nil {
			return nil, fmt.Errorf("failed to parse push event: %w", err)
		}
		return &event, nil

	case "Merge Request Hook":
		var event GitLabMergeRequestEvent
		if err := json.Unmarshal(payload, &event); err != nil {
			return nil, fmt.Errorf("failed to parse merge request event: %w", err)
		}
		return &event, nil

	case "Pipeline Hook":
		var event GitLabPipelineEvent
		if err := json.Unmarshal(payload, &event); err != nil {
			return nil, fmt.Errorf("failed to parse pipeline event: %w", err)
		}
		return &event, nil

	default:
		return nil, fmt.Errorf("unsupported event type: %s", eventType)
	}
}
