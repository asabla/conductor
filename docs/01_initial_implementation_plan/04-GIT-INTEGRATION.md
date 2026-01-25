# Git Provider Integration

## Overview

The test harness must interact with Git hosting platforms to discover repositories, fetch test manifests, and respond to events. The Git Provider Abstraction provides a unified interface that supports multiple platforms (GitHub, GitLab, Bitbucket) while allowing platform-specific features.

## Abstraction Design

```
┌─────────────────────────────────────────────────────────────────────────┐
│                       Git Provider Interface                            │
├─────────────────────────────────────────────────────────────────────────┤
│  ListRepositories(org) → []Repository                                   │
│  GetRepository(org, name) → Repository                                  │
│  GetFile(repo, path, ref) → FileContent                                 │
│  ListRefs(repo) → []Ref                                                 │
│  GetCloneCredentials(repo) → Credentials                                │
│  ParseWebhookEvent(request) → Event                                     │
│  ValidateWebhookSignature(request, secret) → bool                       │
└──────────────────────────┬──────────────────────────────────────────────┘
                           │
         ┌─────────────────┼─────────────────┐
         │                 │                 │
         ▼                 ▼                 ▼
┌─────────────────┐ ┌─────────────────┐ ┌─────────────────┐
│ GitHub Adapter  │ │ GitLab Adapter  │ │Bitbucket Adapter│
└─────────────────┘ └─────────────────┘ └─────────────────┘
```

## Interface Definition

### Core Types

**Repository**:
- ID: unique identifier within provider
- Organization: org/group/workspace name
- Name: repository name
- FullName: org/name
- DefaultBranch: main, master, etc.
- CloneURL: HTTPS clone URL
- Private: whether repo requires authentication

**Ref**:
- Name: branch name, tag name, or commit SHA
- Type: branch, tag, or commit
- SHA: commit hash

**FileContent**:
- Path: file path in repo
- Content: file content (bytes)
- SHA: blob hash
- Encoding: content encoding (typically utf-8)

**Credentials**:
- Type: token, ssh, or basic
- Token: access token (for HTTPS)
- SSHKey: private key (for SSH)
- Username/Password: basic auth (legacy)

**Event**:
- Type: push, pull_request, tag, etc.
- Repository: affected repository
- Ref: branch/tag involved
- Commits: list of commits (for push)
- Action: opened, closed, merged (for PRs)

### Interface Methods

**ListRepositories(org string) → []Repository**

Lists all repositories in an organization/group that the harness has access to. Used for initial discovery and periodic scanning.

Considerations:
- Pagination required for large orgs
- Filter by topic/label if supported
- Respect rate limits

**GetRepository(org, name string) → Repository**

Gets details for a specific repository. Used to fetch metadata before operations.

**GetFile(repo Repository, path string, ref string) → FileContent**

Retrieves a single file from a repository at a specific ref. Primary use: fetching `.testharness.yaml` manifests.

Considerations:
- Ref can be branch name, tag name, or commit SHA
- Should return error if file doesn't exist (not found vs. other errors)
- Content may be large—consider streaming for big files

**ListRefs(repo Repository) → []Ref**

Lists branches and tags in a repository. Used to understand available refs for testing.

Considerations:
- May want to filter (only branches, only tags, matching pattern)
- Pagination for repos with many refs

**GetCloneCredentials(repo Repository) → Credentials**

Gets credentials suitable for cloning the repository. The agent will use these to clone.

Options:
- Generate short-lived token (preferred)
- Return configured deploy key
- Return PAT (less secure, but simple)

**ParseWebhookEvent(request *http.Request) → Event**

Parses an incoming webhook payload into a structured event.

Platform differences:
- GitHub: X-GitHub-Event header, JSON body
- GitLab: X-Gitlab-Event header, JSON body
- Bitbucket: different structure entirely

**ValidateWebhookSignature(request *http.Request, secret string) → bool**

Validates that webhook came from the expected source.

Platform differences:
- GitHub: X-Hub-Signature-256, HMAC-SHA256
- GitLab: X-Gitlab-Token, direct comparison
- Bitbucket: various methods

## Provider-Specific Notes

### GitHub

**Authentication**:
- GitHub App (recommended): generates installation tokens, fine-grained permissions
- Personal Access Token (PAT): simpler, but tied to user, broader permissions
- OAuth App: for user-facing flows, not recommended for harness

**API**:
- REST API v3 for most operations
- GraphQL API v4 for efficient bulk queries
- Rate limits: 5000/hour for authenticated requests

**Webhooks**:
- Push events for branch/tag updates
- Pull request events for PR workflows
- Check suite events for CI integration

**Special features**:
- Check Runs API: report test status directly on commits
- Commit status API: simpler status reporting
- Topics: can filter repos by topic

