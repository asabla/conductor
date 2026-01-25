//go:build integration

// Package notification provides integration tests for notification services.
package notification

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/conductor/conductor/internal/database"
	"github.com/conductor/conductor/pkg/testutil"
)

// testInfra holds shared infrastructure for notification integration tests.
var testInfra struct {
	db       *database.DB
	postgres *testutil.PostgresContainer
	mailhog  *testutil.MailhogContainer
}

func TestMain(m *testing.M) {
	if !testutil.IsDockerAvailable() {
		os.Exit(0)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Start postgres
	pg, err := testutil.NewPostgresContainer(ctx, testutil.DefaultPostgresConfig())
	if err != nil {
		panic("failed to start postgres: " + err.Error())
	}
	testInfra.postgres = pg

	// Create database connection
	dbCfg := database.DefaultConfig(pg.ConnStr)
	dbCfg.MaxConns = 5
	dbCfg.MinConns = 1
	db, err := database.New(ctx, dbCfg)
	if err != nil {
		pg.Terminate(ctx)
		panic("failed to create database: " + err.Error())
	}
	testInfra.db = db

	// Run migrations
	migrationsFS := os.DirFS("../../migrations")
	migrator, err := database.NewMigratorFromFS(db, migrationsFS)
	if err != nil {
		db.Close()
		pg.Terminate(ctx)
		panic("failed to create migrator: " + err.Error())
	}
	if _, err := migrator.Up(ctx); err != nil {
		db.Close()
		pg.Terminate(ctx)
		panic("failed to run migrations: " + err.Error())
	}

	// Start mailhog for email tests
	mh, err := testutil.NewMailhogContainer(ctx)
	if err != nil {
		db.Close()
		pg.Terminate(ctx)
		panic("failed to start mailhog: " + err.Error())
	}
	testInfra.mailhog = mh

	// Run tests
	code := m.Run()

	// Cleanup
	mh.Terminate(context.Background())
	db.Close()
	pg.Terminate(context.Background())

	os.Exit(code)
}

// ============================================================================
// SLACK CHANNEL TESTS
// ============================================================================

func TestSlackChannel_Send(t *testing.T) {
	if !testutil.IsDockerAvailable() {
		t.Skip("Docker not available")
	}

	t.Run("successful send", func(t *testing.T) {
		// Create mock Slack webhook server
		var receivedPayload map[string]interface{}
		var mu sync.Mutex
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			defer mu.Unlock()

			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &receivedPayload)

			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok"))
		}))
		defer server.Close()

		channel := NewSlackChannel(SlackConfig{
			WebhookURL: server.URL,
			Channel:    "#test-channel",
			Username:   "Conductor Bot",
			IconEmoji:  ":robot_face:",
		}, nil)

		notification := &Notification{
			ID:          uuid.New(),
			Type:        NotificationTypeRunPassed,
			ServiceName: "test-service",
			Title:       "Tests Passed",
			Message:     "All tests passed successfully",
			CreatedAt:   time.Now(),
			Summary: &RunSummary{
				TotalTests:   10,
				PassedTests:  10,
				FailedTests:  0,
				SkippedTests: 0,
				DurationMs:   5000,
				Branch:       "main",
				CommitSHA:    "abc123def456",
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err := channel.Send(ctx, notification)
		require.NoError(t, err)

		mu.Lock()
		defer mu.Unlock()

		// Verify payload structure
		require.NotNil(t, receivedPayload)
		assert.Contains(t, receivedPayload, "attachments")
	})

	t.Run("failed send - server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("internal server error"))
		}))
		defer server.Close()

		channel := NewSlackChannel(SlackConfig{
			WebhookURL: server.URL,
		}, nil)

		notification := &Notification{
			ID:          uuid.New(),
			Type:        NotificationTypeRunFailed,
			ServiceName: "test-service",
			Title:       "Tests Failed",
			Message:     "Some tests failed",
			CreatedAt:   time.Now(),
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err := channel.Send(ctx, notification)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "500")
	})

	t.Run("rate limiting with retry", func(t *testing.T) {
		attempts := 0
		var mu sync.Mutex
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			attempts++
			currentAttempt := attempts
			mu.Unlock()

			if currentAttempt < 3 {
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}

			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok"))
		}))
		defer server.Close()

		channel := NewSlackChannel(SlackConfig{
			WebhookURL: server.URL,
		}, nil)

		notification := &Notification{
			ID:          uuid.New(),
			Type:        NotificationTypeRunPassed,
			ServiceName: "test-service",
			Title:       "Test",
			Message:     "Test message",
			CreatedAt:   time.Now(),
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		err := channel.Send(ctx, notification)
		require.NoError(t, err)

		mu.Lock()
		assert.GreaterOrEqual(t, attempts, 3)
		mu.Unlock()
	})

	t.Run("validate configuration", func(t *testing.T) {
		// Missing webhook URL and token
		channel := NewSlackChannel(SlackConfig{}, nil)
		err := channel.Validate()
		require.Error(t, err)

		// Token without channel
		channel = NewSlackChannel(SlackConfig{
			Token: "xoxb-test-token",
		}, nil)
		err = channel.Validate()
		require.Error(t, err)

		// Token with channel
		channel = NewSlackChannel(SlackConfig{
			Token:   "xoxb-test-token",
			Channel: "#alerts",
		}, nil)
		err = channel.Validate()
		require.NoError(t, err)

		// With webhook URL
		channel = NewSlackChannel(SlackConfig{
			WebhookURL: "https://hooks.slack.com/services/xxx",
		}, nil)
		err = channel.Validate()
		require.NoError(t, err)
	})
}

