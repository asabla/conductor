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
	// DefaultBitbucketBaseURL is the default Bitbucket API base URL.
	DefaultBitbucketBaseURL = "https://api.bitbucket.org/2.0"
)

// BitbucketProvider implements the Provider interface for Bitbucket.
type BitbucketProvider struct {
	client      *http.Client
	baseURL     string
	username    string
	appPassword string
	userAgent   string
	logger      *slog.Logger

	// Rate limiting
	rateLimitMu        sync.RWMutex
	rateLimitRemaining int
	rateLimitReset     time.Time
}

// BitbucketConfig extends Config with Bitbucket-specific fields.
type BitbucketConfig struct {
	Config
	Username    string
	AppPassword string
}

// NewBitbucketProvider creates a new Bitbucket provider.
func NewBitbucketProvider(cfg Config) (*BitbucketProvider, error) {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = DefaultBitbucketBaseURL
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	return &BitbucketProvider{
		client:             &http.Client{Timeout: DefaultHTTPTimeout},
		baseURL:            baseURL,
		appPassword:        cfg.Token, // Use Token as app password for basic auth
		userAgent:          DefaultUserAgent,
		logger:             slog.Default().With("component", "bitbucket_provider"),
		rateLimitRemaining: -1,
	}, nil
}

// NewBitbucketProviderWithAuth creates a new Bitbucket provider with explicit auth.
func NewBitbucketProviderWithAuth(cfg BitbucketConfig) (*BitbucketProvider, error) {
	p, err := NewBitbucketProvider(cfg.Config)
	if err != nil {
		return nil, err
	}
	p.username = cfg.Username
	p.appPassword = cfg.AppPassword
	return p, nil
}

// NewBitbucketProviderWithLogger creates a new Bitbucket provider with a custom logger.
func NewBitbucketProviderWithLogger(cfg Config, logger *slog.Logger) (*BitbucketProvider, error) {
	p, err := NewBitbucketProvider(cfg)
	if err != nil {
		return nil, err
	}
	if logger != nil {
		p.logger = logger.With("component", "bitbucket_provider")
	}
	return p, nil
}

// GetRepository retrieves repository information.
func (b *BitbucketProvider) GetRepository(ctx context.Context, owner, repo string) (*Repository, error) {
	apiURL := fmt.Sprintf("%s/repositories/%s/%s", b.baseURL, owner, repo)

	var result bitbucketRepository
	if err := b.doRequestWithRetry(ctx, "GET", apiURL, nil, &result); err != nil {
		return nil, fmt.Errorf("failed to get repository: %w", err)
	}

	return &Repository{
		ID:            0, // Bitbucket uses UUIDs, not int64
		Name:          result.Name,
		FullName:      result.FullName,
		Description:   result.Description,
		DefaultBranch: result.MainBranch.Name,
		Private:       result.IsPrivate,
		HTMLURL:       result.Links.HTML.Href,
		CloneURL:      getCloneURL(result.Links.Clone, "https"),
		SSHURL:        getCloneURL(result.Links.Clone, "ssh"),
	}, nil
}

// GetFile retrieves file content from a repository.
func (b *BitbucketProvider) GetFile(ctx context.Context, owner, repo, path, ref string) ([]byte, error) {
	if ref == "" {
		ref = "HEAD"
	}
	apiURL := fmt.Sprintf("%s/repositories/%s/%s/src/%s/%s", b.baseURL, owner, repo, ref, path)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	b.setHeaders(req)

	resp, err := b.doWithRateLimitAndRetry(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("file not found: %s", path)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Bitbucket API error (status %d): %s", resp.StatusCode, string(body))
	}

	return io.ReadAll(resp.Body)
}

// ListFiles lists files in a directory.
func (b *BitbucketProvider) ListFiles(ctx context.Context, owner, repo, path, ref string) ([]FileInfo, error) {
	if ref == "" {
		ref = "HEAD"
	}

	apiURL := fmt.Sprintf("%s/repositories/%s/%s/src/%s/%s", b.baseURL, owner, repo, ref, path)
	if path == "" || path == "." {
		apiURL = fmt.Sprintf("%s/repositories/%s/%s/src/%s/", b.baseURL, owner, repo, ref)
	}

	var result bitbucketSrcResponse
	if err := b.doRequestWithRetry(ctx, "GET", apiURL, nil, &result); err != nil {
		if strings.Contains(err.Error(), "404") {
			return nil, fmt.Errorf("directory not found: %s", path)
		}
		return nil, fmt.Errorf("failed to list files: %w", err)
	}

	files := make([]FileInfo, len(result.Values))
	for i, e := range result.Values {
		fileType := "file"
		if e.Type == "commit_directory" {
			fileType = "dir"
		}
		files[i] = FileInfo{
			Name: getFileName(e.Path),
			Path: e.Path,
			Type: fileType,
			Size: e.Size,
			SHA:  e.Commit.Hash,
		}
	}

	return files, nil
}

