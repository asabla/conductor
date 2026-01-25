# Git Integration

This guide covers how to integrate Conductor with your Git hosting providers to automatically trigger test runs on code changes.

## Overview

Conductor supports integration with multiple Git providers:

| Provider | Status | Features |
|----------|--------|----------|
| GitHub | Full Support | Webhooks, Check Runs, Commit Status, PR Comments |
| GitLab | Full Support | Webhooks, Pipeline Status, MR Comments |
| Bitbucket | Full Support | Webhooks, Build Status, PR Comments |

## GitHub Integration

### Authentication Methods

Conductor supports two authentication methods for GitHub:

#### Personal Access Token (PAT)

Simpler setup, suitable for smaller deployments.

1. Go to GitHub Settings > Developer settings > Personal access tokens > Tokens (classic)
2. Generate a new token with the following scopes:
   - `repo` (Full control of private repositories)
   - `read:org` (Read org membership)
   - `write:repo_hook` (Write repository hooks)

3. Configure in Conductor:

```yaml
# control-plane.yaml
git:
  providers:
    - name: github-main
      provider: github
      token: ${GITHUB_TOKEN}
      webhook_secret: ${GITHUB_WEBHOOK_SECRET}
```

#### GitHub App (Recommended)

More secure with fine-grained permissions and higher rate limits.

1. **Create a GitHub App:**
   - Go to GitHub Settings > Developer settings > GitHub Apps > New GitHub App
   - Set the following:
     - **Name:** `Conductor Test Harness`
     - **Homepage URL:** Your Conductor dashboard URL
     - **Webhook URL:** `https://your-conductor.example.com/api/v1/webhooks/github`
     - **Webhook secret:** Generate a secure random string

2. **Configure Permissions:**
   
   | Permission | Access | Purpose |
   |------------|--------|---------|
   | Contents | Read | Fetch test manifests |
   | Metadata | Read | Repository information |
   | Commit statuses | Write | Report test status |
   | Checks | Write | Create check runs |
   | Pull requests | Write | Post PR comments |
   | Webhooks | Read & Write | Manage webhooks |

3. **Subscribe to Events:**
   - Push
   - Pull request
   - Check suite
   - Check run

4. **Generate Private Key:**
   - After creating the app, generate and download a private key
   - Store securely (e.g., Kubernetes secret)

5. **Install the App:**
   - Install on your organization or specific repositories
   - Note the Installation ID

6. **Configure Conductor:**

```yaml
# control-plane.yaml
git:
  providers:
    - name: github-app
      provider: github
      app_id: 12345
      app_private_key_path: /etc/conductor/github-app.pem
      # Or inline:
      # app_private_key: |
      #   -----BEGIN RSA PRIVATE KEY-----
      #   ...
      #   -----END RSA PRIVATE KEY-----
      installation_id: 67890
      webhook_secret: ${GITHUB_WEBHOOK_SECRET}
```

### Webhook Configuration

#### Manual Setup

1. Go to your repository Settings > Webhooks > Add webhook
2. Configure:
   - **Payload URL:** `https://your-conductor.example.com/api/v1/webhooks/github`
   - **Content type:** `application/json`
   - **Secret:** Same as `webhook_secret` in config
   - **Events:** Select individual events:
     - Push
     - Pull requests
     - (Optional) Check suites

#### Automatic Registration

Conductor can automatically register webhooks if the token has `admin:repo_hook` scope:

```yaml
git:
  providers:
    - name: github-main
      provider: github
      token: ${GITHUB_TOKEN}
      webhook_secret: ${GITHUB_WEBHOOK_SECRET}
      auto_register_webhooks: true
      organizations:
        - my-org
```

### Commit Status Reporting

Conductor automatically reports test status to GitHub commits:

```
conductor/unit-tests     pending    "Tests queued"
conductor/unit-tests     success    "15 passed"
conductor/unit-tests     failure    "3 failed, 12 passed"
```

Configure the status context prefix:

```yaml
git:
  status_context_prefix: "conductor"  # Results in "conductor/suite-name"
```

### Check Runs API

For richer CI integration, Conductor uses the GitHub Checks API:

- **In Progress:** Shows test execution progress
- **Completed:** Displays detailed results with annotations
- **Summary:** Test counts, duration, failure details

