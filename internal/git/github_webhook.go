package git

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// WebhookHandler handles GitHub webhook events.
type WebhookHandler struct {
	secret       string
	logger       *slog.Logger
	onPush       func(ctx context.Context, event *PushEvent) error
	onPR         func(ctx context.Context, event *PullRequestEvent) error
	onCheckSuite func(ctx context.Context, event *CheckSuiteEvent) error
}

// WebhookHandlerConfig configures the webhook handler.
type WebhookHandlerConfig struct {
	Secret        string
	Logger        *slog.Logger
	OnPush        func(ctx context.Context, event *PushEvent) error
	OnPullRequest func(ctx context.Context, event *PullRequestEvent) error
	OnCheckSuite  func(ctx context.Context, event *CheckSuiteEvent) error
}

// NewWebhookHandler creates a new webhook handler.
func NewWebhookHandler(cfg WebhookHandlerConfig) *WebhookHandler {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &WebhookHandler{
		secret:       cfg.Secret,
		logger:       logger.With("component", "github_webhook"),
		onPush:       cfg.OnPush,
		onPR:         cfg.OnPullRequest,
		onCheckSuite: cfg.OnCheckSuite,
	}
}

// ValidateSignature validates the webhook signature.
func ValidateSignature(payload []byte, signature, secret string) bool {
	if secret == "" {
		// No secret configured, skip validation
		return true
	}

	if signature == "" {
		return false
	}

	// GitHub sends signature in format "sha256=<hash>"
	const prefix = "sha256="
	if !strings.HasPrefix(signature, prefix) {
		return false
	}

	expectedSig := signature[len(prefix):]

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	actualSig := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(expectedSig), []byte(actualSig))
}

// ParseEvent parses a webhook event from the payload.
func ParseEvent(eventType string, payload []byte) (interface{}, error) {
	switch eventType {
	case "push":
		var event GitHubPushEvent
		if err := json.Unmarshal(payload, &event); err != nil {
			return nil, fmt.Errorf("failed to parse push event: %w", err)
		}
		return convertPushEvent(&event), nil

	case "pull_request":
		var event GitHubPullRequestEvent
		if err := json.Unmarshal(payload, &event); err != nil {
			return nil, fmt.Errorf("failed to parse pull_request event: %w", err)
		}
		return convertPullRequestEvent(&event), nil

	case "check_suite":
		var event GitHubCheckSuiteEvent
		if err := json.Unmarshal(payload, &event); err != nil {
			return nil, fmt.Errorf("failed to parse check_suite event: %w", err)
		}
		return convertCheckSuiteEvent(&event), nil

	case "check_run":
		var event GitHubCheckRunEvent
		if err := json.Unmarshal(payload, &event); err != nil {
			return nil, fmt.Errorf("failed to parse check_run event: %w", err)
		}
		return convertCheckRunEvent(&event), nil

	case "ping":
		var event GitHubPingEvent
		if err := json.Unmarshal(payload, &event); err != nil {
			return nil, fmt.Errorf("failed to parse ping event: %w", err)
		}
		return &PingEvent{
			Zen:    event.Zen,
			HookID: event.HookID,
		}, nil

	default:
		return nil, fmt.Errorf("unsupported event type: %s", eventType)
	}
}

// HandleWebhook processes a webhook event.
func (h *WebhookHandler) HandleWebhook(ctx context.Context, eventType string, deliveryID string, payload []byte, signature string) error {
	// Validate signature
	if !ValidateSignature(payload, signature, h.secret) {
		h.logger.Warn("invalid webhook signature",
			"event_type", eventType,
			"delivery_id", deliveryID,
		)
		return fmt.Errorf("invalid webhook signature")
	}

	h.logger.Info("processing webhook",
		"event_type", eventType,
		"delivery_id", deliveryID,
	)

	// Parse and handle the event
	event, err := ParseEvent(eventType, payload)
	if err != nil {
		// For unsupported events, just log and return success
		if strings.Contains(err.Error(), "unsupported event type") {
			h.logger.Debug("ignoring unsupported event type",
				"event_type", eventType,
			)
			return nil
		}
		return err
	}

	switch e := event.(type) {
	case *PushEvent:
		return h.HandlePush(ctx, e)
	case *PullRequestEvent:
		return h.HandlePullRequest(ctx, e)
	case *CheckSuiteEvent:
		return h.HandleCheckSuite(ctx, e)
	case *CheckRunEvent:
		// Check runs are typically handled via check suite
		h.logger.Debug("ignoring check_run event (handled via check_suite)")
		return nil
	case *PingEvent:
		h.logger.Info("received ping event", "zen", e.Zen)
		return nil
	default:
		h.logger.Debug("unhandled event type", "event_type", eventType)
		return nil
	}
}

