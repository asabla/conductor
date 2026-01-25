# Notifications

Conductor provides a flexible notification system to keep your team informed about test results, agent status, and other important events.

## Overview

The notification system supports multiple channels:

| Channel | Description | Use Case |
|---------|-------------|----------|
| Slack | Slack webhooks with rich formatting | Team channels |
| Email | SMTP email with HTML/plain text | Individual alerts |
| Webhook | Generic HTTP webhooks | Custom integrations |
| Microsoft Teams | Teams Adaptive Cards | Enterprise teams |

## Notification Types

Conductor sends notifications for the following events:

| Event | Description |
|-------|-------------|
| `run.passed` | All tests in a run passed |
| `run.failed` | One or more tests failed |
| `run.error` | Run encountered an error (not test failure) |
| `run.timeout` | Run exceeded timeout limit |
| `run.started` | Test run has started |
| `run.recovered` | Tests passed after previous failure |
| `flaky.detected` | Flaky test pattern detected |
| `agent.online` | Agent connected to control plane |
| `agent.offline` | Agent disconnected or heartbeat timeout |

---

## Slack Integration

### Creating a Slack Webhook

1. Go to [Slack API Apps](https://api.slack.com/apps)
2. Create a new app or select existing
3. Enable **Incoming Webhooks**
4. Add a new webhook to your workspace
5. Select the channel for notifications
6. Copy the webhook URL

### Configuration

#### Via API

```bash
# Create a Slack channel
curl -X POST https://conductor.example.com/api/v1/notification-channels \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "slack-alerts",
    "type": "slack",
    "enabled": true,
    "config": {
      "webhook_url": "https://hooks.slack.com/services/T00/B00/xxxx",
      "channel": "#test-alerts",
      "username": "Conductor",
      "icon_emoji": ":robot_face:"
    }
  }'
```

#### Via Dashboard

1. Navigate to Settings > Notifications
2. Click "Add Channel"
3. Select "Slack"
4. Enter webhook URL and optional settings
5. Click "Test" to verify
6. Save the channel

### Slack Message Format

Conductor sends rich Slack messages with:

- Color-coded status (green/red/yellow)
- Test summary (passed, failed, skipped counts)
- Duration and timing
- Branch and commit information
- "View Details" button linking to dashboard

Example message:

```
+------------------------------------------+
| Test Run Failed                          |
+------------------------------------------+
| payment-service unit tests failed        |
|                                          |
| Tests: 47 total, 44 passed, 3 failed     |
| Duration: 2m 34s                         |
| Branch: feature/new-checkout             |
| Commit: abc1234                          |
|                                          |
| Error:                                   |
| TestPaymentProcessing: expected 200...   |
|                                          |
| [View Details]                           |
+------------------------------------------+
| Service: payment-service | 10:30 AM      |
+------------------------------------------+
```

### Advanced Slack Options

```json
{
  "name": "slack-critical",
  "type": "slack",
  "config": {
    "webhook_url": "https://hooks.slack.com/services/...",
    "channel": "#critical-alerts",
    "username": "Conductor Alerts",
    "icon_emoji": ":rotating_light:",
    "mention_users": ["U12345", "U67890"],
    "mention_on_failure": true
  }
}
```

---

## Email Integration

### SMTP Configuration

Configure SMTP settings for email notifications:

```yaml
# control-plane.yaml
notifications:
  email:
    smtp_host: smtp.example.com
    smtp_port: 587
    username: notifications@example.com
    password: ${SMTP_PASSWORD}
    from_address: conductor@example.com
    from_name: "Conductor Test Harness"
    use_tls: true
    skip_verify: false  # Set true for self-signed certs
```

### Creating an Email Channel

```bash
curl -X POST https://conductor.example.com/api/v1/notification-channels \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "email-team",
    "type": "email",
    "enabled": true,
    "config": {
      "recipients": [
        "team@example.com",
        "lead@example.com"
      ],
      "cc": [
        "manager@example.com"
      ],
      "include_logs": false
    }
  }'
```

### Email Format

Emails are sent in multipart format (HTML + plain text):

**Subject:** `[FAIL] payment-service - Unit Tests Failed`

**Body includes:**
- Status banner with color
- Test result summary table
- Failed test details
- Link to dashboard
- Timestamp and service information

### Email Templates

Customize email templates by providing your own:

```yaml
notifications:
  email:
    templates:
      html_template_path: /etc/conductor/templates/email.html
      plain_template_path: /etc/conductor/templates/email.txt
```

Template variables available:

| Variable | Description |
|----------|-------------|
| `{{.Title}}` | Notification title |
| `{{.Message}}` | Main message |
| `{{.ServiceName}}` | Service name |
| `{{.Summary.TotalTests}}` | Total test count |
| `{{.Summary.PassedTests}}` | Passed count |
| `{{.Summary.FailedTests}}` | Failed count |
| `{{.Summary.Duration}}` | Run duration |
| `{{.URL}}` | Dashboard link |

---

## Webhook Integration

Send notifications to any HTTP endpoint for custom integrations.

### Creating a Webhook Channel

```bash
curl -X POST https://conductor.example.com/api/v1/notification-channels \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "custom-webhook",
    "type": "webhook",
    "enabled": true,
    "config": {
      "url": "https://your-service.example.com/conductor-events",
      "headers": {
        "Authorization": "Bearer your-token",
        "X-Custom-Header": "value"
      },
      "secret": "webhook-signing-secret",
      "timeout_seconds": 30
    }
  }'
```

### Webhook Payload Format

```json
{
  "event": "run.failed",
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "timestamp": "2024-01-15T10:30:00Z",
  "title": "Test Run Failed",
  "message": "payment-service unit tests failed",
  "url": "https://conductor.example.com/runs/abc123",
  "serviceId": "service-uuid",
  "serviceName": "payment-service",
  "runId": "run-uuid",
  "summary": {
    "total": 50,
    "passed": 47,
    "failed": 3,
    "skipped": 0,
    "durationMs": 154000,
    "branch": "main",
    "commitSha": "abc1234567890",
    "errorMessage": "TestPaymentProcessing: expected 200, got 500"
  },
  "metadata": {
    "custom_field": "value"
  }
}
```

### Webhook Signature Verification

When a `secret` is configured, Conductor signs payloads with HMAC-SHA256:

```
X-Conductor-Signature: sha256=<hex-encoded-signature>
X-Conductor-Signature-256: sha256=<hex-encoded-signature>
X-Conductor-Timestamp: 1705315800
```

Verify the signature in your receiver:

```python
import hmac
import hashlib

def verify_signature(payload, signature, secret):
    expected = hmac.new(
        secret.encode(),
        payload,
        hashlib.sha256
    ).hexdigest()
    return hmac.compare_digest(f"sha256={expected}", signature)
```

```go
func verifySignature(payload []byte, signature, secret string) bool {
    mac := hmac.New(sha256.New, []byte(secret))
    mac.Write(payload)
    expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
    return hmac.Equal([]byte(expected), []byte(signature))
}
```

---

## Microsoft Teams Integration

### Creating a Teams Webhook

1. In Teams, go to the channel for notifications
2. Click "..." > Connectors
3. Find "Incoming Webhook" and click Configure
4. Name the webhook and optionally upload an image
5. Copy the webhook URL

### Creating a Teams Channel

```bash
curl -X POST https://conductor.example.com/api/v1/notification-channels \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "teams-alerts",
    "type": "teams",
    "enabled": true,
    "config": {
      "webhook_url": "https://outlook.office.com/webhook/..."
    }
  }'
```

### Teams Card Format

Conductor sends Adaptive Cards to Teams:

- Header with status and color
- Fact set with test results
- Error details (if applicable)
- "View Details" action button
- Service and timestamp context

---

## Notification Rules

Rules determine when and where notifications are sent.

### Creating a Rule

```bash
curl -X POST https://conductor.example.com/api/v1/notification-rules \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "critical-failures",
    "channel_id": "channel-uuid",
    "service_id": "service-uuid",  
    "enabled": true,
    "trigger_on": ["run.failed", "run.error", "run.timeout"],
    "conditions": {
      "branches": ["main", "release/*"],
      "min_failed_tests": 1
    }
  }'
```

### Rule Properties

| Property | Type | Description |
|----------|------|-------------|
| `name` | string | Rule name |
| `channel_id` | UUID | Target notification channel |
| `service_id` | UUID | Specific service (null = all services) |
| `enabled` | boolean | Whether rule is active |
| `trigger_on` | array | Events that trigger the rule |
| `conditions` | object | Additional filtering conditions |

### Global vs Service Rules

**Global Rule** (applies to all services):

```json
{
  "name": "all-failures-to-slack",
  "channel_id": "slack-channel-id",
  "service_id": null,
  "trigger_on": ["run.failed"]
}
```

**Service-Specific Rule:**

```json
{
  "name": "payment-critical",
  "channel_id": "pagerduty-channel-id",
  "service_id": "payment-service-id",
  "trigger_on": ["run.failed", "run.error"]
}
```

### Trigger Events

| Event | When Triggered |
|-------|----------------|
| `run.passed` | All tests pass |
| `run.failed` | Any test fails |
| `run.error` | Infrastructure/execution error |
| `run.timeout` | Run exceeds timeout |
| `run.started` | Run begins execution |
| `run.recovered` | Pass after previous failure |
| `flaky.detected` | Flaky pattern identified |
| `agent.online` | Agent connects |
| `agent.offline` | Agent disconnects |
| `always` | Every event (use sparingly) |

### Conditions

Filter notifications with conditions:

```json
{
  "conditions": {
    "branches": ["main", "develop"],
    "exclude_branches": ["dependabot/*"],
    "tags": ["critical", "smoke"],
    "min_failed_tests": 1,
    "environments": ["production", "staging"]
  }
}
```

---

## Throttling

Prevent notification spam with built-in throttling:

```yaml
# control-plane.yaml
notifications:
  throttle_duration: 5m  # Suppress duplicates for 5 minutes
```

Throttling is per:
- Rule ID
- Service ID  
- Event type

Example: If `run.failed` for `payment-service` triggers a rule, subsequent `run.failed` events for the same service won't trigger that rule for 5 minutes.

---

## Testing Notifications

### Test a Channel

```bash
# Send test notification to a channel
curl -X POST https://conductor.example.com/api/v1/notification-channels/{id}/test \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "message": "This is a test notification from Conductor"
  }'
```

### Response

```json
{
  "success": true,
  "channel_id": "uuid",
  "channel_type": "slack",
  "latency_ms": 234,
  "sent_at": "2024-01-15T10:30:00Z"
}
```

---

## Configuration Reference

### Control Plane Configuration

```yaml
notifications:
  # Worker configuration
  worker_count: 5          # Parallel notification workers
  queue_size: 1000         # Notification queue size
  default_timeout: 30s     # Send timeout per notification
  
  # Throttling
  throttle_duration: 5m    # Duplicate suppression window
  
  # Retry configuration
  retry_attempts: 3        # Retries on failure
  
  # Base URL for links in notifications
  base_url: https://conductor.example.com
  
  # Email SMTP settings
  email:
    smtp_host: smtp.example.com
    smtp_port: 587
    username: ${SMTP_USERNAME}
    password: ${SMTP_PASSWORD}
    from_address: conductor@example.com
    from_name: Conductor
    use_tls: true
```

### Environment Variables

| Variable | Description |
|----------|-------------|
| `SMTP_HOST` | SMTP server hostname |
| `SMTP_PORT` | SMTP server port |
| `SMTP_USERNAME` | SMTP authentication username |
| `SMTP_PASSWORD` | SMTP authentication password |
| `NOTIFICATION_BASE_URL` | Base URL for notification links |

---

## Best Practices

### 1. Use Specific Rules

Create focused rules rather than catch-all notifications:

```json
// Good: Specific rules
{"trigger_on": ["run.failed"], "service_id": "critical-service"}
{"trigger_on": ["run.recovered"], "service_id": "critical-service"}

// Avoid: Overly broad
{"trigger_on": ["always"], "service_id": null}
```

### 2. Leverage Throttling

Set appropriate throttle durations to prevent alert fatigue:

- Flaky tests: 1 hour throttle
- Critical failures: 5 minute throttle
- Recovery notifications: No throttle

### 3. Channel per Audience

Create separate channels for different audiences:

- `#dev-alerts` - All failures for developers
- `#ops-critical` - Only production failures
- Email to managers - Daily summary only

### 4. Test Before Production

Always test notification channels before relying on them:

```bash
curl -X POST .../notification-channels/{id}/test
```

### 5. Monitor Notification Health

Check notification metrics:

```bash
curl https://conductor.example.com/api/v1/metrics | grep notification
```

Key metrics:
- `notification_sent_total` - Total notifications sent
- `notification_failed_total` - Failed sends
- `notification_latency_seconds` - Send latency

---

## Troubleshooting

### Notifications Not Sending

1. **Check channel is enabled:**
   ```bash
   curl .../api/v1/notification-channels/{id}
   # Verify "enabled": true
   ```

2. **Verify rule configuration:**
   ```bash
   curl .../api/v1/notification-rules?service_id={service}
   # Check trigger_on matches event type
   ```

3. **Check throttling:**
   - Recent duplicate may be throttled
   - Wait for throttle_duration to expire

4. **Review logs:**
   ```bash
   kubectl logs -l app=conductor-control-plane | grep notification
   ```

### Slack Errors

| Error | Solution |
|-------|----------|
| `channel_not_found` | Verify webhook URL is correct |
| `invalid_payload` | Check message formatting |
| `rate_limited` | Reduce notification frequency |

### Email Errors

| Error | Solution |
|-------|----------|
| `authentication failed` | Check SMTP credentials |
| `connection refused` | Verify SMTP host/port |
| `TLS handshake failed` | Check TLS settings |

### Webhook Errors

| Error | Solution |
|-------|----------|
| `connection timeout` | Increase timeout_seconds |
| `401 Unauthorized` | Verify authentication headers |
| `signature mismatch` | Check secret configuration |

---

## Next Steps

- [Git Integration](git-integration.md) - Configure commit status and PR comments
- [API Reference](api.md) - Full notification API documentation
- [Configuration](configuration.md) - Complete configuration options
