package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/conductor/conductor/internal/database"
)

// SlackChannel implements the Channel interface for Slack notifications.
type SlackChannel struct {
	webhookURL string
	channel    string
	username   string
	iconEmoji  string
	token      string // Optional Bot token for API calls
	apiBaseURL string
	client     *http.Client
	logger     *slog.Logger
}

// SlackConfig contains configuration for a Slack channel.
type SlackConfig struct {
	WebhookURL string
	Channel    string
	Username   string
	IconEmoji  string
	Token      string
	APIBaseURL string
}

const defaultSlackAPIBaseURL = "https://slack.com/api"

// NewSlackChannel creates a new Slack notification channel.
func NewSlackChannel(cfg SlackConfig, logger *slog.Logger) *SlackChannel {
	if logger == nil {
		logger = slog.Default()
	}

	apiBaseURL := cfg.APIBaseURL
	if apiBaseURL == "" {
		apiBaseURL = defaultSlackAPIBaseURL
	}

	return &SlackChannel{
		webhookURL: cfg.WebhookURL,
		channel:    cfg.Channel,
		username:   cfg.Username,
		iconEmoji:  cfg.IconEmoji,
		token:      cfg.Token,
		apiBaseURL: apiBaseURL,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger.With("channel", "slack"),
	}
}

// Type returns the channel type.
func (c *SlackChannel) Type() database.ChannelType {
	return database.ChannelTypeSlack
}

// Validate validates the Slack configuration.
func (c *SlackChannel) Validate() error {
	if c.webhookURL == "" && c.token == "" {
		return fmt.Errorf("either webhook URL or token is required")
	}
	if c.webhookURL == "" && c.token != "" && c.channel == "" {
		return fmt.Errorf("channel is required when using a Slack token")
	}
	return nil
}

// Send sends a notification to Slack.
func (c *SlackChannel) Send(ctx context.Context, notification *Notification) error {
	if c.webhookURL != "" {
		return c.sendWebhook(ctx, notification)
	}

	return c.sendAPI(ctx, notification)
}

func (c *SlackChannel) sendWebhook(ctx context.Context, notification *Notification) error {
	payload := c.formatMessage(notification)

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal Slack payload: %w", err)
	}

	// Send with retry
	var lastErr error
	maxRetries := 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.webhookURL, bytes.NewReader(jsonPayload))
		if err != nil {
			return fmt.Errorf("failed to create Slack request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")

		resp, err := c.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("Slack request failed: %w", err)
			c.logger.Warn("Slack request failed, retrying",
				"attempt", attempt+1,
				"error", err,
			)
			continue
		}

		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			// Slack webhooks return "ok" on success
			if string(body) == "ok" || resp.StatusCode == http.StatusOK {
				c.logger.Debug("Slack notification sent",
					"notification_type", notification.Type,
				)
				return nil
			}
		}

		lastErr = fmt.Errorf("Slack returned status %d: %s", resp.StatusCode, string(body))

		// Don't retry on client errors except rate limits
		if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != 429 {
			return lastErr
		}

		// Check for rate limiting
		if resp.StatusCode == 429 {
			retryAfter := resp.Header.Get("Retry-After")
			c.logger.Warn("Slack rate limited",
				"retry_after", retryAfter,
			)
		}
	}

	return lastErr
}

func (c *SlackChannel) sendAPI(ctx context.Context, notification *Notification) error {
	if c.token == "" {
		return fmt.Errorf("slack token is required")
	}
	if c.channel == "" {
		return fmt.Errorf("slack channel is required")
	}

	payload := c.formatMessage(notification)
	payload["channel"] = c.channel
	if _, ok := payload["text"]; !ok {
		payload["text"] = fmt.Sprintf("%s\n%s", notification.Title, notification.Message)
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal Slack payload: %w", err)
	}

	// Send with retry
	var lastErr error
	maxRetries := 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiBaseURL+"/chat.postMessage", bytes.NewReader(jsonPayload))
		if err != nil {
			return fmt.Errorf("failed to create Slack API request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("Content-Type", "application/json; charset=utf-8")

		resp, err := c.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("Slack API request failed: %w", err)
			c.logger.Warn("Slack API request failed, retrying",
				"attempt", attempt+1,
				"error", err,
			)
			continue
		}

		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			var apiResp struct {
				OK    bool   `json:"ok"`
				Error string `json:"error"`
			}
			if err := json.Unmarshal(body, &apiResp); err == nil {
				if apiResp.OK {
					c.logger.Debug("Slack API notification sent",
						"notification_type", notification.Type,
					)
					return nil
				}
				lastErr = fmt.Errorf("Slack API error: %s", apiResp.Error)
			} else {
				return nil
			}
		} else {
			lastErr = fmt.Errorf("Slack API returned status %d: %s", resp.StatusCode, string(body))
		}

		if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != 429 {
			return lastErr
		}

		if resp.StatusCode == 429 {
			retryAfter := resp.Header.Get("Retry-After")
			c.logger.Warn("Slack API rate limited",
				"retry_after", retryAfter,
			)
		}
	}

	return lastErr
}