// ============================================================================
// WEBHOOK CHANNEL TESTS
// ============================================================================

func TestWebhookChannel_Send(t *testing.T) {
	if !testutil.IsDockerAvailable() {
		t.Skip("Docker not available")
	}

	t.Run("successful send with custom headers", func(t *testing.T) {
		var receivedHeaders http.Header
		var receivedPayload map[string]interface{}
		var mu sync.Mutex

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			defer mu.Unlock()

			receivedHeaders = r.Header.Clone()
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &receivedPayload)

			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		channel := NewWebhookChannel(WebhookConfig{
			URL: server.URL,
			Headers: map[string]string{
				"X-Custom-Header": "custom-value",
				"Authorization":   "Bearer test-token",
			},
		}, nil)

		notification := &Notification{
			ID:          uuid.New(),
			Type:        NotificationTypeRunFailed,
			ServiceName: "test-service",
			Title:       "Tests Failed",
			Message:     "3 tests failed",
			CreatedAt:   time.Now(),
			Summary: &RunSummary{
				TotalTests:   10,
				PassedTests:  7,
				FailedTests:  3,
				SkippedTests: 0,
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err := channel.Send(ctx, notification)
		require.NoError(t, err)

		mu.Lock()
		defer mu.Unlock()

		// Verify headers
		assert.Equal(t, "custom-value", receivedHeaders.Get("X-Custom-Header"))
		assert.Equal(t, "Bearer test-token", receivedHeaders.Get("Authorization"))
		assert.Equal(t, "application/json", receivedHeaders.Get("Content-Type"))

		// Verify payload
		assert.Equal(t, string(NotificationTypeRunFailed), receivedPayload["event"])
		assert.Equal(t, "test-service", receivedPayload["serviceName"])
	})

	t.Run("HMAC signature verification", func(t *testing.T) {
		var receivedSignature string
		var mu sync.Mutex
		secret := "my-webhook-secret"

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			defer mu.Unlock()

			receivedSignature = r.Header.Get("X-Conductor-Signature-256")
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		channel := NewWebhookChannel(WebhookConfig{
			URL:    server.URL,
			Secret: secret,
		}, nil)

		notification := &Notification{
			ID:          uuid.New(),
			Type:        NotificationTypeRunPassed,
			ServiceName: "test-service",
			Title:       "Test",
			Message:     "Test",
			CreatedAt:   time.Now(),
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err := channel.Send(ctx, notification)
		require.NoError(t, err)

		mu.Lock()
		defer mu.Unlock()

		// Verify signature was sent
		assert.NotEmpty(t, receivedSignature)
		assert.True(t, len(receivedSignature) > len("sha256="))
	})

	t.Run("client error no retry", func(t *testing.T) {
		attempts := 0
		var mu sync.Mutex

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			attempts++
			mu.Unlock()

			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("bad request"))
		}))
		defer server.Close()

		channel := NewWebhookChannel(WebhookConfig{
			URL: server.URL,
		}, nil)

		notification := &Notification{
			ID:          uuid.New(),
			Type:        NotificationTypeRunPassed,
			ServiceName: "test-service",
			Title:       "Test",
			Message:     "Test",
			CreatedAt:   time.Now(),
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err := channel.Send(ctx, notification)
		require.Error(t, err)

		mu.Lock()
		// Should not retry on 4xx errors (except 429)
		assert.Equal(t, 1, attempts)
		mu.Unlock()
	})
}

