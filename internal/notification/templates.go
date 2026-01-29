package notification

import (
	"fmt"
	"strings"
	"time"
)

// Template constants for different notification types
const (
	// DefaultTitleTemplate is the default title template
	DefaultTitleTemplate = "{{.ServiceName}}: {{.Status}}"
	// DefaultMessageTemplate is the default message template
	DefaultMessageTemplate = "Test run {{.RunID}} completed with status {{.Status}}"
)

// TemplateVars contains variables available in notification templates.
type TemplateVars struct {
	ServiceName    string
	ServiceID      string
	RunID          string
	Status         string
	TotalTests     int
	PassedTests    int
	FailedTests    int
	SkippedTests   int
	DurationMs     int64
	Branch         string
	CommitSHA      string
	ErrorMessage   string
	TestName       string
	FlakinessScore float64
	FlakyRuns      int
	TotalRuns      int
	QuarantinedBy  string
	URL            string
	Timestamp      time.Time
}

// RunStartedTemplate returns a notification for run started events.
func RunStartedTemplate(vars TemplateVars) (title, message string) {
	title = fmt.Sprintf("Test Run Started - %s", vars.ServiceName)

	var parts []string
	parts = append(parts, fmt.Sprintf("A new test run has started for *%s*.", vars.ServiceName))

	if vars.Branch != "" {
		parts = append(parts, fmt.Sprintf("Branch: `%s`", vars.Branch))
	}
	if vars.CommitSHA != "" {
		shortSHA := vars.CommitSHA
		if len(shortSHA) > 7 {
			shortSHA = shortSHA[:7]
		}
		parts = append(parts, fmt.Sprintf("Commit: `%s`", shortSHA))
	}

	message = strings.Join(parts, "\n")
	return
}

// RunPassedTemplate returns a notification for run passed events.
func RunPassedTemplate(vars TemplateVars) (title, message string) {
	title = fmt.Sprintf("Tests Passed - %s", vars.ServiceName)

	var parts []string
	parts = append(parts, fmt.Sprintf("All tests passed for *%s*!", vars.ServiceName))
	parts = append(parts, "")
	parts = append(parts, fmt.Sprintf("*Results:* %d/%d tests passed",
		vars.PassedTests, vars.TotalTests))

	if vars.SkippedTests > 0 {
		parts = append(parts, fmt.Sprintf("*Skipped:* %d tests", vars.SkippedTests))
	}

	if vars.DurationMs > 0 {
		duration := time.Duration(vars.DurationMs) * time.Millisecond
		parts = append(parts, fmt.Sprintf("*Duration:* %s", duration.Round(time.Second)))
	}

	message = strings.Join(parts, "\n")
	return
}

// RunFailedTemplate returns a notification for run failed events.
func RunFailedTemplate(vars TemplateVars) (title, message string) {
	title = fmt.Sprintf("Tests Failed - %s", vars.ServiceName)

	var parts []string
	parts = append(parts, fmt.Sprintf("Test failures detected in *%s*.", vars.ServiceName))
	parts = append(parts, "")
	parts = append(parts, fmt.Sprintf("*Results:* %d passed, *%d failed*, %d skipped out of %d total",
		vars.PassedTests, vars.FailedTests, vars.SkippedTests, vars.TotalTests))

	if vars.DurationMs > 0 {
		duration := time.Duration(vars.DurationMs) * time.Millisecond
		parts = append(parts, fmt.Sprintf("*Duration:* %s", duration.Round(time.Second)))
	}

	if vars.ErrorMessage != "" {
		parts = append(parts, "")
		parts = append(parts, fmt.Sprintf("*Error:*\n```%s```", truncateString(vars.ErrorMessage, 500)))
	}

	message = strings.Join(parts, "\n")
	return
}

// RunRecoveredTemplate returns a notification for run recovered events.
func RunRecoveredTemplate(vars TemplateVars) (title, message string) {
	title = fmt.Sprintf("Tests Recovered - %s", vars.ServiceName)

	var parts []string
	parts = append(parts, fmt.Sprintf("Tests are passing again for *%s*!", vars.ServiceName))
	parts = append(parts, "")
	parts = append(parts, fmt.Sprintf("*Results:* %d/%d tests passed", vars.PassedTests, vars.TotalTests))

	if vars.Branch != "" {
		parts = append(parts, fmt.Sprintf("*Branch:* `%s`", vars.Branch))
	}

	message = strings.Join(parts, "\n")
	return
}