// formatMessage formats the notification as Slack blocks.
func (c *SlackChannel) formatMessage(notification *Notification) map[string]interface{} {
	color := c.getColor(notification.Type)

	// Build the main attachment
	attachment := map[string]interface{}{
		"color": color,
		"blocks": []map[string]interface{}{
			{
				"type": "header",
				"text": map[string]interface{}{
					"type":  "plain_text",
					"text":  notification.Title,
					"emoji": true,
				},
			},
			{
				"type": "section",
				"text": map[string]interface{}{
					"type": "mrkdwn",
					"text": notification.Message,
				},
			},
		},
	}

	blocks := attachment["blocks"].([]map[string]interface{})

	// Add summary fields if available
	if notification.Summary != nil {
		fields := []map[string]interface{}{}

		// Test results
		fields = append(fields, map[string]interface{}{
			"type": "mrkdwn",
			"text": fmt.Sprintf("*Tests:* %d total, %d passed, %d failed, %d skipped",
				notification.Summary.TotalTests,
				notification.Summary.PassedTests,
				notification.Summary.FailedTests,
				notification.Summary.SkippedTests),
		})

		// Duration
		if notification.Summary.DurationMs > 0 {
			duration := time.Duration(notification.Summary.DurationMs) * time.Millisecond
			fields = append(fields, map[string]interface{}{
				"type": "mrkdwn",
				"text": fmt.Sprintf("*Duration:* %s", duration.Round(time.Second)),
			})
		}

		// Branch
		if notification.Summary.Branch != "" {
			fields = append(fields, map[string]interface{}{
				"type": "mrkdwn",
				"text": fmt.Sprintf("*Branch:* `%s`", notification.Summary.Branch),
			})
		}

		// Commit
		if notification.Summary.CommitSHA != "" {
			shortSHA := notification.Summary.CommitSHA
			if len(shortSHA) > 7 {
				shortSHA = shortSHA[:7]
			}
			fields = append(fields, map[string]interface{}{
				"type": "mrkdwn",
				"text": fmt.Sprintf("*Commit:* `%s`", shortSHA),
			})
		}

		if len(fields) > 0 {
			blocks = append(blocks, map[string]interface{}{
				"type":   "section",
				"fields": fields,
			})
		}

		// Error message if present
		if notification.Summary.ErrorMessage != "" {
			blocks = append(blocks, map[string]interface{}{
				"type": "section",
				"text": map[string]interface{}{
					"type": "mrkdwn",
					"text": fmt.Sprintf("*Error:*\n```%s```", truncate(notification.Summary.ErrorMessage, 500)),
				},
			})
		}
	}

	// Add action button if URL is provided
	if notification.URL != "" {
		blocks = append(blocks, map[string]interface{}{
			"type": "actions",
			"elements": []map[string]interface{}{
				{
					"type": "button",
					"text": map[string]interface{}{
						"type":  "plain_text",
						"text":  "View Details",
						"emoji": true,
					},
					"url": notification.URL,
				},
			},
		})
	}

	// Add divider and context
	blocks = append(blocks, map[string]interface{}{
		"type": "divider",
	})

	contextElements := []map[string]interface{}{
		{
			"type": "mrkdwn",
			"text": fmt.Sprintf("*Service:* %s", notification.ServiceName),
		},
		{
			"type": "mrkdwn",
			"text": fmt.Sprintf("*Time:* <!date^%d^{date_short_pretty} at {time}|%s>",
				notification.CreatedAt.Unix(),
				notification.CreatedAt.Format(time.RFC3339)),
		},
	}

	blocks = append(blocks, map[string]interface{}{
		"type":     "context",
		"elements": contextElements,
	})

	attachment["blocks"] = blocks

	payload := map[string]interface{}{
		"attachments": []map[string]interface{}{attachment},
	}

	if c.channel != "" {
		payload["channel"] = c.channel
	}
	if c.username != "" {
		payload["username"] = c.username
	}
	if c.iconEmoji != "" {
		payload["icon_emoji"] = c.iconEmoji
	}

	return payload
}

// getColor returns the appropriate color for the notification type.
func (c *SlackChannel) getColor(notificationType NotificationType) string {
	switch notificationType {
	case NotificationTypeRunPassed, NotificationTypeRunRecovered:
		return "#36a64f" // Green
	case NotificationTypeRunFailed, NotificationTypeRunError, NotificationTypeRunTimeout:
		return "#dc3545" // Red
	case NotificationTypeFlakyDetected:
		return "#ffc107" // Yellow/Warning
	case NotificationTypeRunStarted:
		return "#17a2b8" // Blue/Info
	case NotificationTypeAgentOffline:
		return "#dc3545" // Red
	case NotificationTypeAgentOnline:
		return "#36a64f" // Green
	default:
		return "#6c757d" // Gray
	}
}

// truncate truncates a string to the specified length.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