// HandlePush handles push events.
func (h *WebhookHandler) HandlePush(ctx context.Context, event *PushEvent) error {
	h.logger.Info("handling push event",
		"repo", event.Repository.FullName,
		"ref", event.Ref,
		"before", event.Before,
		"after", event.After,
		"pusher", event.Pusher.Name,
	)

	// Skip if deleting a branch
	if event.Deleted {
		h.logger.Debug("ignoring branch deletion", "ref", event.Ref)
		return nil
	}

	if h.onPush != nil {
		return h.onPush(ctx, event)
	}

	return nil
}

// HandlePullRequest handles pull request events.
func (h *WebhookHandler) HandlePullRequest(ctx context.Context, event *PullRequestEvent) error {
	h.logger.Info("handling pull request event",
		"repo", event.Repository.FullName,
		"action", event.Action,
		"pr_number", event.Number,
		"pr_title", event.PullRequest.Title,
		"head_sha", event.PullRequest.Head.SHA,
	)

	// Only handle specific actions
	switch event.Action {
	case "opened", "synchronize", "reopened":
		if h.onPR != nil {
			return h.onPR(ctx, event)
		}
	default:
		h.logger.Debug("ignoring PR action", "action", event.Action)
	}

	return nil
}

// HandleCheckSuite handles check suite events.
func (h *WebhookHandler) HandleCheckSuite(ctx context.Context, event *CheckSuiteEvent) error {
	h.logger.Info("handling check suite event",
		"repo", event.Repository.FullName,
		"action", event.Action,
		"head_sha", event.CheckSuite.HeadSHA,
	)

	// Only handle "requested" and "rerequested" actions
	switch event.Action {
	case "requested", "rerequested":
		if h.onCheckSuite != nil {
			return h.onCheckSuite(ctx, event)
		}
	default:
		h.logger.Debug("ignoring check suite action", "action", event.Action)
	}

	return nil
}

// GitHub webhook event types (raw JSON structures)

// GitHubPushEvent represents a raw GitHub push webhook event.
type GitHubPushEvent struct {
	Ref        string `json:"ref"`
	Before     string `json:"before"`
	After      string `json:"after"`
	Created    bool   `json:"created"`
	Deleted    bool   `json:"deleted"`
	Forced     bool   `json:"forced"`
	HeadCommit *struct {
		ID        string    `json:"id"`
		Message   string    `json:"message"`
		Timestamp time.Time `json:"timestamp"`
		Author    struct {
			Name     string `json:"name"`
			Email    string `json:"email"`
			Username string `json:"username"`
		} `json:"author"`
		Committer struct {
			Name     string `json:"name"`
			Email    string `json:"email"`
			Username string `json:"username"`
		} `json:"committer"`
		Added    []string `json:"added"`
		Removed  []string `json:"removed"`
		Modified []string `json:"modified"`
	} `json:"head_commit"`
	Commits []struct {
		ID        string    `json:"id"`
		Message   string    `json:"message"`
		Timestamp time.Time `json:"timestamp"`
		Author    struct {
			Name     string `json:"name"`
			Email    string `json:"email"`
			Username string `json:"username"`
		} `json:"author"`
		Added    []string `json:"added"`
		Removed  []string `json:"removed"`
		Modified []string `json:"modified"`
	} `json:"commits"`
	Repository struct {
		ID            int64  `json:"id"`
		Name          string `json:"name"`
		FullName      string `json:"full_name"`
		Private       bool   `json:"private"`
		HTMLURL       string `json:"html_url"`
		CloneURL      string `json:"clone_url"`
		DefaultBranch string `json:"default_branch"`
		Owner         struct {
			Login string `json:"login"`
		} `json:"owner"`
	} `json:"repository"`
	Pusher struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"pusher"`
	Sender struct {
		Login string `json:"login"`
		ID    int64  `json:"id"`
	} `json:"sender"`
}

// GitHubPullRequestEvent represents a raw GitHub pull request webhook event.
type GitHubPullRequestEvent struct {
	Action      string `json:"action"`
	Number      int    `json:"number"`
	PullRequest struct {
		ID        int64     `json:"id"`
		Number    int       `json:"number"`
		State     string    `json:"state"`
		Title     string    `json:"title"`
		Body      string    `json:"body"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Head      struct {
			Ref  string `json:"ref"`
			SHA  string `json:"sha"`
			Repo struct {
				Name     string `json:"name"`
				FullName string `json:"full_name"`
			} `json:"repo"`
		} `json:"head"`
		Base struct {
			Ref  string `json:"ref"`
			SHA  string `json:"sha"`
			Repo struct {
				Name     string `json:"name"`
				FullName string `json:"full_name"`
			} `json:"repo"`
		} `json:"base"`
		User struct {
			Login string `json:"login"`
			ID    int64  `json:"id"`
		} `json:"user"`
	} `json:"pull_request"`
	Repository struct {
		ID            int64  `json:"id"`
		Name          string `json:"name"`
		FullName      string `json:"full_name"`
		Private       bool   `json:"private"`
		HTMLURL       string `json:"html_url"`
		CloneURL      string `json:"clone_url"`
		DefaultBranch string `json:"default_branch"`
		Owner         struct {
			Login string `json:"login"`
		} `json:"owner"`
	} `json:"repository"`
	Sender struct {
		Login string `json:"login"`
		ID    int64  `json:"id"`
	} `json:"sender"`
}

