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

// TeamsChannel implements the Channel interface for Microsoft Teams notifications.
type TeamsChannel struct {
	webhookURL string
	client     *http.Client
	logger     *slog.Logger
}

// TeamsConfig contains configuration for a Teams channel.
type TeamsConfig struct {
	WebhookURL string
}

// NewTeamsChannel creates a new Teams notification channel.
func NewTeamsChannel(cfg TeamsConfig, logger *slog.Logger) *TeamsChannel {
	if logger == nil {
		logger = slog.Default()
	}

	return &TeamsChannel{
		webhookURL: cfg.WebhookURL,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger.With("channel", "teams"),
	}
}

// Type returns the channel type.
func (c *TeamsChannel) Type() database.ChannelType {
	return database.ChannelTypeTeams
}

// Validate validates the Teams configuration.
func (c *TeamsChannel) Validate() error {
	if c.webhookURL == "" {
		return fmt.Errorf("Teams webhook URL is required")
	}
	return nil
}

// Send sends a notification to Microsoft Teams.
func (c *TeamsChannel) Send(ctx context.Context, notification *Notification) error {
	payload := c.formatCard(notification)

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal Teams payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.webhookURL, bytes.NewReader(jsonPayload))
	if err != nil {
		return fmt.Errorf("failed to create Teams request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

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

		resp, err := c.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("Teams request failed: %w", err)
			c.logger.Warn("Teams request failed, retrying",
				"attempt", attempt+1,
				"error", err,
			)
			continue
		}

		defer resp.Body.Close()

		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

		if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusAccepted {
			c.logger.Debug("Teams notification sent",
				"notification_type", notification.Type,
			)
			return nil
		}

		lastErr = fmt.Errorf("Teams returned status %d: %s", resp.StatusCode, string(body))

		// Don't retry on client errors except rate limits
		if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != 429 {
			return lastErr
		}
	}

	return lastErr
}

// formatCard formats the notification as a Teams Adaptive Card.
func (c *TeamsChannel) formatCard(notification *Notification) map[string]interface{} {
	themeColor := c.getThemeColor(notification.Type)

	// Build card body
	body := []map[string]interface{}{
		{
			"type":   "TextBlock",
			"text":   notification.Title,
			"size":   "Large",
			"weight": "Bolder",
			"wrap":   true,
		},
		{
			"type":    "TextBlock",
			"text":    notification.Message,
			"wrap":    true,
			"spacing": "Medium",
		},
	}

	// Add summary facts if available
	if notification.Summary != nil {
		facts := []map[string]interface{}{
			{
				"title": "Total Tests",
				"value": fmt.Sprintf("%d", notification.Summary.TotalTests),
			},
			{
				"title": "Passed",
				"value": fmt.Sprintf("%d", notification.Summary.PassedTests),
			},
			{
				"title": "Failed",
				"value": fmt.Sprintf("%d", notification.Summary.FailedTests),
			},
		}

		if notification.Summary.SkippedTests > 0 {
			facts = append(facts, map[string]interface{}{
				"title": "Skipped",
				"value": fmt.Sprintf("%d", notification.Summary.SkippedTests),
			})
		}

		if notification.Summary.DurationMs > 0 {
			duration := time.Duration(notification.Summary.DurationMs) * time.Millisecond
			facts = append(facts, map[string]interface{}{
				"title": "Duration",
				"value": duration.Round(time.Second).String(),
			})
		}

		if notification.Summary.Branch != "" {
			facts = append(facts, map[string]interface{}{
				"title": "Branch",
				"value": notification.Summary.Branch,
			})
		}

		if notification.Summary.CommitSHA != "" {
			shortSHA := notification.Summary.CommitSHA
			if len(shortSHA) > 7 {
				shortSHA = shortSHA[:7]
			}
			facts = append(facts, map[string]interface{}{
				"title": "Commit",
				"value": shortSHA,
			})
		}

		body = append(body, map[string]interface{}{
			"type":  "FactSet",
			"facts": facts,
		})

		// Add error message if present
		if notification.Summary.ErrorMessage != "" {
			body = append(body, map[string]interface{}{
				"type":    "TextBlock",
				"text":    "Error Details:",
				"weight":  "Bolder",
				"spacing": "Medium",
			})
			body = append(body, map[string]interface{}{
				"type":     "TextBlock",
				"text":     truncateTeamsText(notification.Summary.ErrorMessage, 1000),
				"wrap":     true,
				"fontType": "Monospace",
				"color":    "Attention",
			})
		}
	}

	// Add service context
	body = append(body, map[string]interface{}{
		"type": "ColumnSet",
		"columns": []map[string]interface{}{
			{
				"type":  "Column",
				"width": "auto",
				"items": []map[string]interface{}{
					{
						"type":     "TextBlock",
						"text":     fmt.Sprintf("Service: %s", notification.ServiceName),
						"isSubtle": true,
						"spacing":  "Medium",
					},
				},
			},
			{
				"type":  "Column",
				"width": "auto",
				"items": []map[string]interface{}{
					{
						"type":     "TextBlock",
						"text":     notification.CreatedAt.Format("2006-01-02 15:04:05"),
						"isSubtle": true,
						"spacing":  "Medium",
					},
				},
			},
		},
	})

	// Build actions
	var actions []map[string]interface{}
	if notification.URL != "" {
		actions = append(actions, map[string]interface{}{
			"type":  "Action.OpenUrl",
			"title": "View Details",
			"url":   notification.URL,
		})
	}

	// Build the complete Adaptive Card
	card := map[string]interface{}{
		"type":    "AdaptiveCard",
		"$schema": "http://adaptivecards.io/schemas/adaptive-card.json",
		"version": "1.4",
		"body":    body,
	}

	if len(actions) > 0 {
		card["actions"] = actions
	}

	// Wrap in Teams message envelope
	return map[string]interface{}{
		"type": "message",
		"attachments": []map[string]interface{}{
			{
				"contentType": "application/vnd.microsoft.card.adaptive",
				"contentUrl":  nil,
				"content":     card,
			},
		},
		"themeColor": themeColor,
	}
}

// getThemeColor returns the appropriate theme color for the notification type.
func (c *TeamsChannel) getThemeColor(notificationType NotificationType) string {
	switch notificationType {
	case NotificationTypeRunPassed, NotificationTypeRunRecovered:
		return "28a745" // Green
	case NotificationTypeRunFailed, NotificationTypeRunError, NotificationTypeRunTimeout:
		return "dc3545" // Red
	case NotificationTypeFlakyDetected, NotificationTypeTestQuarantined:
		return "ffc107" // Yellow
	case NotificationTypeRunStarted:
		return "17a2b8" // Blue
	case NotificationTypeAgentOffline:
		return "dc3545" // Red
	case NotificationTypeAgentOnline:
		return "28a745" // Green
	default:
		return "6c757d" // Gray
	}
}

// truncateTeamsText truncates text for Teams cards.
func truncateTeamsText(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
