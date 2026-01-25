package notification

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/conductor/conductor/internal/database"
)

// WebhookChannel implements the Channel interface for generic webhooks.
type WebhookChannel struct {
	url     string
	headers map[string]string
	secret  string
	timeout time.Duration
	client  *http.Client
	logger  *slog.Logger
}

// WebhookConfig contains configuration for a webhook channel.
type WebhookConfig struct {
	URL            string
	Headers        map[string]string
	Secret         string
	TimeoutSeconds int
}

// NewWebhookChannel creates a new webhook notification channel.
func NewWebhookChannel(cfg WebhookConfig, logger *slog.Logger) *WebhookChannel {
	if logger == nil {
		logger = slog.Default()
	}

	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	return &WebhookChannel{
		url:     cfg.URL,
		headers: cfg.Headers,
		secret:  cfg.Secret,
		timeout: timeout,
		client: &http.Client{
			Timeout: timeout,
		},
		logger: logger.With("channel", "webhook"),
	}
}

// Type returns the channel type.
func (c *WebhookChannel) Type() database.ChannelType {
	return database.ChannelTypeWebhook
}

// Validate validates the webhook configuration.
func (c *WebhookChannel) Validate() error {
	if c.url == "" {
		return fmt.Errorf("webhook URL is required")
	}
	return nil
}

// Send sends a notification via webhook.
func (c *WebhookChannel) Send(ctx context.Context, notification *Notification) error {
	payload := c.formatPayload(notification)

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(jsonPayload))
	if err != nil {
		return fmt.Errorf("failed to create webhook request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Conductor/1.0")

	// Add custom headers
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}

	// Sign payload with HMAC if secret is provided
	if c.secret != "" {
		signature := c.signPayload(jsonPayload)
		req.Header.Set("X-Conductor-Signature", signature)
		req.Header.Set("X-Conductor-Signature-256", "sha256="+signature)
	}

	// Add timestamp header
	timestamp := time.Now().Unix()
	req.Header.Set("X-Conductor-Timestamp", fmt.Sprintf("%d", timestamp))

	// Send with retry
	var lastErr error
	maxRetries := 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 1s, 2s, 4s
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}

		resp, err := c.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("webhook request failed: %w", err)
			c.logger.Warn("webhook request failed, retrying",
				"attempt", attempt+1,
				"error", err,
			)
			continue
		}

		defer resp.Body.Close()

		// Read response body for error reporting
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			c.logger.Debug("webhook notification sent",
				"status", resp.StatusCode,
				"notification_type", notification.Type,
			)
			return nil
		}

		lastErr = fmt.Errorf("webhook returned status %d: %s", resp.StatusCode, string(body))

		// Don't retry on client errors (4xx) except 429 (rate limit)
		if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != 429 {
			return lastErr
		}

		c.logger.Warn("webhook returned error, retrying",
			"attempt", attempt+1,
			"status", resp.StatusCode,
		)
	}

	return lastErr
}

// signPayload creates an HMAC-SHA256 signature of the payload.
func (c *WebhookChannel) signPayload(payload []byte) string {
	mac := hmac.New(sha256.New, []byte(c.secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// formatPayload formats the notification as a webhook payload.
func (c *WebhookChannel) formatPayload(notification *Notification) map[string]interface{} {
	payload := map[string]interface{}{
		"event":       string(notification.Type),
		"id":          notification.ID.String(),
		"timestamp":   notification.CreatedAt.UTC().Format(time.RFC3339),
		"title":       notification.Title,
		"message":     notification.Message,
		"url":         notification.URL,
		"serviceName": notification.ServiceName,
	}

	if notification.ServiceID != nil {
		payload["serviceId"] = notification.ServiceID.String()
	}

	if notification.RunID != nil {
		payload["runId"] = notification.RunID.String()
	}

	if notification.Summary != nil {
		payload["summary"] = map[string]interface{}{
			"total":        notification.Summary.TotalTests,
			"passed":       notification.Summary.PassedTests,
			"failed":       notification.Summary.FailedTests,
			"skipped":      notification.Summary.SkippedTests,
			"durationMs":   notification.Summary.DurationMs,
			"branch":       notification.Summary.Branch,
			"commitSha":    notification.Summary.CommitSHA,
			"errorMessage": notification.Summary.ErrorMessage,
		}
	}

	if len(notification.Metadata) > 0 {
		payload["metadata"] = notification.Metadata
	}

	return payload
}

// WebhookPayload represents the JSON payload sent to webhooks.
type WebhookPayload struct {
	Event       string                 `json:"event"`
	ID          string                 `json:"id"`
	Timestamp   string                 `json:"timestamp"`
	Title       string                 `json:"title"`
	Message     string                 `json:"message"`
	URL         string                 `json:"url,omitempty"`
	ServiceID   string                 `json:"serviceId,omitempty"`
	ServiceName string                 `json:"serviceName,omitempty"`
	RunID       string                 `json:"runId,omitempty"`
	Summary     *WebhookSummary        `json:"summary,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// WebhookSummary represents the test run summary in a webhook payload.
type WebhookSummary struct {
	Total        int    `json:"total"`
	Passed       int    `json:"passed"`
	Failed       int    `json:"failed"`
	Skipped      int    `json:"skipped"`
	DurationMs   int64  `json:"durationMs"`
	Branch       string `json:"branch,omitempty"`
	CommitSHA    string `json:"commitSha,omitempty"`
	ErrorMessage string `json:"errorMessage,omitempty"`
}
