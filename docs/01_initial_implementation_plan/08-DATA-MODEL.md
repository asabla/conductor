# Data Model

## Overview

This document defines the data structures used throughout the test harness system. It covers the database schema for the control plane, the canonical formats for test results, and the structures used in API communication.

## Database Schema (PostgreSQL)

### Core Entities

#### services

Represents a registered service/project with tests.

```sql
CREATE TABLE services (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            VARCHAR(255) NOT NULL UNIQUE,
    display_name    VARCHAR(255),
    owner           VARCHAR(255),
    
    -- Repository reference
    git_provider    VARCHAR(50) NOT NULL,
    git_org         VARCHAR(255) NOT NULL,
    git_repo        VARCHAR(255) NOT NULL,
    manifest_path   VARCHAR(500) DEFAULT '.testharness.yaml',
    
    -- Configuration
    network_zones   TEXT[],
    default_timeout INTERVAL DEFAULT '30 minutes',
    default_env     JSONB DEFAULT '{}',
    
    -- Contact
    contact_slack   VARCHAR(255),
    contact_email   VARCHAR(255),
    
    -- Sync tracking
    manifest_sha    VARCHAR(40),
    last_synced_at  TIMESTAMP WITH TIME ZONE,
    sync_error      TEXT,
    
    -- Status
    active          BOOLEAN DEFAULT true,
    
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_services_git ON services(git_provider, git_org, git_repo);
CREATE INDEX idx_services_owner ON services(owner);
```

#### test_definitions

Individual test definitions from manifests.

```sql
CREATE TABLE test_definitions (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    service_id          UUID NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    
    name                VARCHAR(255) NOT NULL,
    description         TEXT,
    tags                TEXT[],
    
    -- Execution
    execution_type      VARCHAR(20) NOT NULL, -- 'subprocess' or 'container'
    execution_config    JSONB NOT NULL,       -- type-specific configuration
    
    -- Results
    result_file         VARCHAR(500) NOT NULL,
    result_format       VARCHAR(50) NOT NULL, -- 'junit', 'jest', 'playwright', etc.
    artifact_paths      TEXT[],
    
    -- Settings
    timeout             INTERVAL,
    env                 JSONB DEFAULT '{}',
    depends_on          TEXT[],
    requires_services   TEXT[],
    exclusive           BOOLEAN DEFAULT false,
    retries             INTEGER DEFAULT 0,
    allow_failure       BOOLEAN DEFAULT false,
    
    created_at          TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at          TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    UNIQUE(service_id, name)
);

CREATE INDEX idx_test_definitions_service ON test_definitions(service_id);
CREATE INDEX idx_test_definitions_tags ON test_definitions USING GIN(tags);
```

#### test_runs

A single invocation of tests (may include multiple test definitions).

```sql
CREATE TABLE test_runs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    
    -- What was tested
    service_id      UUID REFERENCES services(id),
    git_ref         VARCHAR(255) NOT NULL,
    git_sha         VARCHAR(40) NOT NULL,
    
    -- Execution context
    agent_id        UUID REFERENCES agents(id),
    triggered_by    VARCHAR(50) NOT NULL,  -- 'webhook', 'schedule', 'manual', 'api'
    trigger_details JSONB,                  -- webhook event, user ID, etc.
    
    -- Status
    status          VARCHAR(20) NOT NULL DEFAULT 'pending',
    -- 'pending', 'running', 'passed', 'failed', 'error', 'cancelled', 'timeout'
    
    -- Timing
    queued_at       TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    started_at      TIMESTAMP WITH TIME ZONE,
    completed_at    TIMESTAMP WITH TIME ZONE,
    
    -- Summary (populated on completion)
    total_tests     INTEGER,
    passed          INTEGER,
    failed          INTEGER,
    skipped         INTEGER,
    errors          INTEGER,
    duration_ms     INTEGER,
    
    -- Configuration snapshot (in case definitions change)
    test_config     JSONB,
    
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_test_runs_service ON test_runs(service_id);
CREATE INDEX idx_test_runs_status ON test_runs(status);
CREATE INDEX idx_test_runs_created ON test_runs(created_at DESC);
CREATE INDEX idx_test_runs_git ON test_runs(git_sha);
```