// FlakyDetectedTemplate returns a notification for flaky test detection.
func FlakyDetectedTemplate(vars TemplateVars) (title, message string) {
	title = fmt.Sprintf("Flaky Test Detected - %s", vars.ServiceName)

	var parts []string
	parts = append(parts, fmt.Sprintf("A flaky test was detected in *%s*.", vars.ServiceName))
	if vars.TestName != "" {
		parts = append(parts, fmt.Sprintf("Test: *%s*", vars.TestName))
	}
	if vars.TotalRuns > 0 {
		parts = append(parts, fmt.Sprintf("Flaky runs: %d/%d (%s)", vars.FlakyRuns, vars.TotalRuns, formatFlakiness(vars.FlakinessScore)))
	}
	parts = append(parts, "")
	parts = append(parts, "Flaky tests can cause intermittent failures and should be investigated.")

	message = strings.Join(parts, "\n")
	return
}

// TestQuarantinedTemplate returns a notification for quarantined tests.
func TestQuarantinedTemplate(vars TemplateVars) (title, message string) {
	title = fmt.Sprintf("Test Quarantined - %s", vars.ServiceName)

	var parts []string
	parts = append(parts, fmt.Sprintf("A test was quarantined in *%s*.", vars.ServiceName))
	if vars.TestName != "" {
		parts = append(parts, fmt.Sprintf("Test: *%s*", vars.TestName))
	}
	if vars.TotalRuns > 0 {
		parts = append(parts, fmt.Sprintf("Flaky runs: %d/%d (%s)", vars.FlakyRuns, vars.TotalRuns, formatFlakiness(vars.FlakinessScore)))
	}
	if vars.QuarantinedBy != "" {
		parts = append(parts, fmt.Sprintf("Quarantined by: %s", vars.QuarantinedBy))
	}

	message = strings.Join(parts, "\n")
	return
}

// RunTimeoutTemplate returns a notification for run timeout events.
func RunTimeoutTemplate(vars TemplateVars) (title, message string) {
	title = fmt.Sprintf("Test Run Timeout - %s", vars.ServiceName)

	var parts []string
	parts = append(parts, fmt.Sprintf("Test run timed out for *%s*.", vars.ServiceName))
	parts = append(parts, "")

	if vars.DurationMs > 0 {
		duration := time.Duration(vars.DurationMs) * time.Millisecond
		parts = append(parts, fmt.Sprintf("*Duration before timeout:* %s", duration.Round(time.Second)))
	}

	if vars.ErrorMessage != "" {
		parts = append(parts, fmt.Sprintf("*Details:* %s", vars.ErrorMessage))
	}

	message = strings.Join(parts, "\n")
	return
}

// RunErrorTemplate returns a notification for run error events.
func RunErrorTemplate(vars TemplateVars) (title, message string) {
	title = fmt.Sprintf("Test Run Error - %s", vars.ServiceName)

	var parts []string
	parts = append(parts, fmt.Sprintf("An error occurred during the test run for *%s*.", vars.ServiceName))
	parts = append(parts, "")

	if vars.ErrorMessage != "" {
		parts = append(parts, fmt.Sprintf("*Error:*\n```%s```", truncateString(vars.ErrorMessage, 500)))
	}

	message = strings.Join(parts, "\n")
	return
}

// AgentOfflineTemplate returns a notification for agent offline events.
func AgentOfflineTemplate(agentName string) (title, message string) {
	title = fmt.Sprintf("Agent Offline - %s", agentName)
	message = fmt.Sprintf("Agent *%s* has gone offline and is no longer available for test execution.", agentName)
	return
}

// AgentOnlineTemplate returns a notification for agent online events.
func AgentOnlineTemplate(agentName string) (title, message string) {
	title = fmt.Sprintf("Agent Online - %s", agentName)
	message = fmt.Sprintf("Agent *%s* is now online and available for test execution.", agentName)
	return
}

// TestNotificationTemplate returns a test notification.
func TestNotificationTemplate(channelName string) (title, message string) {
	title = "Test Notification - Conductor"
	message = fmt.Sprintf("This is a test notification sent to verify the *%s* channel configuration.", channelName)
	return
}

// truncateString truncates a string to the specified length.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func formatFlakiness(score float64) string {
	if score <= 1 {
		return fmt.Sprintf("%.1f%%", score*100)
	}
	return fmt.Sprintf("%.1f%%", score)
}

// GetTemplateForType returns the appropriate template function for a notification type.
func GetTemplateForType(notificationType NotificationType, vars TemplateVars) (title, message string) {
	switch notificationType {
	case NotificationTypeRunStarted:
		return RunStartedTemplate(vars)
	case NotificationTypeRunPassed:
		return RunPassedTemplate(vars)
	case NotificationTypeRunFailed:
		return RunFailedTemplate(vars)
	case NotificationTypeRunRecovered:
		return RunRecoveredTemplate(vars)
	case NotificationTypeFlakyDetected:
		return FlakyDetectedTemplate(vars)
	case NotificationTypeTestQuarantined:
		return TestQuarantinedTemplate(vars)
	case NotificationTypeRunTimeout:
		return RunTimeoutTemplate(vars)
	case NotificationTypeRunError:
		return RunErrorTemplate(vars)
	default:
		return "Notification", "A notification event occurred."
	}
}