// GitHubCheckSuiteEvent represents a raw GitHub check suite webhook event.
type GitHubCheckSuiteEvent struct {
	Action     string `json:"action"`
	CheckSuite struct {
		ID         int64  `json:"id"`
		HeadBranch string `json:"head_branch"`
		HeadSHA    string `json:"head_sha"`
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
		App        struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
			Slug string `json:"slug"`
		} `json:"app"`
		PullRequests []struct {
			Number int `json:"number"`
			Head   struct {
				Ref string `json:"ref"`
				SHA string `json:"sha"`
			} `json:"head"`
			Base struct {
				Ref string `json:"ref"`
				SHA string `json:"sha"`
			} `json:"base"`
		} `json:"pull_requests"`
	} `json:"check_suite"`
	Repository struct {
		ID            int64  `json:"id"`
		Name          string `json:"name"`
		FullName      string `json:"full_name"`
		Private       bool   `json:"private"`
		HTMLURL       string `json:"html_url"`
		CloneURL      string `json:"clone_url"`
		DefaultBranch string `json:"default_branch"`
		Owner         struct {
			Login string `json:"login"`
		} `json:"owner"`
	} `json:"repository"`
	Sender struct {
		Login string `json:"login"`
		ID    int64  `json:"id"`
	} `json:"sender"`
}

// GitHubCheckRunEvent represents a raw GitHub check run webhook event.
type GitHubCheckRunEvent struct {
	Action   string `json:"action"`
	CheckRun struct {
		ID         int64  `json:"id"`
		HeadSHA    string `json:"head_sha"`
		Name       string `json:"name"`
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
		CheckSuite struct {
			ID int64 `json:"id"`
		} `json:"check_suite"`
	} `json:"check_run"`
	Repository struct {
		ID       int64  `json:"id"`
		Name     string `json:"name"`
		FullName string `json:"full_name"`
		Private  bool   `json:"private"`
		Owner    struct {
			Login string `json:"login"`
		} `json:"owner"`
	} `json:"repository"`
	Sender struct {
		Login string `json:"login"`
		ID    int64  `json:"id"`
	} `json:"sender"`
}

// GitHubPingEvent represents a GitHub ping webhook event.
type GitHubPingEvent struct {
	Zen    string `json:"zen"`
	HookID int64  `json:"hook_id"`
	Hook   struct {
		Type   string   `json:"type"`
		ID     int64    `json:"id"`
		Name   string   `json:"name"`
		Active bool     `json:"active"`
		Events []string `json:"events"`
	} `json:"hook"`
}

// Converted event types (internal representation)

// PushEvent represents a converted push event.
type PushEvent struct {
	Ref        string
	Before     string
	After      string
	Created    bool
	Deleted    bool
	Forced     bool
	HeadCommit *CommitInfo
	Commits    []CommitInfo
	Repository RepositoryInfo
	Pusher     UserInfo
	Sender     UserInfo
}

// PullRequestEvent represents a converted pull request event.
type PullRequestEvent struct {
	Action      string
	Number      int
	PullRequest PullRequestInfo
	Repository  RepositoryInfo
	Sender      UserInfo
}