#### test_results

Individual test results within a run.

```sql
CREATE TABLE test_results (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id              UUID NOT NULL REFERENCES test_runs(id) ON DELETE CASCADE,
    test_definition_id  UUID REFERENCES test_definitions(id),
    
    -- Test identification
    name                VARCHAR(500) NOT NULL,
    suite               VARCHAR(500),
    
    -- Result
    status              VARCHAR(20) NOT NULL, -- 'pass', 'fail', 'skip', 'error'
    duration_ms         INTEGER,
    
    -- Failure details
    error_message       TEXT,
    stack_trace         TEXT,
    
    -- Additional metadata
    metadata            JSONB DEFAULT '{}',
    
    -- Retry tracking
    attempt             INTEGER DEFAULT 1,
    
    created_at          TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_test_results_run ON test_results(run_id);
CREATE INDEX idx_test_results_status ON test_results(status);
CREATE INDEX idx_test_results_name ON test_results(name);
```

#### artifacts

Files collected during test execution.

```sql
CREATE TABLE artifacts (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id          UUID NOT NULL REFERENCES test_runs(id) ON DELETE CASCADE,
    result_id       UUID REFERENCES test_results(id) ON DELETE CASCADE,
    
    name            VARCHAR(500) NOT NULL,
    path            VARCHAR(500) NOT NULL,
    content_type    VARCHAR(100),
    size_bytes      BIGINT,
    
    -- Storage location
    storage_backend VARCHAR(50) NOT NULL,  -- 's3', 'local', etc.
    storage_key     VARCHAR(1000) NOT NULL,
    
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_artifacts_run ON artifacts(run_id);
CREATE INDEX idx_artifacts_result ON artifacts(result_id);
```

#### agents

Registered agents.

```sql
CREATE TABLE agents (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    
    name            VARCHAR(255) NOT NULL,
    version         VARCHAR(50),
    
    -- Capabilities
    network_zones   TEXT[],
    runtimes        JSONB,      -- [{name: "python", version: "3.11"}, ...]
    max_parallel    INTEGER DEFAULT 4,
    docker_available BOOLEAN DEFAULT false,
    resources       JSONB,      -- {cpu_cores: 4, memory_mb: 8192, disk_mb: 50000}
    
    -- Status
    status          VARCHAR(20) NOT NULL DEFAULT 'offline',
    -- 'idle', 'busy', 'draining', 'offline'
    last_heartbeat  TIMESTAMP WITH TIME ZONE,
    current_runs    UUID[],     -- run IDs currently executing
    
    -- Authentication
    token_hash      VARCHAR(255),
    
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_agents_status ON agents(status);
CREATE INDEX idx_agents_zones ON agents USING GIN(network_zones);
```

#### notification_rules

Rules for sending notifications.

```sql
CREATE TABLE notification_rules (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    
    name            VARCHAR(255) NOT NULL,
    enabled         BOOLEAN DEFAULT true,
    
    -- Conditions
    trigger_on      VARCHAR(50) NOT NULL,  -- 'failure', 'recovery', 'always'
    service_filter  UUID[],                 -- specific services, or null for all
    tag_filter      TEXT[],                 -- specific tags, or null for all
    
    -- Destination
    channel_type    VARCHAR(50) NOT NULL,   -- 'slack', 'email', 'webhook'
    channel_config  JSONB NOT NULL,         -- {webhook_url: ...} or {email: ...}
    
    -- Rate limiting
    cooldown_minutes INTEGER DEFAULT 15,
    last_sent_at    TIMESTAMP WITH TIME ZONE,
    
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

#### scheduled_runs

Scheduled/recurring test runs.

```sql
CREATE TABLE scheduled_runs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    
    name            VARCHAR(255) NOT NULL,
    enabled         BOOLEAN DEFAULT true,
    
    -- What to run
    service_id      UUID REFERENCES services(id),  -- specific service, or null
    tag_filter      TEXT[],                         -- tests matching these tags
    git_ref         VARCHAR(255) DEFAULT 'main',   -- branch/tag to test
    
    -- Schedule
    cron_expression VARCHAR(100) NOT NULL,
    timezone        VARCHAR(50) DEFAULT 'UTC',
    
    -- Tracking
    last_run_at     TIMESTAMP WITH TIME ZONE,
    next_run_at     TIMESTAMP WITH TIME ZONE,
    
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