### GitLab

**Authentication**:
- Project/Group access tokens (recommended)
- Personal access tokens
- OAuth2 for user-facing flows

**API**:
- REST API v4
- GraphQL API available
- Rate limits vary by instance

**Webhooks**:
- Push events
- Merge request events
- Pipeline events

**Special features**:
- Native CI integration (can trigger via .gitlab-ci.yml)
- Built-in container registry
- Self-hosted common—may need custom base URL

### Bitbucket

**Authentication**:
- App passwords (simple)
- OAuth consumers (for integrations)
- Repository/workspace access tokens

**API**:
- REST API 2.0
- Different structure than GitHub/GitLab

**Webhooks**:
- Repository push events
- Pull request events

**Special features**:
- Pipelines integration
- Bitbucket Server (self-hosted) has different API

## Configuration

Each provider requires configuration:

```yaml
git_providers:
  - name: github-main
    type: github
    # GitHub App authentication (recommended)
    app_id: 12345
    app_private_key_path: /etc/testharness/github-app.pem
    # Or PAT authentication
    # token: ghp_xxxx
    
    # Organizations to scan
    organizations:
      - my-org
      - other-org
    
    # Webhook secret for validating incoming hooks
    webhook_secret: ${GITHUB_WEBHOOK_SECRET}
    
    # Optional: only include repos with these topics
    topic_filter:
      - has-tests
      - testharness
  
  - name: gitlab-internal
    type: gitlab
    base_url: https://gitlab.internal.example.com
    token: ${GITLAB_TOKEN}
    groups:
      - platform
      - services
    webhook_secret: ${GITLAB_WEBHOOK_SECRET}
```

## Webhook Handling

### Webhook Registration

Webhooks should be registered to notify the harness of:
- Push events (new commits to test)
- Pull/merge request events (test before merge)
- Tag events (test releases)

Registration can be:
- Manual: admin configures webhooks in each repo
- Automatic: harness uses API to register webhooks

### Webhook Processing Flow

```
Git Provider                     Control Plane
     │                                │
     │  POST /webhooks/github         │
     │───────────────────────────────►│
     │                                │
     │                                │ 1. Validate signature
     │                                │ 2. Parse event
     │                                │ 3. Identify repository
     │                                │ 4. Determine tests to run
     │                                │ 5. Create test run
     │                                │
     │         202 Accepted           │
     │◄───────────────────────────────│
```

### Event Types to Handle

**Push Event**:
- Branch push: run tests for that branch
- Default branch push: run full test suite
- Consider: only run tests affected by changed files

**Pull/Merge Request Event**:
- Opened: run tests
- Synchronized (new commits): run tests again
- Closed/merged: no action (or cleanup)

**Tag Event**:
- New tag: run release test suite
- Consider: different test suites for releases vs. branches

## Clone Operations

When agents need to clone repositories:

1. Agent requests clone credentials from control plane
2. Control plane uses Git Provider to get credentials
3. Credentials returned to agent (short-lived if possible)
4. Agent clones using HTTPS with token auth

**Credential lifetime**:
- Prefer short-lived tokens (1 hour)
- For GitHub App: generate installation token on-demand
- For PAT: can't control lifetime, use sparingly

**Clone methods**:
- HTTPS with token: `https://x-access-token:{token}@github.com/org/repo.git`
- SSH with deploy key: requires key distribution to agents

Recommendation: HTTPS with short-lived tokens for simplicity and security.

## Caching Strategy

To reduce API calls and respect rate limits:

**Repository list cache**:
- Cache org repository list for 5 minutes
- Invalidate on webhook event or manual refresh

**File content cache**:
- Cache manifest content keyed by (repo, path, SHA)
- SHA-based cache is indefinitely valid (immutable)
- Branch-based requests: resolve to SHA, then cache

**Ref cache**:
- Cache ref list for 1 minute
- Invalidate on push webhook

## Error Handling

**Rate limiting**:
- Detect rate limit responses (HTTP 429, X-RateLimit headers)
- Implement backoff and retry
- Log warnings, alert if persistent

**Authentication failures**:
- Token expired: refresh or alert
- Permissions changed: alert operator

**Network errors**:
- Retry with backoff
- Circuit breaker for repeated failures

**Not found errors**:
- Repo deleted or access revoked: remove from registry
- File not found: mark service as having no tests

## Future Considerations

**Multi-provider workflows**:
- Tests in one provider, code in another
- Cross-provider dependencies

**Monorepo support**:
- Multiple services in one repo
- Detect which services changed based on file paths
- Run subset of tests based on changes

**Self-hosted providers**:
- GitHub Enterprise: same API, different base URL
- GitLab self-hosted: common, ensure base URL configurable
- Gitea/Gogs: compatible API, could add adapters