// CheckSuiteEvent represents a converted check suite event.
type CheckSuiteEvent struct {
	Action     string
	CheckSuite CheckSuiteInfo
	Repository RepositoryInfo
	Sender     UserInfo
}

// CheckRunEvent represents a converted check run event.
type CheckRunEvent struct {
	Action     string
	CheckRun   CheckRunInfo
	Repository RepositoryInfo
	Sender     UserInfo
}

// PingEvent represents a ping event.
type PingEvent struct {
	Zen    string
	HookID int64
}

// RepositoryInfo contains repository information from webhooks.
type RepositoryInfo struct {
	ID            int64
	Name          string
	FullName      string
	Private       bool
	HTMLURL       string
	CloneURL      string
	DefaultBranch string
	Owner         string
}

// UserInfo contains user information from webhooks.
type UserInfo struct {
	Login string
	Name  string
	Email string
	ID    int64
}

// PullRequestInfo contains pull request information from webhooks.
type PullRequestInfo struct {
	ID        int64
	Number    int
	State     string
	Title     string
	Body      string
	CreatedAt time.Time
	UpdatedAt time.Time
	Head      RefInfo
	Base      RefInfo
	User      UserInfo
}

// RefInfo contains branch/ref information.
type RefInfo struct {
	Ref          string
	SHA          string
	RepoName     string
	RepoFullName string
}

// CheckSuiteInfo contains check suite information.
type CheckSuiteInfo struct {
	ID           int64
	HeadBranch   string
	HeadSHA      string
	Status       string
	Conclusion   string
	AppID        int64
	AppName      string
	PullRequests []PullRequestRef
}

// PullRequestRef is a reference to a pull request.
type PullRequestRef struct {
	Number  int
	HeadRef string
	HeadSHA string
	BaseRef string
	BaseSHA string
}

// CheckRunInfo contains check run information.
type CheckRunInfo struct {
	ID           int64
	HeadSHA      string
	Name         string
	Status       string
	Conclusion   string
	CheckSuiteID int64
}

// Conversion functions

func convertPushEvent(e *GitHubPushEvent) *PushEvent {
	result := &PushEvent{
		Ref:     e.Ref,
		Before:  e.Before,
		After:   e.After,
		Created: e.Created,
		Deleted: e.Deleted,
		Forced:  e.Forced,
		Repository: RepositoryInfo{
			ID:            e.Repository.ID,
			Name:          e.Repository.Name,
			FullName:      e.Repository.FullName,
			Private:       e.Repository.Private,
			HTMLURL:       e.Repository.HTMLURL,
			CloneURL:      e.Repository.CloneURL,
			DefaultBranch: e.Repository.DefaultBranch,
			Owner:         e.Repository.Owner.Login,
		},
		Pusher: UserInfo{
			Name:  e.Pusher.Name,
			Email: e.Pusher.Email,
		},
		Sender: UserInfo{
			Login: e.Sender.Login,
			ID:    e.Sender.ID,
		},
	}

	if e.HeadCommit != nil {
		result.HeadCommit = &CommitInfo{
			ID:        e.HeadCommit.ID,
			Message:   e.HeadCommit.Message,
			Author:    e.HeadCommit.Author.Name,
			Timestamp: e.HeadCommit.Timestamp,
			Added:     e.HeadCommit.Added,
			Removed:   e.HeadCommit.Removed,
			Modified:  e.HeadCommit.Modified,
		}
	}

	for _, c := range e.Commits {
		result.Commits = append(result.Commits, CommitInfo{
			ID:        c.ID,
			Message:   c.Message,
			Author:    c.Author.Name,
			Timestamp: c.Timestamp,
			Added:     c.Added,
			Removed:   c.Removed,
			Modified:  c.Modified,
		})
	}

	return result
}