### Analytics Tables

#### daily_stats

Pre-aggregated daily statistics for efficient trending.

```sql
CREATE TABLE daily_stats (
    date            DATE NOT NULL,
    service_id      UUID REFERENCES services(id),
    
    total_runs      INTEGER DEFAULT 0,
    passed_runs     INTEGER DEFAULT 0,
    failed_runs     INTEGER DEFAULT 0,
    
    total_tests     INTEGER DEFAULT 0,
    passed_tests    INTEGER DEFAULT 0,
    failed_tests    INTEGER DEFAULT 0,
    
    total_duration_ms BIGINT DEFAULT 0,
    
    PRIMARY KEY (date, service_id)
);

CREATE INDEX idx_daily_stats_date ON daily_stats(date DESC);
```

#### flaky_tests

Tracking for flaky test detection.

```sql
CREATE TABLE flaky_tests (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    test_definition_id  UUID NOT NULL REFERENCES test_definitions(id),
    
    -- Detection window
    window_start        DATE NOT NULL,
    window_end          DATE NOT NULL,
    
    -- Metrics
    total_runs          INTEGER NOT NULL,
    pass_count          INTEGER NOT NULL,
    fail_count          INTEGER NOT NULL,
    flip_count          INTEGER NOT NULL,  -- transitions between pass/fail
    flakiness_score     FLOAT NOT NULL,    -- 0.0 to 1.0
    
    -- Status
    quarantined         BOOLEAN DEFAULT false,
    issue_url           VARCHAR(500),
    
    created_at          TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at          TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_flaky_tests_score ON flaky_tests(flakiness_score DESC);
```

## Canonical Result Format

When results are collected from various formats (JUnit, Jest, etc.), they are normalized to this canonical format.

### TestRunResult

```json
{
  "run_id": "uuid",
  "status": "passed|failed|error|cancelled|timeout",
  "summary": {
    "total": 42,
    "passed": 40,
    "failed": 1,
    "skipped": 1,
    "errors": 0,
    "duration_ms": 12345
  },
  "tests": [
    {
      "id": "uuid",
      "name": "test_user_creation",
      "suite": "UserServiceTest",
      "status": "pass|fail|skip|error",
      "duration_ms": 123,
      "error": {
        "message": "AssertionError: expected 200, got 500",
        "stack_trace": "...",
        "type": "AssertionError"
      },
      "metadata": {
        "file": "tests/test_user.py",
        "line": 42
      },
      "artifacts": [
        {
          "name": "screenshot.png",
          "path": "screenshots/test_user_creation.png",
          "content_type": "image/png"
        }
      ]
    }
  ],
  "artifacts": [
    {
      "name": "coverage.xml",
      "path": "coverage/coverage.xml",
      "content_type": "application/xml"
    }
  ]
}
```

## Result Format Parsers

### JUnit XML

Input:
```xml
<testsuite name="UserTests" tests="3" failures="1" errors="0" skipped="0" time="1.234">
  <testcase classname="UserTests" name="test_create" time="0.5"/>
  <testcase classname="UserTests" name="test_update" time="0.3">
    <failure message="AssertionError">Expected 200, got 500
      at test_user.py:42</failure>
  </testcase>
  <testcase classname="UserTests" name="test_delete" time="0.4"/>
</testsuite>
```

Mapping:
- `testsuite.name` → suite
- `testcase.name` → name
- `testcase.time` → duration_ms (convert to ms)
- No failure/error → status: pass
- `<failure>` → status: fail, error.message, error.stack_trace
- `<error>` → status: error
- `<skipped>` → status: skip

### Jest JSON

