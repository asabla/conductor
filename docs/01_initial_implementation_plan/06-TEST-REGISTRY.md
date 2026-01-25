# Test Registry and Manifests

## Overview

The test registry is the control plane's knowledge of what tests exist across the organization. Tests are defined in manifest files within each repository and discovered automatically by the harness.

## Manifest File

Each repository that contains tests includes a `.testharness.yaml` file at the repository root (or a configured path).

### Manifest Schema

```yaml
# .testharness.yaml

# Required: Service/project identifier
service: user-service

# Optional: Human-readable name
name: User Service

# Optional: Team or owner
owner: platform-team

# Optional: Contact for notifications
contact:
  slack: "#platform-alerts"
  email: platform@example.com

# Optional: Network zone(s) this service lives in
# Tests will only run on agents that can reach this zone
network_zones:
  - internal-a

# Optional: Default timeout for all tests (can be overridden per-test)
default_timeout: 10m

# Optional: Default environment variables for all tests
default_env:
  LOG_LEVEL: debug

# Required: Test definitions
tests:
  - name: unit
    # ... test definition
  
  - name: integration
    # ... test definition
```

### Test Definition Schema

```yaml
tests:
  - name: unit                          # Required: unique within service
    description: "Unit tests"           # Optional: human-readable description
    tags:                               # Optional: for filtering
      - unit
      - fast
    
    # Execution configuration (choose one type)
    type: subprocess                    # or "container"
    
    # --- Subprocess execution ---
    command: pytest tests/unit          # Required for subprocess
    args:                               # Optional: additional arguments
      - -v
      - --junitxml=results.xml
    working_dir: .                      # Optional: relative to repo root
    runtime: python:3.11                # Optional: required runtime
    
    # --- Container execution ---
    # type: container
    # image: python:3.11-slim           # Required for container
    # command: pytest tests/unit        # Optional: override entrypoint
    # mounts:                           # Optional: additional mounts
    #   - source: ./fixtures
    #     target: /fixtures
    #     readonly: true
    # network_mode: host                # Optional: host, bridge, none
    # resource_limits:                  # Optional
    #   cpu: 2
    #   memory: 4G
    
    # Result collection
    result_file: results.xml            # Required: where test output is written
    result_format: junit                # Required: junit, jest, playwright, tap, json
    
    # Optional: paths to collect as artifacts
    artifacts:
      - screenshots/
      - coverage/
    
    # Optional: override defaults
    timeout: 5m
    env:
      DATABASE_URL: postgres://localhost/test
    
    # Optional: test dependencies
    # These tests must pass before this test runs
    depends_on:
      - unit
    
    # Optional: services that must be reachable
    requires_services:
      - postgres
      - redis
    
    # Optional: run this test exclusively (no parallel tests)
    exclusive: false
    
    # Optional: retry failed tests
    retries: 0
    
    # Optional: allow failure without failing the run
    allow_failure: false
```

### Complete Example

```yaml
service: order-service
name: Order Service
owner: commerce-team
contact:
  slack: "#commerce-alerts"
network_zones:
  - internal-commerce
default_timeout: 15m

tests:
  - name: unit
    description: "Fast unit tests, no external dependencies"
    tags: [unit, fast]
    type: subprocess
    command: go test ./...
    args:
      - -v
      - -race
      - -coverprofile=coverage.out
    result_file: test-results.json
    result_format: json
    artifacts:
      - coverage.out
    timeout: 5m

  - name: integration
    description: "Integration tests against test database"
    tags: [integration]
    type: container
    image: order-service-test:${GIT_SHA}
    command: go test ./integration/...
    args: [-v]
    env:
      DATABASE_URL: ${TEST_DATABASE_URL}
      REDIS_URL: ${TEST_REDIS_URL}
    result_file: /output/results.xml
    result_format: junit
    timeout: 20m
    depends_on:
      - unit
    requires_services:
      - order-db
      - redis

  - name: e2e
    description: "End-to-end browser tests"
    tags: [e2e, slow]
    type: container
    image: mcr.microsoft.com/playwright:latest
    command: npx playwright test
    env:
      BASE_URL: https://staging.example.com
    result_file: playwright-report/results.json
    result_format: playwright
    artifacts:
      - playwright-report/
      - test-results/
    timeout: 30m
    depends_on:
      - integration
    retries: 2  # E2E tests can be flaky

  - name: load
    description: "Load tests - runs nightly only"
    tags: [load, nightly]
    type: container
    image: grafana/k6:latest
    command: k6 run load-tests/main.js
    result_file: results.json
    result_format: json
    artifacts:
      - results.json
    timeout: 1h
    exclusive: true  # Don't run other tests while load testing
    allow_failure: true  # Informational, don't fail pipeline
```

## Registry Data Model

The control plane stores parsed manifests in a structured registry.

### Entities

**Service**
```
Service {
  id: UUID
  name: string (unique)
  display_name: string
  owner: string
  contact: Contact
  repository: RepositoryRef
  network_zones: []string
  default_timeout: Duration
  default_env: map<string, string>
  manifest_sha: string (commit SHA manifest was read from)
  last_synced: Timestamp
  created_at: Timestamp
  updated_at: Timestamp
}
```

