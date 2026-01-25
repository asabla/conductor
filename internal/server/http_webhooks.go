package server

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/conductor/conductor/internal/database"
)

// WebhookHandler handles incoming webhooks from git providers.
type WebhookHandler struct {
	logger      zerolog.Logger
	serviceRepo WebhookServiceRepository
	scheduler   RunScheduler

	// Secrets for webhook validation
	githubSecret    string
	gitlabSecret    string
	bitbucketSecret string

	// Base URL for constructing callback URLs
	baseURL string
}

// WebhookServiceRepository defines the interface for service lookup in webhooks.
type WebhookServiceRepository interface {
	List(ctx context.Context, page database.Pagination) ([]database.Service, error)
}

// RunScheduler defines the interface for scheduling test runs.
type RunScheduler interface {
	ScheduleRun(ctx context.Context, req ScheduleRunRequest) (*database.TestRun, error)
}

// ScheduleRunRequest contains parameters for scheduling a test run.
type ScheduleRunRequest struct {
	ServiceID   uuid.UUID
	GitRef      string
	GitSHA      string
	TriggerType database.TriggerType
	TriggeredBy string
	Priority    int
}

// WebhookConfig holds configuration for the webhook handler.
type WebhookConfig struct {
	GithubSecret    string
	GitlabSecret    string
	BitbucketSecret string
	BaseURL         string
}

// NewWebhookHandler creates a new webhook handler.
func NewWebhookHandler(
	cfg WebhookConfig,
	serviceRepo WebhookServiceRepository,
	scheduler RunScheduler,
	logger zerolog.Logger,
) *WebhookHandler {
	return &WebhookHandler{
		logger:          logger.With().Str("component", "webhook_handler").Logger(),
		serviceRepo:     serviceRepo,
		scheduler:       scheduler,
		githubSecret:    cfg.GithubSecret,
		gitlabSecret:    cfg.GitlabSecret,
		bitbucketSecret: cfg.BitbucketSecret,
		baseURL:         cfg.BaseURL,
	}
}

// RegisterRoutes registers webhook routes on the given mux.
func (h *WebhookHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/webhooks/github", h.HandleGitHubWebhook)
	mux.HandleFunc("POST /api/v1/webhooks/gitlab", h.HandleGitLabWebhook)
	mux.HandleFunc("POST /api/v1/webhooks/bitbucket", h.HandleBitbucketWebhook)

	// Also register without /api/v1 prefix for compatibility
	mux.HandleFunc("POST /webhooks/github", h.HandleGitHubWebhook)
	mux.HandleFunc("POST /webhooks/gitlab", h.HandleGitLabWebhook)
	mux.HandleFunc("POST /webhooks/bitbucket", h.HandleBitbucketWebhook)
}