Input:
```json
{
  "numTotalTests": 3,
  "numPassedTests": 2,
  "numFailedTests": 1,
  "testResults": [
    {
      "name": "/path/to/test.js",
      "assertionResults": [
        {
          "fullName": "UserService should create user",
          "status": "passed",
          "duration": 50
        },
        {
          "fullName": "UserService should handle errors",
          "status": "failed",
          "failureMessages": ["Expected 200 to equal 500"]
        }
      ]
    }
  ]
}
```

### Playwright JSON

Input:
```json
{
  "suites": [
    {
      "title": "Login",
      "specs": [
        {
          "title": "should login successfully",
          "tests": [
            {
              "status": "expected",
              "duration": 5000,
              "results": [{"status": "passed"}]
            }
          ]
        }
      ]
    }
  ]
}
```

### Go test JSON

Input (one JSON object per line):
```json
{"Action":"run","Test":"TestCreate"}
{"Action":"output","Test":"TestCreate","Output":"=== RUN   TestCreate\n"}
{"Action":"pass","Test":"TestCreate","Elapsed":0.5}
```

## API Data Transfer Objects

### RunSummary (list view)

```json
{
  "id": "uuid",
  "service": {
    "id": "uuid",
    "name": "user-service"
  },
  "git_ref": "main",
  "git_sha": "abc1234",
  "status": "failed",
  "triggered_by": "webhook",
  "started_at": "2024-01-15T10:30:00Z",
  "completed_at": "2024-01-15T10:35:00Z",
  "duration_ms": 300000,
  "summary": {
    "total": 42,
    "passed": 40,
    "failed": 2,
    "skipped": 0
  }
}
```

### RunDetails (detail view)

```json
{
  "id": "uuid",
  "service": {
    "id": "uuid",
    "name": "user-service",
    "owner": "platform-team"
  },
  "git_ref": "main",
  "git_sha": "abc1234def5678...",
  "status": "failed",
  "triggered_by": "webhook",
  "trigger_details": {
    "event": "push",
    "sender": "developer@example.com"
  },
  "agent": {
    "id": "uuid",
    "name": "agent-zone-a-1"
  },
  "queued_at": "2024-01-15T10:29:00Z",
  "started_at": "2024-01-15T10:30:00Z",
  "completed_at": "2024-01-15T10:35:00Z",
  "summary": {
    "total": 42,
    "passed": 40,
    "failed": 2,
    "skipped": 0,
    "duration_ms": 300000
  },
  "results": [
    // TestResult objects
  ],
  "artifacts": [
    // Artifact objects
  ]
}
```

### AgentStatus

```json
{
  "id": "uuid",
  "name": "agent-zone-a-1",
  "status": "busy",
  "version": "1.2.3",
  "network_zones": ["internal-a"],
  "capabilities": {
    "runtimes": [
      {"name": "python", "version": "3.11"},
      {"name": "node", "version": "20"}
    ],
    "docker_available": true,
    "max_parallel": 4
  },
  "resources": {
    "cpu_percent": 45.2,
    "memory_percent": 62.1,
    "disk_percent": 23.4
  },
  "current_runs": ["uuid1", "uuid2"],
  "last_heartbeat": "2024-01-15T10:34:55Z"
}
```

## Data Retention

### Retention Policies

| Data Type | Default Retention | Notes |
|-----------|-------------------|-------|
| Test runs | 90 days | Full details including results |
| Test results | 90 days | Deleted with parent run |
| Artifacts | 30 days | Large files, shorter retention |
| Daily stats | 2 years | Aggregated, small footprint |
| Flaky test tracking | 90 days | Rolling window |
| Agent history | 30 days | Connection/status history |

### Cleanup Jobs

Scheduled jobs to enforce retention:
- Daily: Delete artifacts older than retention period
- Daily: Delete test runs older than retention period (cascades to results)
- Weekly: Vacuum database to reclaim space

### Archival

For compliance or long-term analysis:
- Export old runs to cold storage (S3 Glacier, etc.) before deletion
- Keep aggregated stats longer than raw data