**TestSuite** (grouping of related tests)
```
TestSuite {
  id: UUID
  service_id: UUID (foreign key)
  name: string
  description: string
  tags: []string
  created_at: Timestamp
  updated_at: Timestamp
}
```

**TestDefinition**
```
TestDefinition {
  id: UUID
  service_id: UUID (foreign key)
  suite_id: UUID (foreign key, optional)
  name: string
  description: string
  tags: []string
  execution_type: ExecutionType
  execution_config: JSON (type-specific config)
  result_file: string
  result_format: ResultFormat
  artifacts: []string
  timeout: Duration
  env: map<string, string>
  depends_on: []string (test names)
  requires_services: []string
  exclusive: bool
  retries: int
  allow_failure: bool
  created_at: Timestamp
  updated_at: Timestamp
}
```

**RepositoryRef**
```
RepositoryRef {
  provider: string (github, gitlab, etc.)
  org: string
  name: string
  default_branch: string
  manifest_path: string (default: .testharness.yaml)
}
```

## Discovery and Sync

### Initial Discovery

1. Admin registers an organization with control plane
2. Git Sync Service lists all repositories in organization
3. For each repository, check for manifest file at default path
4. If found, parse and validate manifest
5. Create Service and TestDefinition records in registry

### Continuous Sync

**Periodic scan** (default: every 15 minutes):
1. List all repositories in registered organizations
2. For each repo with a manifest, fetch current manifest
3. Compare hash to stored manifest_sha
4. If changed, re-parse and update registry

**Webhook-triggered sync**:
1. Receive push webhook for a repository
2. If push includes changes to manifest file, trigger immediate sync
3. Fetch new manifest at pushed ref
4. Update registry

### Manifest Validation

On sync, manifests are validated:

**Syntax validation**:
- Valid YAML
- Required fields present
- Field types correct

**Semantic validation**:
- Test names unique within service
- Referenced dependencies exist
- Result formats recognized
- Timeouts within limits

**Warnings** (logged but not rejected):
- Unknown fields (may be future features)
- Deprecated fields

### Handling Errors

| Error | Behavior |
|-------|----------|
| Manifest not found | Service removed from registry (or marked inactive) |
| Invalid YAML | Reject, keep previous version, alert owner |
| Validation failure | Reject, keep previous version, alert owner |
| Git provider error | Retry, keep previous version |

## Querying the Registry

The registry supports queries needed by the scheduler and dashboard.

### Query Patterns

**By service**:
```
GetTestsByService(service_id) → []TestDefinition
```

**By tags**:
```
GetTestsByTags(tags: []string, match: ALL|ANY) → []TestDefinition
```

**By network zone**:
```
GetTestsRequiringZone(zone: string) → []TestDefinition
```

**By repository/ref**:
```
GetTestsForRef(repo: RepositoryRef, ref: string) → []TestDefinition
```

**Runnable tests** (considering dependencies):
```
GetRunnableTests(service_id, already_passed: []string) → []TestDefinition
```

## Variable Substitution

Manifests support variable substitution for dynamic values.

### Built-in Variables

| Variable | Description |
|----------|-------------|
| `${GIT_SHA}` | Full commit SHA being tested |
| `${GIT_SHORT_SHA}` | Short (7 char) commit SHA |
| `${GIT_REF}` | Branch or tag name |
| `${GIT_BRANCH}` | Branch name (empty for tags) |
| `${GIT_TAG}` | Tag name (empty for branches) |
| `${SERVICE_NAME}` | Service name from manifest |
| `${RUN_ID}` | Unique test run identifier |
| `${TIMESTAMP}` | ISO timestamp of run start |

### Secret Variables

Secrets are referenced but not stored in manifests:
```yaml
env:
  DATABASE_URL: ${secrets.TEST_DATABASE_URL}
  API_KEY: ${secrets.EXTERNAL_API_KEY}
```

Secrets are:
- Stored in control plane's secret store
- Injected at execution time
- Never logged or displayed

### Environment-Specific Variables

```yaml
env:
  BASE_URL: ${env.BASE_URL}  # Provided by control plane based on target environment
```

## Versioning

The registry tracks manifest versions:
- `manifest_sha`: commit SHA the manifest was read from
- Allows running tests as defined at a specific point in time
- Historical queries: "what tests existed for commit X?"

### Accessing Historical Definitions

```
GetTestDefinitionsAtRef(service_id, ref: string) → []TestDefinition
```

This fetches the manifest from Git at the specified ref, parses it, and returns definitions without storing them (ephemeral).

## Schema Evolution

As the manifest schema evolves:

**Adding optional fields**: Safe, old manifests still valid.

**Adding required fields**: Requires migration period.
1. Add as optional
2. Warn if missing
3. Later, make required

**Removing fields**: Warn on use, ignore value.

**Changing field semantics**: Version the schema.
```yaml
schema_version: 2
```
