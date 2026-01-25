package server

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/conductor/conductor/internal/database"
)

// mockServiceRepo implements WebhookServiceRepository for testing.
type mockServiceRepo struct {
	services []database.Service
	err      error
}

func (m *mockServiceRepo) List(ctx context.Context, page database.Pagination) ([]database.Service, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.services, nil
}

// mockScheduler implements RunScheduler for testing.
type mockScheduler struct {
	runs     []*database.TestRun
	err      error
	requests []ScheduleRunRequest
}

func (m *mockScheduler) ScheduleRun(ctx context.Context, req ScheduleRunRequest) (*database.TestRun, error) {
	m.requests = append(m.requests, req)
	if m.err != nil {
		return nil, m.err
	}
	if len(m.runs) > 0 {
		return m.runs[0], nil
	}
	run := &database.TestRun{
		ID:     uuid.New(),
		Status: database.RunStatusPending,
	}
	return run, nil
}

func TestValidateGitHubSignature(t *testing.T) {
	secret := "test-secret"
	payload := []byte(`{"ref": "refs/heads/main"}`)

	// Generate valid signature
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	validSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	tests := []struct {
		name      string
		payload   []byte
		signature string
		secret    string
		want      bool
	}{
		{
			name:      "valid signature",
			payload:   payload,
			signature: validSig,
			secret:    secret,
			want:      true,
		},
		{
			name:      "invalid signature",
			payload:   payload,
			signature: "sha256=invalid",
			secret:    secret,
			want:      false,
		},
		{
			name:      "empty signature with secret",
			payload:   payload,
			signature: "",
			secret:    secret,
			want:      false,
		},
		{
			name:      "no secret configured - allow all",
			payload:   payload,
			signature: "",
			secret:    "",
			want:      true,
		},
		{
			name:      "missing sha256 prefix",
			payload:   payload,
			signature: hex.EncodeToString(mac.Sum(nil)),
			secret:    secret,
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validateGitHubSignature(tt.payload, tt.signature, tt.secret)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestHandleGitHubWebhook_Push(t *testing.T) {
	logger := zerolog.Nop()
	serviceID := uuid.New()

	serviceRepo := &mockServiceRepo{
		services: []database.Service{
			{
				ID:     serviceID,
				Name:   "test-service",
				GitURL: "https://github.com/owner/repo",
			},
		},
	}

	scheduler := &mockScheduler{}

	handler := NewWebhookHandler(
		WebhookConfig{GithubSecret: "test-secret"},
		serviceRepo,
		scheduler,
		logger,
	)

	payload := map[string]interface{}{
		"ref":     "refs/heads/main",
		"before":  "0000000000000000000000000000000000000000",
		"after":   "abc123def456",
		"deleted": false,
		"pusher": map[string]string{
			"name":  "test-user",
			"email": "test@example.com",
		},
		"repository": map[string]interface{}{
			"name":      "repo",
			"full_name": "owner/repo",
			"owner": map[string]string{
				"login": "owner",
			},
		},
		"sender": map[string]string{
			"login": "test-user",
		},
	}
	payloadBytes, _ := json.Marshal(payload)

	// Generate valid signature
	mac := hmac.New(sha256.New, []byte("test-secret"))
	mac.Write(payloadBytes)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(payloadBytes))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-GitHub-Delivery", "test-delivery-id")
	req.Header.Set("X-Hub-Signature-256", signature)

	rr := httptest.NewRecorder()
	handler.HandleGitHubWebhook(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	require.Len(t, scheduler.requests, 1)
	assert.Equal(t, serviceID, scheduler.requests[0].ServiceID)
	assert.Equal(t, "main", scheduler.requests[0].GitRef)
	assert.Equal(t, "abc123def456", scheduler.requests[0].GitSHA)
	assert.Equal(t, "test-user", scheduler.requests[0].TriggeredBy)
}

func TestHandleGitHubWebhook_InvalidSignature(t *testing.T) {
	logger := zerolog.Nop()

	handler := NewWebhookHandler(
		WebhookConfig{GithubSecret: "test-secret"},
		&mockServiceRepo{},
		&mockScheduler{},
		logger,
	)

	payload := []byte(`{"ref": "refs/heads/main"}`)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(payload))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-Hub-Signature-256", "sha256=invalid")

	rr := httptest.NewRecorder()
	handler.HandleGitHubWebhook(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestHandleGitHubWebhook_PullRequest(t *testing.T) {
	logger := zerolog.Nop()
	serviceID := uuid.New()

	serviceRepo := &mockServiceRepo{
		services: []database.Service{
			{
				ID:     serviceID,
				Name:   "test-service",
				GitURL: "https://github.com/owner/repo",
			},
		},
	}

	scheduler := &mockScheduler{}

	handler := NewWebhookHandler(
		WebhookConfig{GithubSecret: ""},
		serviceRepo,
		scheduler,
		logger,
	)

	payload := map[string]interface{}{
		"action": "opened",
		"number": 42,
		"pull_request": map[string]interface{}{
			"head": map[string]string{
				"ref": "feature-branch",
				"sha": "pr-sha-123",
			},
		},
		"repository": map[string]interface{}{
			"name":      "repo",
			"full_name": "owner/repo",
			"owner": map[string]string{
				"login": "owner",
			},
		},
		"sender": map[string]string{
			"login": "pr-author",
		},
	}
	payloadBytes, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(payloadBytes))
	req.Header.Set("X-GitHub-Event", "pull_request")
	req.Header.Set("X-GitHub-Delivery", "test-delivery-id")

	rr := httptest.NewRecorder()
	handler.HandleGitHubWebhook(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	require.Len(t, scheduler.requests, 1)
	assert.Equal(t, serviceID, scheduler.requests[0].ServiceID)
	assert.Equal(t, "feature-branch", scheduler.requests[0].GitRef)
	assert.Equal(t, "pr-sha-123", scheduler.requests[0].GitSHA)
	assert.Equal(t, "pr-author", scheduler.requests[0].TriggeredBy)
	assert.Equal(t, 1, scheduler.requests[0].Priority) // PRs get higher priority
}

func TestHandleGitHubWebhook_Ping(t *testing.T) {
	logger := zerolog.Nop()

	handler := NewWebhookHandler(
		WebhookConfig{GithubSecret: ""},
		&mockServiceRepo{},
		&mockScheduler{},
		logger,
	)

	payload := []byte(`{"zen": "test"}`)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(payload))
	req.Header.Set("X-GitHub-Event", "ping")

	rr := httptest.NewRecorder()
	handler.HandleGitHubWebhook(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestHandleGitHubWebhook_IgnoreBranchDeletion(t *testing.T) {
	logger := zerolog.Nop()

	scheduler := &mockScheduler{}

	handler := NewWebhookHandler(
		WebhookConfig{GithubSecret: ""},
		&mockServiceRepo{},
		scheduler,
		logger,
	)

	payload := map[string]interface{}{
		"ref":     "refs/heads/deleted-branch",
		"deleted": true,
		"pusher": map[string]string{
			"name": "test-user",
		},
		"repository": map[string]interface{}{
			"name":      "repo",
			"full_name": "owner/repo",
			"owner": map[string]string{
				"login": "owner",
			},
		},
	}
	payloadBytes, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(payloadBytes))
	req.Header.Set("X-GitHub-Event", "push")

	rr := httptest.NewRecorder()
	handler.HandleGitHubWebhook(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Empty(t, scheduler.requests) // No run scheduled for deletion
}

func TestHandleGitLabWebhook_Push(t *testing.T) {
	logger := zerolog.Nop()
	serviceID := uuid.New()

	serviceRepo := &mockServiceRepo{
		services: []database.Service{
			{
				ID:     serviceID,
				Name:   "test-service",
				GitURL: "https://gitlab.com/owner/repo",
			},
		},
	}

	scheduler := &mockScheduler{}

	handler := NewWebhookHandler(
		WebhookConfig{GitlabSecret: "gitlab-token"},
		serviceRepo,
		scheduler,
		logger,
	)

	payload := map[string]interface{}{
		"ref":           "refs/heads/main",
		"before":        "0000000000000000000000000000000000000000",
		"after":         "gitlab-sha-123",
		"user_username": "gitlab-user",
		"project": map[string]string{
			"path_with_namespace": "owner/repo",
		},
	}
	payloadBytes, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/gitlab", bytes.NewReader(payloadBytes))
	req.Header.Set("X-Gitlab-Event", "Push Hook")
	req.Header.Set("X-Gitlab-Token", "gitlab-token")

	rr := httptest.NewRecorder()
	handler.HandleGitLabWebhook(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	require.Len(t, scheduler.requests, 1)
	assert.Equal(t, serviceID, scheduler.requests[0].ServiceID)
	assert.Equal(t, "main", scheduler.requests[0].GitRef)
	assert.Equal(t, "gitlab-sha-123", scheduler.requests[0].GitSHA)
}

func TestHandleGitLabWebhook_InvalidToken(t *testing.T) {
	logger := zerolog.Nop()

	handler := NewWebhookHandler(
		WebhookConfig{GitlabSecret: "correct-token"},
		&mockServiceRepo{},
		&mockScheduler{},
		logger,
	)

	payload := []byte(`{"ref": "refs/heads/main"}`)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/gitlab", bytes.NewReader(payload))
	req.Header.Set("X-Gitlab-Event", "Push Hook")
	req.Header.Set("X-Gitlab-Token", "wrong-token")

	rr := httptest.NewRecorder()
	handler.HandleGitLabWebhook(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestHandleBitbucketWebhook_Push(t *testing.T) {
	logger := zerolog.Nop()
	serviceID := uuid.New()

	serviceRepo := &mockServiceRepo{
		services: []database.Service{
			{
				ID:     serviceID,
				Name:   "test-service",
				GitURL: "https://bitbucket.org/owner/repo",
			},
		},
	}

	scheduler := &mockScheduler{}

	handler := NewWebhookHandler(
		WebhookConfig{BitbucketSecret: "bb-secret"},
		serviceRepo,
		scheduler,
		logger,
	)

	payload := map[string]interface{}{
		"push": map[string]interface{}{
			"changes": []map[string]interface{}{
				{
					"new": map[string]interface{}{
						"name": "main",
						"target": map[string]string{
							"hash": "bb-sha-456",
						},
					},
					"closed": false,
				},
			},
		},
		"repository": map[string]string{
			"full_name": "owner/repo",
		},
		"actor": map[string]string{
			"username": "bb-user",
		},
	}
	payloadBytes, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/bitbucket", bytes.NewReader(payloadBytes))
	req.Header.Set("X-Event-Key", "repo:push")
	req.Header.Set("Authorization", "Bearer bb-secret")

	rr := httptest.NewRecorder()
	handler.HandleBitbucketWebhook(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	require.Len(t, scheduler.requests, 1)
	assert.Equal(t, serviceID, scheduler.requests[0].ServiceID)
	assert.Equal(t, "main", scheduler.requests[0].GitRef)
	assert.Equal(t, "bb-sha-456", scheduler.requests[0].GitSHA)
}

func TestHandleBitbucketWebhook_InvalidAuth(t *testing.T) {
	logger := zerolog.Nop()

	handler := NewWebhookHandler(
		WebhookConfig{BitbucketSecret: "correct-secret"},
		&mockServiceRepo{},
		&mockScheduler{},
		logger,
	)

	payload := []byte(`{}`)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/bitbucket", bytes.NewReader(payload))
	req.Header.Set("X-Event-Key", "repo:push")
	req.Header.Set("Authorization", "Bearer wrong-secret")

	rr := httptest.NewRecorder()
	handler.HandleBitbucketWebhook(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestFindServiceByRepo(t *testing.T) {
	logger := zerolog.Nop()
	serviceID := uuid.New()

	tests := []struct {
		name      string
		services  []database.Service
		owner     string
		repo      string
		fullName  string
		wantFound bool
		wantSvcID uuid.UUID
	}{
		{
			name: "finds by full github URL",
			services: []database.Service{
				{ID: serviceID, Name: "svc", GitURL: "https://github.com/owner/repo"},
			},
			owner:     "owner",
			repo:      "repo",
			fullName:  "owner/repo",
			wantFound: true,
			wantSvcID: serviceID,
		},
		{
			name: "finds by ssh URL",
			services: []database.Service{
				{ID: serviceID, Name: "svc", GitURL: "git@github.com:owner/repo.git"},
			},
			owner:     "owner",
			repo:      "repo",
			fullName:  "owner/repo",
			wantFound: true,
			wantSvcID: serviceID,
		},
		{
			name: "finds case insensitive",
			services: []database.Service{
				{ID: serviceID, Name: "svc", GitURL: "https://github.com/Owner/Repo"},
			},
			owner:     "owner",
			repo:      "repo",
			fullName:  "owner/repo",
			wantFound: true,
			wantSvcID: serviceID,
		},
		{
			name: "not found",
			services: []database.Service{
				{ID: serviceID, Name: "svc", GitURL: "https://github.com/other/other"},
			},
			owner:     "owner",
			repo:      "repo",
			fullName:  "owner/repo",
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewWebhookHandler(
				WebhookConfig{},
				&mockServiceRepo{services: tt.services},
				&mockScheduler{},
				logger,
			)

			svc, err := handler.findServiceByRepo(context.Background(), tt.owner, tt.repo, tt.fullName)
			if tt.wantFound {
				require.NoError(t, err)
				assert.Equal(t, tt.wantSvcID, svc.ID)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestRegisterRoutes(t *testing.T) {
	logger := zerolog.Nop()

	handler := NewWebhookHandler(
		WebhookConfig{},
		&mockServiceRepo{},
		&mockScheduler{},
		logger,
	)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Verify routes are registered by making requests
	routes := []string{
		"/api/v1/webhooks/github",
		"/api/v1/webhooks/gitlab",
		"/api/v1/webhooks/bitbucket",
		"/webhooks/github",
		"/webhooks/gitlab",
		"/webhooks/bitbucket",
	}

	for _, route := range routes {
		t.Run(route, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, route, bytes.NewReader([]byte(`{}`)))
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)
			// Should get 200 OK (events are ignored if not recognized)
			// or 400/401 for validation issues - but NOT 404
			assert.NotEqual(t, http.StatusNotFound, rr.Code, "route %s should be registered", route)
		})
	}
}