// ============================================================================
// TEAMS CHANNEL TESTS
// ============================================================================

func TestTeamsChannel_Send(t *testing.T) {
	if !testutil.IsDockerAvailable() {
		t.Skip("Docker not available")
	}

	t.Run("successful send", func(t *testing.T) {
		var receivedPayload map[string]interface{}
		var mu sync.Mutex

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			defer mu.Unlock()

			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &receivedPayload)

			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		channel := NewTeamsChannel(TeamsConfig{
			WebhookURL: server.URL,
		}, nil)

		notification := &Notification{
			ID:          uuid.New(),
			Type:        NotificationTypeRunRecovered,
			ServiceName: "test-service",
			Title:       "Tests Recovered",
			Message:     "Tests are passing again",
			URL:         "https://conductor.example.com/runs/123",
			CreatedAt:   time.Now(),
			Summary: &RunSummary{
				TotalTests:   20,
				PassedTests:  20,
				FailedTests:  0,
				SkippedTests: 0,
				DurationMs:   12000,
				Branch:       "feature-branch",
				CommitSHA:    "def456789",
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err := channel.Send(ctx, notification)
		require.NoError(t, err)

		mu.Lock()
		defer mu.Unlock()

		// Verify adaptive card structure
		assert.Equal(t, "message", receivedPayload["type"])
		attachments, ok := receivedPayload["attachments"].([]interface{})
		require.True(t, ok)
		require.Len(t, attachments, 1)
	})

	t.Run("validate configuration", func(t *testing.T) {
		// Missing webhook URL
		channel := NewTeamsChannel(TeamsConfig{}, nil)
		err := channel.Validate()
		require.Error(t, err)

		// With webhook URL
		channel = NewTeamsChannel(TeamsConfig{
			WebhookURL: "https://outlook.office.com/webhook/xxx",
		}, nil)
		err = channel.Validate()
		require.NoError(t, err)
	})
}

// ============================================================================
// EMAIL CHANNEL TESTS
// ============================================================================

func TestEmailChannel_Send(t *testing.T) {
	if !testutil.IsDockerAvailable() {
		t.Skip("Docker not available")
	}

	t.Run("successful send via mailhog", func(t *testing.T) {
		smtpPort, _ := strconv.Atoi(testInfra.mailhog.SMTPPort)
		channel, err := NewEmailChannel(EmailConfig{
			SMTPHost:    testInfra.mailhog.SMTPHost,
			SMTPPort:    smtpPort,
			FromAddress: "conductor@example.com",
			FromName:    "Conductor",
			Recipients:  []string{"test@example.com"},
			UseTLS:      false,
		}, nil)
		require.NoError(t, err)

		notification := &Notification{
			ID:          uuid.New(),
			Type:        NotificationTypeRunFailed,
			ServiceName: "test-service",
			Title:       "Tests Failed",
			Message:     "5 tests failed in the latest run",
			URL:         "https://conductor.example.com/runs/456",
			CreatedAt:   time.Now(),
			Summary: &RunSummary{
				TotalTests:   50,
				PassedTests:  45,
				FailedTests:  5,
				SkippedTests: 0,
				DurationMs:   30000,
				Branch:       "main",
				CommitSHA:    "abc123",
				ErrorMessage: "AssertionError: expected 5 but got 3",
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		err = channel.Send(ctx, notification)
		require.NoError(t, err)

		// Verify email was received via mailhog API
		time.Sleep(500 * time.Millisecond) // Give mailhog time to process

		messages, err := testInfra.mailhog.GetMessages(ctx)
		require.NoError(t, err)
		require.NotEmpty(t, messages)

		// Find our message
		var found bool
		for _, msg := range messages {
			if len(msg.Content.Headers.Subject) > 0 &&
				containsString(msg.Content.Headers.Subject[0], "Tests Failed") {
				found = true
				// Verify recipient
				if len(msg.Content.Headers.To) > 0 {
					assert.Contains(t, msg.Content.Headers.To[0], "test@example.com")
				}
				break
			}
		}
		assert.True(t, found, "expected email not found in mailhog")
	})

	t.Run("validate configuration", func(t *testing.T) {
		// Missing required fields
		_, err := NewEmailChannel(EmailConfig{}, nil)
		require.NoError(t, err) // Creation succeeds

		channel, _ := NewEmailChannel(EmailConfig{}, nil)
		err = channel.Validate()
		require.Error(t, err)

		// Valid configuration
		channel, _ = NewEmailChannel(EmailConfig{
			SMTPHost:    "localhost",
			SMTPPort:    25,
			FromAddress: "test@example.com",
			Recipients:  []string{"user@example.com"},
		}, nil)
		err = channel.Validate()
		require.NoError(t, err)
	})
}

// ============================================================================
// RULE ENGINE TESTS
// ============================================================================

func TestRuleEngine_Evaluate(t *testing.T) {
	if !testutil.IsDockerAvailable() {
		t.Skip("Docker not available")
	}

	t.Run("match failure trigger", func(t *testing.T) {
		engine := NewRuleEngine(5 * time.Minute)

		serviceID := uuid.New()
		channelID := uuid.New()

		rules := []database.NotificationRule{
			{
				ID:        uuid.New(),
				ServiceID: &serviceID,
				ChannelID: channelID,
				TriggerOn: []database.TriggerEvent{database.TriggerEventFailure},
				Enabled:   true,
			},
		}

		channels := map[uuid.UUID]*database.NotificationChannel{
			channelID: {
				ID:      channelID,
				Type:    database.ChannelTypeSlack,
				Name:    "Test Slack",
				Enabled: true,
			},
		}

		event := &Event{
			Type:        NotificationTypeRunFailed,
			ServiceID:   serviceID,
			ServiceName: "test-service",
			Timestamp:   time.Now(),
		}

		matches := engine.Evaluate(rules, channels, event)
		assert.Len(t, matches, 1)
		assert.Equal(t, channelID, matches[0].Channel.ID)
	})

	t.Run("no match on different trigger", func(t *testing.T) {
		engine := NewRuleEngine(5 * time.Minute)

		serviceID := uuid.New()
		channelID := uuid.New()

		rules := []database.NotificationRule{
			{
				ID:        uuid.New(),
				ServiceID: &serviceID,
				ChannelID: channelID,
				TriggerOn: []database.TriggerEvent{database.TriggerEventFailure},
				Enabled:   true,
			},
		}

		channels := map[uuid.UUID]*database.NotificationChannel{
			channelID: {
				ID:      channelID,
				Type:    database.ChannelTypeSlack,
				Name:    "Test Slack",
				Enabled: true,
			},
		}

		event := &Event{
			Type:        NotificationTypeRunPassed, // Passed, not failed
			ServiceID:   serviceID,
			ServiceName: "test-service",
			Timestamp:   time.Now(),
		}

		matches := engine.Evaluate(rules, channels, event)
		assert.Empty(t, matches)
	})

	t.Run("throttling prevents duplicate notifications", func(t *testing.T) {
		engine := NewRuleEngine(1 * time.Minute)

		serviceID := uuid.New()
		channelID := uuid.New()
		ruleID := uuid.New()

		rules := []database.NotificationRule{
			{
				ID:        ruleID,
				ServiceID: &serviceID,
				ChannelID: channelID,
				TriggerOn: []database.TriggerEvent{database.TriggerEventFailure},
				Enabled:   true,
			},
		}

		channels := map[uuid.UUID]*database.NotificationChannel{
			channelID: {
				ID:      channelID,
				Type:    database.ChannelTypeSlack,
				Name:    "Test Slack",
				Enabled: true,
			},
		}

		event := &Event{
			Type:        NotificationTypeRunFailed,
			ServiceID:   serviceID,
			ServiceName: "test-service",
			Timestamp:   time.Now(),
		}

		// First evaluation should match
		matches := engine.Evaluate(rules, channels, event)
		assert.Len(t, matches, 1)

		// Mark as sent
		engine.MarkSent(ruleID, event)

		// Second evaluation should be throttled
		matches = engine.Evaluate(rules, channels, event)
		assert.Empty(t, matches)
	})

	t.Run("disabled rule not matched", func(t *testing.T) {
		engine := NewRuleEngine(5 * time.Minute)

		serviceID := uuid.New()
		channelID := uuid.New()

		rules := []database.NotificationRule{
			{
				ID:        uuid.New(),
				ServiceID: &serviceID,
				ChannelID: channelID,
				TriggerOn: []database.TriggerEvent{database.TriggerEventFailure},
				Enabled:   true,
			},
		}

		channels := map[uuid.UUID]*database.NotificationChannel{
			channelID: {
				ID:      channelID,
				Type:    database.ChannelTypeSlack,
				Name:    "Test Slack",
				Enabled: true,
			},
		}

		event := &Event{
			Type:        NotificationTypeRunFailed,
			ServiceID:   serviceID,
			ServiceName: "test-service",
			Timestamp:   time.Now(),
		}

		matches := engine.Evaluate(rules, channels, event)
		assert.Empty(t, matches)
	})

	t.Run("global rule matches all services", func(t *testing.T) {
		engine := NewRuleEngine(5 * time.Minute)

		channelID := uuid.New()

		rules := []database.NotificationRule{
			{
				ID:        uuid.New(),
				ServiceID: nil, // Global rule - no service ID
				ChannelID: channelID,
				TriggerOn: []database.TriggerEvent{database.TriggerEventFailure},
				Enabled:   true,
			},
		}

		channels := map[uuid.UUID]*database.NotificationChannel{
			channelID: {
				ID:      channelID,
				Type:    database.ChannelTypeWebhook,
				Name:    "Test Webhook",
				Enabled: true,
			},
		}

		// Should match any service
		event := &Event{
			Type:        NotificationTypeRunFailed,
			ServiceID:   uuid.New(), // Random service ID
			ServiceName: "any-service",
			Timestamp:   time.Now(),
		}

		matches := engine.Evaluate(rules, channels, event)
		assert.Len(t, matches, 1)
	})
}

// ============================================================================
// NOTIFICATION SERVICE TESTS
// ============================================================================

func TestNotificationService_Integration(t *testing.T) {
	if !testutil.IsDockerAvailable() {
		t.Skip("Docker not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Create a fresh database for this test
	pg, err := testutil.NewPostgresContainer(ctx, testutil.DefaultPostgresConfig())
	require.NoError(t, err)
	defer pg.Terminate(ctx)

	dbCfg := database.DefaultConfig(pg.ConnStr)
	dbCfg.MaxConns = 5
	dbCfg.MinConns = 1
	db, err := database.New(ctx, dbCfg)
	require.NoError(t, err)
	defer db.Close()

	// Run migrations
	migrationsFS := os.DirFS("../../migrations")
	migrator, err := database.NewMigratorFromFS(db, migrationsFS)
	require.NoError(t, err)
	_, err = migrator.Up(ctx)
	require.NoError(t, err)

	repo := database.NewNotificationRepo(db)

	t.Run("service lifecycle", func(t *testing.T) {
		service := NewService(Config{
			WorkerCount:      2,
			QueueSize:        100,
			DefaultTimeout:   10 * time.Second,
			ThrottleDuration: 1 * time.Minute,
			BaseURL:          "https://conductor.example.com",
		}, repo, nil)

		// Start service
		err := service.Start(ctx)
		require.NoError(t, err)

		// Starting again should fail
		err = service.Start(ctx)
		require.Error(t, err)

		// Stop service
		stopCtx, stopCancel := context.WithTimeout(ctx, 5*time.Second)
		defer stopCancel()

		err = service.Stop(stopCtx)
		require.NoError(t, err)

		// Stopping again should be no-op
		err = service.Stop(stopCtx)
		require.NoError(t, err)
	})

	t.Run("test channel", func(t *testing.T) {
		// Create mock webhook server
		var receivedPayload map[string]interface{}
		var mu sync.Mutex
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			defer mu.Unlock()

			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &receivedPayload)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		// Create notification channel in database
		channelConfig := database.WebhookChannelConfig{
			URL: server.URL,
		}
		configJSON, _ := json.Marshal(channelConfig)

		channel := &database.NotificationChannel{
			ID:      uuid.New(),
			Name:    "Test Webhook Channel",
			Type:    database.ChannelTypeWebhook,
			Config:  configJSON,
			Enabled: true,
		}
		err := repo.CreateChannel(ctx, channel)
		require.NoError(t, err)

		// Create service and test channel
		service := NewService(DefaultConfig(), repo, nil)
		err = service.Start(ctx)
		require.NoError(t, err)
		defer service.Stop(ctx)

		result, err := service.TestChannel(ctx, channel.ID, "Test message from integration test")
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.Success)
		assert.Equal(t, database.ChannelTypeWebhook, result.ChannelType)

		// Verify payload was received
		mu.Lock()
		defer mu.Unlock()
		require.NotNil(t, receivedPayload)
		assert.Equal(t, string(NotificationTypeTest), receivedPayload["event"])
	})

	t.Run("process rules and send notifications", func(t *testing.T) {
		// Create mock webhook server that tracks requests
		var requests []map[string]interface{}
		var mu sync.Mutex
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			defer mu.Unlock()

			body, _ := io.ReadAll(r.Body)
			var payload map[string]interface{}
			json.Unmarshal(body, &payload)
			requests = append(requests, payload)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		// Create service
		serviceID := uuid.New()
		owner := "test-owner"
		svc := &database.Service{
			ID:     serviceID,
			Name:   "notification-test-service",
			GitURL: "https://github.com/test/repo",
			Owner:  &owner,
		}
		serviceRepo := database.NewServiceRepo(db)
		err := serviceRepo.Create(ctx, svc)
		require.NoError(t, err)

		// Create notification channel
		channelConfig := database.WebhookChannelConfig{URL: server.URL}
		configJSON, _ := json.Marshal(channelConfig)
		channel := &database.NotificationChannel{
			ID:      uuid.New(),
			Name:    "Rule Test Channel",
			Type:    database.ChannelTypeWebhook,
			Config:  configJSON,
			Enabled: true,
		}
		err = repo.CreateChannel(ctx, channel)
		require.NoError(t, err)

		// Create notification rule
		rule := &database.NotificationRule{
			ID:        uuid.New(),
			ServiceID: &serviceID,
			ChannelID: channel.ID,
			TriggerOn: []database.TriggerEvent{database.TriggerEventFailure},
			Enabled:   true,
		}
		err = repo.CreateRule(ctx, rule)
		require.NoError(t, err)

		// Create and start notification service
		notifService := NewService(Config{
			WorkerCount:      2,
			QueueSize:        100,
			DefaultTimeout:   10 * time.Second,
			ThrottleDuration: 1 * time.Second, // Short throttle for test
			BaseURL:          "https://conductor.example.com",
		}, repo, nil)

		err = notifService.Start(ctx)
		require.NoError(t, err)
		defer notifService.Stop(ctx)

		// Refresh channel into service cache
		err = notifService.RefreshChannel(ctx, channel.ID)
		require.NoError(t, err)

		// Create failure event
		runID := uuid.New()
		event := &Event{
			Type:        NotificationTypeRunFailed,
			ServiceID:   serviceID,
			ServiceName: "notification-test-service",
			RunID:       &runID,
			Run: &database.TestRun{
				ID:           runID,
				ServiceID:    serviceID,
				Status:       database.RunStatusFailed,
				TotalTests:   10,
				PassedTests:  7,
				FailedTests:  3,
				SkippedTests: 0,
			},
			Timestamp: time.Now(),
		}

		// Process rules
		results, err := notifService.ProcessRules(ctx, event)
		require.NoError(t, err)

		// Wait for async processing
		time.Sleep(1 * time.Second)

		mu.Lock()
		defer mu.Unlock()

		// Should have sent notification
		assert.Len(t, results, 1)
		if len(results) > 0 {
			assert.True(t, results[0].Success)
		}

		// Verify webhook received the notification
		assert.NotEmpty(t, requests)
	})
}

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

func TestDetermineNotificationType(t *testing.T) {
	t.Run("passed run", func(t *testing.T) {
		run := &database.TestRun{Status: database.RunStatusPassed}
		notifType := DetermineNotificationType(run, nil)
		assert.Equal(t, NotificationTypeRunPassed, notifType)
	})

	t.Run("failed run", func(t *testing.T) {
		run := &database.TestRun{Status: database.RunStatusFailed}
		notifType := DetermineNotificationType(run, nil)
		assert.Equal(t, NotificationTypeRunFailed, notifType)
	})

	t.Run("recovery after failure", func(t *testing.T) {
		current := &database.TestRun{Status: database.RunStatusPassed}
		previous := &database.TestRun{Status: database.RunStatusFailed}
		notifType := DetermineNotificationType(current, previous)
		assert.Equal(t, NotificationTypeRunRecovered, notifType)
	})

	t.Run("error run", func(t *testing.T) {
		run := &database.TestRun{Status: database.RunStatusError}
		notifType := DetermineNotificationType(run, nil)
		assert.Equal(t, NotificationTypeRunError, notifType)
	})

	t.Run("timeout run", func(t *testing.T) {
		run := &database.TestRun{Status: database.RunStatusTimeout}
		notifType := DetermineNotificationType(run, nil)
		assert.Equal(t, NotificationTypeRunTimeout, notifType)
	})
}

func TestShouldNotifyOnFirstFailure(t *testing.T) {
	t.Run("first failure with no previous", func(t *testing.T) {
		current := &database.TestRun{Status: database.RunStatusFailed}
		assert.True(t, ShouldNotifyOnFirstFailure(current, nil))
	})

	t.Run("first failure after pass", func(t *testing.T) {
		current := &database.TestRun{Status: database.RunStatusFailed}
		previous := &database.TestRun{Status: database.RunStatusPassed}
		assert.True(t, ShouldNotifyOnFirstFailure(current, previous))
	})

	t.Run("consecutive failure", func(t *testing.T) {
		current := &database.TestRun{Status: database.RunStatusFailed}
		previous := &database.TestRun{Status: database.RunStatusFailed}
		assert.False(t, ShouldNotifyOnFirstFailure(current, previous))
	})

	t.Run("passing run", func(t *testing.T) {
		current := &database.TestRun{Status: database.RunStatusPassed}
		assert.False(t, ShouldNotifyOnFirstFailure(current, nil))
	})
}

func TestIsRecovery(t *testing.T) {
	t.Run("recovery from failure", func(t *testing.T) {
		current := &database.TestRun{Status: database.RunStatusPassed}
		previous := &database.TestRun{Status: database.RunStatusFailed}
		assert.True(t, IsRecovery(current, previous))
	})

	t.Run("recovery from error", func(t *testing.T) {
		current := &database.TestRun{Status: database.RunStatusPassed}
		previous := &database.TestRun{Status: database.RunStatusError}
		assert.True(t, IsRecovery(current, previous))
	})

	t.Run("recovery from timeout", func(t *testing.T) {
		current := &database.TestRun{Status: database.RunStatusPassed}
		previous := &database.TestRun{Status: database.RunStatusTimeout}
		assert.True(t, IsRecovery(current, previous))
	})

	t.Run("not recovery - consecutive pass", func(t *testing.T) {
		current := &database.TestRun{Status: database.RunStatusPassed}
		previous := &database.TestRun{Status: database.RunStatusPassed}
		assert.False(t, IsRecovery(current, previous))
	})

	t.Run("not recovery - no previous", func(t *testing.T) {
		current := &database.TestRun{Status: database.RunStatusPassed}
		assert.False(t, IsRecovery(current, nil))
	})
}

// ============================================================================
// TEMPLATE TESTS
// ============================================================================

func TestTemplates(t *testing.T) {
	t.Run("run started template", func(t *testing.T) {
		vars := TemplateVars{
			ServiceName: "my-service",
			Branch:      "feature/new-feature",
			CommitSHA:   "abc123def456789",
		}
		title, message := RunStartedTemplate(vars)
		assert.Contains(t, title, "my-service")
		assert.Contains(t, message, "feature/new-feature")
		assert.Contains(t, message, "abc123d") // Short SHA
	})

	t.Run("run passed template", func(t *testing.T) {
		vars := TemplateVars{
			ServiceName:  "my-service",
			TotalTests:   100,
			PassedTests:  100,
			SkippedTests: 5,
			DurationMs:   5000,
		}
		title, message := RunPassedTemplate(vars)
		assert.Contains(t, title, "Passed")
		assert.Contains(t, message, "100/100")
		assert.Contains(t, message, "5 tests") // Skipped
	})

	t.Run("run failed template", func(t *testing.T) {
		vars := TemplateVars{
			ServiceName:  "my-service",
			TotalTests:   100,
			PassedTests:  90,
			FailedTests:  10,
			SkippedTests: 0,
			DurationMs:   10000,
			ErrorMessage: "AssertionError: expected true but got false",
		}
		title, message := RunFailedTemplate(vars)
		assert.Contains(t, title, "Failed")
		assert.Contains(t, message, "10 failed")
		assert.Contains(t, message, "AssertionError")
	})

	t.Run("run recovered template", func(t *testing.T) {
		vars := TemplateVars{
			ServiceName: "my-service",
			TotalTests:  50,
			PassedTests: 50,
			Branch:      "main",
		}
		title, message := RunRecoveredTemplate(vars)
		assert.Contains(t, title, "Recovered")
		assert.Contains(t, message, "passing again")
	})
}

// Helper to check if a string contains a substring
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsString(s[1:], substr) || s[:len(substr)] == substr)
}