func convertPullRequestEvent(e *GitHubPullRequestEvent) *PullRequestEvent {
	return &PullRequestEvent{
		Action: e.Action,
		Number: e.Number,
		PullRequest: PullRequestInfo{
			ID:        e.PullRequest.ID,
			Number:    e.PullRequest.Number,
			State:     e.PullRequest.State,
			Title:     e.PullRequest.Title,
			Body:      e.PullRequest.Body,
			CreatedAt: e.PullRequest.CreatedAt,
			UpdatedAt: e.PullRequest.UpdatedAt,
			Head: RefInfo{
				Ref:          e.PullRequest.Head.Ref,
				SHA:          e.PullRequest.Head.SHA,
				RepoName:     e.PullRequest.Head.Repo.Name,
				RepoFullName: e.PullRequest.Head.Repo.FullName,
			},
			Base: RefInfo{
				Ref:          e.PullRequest.Base.Ref,
				SHA:          e.PullRequest.Base.SHA,
				RepoName:     e.PullRequest.Base.Repo.Name,
				RepoFullName: e.PullRequest.Base.Repo.FullName,
			},
			User: UserInfo{
				Login: e.PullRequest.User.Login,
				ID:    e.PullRequest.User.ID,
			},
		},
		Repository: RepositoryInfo{
			ID:            e.Repository.ID,
			Name:          e.Repository.Name,
			FullName:      e.Repository.FullName,
			Private:       e.Repository.Private,
			HTMLURL:       e.Repository.HTMLURL,
			CloneURL:      e.Repository.CloneURL,
			DefaultBranch: e.Repository.DefaultBranch,
			Owner:         e.Repository.Owner.Login,
		},
		Sender: UserInfo{
			Login: e.Sender.Login,
			ID:    e.Sender.ID,
		},
	}
}

func convertCheckSuiteEvent(e *GitHubCheckSuiteEvent) *CheckSuiteEvent {
	result := &CheckSuiteEvent{
		Action: e.Action,
		CheckSuite: CheckSuiteInfo{
			ID:         e.CheckSuite.ID,
			HeadBranch: e.CheckSuite.HeadBranch,
			HeadSHA:    e.CheckSuite.HeadSHA,
			Status:     e.CheckSuite.Status,
			Conclusion: e.CheckSuite.Conclusion,
			AppID:      e.CheckSuite.App.ID,
			AppName:    e.CheckSuite.App.Name,
		},
		Repository: RepositoryInfo{
			ID:            e.Repository.ID,
			Name:          e.Repository.Name,
			FullName:      e.Repository.FullName,
			Private:       e.Repository.Private,
			HTMLURL:       e.Repository.HTMLURL,
			CloneURL:      e.Repository.CloneURL,
			DefaultBranch: e.Repository.DefaultBranch,
			Owner:         e.Repository.Owner.Login,
		},
		Sender: UserInfo{
			Login: e.Sender.Login,
			ID:    e.Sender.ID,
		},
	}

	for _, pr := range e.CheckSuite.PullRequests {
		result.CheckSuite.PullRequests = append(result.CheckSuite.PullRequests, PullRequestRef{
			Number:  pr.Number,
			HeadRef: pr.Head.Ref,
			HeadSHA: pr.Head.SHA,
			BaseRef: pr.Base.Ref,
			BaseSHA: pr.Base.SHA,
		})
	}

	return result
}

func convertCheckRunEvent(e *GitHubCheckRunEvent) *CheckRunEvent {
	return &CheckRunEvent{
		Action: e.Action,
		CheckRun: CheckRunInfo{
			ID:           e.CheckRun.ID,
			HeadSHA:      e.CheckRun.HeadSHA,
			Name:         e.CheckRun.Name,
			Status:       e.CheckRun.Status,
			Conclusion:   e.CheckRun.Conclusion,
			CheckSuiteID: e.CheckRun.CheckSuite.ID,
		},
		Repository: RepositoryInfo{
			ID:       e.Repository.ID,
			Name:     e.Repository.Name,
			FullName: e.Repository.FullName,
			Private:  e.Repository.Private,
			Owner:    e.Repository.Owner.Login,
		},
		Sender: UserInfo{
			Login: e.Sender.Login,
			ID:    e.Sender.ID,
		},
	}
}

// GetBranch extracts the branch name from a ref (e.g., "refs/heads/main" -> "main").
func GetBranch(ref string) string {
	const prefix = "refs/heads/"
	if strings.HasPrefix(ref, prefix) {
		return strings.TrimPrefix(ref, prefix)
	}
	return ref
}

// IsDefaultBranch checks if the ref points to the default branch.
func IsDefaultBranch(ref, defaultBranch string) bool {
	branch := GetBranch(ref)
	return branch == defaultBranch
}

// IsBranchRef checks if the ref is a branch ref (not a tag).
func IsBranchRef(ref string) bool {
	return strings.HasPrefix(ref, "refs/heads/")
}

// IsTagRef checks if the ref is a tag ref.
func IsTagRef(ref string) bool {
	return strings.HasPrefix(ref, "refs/tags/")
}