// GetBranch retrieves branch information.
func (b *BitbucketProvider) GetBranch(ctx context.Context, owner, repo, branch string) (*Branch, error) {
	apiURL := fmt.Sprintf("%s/repositories/%s/%s/refs/branches/%s", b.baseURL, owner, repo, url.PathEscape(branch))

	var result bitbucketBranch
	if err := b.doRequestWithRetry(ctx, "GET", apiURL, nil, &result); err != nil {
		return nil, fmt.Errorf("failed to get branch: %w", err)
	}

	return &Branch{
		Name:      result.Name,
		SHA:       result.Target.Hash,
		Protected: false, // Bitbucket handles branch permissions differently
	}, nil
}

// GetDefaultBranch returns the default branch for a repository.
func (b *BitbucketProvider) GetDefaultBranch(ctx context.Context, owner, repo string) (string, error) {
	repoInfo, err := b.GetRepository(ctx, owner, repo)
	if err != nil {
		return "", err
	}
	return repoInfo.DefaultBranch, nil
}

// CreateCommitStatus creates a build status on a commit.
func (b *BitbucketProvider) CreateCommitStatus(ctx context.Context, owner, repo, sha string, status CommitStatus) error {
	apiURL := fmt.Sprintf("%s/repositories/%s/%s/commit/%s/statuses/build", b.baseURL, owner, repo, sha)

	// Map status states to Bitbucket states
	bitbucketState := mapToBitbucketState(status.State)

	payload := bitbucketBuildStatus{
		State:       bitbucketState,
		Key:         status.Context,
		Name:        status.Context,
		Description: status.Description,
		URL:         status.TargetURL,
	}

	if err := b.doRequestWithRetry(ctx, "POST", apiURL, payload, nil); err != nil {
		return fmt.Errorf("failed to create commit status: %w", err)
	}

	b.logger.Debug("created commit status",
		"owner", owner,
		"repo", repo,
		"sha", sha,
		"state", bitbucketState,
		"key", status.Context,
	)

	return nil
}

// GetPullRequest retrieves pull request details.
func (b *BitbucketProvider) GetPullRequest(ctx context.Context, owner, repo string, number int) (*PullRequest, error) {
	apiURL := fmt.Sprintf("%s/repositories/%s/%s/pullrequests/%d", b.baseURL, owner, repo, number)

	var bbPR bitbucketPullRequest
	if err := b.doRequestWithRetry(ctx, "GET", apiURL, nil, &bbPR); err != nil {
		if strings.Contains(err.Error(), "404") {
			return nil, fmt.Errorf("pull request not found: %d", number)
		}
		return nil, fmt.Errorf("failed to get pull request: %w", err)
	}

	state := bbPR.State
	if state == "MERGED" {
		state = "closed"
	} else {
		state = strings.ToLower(state)
	}

	pr := &PullRequest{
		Number:    bbPR.ID,
		Title:     bbPR.Title,
		Body:      bbPR.Description,
		State:     state,
		HeadRef:   bbPR.Source.Branch.Name,
		HeadSHA:   bbPR.Source.Commit.Hash,
		BaseRef:   bbPR.Destination.Branch.Name,
		BaseSHA:   bbPR.Destination.Commit.Hash,
		Author:    bbPR.Author.Nickname,
		CreatedAt: bbPR.CreatedOn,
		UpdatedAt: bbPR.UpdatedOn,
	}

	if bbPR.MergeCommit != nil {
		pr.MergeCommit = bbPR.MergeCommit.Hash
	}

	return pr, nil
}

// CreateComment posts a comment on a pull request.
func (b *BitbucketProvider) CreateComment(ctx context.Context, owner, repo string, prNumber int, body string) error {
	apiURL := fmt.Sprintf("%s/repositories/%s/%s/pullrequests/%d/comments", b.baseURL, owner, repo, prNumber)

	payload := bitbucketCommentRequest{
		Content: bitbucketContent{Raw: body},
	}

	if err := b.doRequestWithRetry(ctx, "POST", apiURL, payload, nil); err != nil {
		return fmt.Errorf("failed to create comment: %w", err)
	}

	b.logger.Debug("created PR comment",
		"owner", owner,
		"repo", repo,
		"pr_number", prNumber,
	)

	return nil
}