// Email HTML template
const emailHTMLTemplate = `<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Title}}</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif; margin: 0; padding: 0; background-color: #f5f5f5; }
        .container { max-width: 600px; margin: 0 auto; background-color: #ffffff; }
        .header { background-color: {{.StatusColor}}; color: #ffffff; padding: 20px; text-align: center; }
        .header h1 { margin: 0; font-size: 24px; font-weight: 600; }
        .content { padding: 20px; }
        .message { color: #333333; line-height: 1.6; margin-bottom: 20px; }
        .summary { background-color: #f8f9fa; border-radius: 8px; padding: 15px; margin-bottom: 20px; }
        .summary-row { display: flex; justify-content: space-between; padding: 8px 0; border-bottom: 1px solid #e9ecef; }
        .summary-row:last-child { border-bottom: none; }
        .summary-label { color: #6c757d; font-weight: 500; }
        .summary-value { color: #333333; font-weight: 600; }
        .passed { color: #28a745; }
        .failed { color: #dc3545; }
        .skipped { color: #6c757d; }
        .error-box { background-color: #fff3f3; border: 1px solid #ffcccc; border-radius: 4px; padding: 12px; margin-top: 15px; }
        .error-box pre { margin: 0; white-space: pre-wrap; word-wrap: break-word; font-size: 12px; color: #721c24; }
        .button { display: inline-block; background-color: {{.StatusColor}}; color: #ffffff; padding: 12px 24px; text-decoration: none; border-radius: 4px; font-weight: 600; margin-top: 15px; }
        .button:hover { opacity: 0.9; }
        .footer { padding: 20px; text-align: center; color: #6c757d; font-size: 12px; border-top: 1px solid #e9ecef; }
        .footer a { color: #6c757d; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>{{.Title}}</h1>
        </div>
        <div class="content">
            <p class="message">{{.Message}}</p>
            
            {{if .Summary}}
            <div class="summary">
                <div class="summary-row">
                    <span class="summary-label">Service</span>
                    <span class="summary-value">{{.ServiceName}}</span>
                </div>
                <div class="summary-row">
                    <span class="summary-label">Total Tests</span>
                    <span class="summary-value">{{.Summary.TotalTests}}</span>
                </div>
                <div class="summary-row">
                    <span class="summary-label">Passed</span>
                    <span class="summary-value passed">{{.Summary.PassedTests}}</span>
                </div>
                <div class="summary-row">
                    <span class="summary-label">Failed</span>
                    <span class="summary-value failed">{{.Summary.FailedTests}}</span>
                </div>
                {{if .Summary.SkippedTests}}
                <div class="summary-row">
                    <span class="summary-label">Skipped</span>
                    <span class="summary-value skipped">{{.Summary.SkippedTests}}</span>
                </div>
                {{end}}
                {{if .Summary.Branch}}
                <div class="summary-row">
                    <span class="summary-label">Branch</span>
                    <span class="summary-value">{{.Summary.Branch}}</span>
                </div>
                {{end}}
                {{if .Summary.CommitSHA}}
                <div class="summary-row">
                    <span class="summary-label">Commit</span>
                    <span class="summary-value">{{.Summary.CommitSHA}}</span>
                </div>
                {{end}}
            </div>
            
            {{if .Summary.ErrorMessage}}
            <div class="error-box">
                <strong>Error Details:</strong>
                <pre>{{.Summary.ErrorMessage}}</pre>
            </div>
            {{end}}
            {{end}}
            
            {{if .URL}}
            <a href="{{.URL}}" class="button">View Details</a>
            {{end}}
        </div>
        <div class="footer">
            <p>Sent by Conductor Test Platform<br>
            {{.CreatedAt}}</p>
            <p>&copy; {{.Year}} Conductor</p>
        </div>
    </div>
</body>
</html>`

// Email plain text template
const emailPlainTemplate = `{{.Title}}

{{.Message}}

{{if .Summary -}}
Service: {{.ServiceName}}
Total Tests: {{.Summary.TotalTests}}
Passed: {{.Summary.PassedTests}}
Failed: {{.Summary.FailedTests}}
{{if .Summary.SkippedTests}}Skipped: {{.Summary.SkippedTests}}{{end}}
{{if .Summary.Branch}}Branch: {{.Summary.Branch}}{{end}}
{{if .Summary.CommitSHA}}Commit: {{.Summary.CommitSHA}}{{end}}

{{if .Summary.ErrorMessage -}}
Error Details:
{{.Summary.ErrorMessage}}
{{end}}
{{end}}
{{if .URL -}}
View details: {{.URL}}
{{end}}

---
Sent by Conductor Test Platform
{{.CreatedAt}}`