// HandleGitHubWebhook handles GitHub webhook events.
func (h *WebhookHandler) HandleGitHubWebhook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	requestID := GetRequestID(ctx)

	h.logger.Info().
		Str("request_id", requestID).
		Msg("received GitHub webhook")

	// Read the payload
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Error().Err(err).Msg("failed to read webhook payload")
		http.Error(w, "failed to read payload", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Validate signature
	signature := r.Header.Get("X-Hub-Signature-256")
	if !validateGitHubSignature(payload, signature, h.githubSecret) {
		h.logger.Warn().
			Str("request_id", requestID).
			Msg("invalid webhook signature")
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	// Get event type and delivery ID
	eventType := r.Header.Get("X-GitHub-Event")
	deliveryID := r.Header.Get("X-GitHub-Delivery")

	h.logger.Debug().
		Str("request_id", requestID).
		Str("event_type", eventType).
		Str("delivery_id", deliveryID).
		Msg("processing GitHub webhook")

	// Parse and handle the event
	if err := h.processGitHubEvent(ctx, eventType, payload); err != nil {
		h.logger.Error().Err(err).
			Str("event_type", eventType).
			Msg("failed to handle webhook event")
		http.Error(w, "failed to handle event", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// processGitHubEvent processes a GitHub webhook event.
func (h *WebhookHandler) processGitHubEvent(ctx context.Context, eventType string, payload []byte) error {
	switch eventType {
	case "push":
		var event githubWebhookPushEvent
		if err := json.Unmarshal(payload, &event); err != nil {
			return fmt.Errorf("failed to parse push event: %w", err)
		}
		return h.handleGitHubPush(ctx, &event)

	case "pull_request":
		var event githubWebhookPREvent
		if err := json.Unmarshal(payload, &event); err != nil {
			return fmt.Errorf("failed to parse PR event: %w", err)
		}
		return h.handleGitHubPR(ctx, &event)

	case "check_suite":
		var event githubWebhookCheckSuiteEvent
		if err := json.Unmarshal(payload, &event); err != nil {
			return fmt.Errorf("failed to parse check suite event: %w", err)
		}
		return h.handleGitHubCheckSuite(ctx, &event)

	case "ping":
		h.logger.Info().Msg("received ping event")
		return nil

	default:
		h.logger.Debug().
			Str("event_type", eventType).
			Msg("ignoring unsupported event type")
		return nil
	}
}

// handleGitHubPush handles a GitHub push event.
func (h *WebhookHandler) handleGitHubPush(ctx context.Context, event *githubWebhookPushEvent) error {
	if event.Deleted {
		h.logger.Debug().
			Str("ref", event.Ref).
			Msg("ignoring branch deletion")
		return nil
	}

	// Only trigger on branch pushes, not tags
	if !strings.HasPrefix(event.Ref, "refs/heads/") {
		h.logger.Debug().
			Str("ref", event.Ref).
			Msg("ignoring non-branch ref")
		return nil
	}

	branch := strings.TrimPrefix(event.Ref, "refs/heads/")

	h.logger.Info().
		Str("repo", event.Repository.FullName).
		Str("branch", branch).
		Str("sha", event.After).
		Str("pusher", event.Pusher.Name).
		Msg("processing push event")

	return h.triggerTestRun(ctx, event.Repository.FullName, event.Repository.Owner.Login,
		event.Repository.Name, branch, event.After, event.Pusher.Name, 0)
}

// handleGitHubPR handles a GitHub pull request event.
func (h *WebhookHandler) handleGitHubPR(ctx context.Context, event *githubWebhookPREvent) error {
	switch event.Action {
	case "opened", "synchronize", "reopened":
		// Continue processing
	default:
		h.logger.Debug().
			Str("action", event.Action).
			Msg("ignoring PR action")
		return nil
	}

	h.logger.Info().
		Str("repo", event.Repository.FullName).
		Int("pr_number", event.Number).
		Str("action", event.Action).
		Str("head_sha", event.PullRequest.Head.SHA).
		Msg("processing PR event")

	return h.triggerTestRun(ctx, event.Repository.FullName, event.Repository.Owner.Login,
		event.Repository.Name, event.PullRequest.Head.Ref, event.PullRequest.Head.SHA,
		event.Sender.Login, 1) // Higher priority for PRs
}

// handleGitHubCheckSuite handles a GitHub check suite event.
func (h *WebhookHandler) handleGitHubCheckSuite(ctx context.Context, event *githubWebhookCheckSuiteEvent) error {
	switch event.Action {
	case "requested", "rerequested":
		// Continue processing
	default:
		h.logger.Debug().
			Str("action", event.Action).
			Msg("ignoring check suite action")
		return nil
	}

	h.logger.Info().
		Str("repo", event.Repository.FullName).
		Str("head_sha", event.CheckSuite.HeadSHA).
		Msg("processing check suite event")

	return h.triggerTestRun(ctx, event.Repository.FullName, event.Repository.Owner.Login,
		event.Repository.Name, event.CheckSuite.HeadBranch, event.CheckSuite.HeadSHA,
		event.Sender.Login, 0)
}

// HandleGitLabWebhook handles GitLab webhook events.
func (h *WebhookHandler) HandleGitLabWebhook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	requestID := GetRequestID(ctx)

	h.logger.Info().
		Str("request_id", requestID).
		Msg("received GitLab webhook")

	// Read the payload
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Error().Err(err).Msg("failed to read webhook payload")
		http.Error(w, "failed to read payload", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Validate token
	token := r.Header.Get("X-Gitlab-Token")
	if h.gitlabSecret != "" && token != h.gitlabSecret {
		h.logger.Warn().
			Str("request_id", requestID).
			Msg("invalid webhook token")
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	// Get event type
	eventType := r.Header.Get("X-Gitlab-Event")

	h.logger.Debug().
		Str("request_id", requestID).
		Str("event_type", eventType).
		Msg("processing GitLab webhook")

	// Parse and handle the event
	if err := h.processGitLabEvent(ctx, eventType, payload); err != nil {
		h.logger.Error().Err(err).
			Str("event_type", eventType).
			Msg("failed to handle webhook event")
		http.Error(w, "failed to handle event", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// processGitLabEvent processes a GitLab webhook event.
func (h *WebhookHandler) processGitLabEvent(ctx context.Context, eventType string, payload []byte) error {
	switch eventType {
	case "Push Hook":
		var event gitlabWebhookPushEvent
		if err := json.Unmarshal(payload, &event); err != nil {
			return fmt.Errorf("failed to parse push event: %w", err)
		}
		return h.handleGitLabPush(ctx, &event)

	case "Merge Request Hook":
		var event gitlabWebhookMREvent
		if err := json.Unmarshal(payload, &event); err != nil {
			return fmt.Errorf("failed to parse MR event: %w", err)
		}
		return h.handleGitLabMR(ctx, &event)

	default:
		h.logger.Debug().
			Str("event_type", eventType).
			Msg("ignoring unsupported event type")
		return nil
	}
}

// handleGitLabPush handles a GitLab push event.
func (h *WebhookHandler) handleGitLabPush(ctx context.Context, event *gitlabWebhookPushEvent) error {
	// Check for branch deletion
	if event.After == "0000000000000000000000000000000000000000" {
		h.logger.Debug().
			Str("ref", event.Ref).
			Msg("ignoring branch deletion")
		return nil
	}

	if !strings.HasPrefix(event.Ref, "refs/heads/") {
		h.logger.Debug().
			Str("ref", event.Ref).
			Msg("ignoring non-branch ref")
		return nil
	}

	branch := strings.TrimPrefix(event.Ref, "refs/heads/")
	parts := strings.SplitN(event.Project.PathWithNamespace, "/", 2)
	owner, repo := "", event.Project.PathWithNamespace
	if len(parts) == 2 {
		owner, repo = parts[0], parts[1]
	}

	h.logger.Info().
		Str("repo", event.Project.PathWithNamespace).
		Str("branch", branch).
		Str("sha", event.After).
		Str("pusher", event.UserUsername).
		Msg("processing push event")

	return h.triggerTestRun(ctx, event.Project.PathWithNamespace, owner, repo, branch,
		event.After, event.UserUsername, 0)
}

// handleGitLabMR handles a GitLab merge request event.
func (h *WebhookHandler) handleGitLabMR(ctx context.Context, event *gitlabWebhookMREvent) error {
	switch event.ObjectAttributes.Action {
	case "open", "reopen", "update":
		// Continue processing
	default:
		h.logger.Debug().
			Str("action", event.ObjectAttributes.Action).
			Msg("ignoring MR action")
		return nil
	}

	parts := strings.SplitN(event.Project.PathWithNamespace, "/", 2)
	owner, repo := "", event.Project.PathWithNamespace
	if len(parts) == 2 {
		owner, repo = parts[0], parts[1]
	}

	h.logger.Info().
		Str("repo", event.Project.PathWithNamespace).
		Int("mr_number", event.ObjectAttributes.IID).
		Str("action", event.ObjectAttributes.Action).
		Msg("processing MR event")

	return h.triggerTestRun(ctx, event.Project.PathWithNamespace, owner, repo,
		event.ObjectAttributes.SourceBranch, event.ObjectAttributes.LastCommit.ID,
		event.User.Username, 1)
}

// HandleBitbucketWebhook handles Bitbucket webhook events.
func (h *WebhookHandler) HandleBitbucketWebhook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	requestID := GetRequestID(ctx)

	h.logger.Info().
		Str("request_id", requestID).
		Msg("received Bitbucket webhook")

	// Read the payload
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Error().Err(err).Msg("failed to read webhook payload")
		http.Error(w, "failed to read payload", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Validate authorization if configured
	if h.bitbucketSecret != "" {
		authHeader := r.Header.Get("Authorization")
		if authHeader != "Bearer "+h.bitbucketSecret {
			h.logger.Warn().
				Str("request_id", requestID).
				Msg("invalid webhook authorization")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}

	// Get event type
	eventType := r.Header.Get("X-Event-Key")

	h.logger.Debug().
		Str("request_id", requestID).
		Str("event_type", eventType).
		Msg("processing Bitbucket webhook")

	// Parse and handle the event
	if err := h.processBitbucketEvent(ctx, eventType, payload); err != nil {
		h.logger.Error().Err(err).
			Str("event_type", eventType).
			Msg("failed to handle webhook event")
		http.Error(w, "failed to handle event", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// processBitbucketEvent processes a Bitbucket webhook event.
func (h *WebhookHandler) processBitbucketEvent(ctx context.Context, eventType string, payload []byte) error {
	switch eventType {
	case "repo:push":
		var event bitbucketWebhookPushEvent
		if err := json.Unmarshal(payload, &event); err != nil {
			return fmt.Errorf("failed to parse push event: %w", err)
		}
		return h.handleBitbucketPush(ctx, &event)

	case "pullrequest:created", "pullrequest:updated":
		var event bitbucketWebhookPREvent
		if err := json.Unmarshal(payload, &event); err != nil {
			return fmt.Errorf("failed to parse PR event: %w", err)
		}
		return h.handleBitbucketPR(ctx, &event)

	default:
		h.logger.Debug().
			Str("event_type", eventType).
			Msg("ignoring unsupported event type")
		return nil
	}
}

// handleBitbucketPush handles a Bitbucket push event.
func (h *WebhookHandler) handleBitbucketPush(ctx context.Context, event *bitbucketWebhookPushEvent) error {
	for _, change := range event.Push.Changes {
		if change.Closed {
			continue
		}

		parts := strings.SplitN(event.Repository.FullName, "/", 2)
		owner, repo := "", event.Repository.FullName
		if len(parts) == 2 {
			owner, repo = parts[0], parts[1]
		}

		h.logger.Info().
			Str("repo", event.Repository.FullName).
			Str("branch", change.New.Name).
			Str("sha", change.New.Target.Hash).
			Msg("processing push event")

		if err := h.triggerTestRun(ctx, event.Repository.FullName, owner, repo,
			change.New.Name, change.New.Target.Hash, event.Actor.Username, 0); err != nil {
			return err
		}
	}
	return nil
}

// handleBitbucketPR handles a Bitbucket pull request event.
func (h *WebhookHandler) handleBitbucketPR(ctx context.Context, event *bitbucketWebhookPREvent) error {
	parts := strings.SplitN(event.Repository.FullName, "/", 2)
	owner, repo := "", event.Repository.FullName
	if len(parts) == 2 {
		owner, repo = parts[0], parts[1]
	}

	h.logger.Info().
		Str("repo", event.Repository.FullName).
		Int("pr_id", event.PullRequest.ID).
		Msg("processing PR event")

	return h.triggerTestRun(ctx, event.Repository.FullName, owner, repo,
		event.PullRequest.Source.Branch.Name, event.PullRequest.Source.Commit.Hash,
		event.Actor.Username, 1)
}

// triggerTestRun schedules a test run for the given repository and commit.
func (h *WebhookHandler) triggerTestRun(ctx context.Context, repoFullName, owner, repo, branch, sha, triggeredBy string, priority int) error {
	// Find service by git URL
	service, err := h.findServiceByRepo(ctx, owner, repo, repoFullName)
	if err != nil {
		h.logger.Debug().
			Str("repo", repoFullName).
			Err(err).
			Msg("no service found for repository")
		return nil // Not an error - repo might not be registered
	}

	if h.scheduler == nil {
		h.logger.Debug().Msg("no scheduler configured")
		return nil
	}

	run, err := h.scheduler.ScheduleRun(ctx, ScheduleRunRequest{
		ServiceID:   service.ID,
		GitRef:      branch,
		GitSHA:      sha,
		TriggerType: database.TriggerTypeWebhook,
		TriggeredBy: triggeredBy,
		Priority:    priority,
	})
	if err != nil {
		return fmt.Errorf("failed to schedule run: %w", err)
	}

	h.logger.Info().
		Str("run_id", run.ID.String()).
		Str("service", service.Name).
		Str("branch", branch).
		Str("sha", sha).
		Msg("scheduled test run from webhook")

	return nil
}

// findServiceByRepo finds a service by repository information.
func (h *WebhookHandler) findServiceByRepo(ctx context.Context, owner, repo, fullName string) (*database.Service, error) {
	patterns := []string{
		fmt.Sprintf("https://github.com/%s", fullName),
		fmt.Sprintf("https://gitlab.com/%s", fullName),
		fmt.Sprintf("https://bitbucket.org/%s", fullName),
		fmt.Sprintf("git@github.com:%s.git", fullName),
		fmt.Sprintf("git@gitlab.com:%s.git", fullName),
		fmt.Sprintf("git@bitbucket.org:%s.git", fullName),
		fullName,
		fmt.Sprintf("%s/%s", owner, repo),
	}

	services, err := h.serviceRepo.List(ctx, database.Pagination{Limit: 1000})
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}

	for _, service := range services {
		for _, pattern := range patterns {
			if strings.Contains(strings.ToLower(service.GitURL), strings.ToLower(pattern)) {
				return &service, nil
			}
		}
	}

	return nil, fmt.Errorf("service not found for repository: %s", fullName)
}

// Signature validation

func validateGitHubSignature(payload []byte, signature, secret string) bool {
	if secret == "" {
		return true
	}
	if signature == "" {
		return false
	}

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

// Webhook event structures (local to avoid import cycles)

type githubWebhookPushEvent struct {
	Ref     string `json:"ref"`
	Before  string `json:"before"`
	After   string `json:"after"`
	Deleted bool   `json:"deleted"`
	Pusher  struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"pusher"`
	Repository struct {
		Name     string `json:"name"`
		FullName string `json:"full_name"`
		Owner    struct {
			Login string `json:"login"`
		} `json:"owner"`
	} `json:"repository"`
	Sender struct {
		Login string `json:"login"`
	} `json:"sender"`
}

type githubWebhookPREvent struct {
	Action      string `json:"action"`
	Number      int    `json:"number"`
	PullRequest struct {
		Head struct {
			Ref string `json:"ref"`
			SHA string `json:"sha"`
		} `json:"head"`
	} `json:"pull_request"`
	Repository struct {
		Name     string `json:"name"`
		FullName string `json:"full_name"`
		Owner    struct {
			Login string `json:"login"`
		} `json:"owner"`
	} `json:"repository"`
	Sender struct {
		Login string `json:"login"`
	} `json:"sender"`
}

type githubWebhookCheckSuiteEvent struct {
	Action     string `json:"action"`
	CheckSuite struct {
		HeadBranch string `json:"head_branch"`
		HeadSHA    string `json:"head_sha"`
	} `json:"check_suite"`
	Repository struct {
		Name     string `json:"name"`
		FullName string `json:"full_name"`
		Owner    struct {
			Login string `json:"login"`
		} `json:"owner"`
	} `json:"repository"`
	Sender struct {
		Login string `json:"login"`
	} `json:"sender"`
}

type gitlabWebhookPushEvent struct {
	Ref          string `json:"ref"`
	Before       string `json:"before"`
	After        string `json:"after"`
	UserUsername string `json:"user_username"`
	Project      struct {
		PathWithNamespace string `json:"path_with_namespace"`
	} `json:"project"`
}

type gitlabWebhookMREvent struct {
	ObjectAttributes struct {
		IID          int    `json:"iid"`
		Action       string `json:"action"`
		SourceBranch string `json:"source_branch"`
		LastCommit   struct {
			ID string `json:"id"`
		} `json:"last_commit"`
	} `json:"object_attributes"`
	Project struct {
		PathWithNamespace string `json:"path_with_namespace"`
	} `json:"project"`
	User struct {
		Username string `json:"username"`
	} `json:"user"`
}

type bitbucketWebhookPushEvent struct {
	Push struct {
		Changes []struct {
			New struct {
				Name   string `json:"name"`
				Target struct {
					Hash string `json:"hash"`
				} `json:"target"`
			} `json:"new"`
			Closed bool `json:"closed"`
		} `json:"changes"`
	} `json:"push"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
	Actor struct {
		Username string `json:"username"`
	} `json:"actor"`
}

type bitbucketWebhookPREvent struct {
	PullRequest struct {
		ID     int `json:"id"`
		Source struct {
			Branch struct {
				Name string `json:"name"`
			} `json:"branch"`
			Commit struct {
				Hash string `json:"hash"`
			} `json:"commit"`
		} `json:"source"`
	} `json:"pullrequest"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
	Actor struct {
		Username string `json:"username"`
	} `json:"actor"`
}