// setHeaders sets common headers for Bitbucket API requests.
func (b *BitbucketProvider) setHeaders(req *http.Request) {
	req.Header.Set("User-Agent", b.userAgent)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	if b.username != "" && b.appPassword != "" {
		req.SetBasicAuth(b.username, b.appPassword)
	} else if b.appPassword != "" {
		// Use Bearer token if only app password is set
		req.Header.Set("Authorization", "Bearer "+b.appPassword)
	}
}

// doRequestWithRetry performs an HTTP request with retry logic.
func (b *BitbucketProvider) doRequestWithRetry(ctx context.Context, method, apiURL string, body interface{}, result interface{}) error {
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

		b.setHeaders(req)

		resp, err := b.doWithRateLimitAndRetry(ctx, req)
		if err != nil {
			lastErr = err
			if isRetryableError(err) {
				b.logger.Debug("retrying request due to error",
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
			if b.handleRateLimitResponse(resp) {
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
			return fmt.Errorf("Bitbucket API error (status %d): %s", resp.StatusCode, string(respBody))
		}

		// Success - parse response if needed
		if result != nil && resp.StatusCode != http.StatusNoContent && resp.ContentLength != 0 {
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
func (b *BitbucketProvider) doWithRateLimitAndRetry(ctx context.Context, req *http.Request) (*http.Response, error) {
	b.rateLimitMu.RLock()
	remaining := b.rateLimitRemaining
	resetTime := b.rateLimitReset
	b.rateLimitMu.RUnlock()

	if remaining == 0 && time.Now().Before(resetTime) {
		waitDuration := time.Until(resetTime)
		b.logger.Info("rate limit exceeded, waiting for reset",
			"wait_duration", waitDuration,
			"reset_time", resetTime,
		)

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(waitDuration):
		}
	}

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}

	b.updateRateLimitFromResponse(resp)

	return resp, nil
}

// updateRateLimitFromResponse updates rate limit tracking from response headers.
func (b *BitbucketProvider) updateRateLimitFromResponse(resp *http.Response) {
	b.rateLimitMu.Lock()
	defer b.rateLimitMu.Unlock()

	// Bitbucket doesn't have standard rate limit headers, but may return Retry-After
	if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
		if seconds, err := strconv.Atoi(retryAfter); err == nil {
			b.rateLimitRemaining = 0
			b.rateLimitReset = time.Now().Add(time.Duration(seconds) * time.Second)
		}
	}
}

// handleRateLimitResponse handles a rate limit response.
func (b *BitbucketProvider) handleRateLimitResponse(resp *http.Response) bool {
	b.updateRateLimitFromResponse(resp)
	return true
}

// mapToBitbucketState maps status states to Bitbucket states.
func mapToBitbucketState(state string) string {
	switch state {
	case "pending":
		return "INPROGRESS"
	case "success":
		return "SUCCESSFUL"
	case "failure", "error":
		return "FAILED"
	default:
		return "INPROGRESS"
	}
}

// getCloneURL extracts a clone URL of the specified type.
func getCloneURL(links []bitbucketCloneLink, linkType string) string {
	for _, link := range links {
		if link.Name == linkType {
			return link.Href
		}
	}
	return ""
}

// getFileName extracts the filename from a path.
func getFileName(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return path
}

// Bitbucket API response structures

type bitbucketRepository struct {
	UUID        string `json:"uuid"`
	Name        string `json:"name"`
	FullName    string `json:"full_name"`
	Description string `json:"description"`
	IsPrivate   bool   `json:"is_private"`
	MainBranch  struct {
		Name string `json:"name"`
	} `json:"mainbranch"`
	Links struct {
		HTML struct {
			Href string `json:"href"`
		} `json:"html"`
		Clone []bitbucketCloneLink `json:"clone"`
	} `json:"links"`
	Owner struct {
		UUID     string `json:"uuid"`
		Username string `json:"username"`
	} `json:"owner"`
}

type bitbucketCloneLink struct {
	Href string `json:"href"`
	Name string `json:"name"`
}

type bitbucketSrcResponse struct {
	Values []bitbucketSrcEntry `json:"values"`
	Next   string              `json:"next"`
}

type bitbucketSrcEntry struct {
	Path   string `json:"path"`
	Type   string `json:"type"` // "commit_file" or "commit_directory"
	Size   int64  `json:"size"`
	Commit struct {
		Hash string `json:"hash"`
	} `json:"commit"`
}

type bitbucketBranch struct {
	Name   string `json:"name"`
	Target struct {
		Hash    string    `json:"hash"`
		Date    time.Time `json:"date"`
		Message string    `json:"message"`
		Author  struct {
			Raw  string `json:"raw"`
			User struct {
				Username string `json:"username"`
			} `json:"user"`
		} `json:"author"`
	} `json:"target"`
}

type bitbucketBuildStatus struct {
	State       string `json:"state"`
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	URL         string `json:"url,omitempty"`
}

type bitbucketPullRequest struct {
	ID          int       `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	State       string    `json:"state"` // OPEN, MERGED, DECLINED, SUPERSEDED
	CreatedOn   time.Time `json:"created_on"`
	UpdatedOn   time.Time `json:"updated_on"`
	Author      struct {
		UUID     string `json:"uuid"`
		Username string `json:"username"`
		Nickname string `json:"nickname"`
	} `json:"author"`
	Source struct {
		Branch struct {
			Name string `json:"name"`
		} `json:"branch"`
		Commit struct {
			Hash string `json:"hash"`
		} `json:"commit"`
	} `json:"source"`
	Destination struct {
		Branch struct {
			Name string `json:"name"`
		} `json:"branch"`
		Commit struct {
			Hash string `json:"hash"`
		} `json:"commit"`
	} `json:"destination"`
	MergeCommit *struct {
		Hash string `json:"hash"`
	} `json:"merge_commit"`
}

type bitbucketCommentRequest struct {
	Content bitbucketContent `json:"content"`
}

type bitbucketContent struct {
	Raw string `json:"raw"`
}

// Bitbucket webhook event types

// BitbucketPushEvent represents a Bitbucket push webhook event.
type BitbucketPushEvent struct {
	Push struct {
		Changes []struct {
			New struct {
				Name   string `json:"name"`
				Type   string `json:"type"` // branch or tag
				Target struct {
					Hash    string    `json:"hash"`
					Date    time.Time `json:"date"`
					Message string    `json:"message"`
					Author  struct {
						Raw  string `json:"raw"`
						User struct {
							Username string `json:"username"`
						} `json:"user"`
					} `json:"author"`
				} `json:"target"`
			} `json:"new"`
			Old struct {
				Name   string `json:"name"`
				Target struct {
					Hash string `json:"hash"`
				} `json:"target"`
			} `json:"old"`
			Created bool `json:"created"`
			Closed  bool `json:"closed"`
			Forced  bool `json:"forced"`
		} `json:"changes"`
	} `json:"push"`
	Repository struct {
		UUID      string `json:"uuid"`
		Name      string `json:"name"`
		FullName  string `json:"full_name"`
		IsPrivate bool   `json:"is_private"`
		Links     struct {
			HTML struct {
				Href string `json:"href"`
			} `json:"html"`
		} `json:"links"`
	} `json:"repository"`
	Actor struct {
		UUID     string `json:"uuid"`
		Username string `json:"username"`
		Nickname string `json:"nickname"`
	} `json:"actor"`
}

// BitbucketPullRequestEvent represents a Bitbucket pull request webhook event.
type BitbucketPullRequestEvent struct {
	PullRequest struct {
		ID          int       `json:"id"`
		Title       string    `json:"title"`
		Description string    `json:"description"`
		State       string    `json:"state"`
		CreatedOn   time.Time `json:"created_on"`
		UpdatedOn   time.Time `json:"updated_on"`
		Source      struct {
			Branch struct {
				Name string `json:"name"`
			} `json:"branch"`
			Commit struct {
				Hash string `json:"hash"`
			} `json:"commit"`
		} `json:"source"`
		Destination struct {
			Branch struct {
				Name string `json:"name"`
			} `json:"branch"`
			Commit struct {
				Hash string `json:"hash"`
			} `json:"commit"`
		} `json:"destination"`
		Author struct {
			UUID     string `json:"uuid"`
			Username string `json:"username"`
		} `json:"author"`
	} `json:"pullrequest"`
	Repository struct {
		UUID      string `json:"uuid"`
		Name      string `json:"name"`
		FullName  string `json:"full_name"`
		IsPrivate bool   `json:"is_private"`
	} `json:"repository"`
	Actor struct {
		UUID     string `json:"uuid"`
		Username string `json:"username"`
	} `json:"actor"`
}

// ParseBitbucketEvent parses a Bitbucket webhook event.
func ParseBitbucketEvent(eventType string, payload []byte) (interface{}, error) {
	switch eventType {
	case "repo:push":
		var event BitbucketPushEvent
		if err := json.Unmarshal(payload, &event); err != nil {
			return nil, fmt.Errorf("failed to parse push event: %w", err)
		}
		return &event, nil

	case "pullrequest:created", "pullrequest:updated", "pullrequest:fulfilled", "pullrequest:rejected":
		var event BitbucketPullRequestEvent
		if err := json.Unmarshal(payload, &event); err != nil {
			return nil, fmt.Errorf("failed to parse pull request event: %w", err)
		}
		return &event, nil

	default:
		return nil, fmt.Errorf("unsupported event type: %s", eventType)
	}
}