Example check run output:

```markdown
## Test Results

**Status:** 3 failed, 47 passed, 2 skipped

### Failed Tests

- `TestUserAuthentication` - Expected status 200, got 401
- `TestDatabaseConnection` - Connection timeout after 30s
- `TestAPIRateLimit` - Rate limit exceeded

### Duration

Total: 2m 34s
```

### Pull Request Comments

Conductor can post test result summaries as PR comments:

```yaml
git:
  pr_comments:
    enabled: true
    on_failure: true      # Comment when tests fail
    on_success: false     # Don't comment on success
    on_recovery: true     # Comment when tests recover
    collapse_passed: true # Collapse passed test details
```

Example PR comment:

> ## Test Results
> 
> | Suite | Status | Passed | Failed | Duration |
> |-------|--------|--------|--------|----------|
> | unit-tests | :x: Failed | 47 | 3 | 2m 34s |
> | integration | :white_check_mark: Passed | 12 | 0 | 5m 12s |
> 
> <details>
> <summary>Failed Tests (3)</summary>
> 
> - `TestUserAuthentication` - Expected status 200, got 401
> - `TestDatabaseConnection` - Connection timeout
> - `TestAPIRateLimit` - Rate limit exceeded
> </details>
>
> [View full results](https://conductor.example.com/runs/abc123)

---

## GitLab Integration

### Authentication

Create a Project or Group Access Token:

1. Go to Project/Group Settings > Access Tokens
2. Create token with scopes:
   - `api` (Full API access)
   - `read_repository` (Read repository)

3. Configure:

```yaml
git:
  providers:
    - name: gitlab-internal
      provider: gitlab
      base_url: https://gitlab.example.com  # For self-hosted
      token: ${GITLAB_TOKEN}
      webhook_secret: ${GITLAB_WEBHOOK_SECRET}
```

### Webhook Setup

1. Go to Project Settings > Webhooks
2. Configure:
   - **URL:** `https://conductor.example.com/api/v1/webhooks/gitlab`
   - **Secret token:** Same as config
   - **Triggers:**
     - Push events
     - Merge request events
     - Tag push events

### Pipeline Status

Conductor reports status via GitLab's Commit Status API:

```yaml
git:
  gitlab:
    pipeline_name: "conductor"
    status_name: "tests"
```

### Merge Request Comments

```yaml
git:
  gitlab:
    mr_comments:
      enabled: true
      on_failure: true
```

---

## Bitbucket Integration

### Authentication

Create an App Password:

1. Go to Personal Settings > App passwords
2. Create with permissions:
   - Repositories: Read
   - Webhooks: Read and Write
   - Pull requests: Write

```yaml
git:
  providers:
    - name: bitbucket
      provider: bitbucket
      username: ${BITBUCKET_USERNAME}
      app_password: ${BITBUCKET_APP_PASSWORD}
      webhook_secret: ${BITBUCKET_WEBHOOK_SECRET}
```

### Webhook Setup

1. Go to Repository Settings > Webhooks
2. Add webhook:
   - **URL:** `https://conductor.example.com/api/v1/webhooks/bitbucket`
   - **Triggers:**
     - Repository: Push
     - Pull Request: Created, Updated

### Build Status

Conductor reports to Bitbucket's Build Status API:

```yaml
git:
  bitbucket:
    build_key: "conductor"
```

---

## Multi-Provider Configuration

Configure multiple providers for organizations using different Git hosts:

```yaml
git:
  providers:
    - name: github-public
      provider: github
      token: ${GITHUB_TOKEN}
      organizations:
        - public-org
    
    - name: gitlab-internal
      provider: gitlab
      base_url: https://gitlab.internal.example.com
      token: ${GITLAB_TOKEN}
      groups:
        - platform
        - services
    
    - name: bitbucket-legacy
      provider: bitbucket
      username: ${BB_USERNAME}
      app_password: ${BB_APP_PASSWORD}
      workspaces:
        - legacy-apps
```

---

## Webhook Event Handling

### Push Events

When code is pushed:

1. Conductor receives webhook
2. Validates signature using `webhook_secret`
3. Fetches `.testharness.yaml` from the pushed commit
4. Creates test run for matching test suites
5. Reports status back to Git provider

### Pull Request Events

| Event | Action |
|-------|--------|
| opened | Run tests for PR head commit |
| synchronize | Run tests for new commits |
| reopened | Run tests again |
| closed | No action (optional cleanup) |

### Event Filtering

Control which events trigger test runs:

```yaml
git:
  events:
    push:
      enabled: true
      branches:
        include:
          - main
          - develop
          - "feature/*"
        exclude:
          - "dependabot/*"
    
    pull_request:
      enabled: true
      target_branches:
        - main
        - develop
```

---

## Repository Discovery

### Automatic Scanning

Conductor can automatically discover repositories with test manifests:

```yaml
git:
  discovery:
    enabled: true
    scan_interval: 1h
    organizations:
      - my-org
    topic_filter:
      - has-tests
      - conductor-enabled
```

### Manual Registration

Register repositories via API:

```bash
curl -X POST https://conductor.example.com/api/v1/services \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "my-service",
    "repository_url": "https://github.com/my-org/my-service",
    "provider": "github"
  }'
```

---

## Clone Credentials

When agents need to clone repositories, Conductor provides short-lived credentials:

### GitHub

- For Apps: Installation token (1 hour expiry)
- For PAT: Uses the configured token

### GitLab

- Uses the configured access token
- Consider using deploy tokens for read-only access

### Bitbucket

- Uses app password with limited scope

### Credential Flow

```
Agent                    Control Plane              Git Provider
  │                           │                          │
  │ RequestCloneCredentials   │                          │
  ├──────────────────────────►│                          │
  │                           │   Generate Token         │
  │                           ├─────────────────────────►│
  │                           │   Token (1hr expiry)     │
  │                           │◄─────────────────────────┤
  │   Credentials             │                          │
  │◄──────────────────────────┤                          │
  │                           │                          │
  │   git clone https://x-access-token:{token}@github.com/...
  │───────────────────────────────────────────────────────►
```

---

## Rate Limiting

Conductor implements rate limit awareness for all Git providers:

### GitHub

- 5,000 requests/hour for authenticated requests
- Conductor tracks `X-RateLimit-Remaining` headers
- Automatically waits when limits are approached

### Configuration

```yaml
git:
  rate_limiting:
    enabled: true
    warning_threshold: 100  # Warn when remaining < 100
    pause_threshold: 10     # Pause requests when remaining < 10
```

### Caching

Reduce API calls with intelligent caching:

```yaml
git:
  caching:
    enabled: true
    repository_ttl: 5m      # Cache repo metadata
    file_content_ttl: 1h    # Cache file contents by SHA (immutable)
    refs_ttl: 1m            # Cache branch/tag lists
```

---

## Troubleshooting

### Webhook Not Received

1. **Check webhook URL:** Ensure it's publicly accessible
2. **Verify signature:** Check `webhook_secret` matches
3. **Review webhook history:** GitHub/GitLab show recent deliveries
4. **Check firewall:** Ensure Git provider IPs are allowed

### Status Not Updating

1. **Check token permissions:** Needs write access to statuses/checks
2. **Verify repository access:** Token must have access to the repo
3. **Check logs:** Look for API errors in control plane logs

```bash
kubectl logs -l app=conductor-control-plane | grep -i github
```

### Rate Limit Exceeded

1. **Use GitHub App:** Higher limits than PAT
2. **Enable caching:** Reduce redundant API calls
3. **Check for loops:** Ensure webhooks aren't triggering themselves

### Clone Failures

1. **Verify credentials:** Test manually with the same token
2. **Check network:** Agent must reach Git provider
3. **Review permissions:** Token needs repository read access

---

## Security Best Practices

1. **Use GitHub Apps** over PATs when possible
2. **Rotate secrets regularly** - especially webhook secrets
3. **Limit token scopes** to minimum required permissions
4. **Use short-lived tokens** for clone operations
5. **Validate webhook signatures** - never skip signature verification
6. **Store secrets securely** - use Kubernetes secrets or vault

## Next Steps

- [Notifications](notifications.md) - Configure alerts for test results
- [Test Manifest](test-manifest.md) - Define tests in your repository
- [API Reference](api.md) - Programmatic access to Git operations
