# Control Plane

## Overview

The control plane is the central coordination service of the test harness. It manages the test registry, schedules work, coordinates agents, stores results, and provides APIs for external consumers.

## Internal Components

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           CONTROL PLANE                                 │
│                                                                         │
│  ┌──────────────────────────────────────────────────────────────────┐  │
│  │                        API Layer                                  │  │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────┐   │  │
│  │  │ gRPC Server │  │ HTTP Server │  │  WebSocket Server       │   │  │
│  │  │ (agents)    │  │ (REST API)  │  │  (dashboard real-time)  │   │  │
│  │  └─────────────┘  └─────────────┘  └─────────────────────────┘   │  │
│  └──────────────────────────────────────────────────────────────────┘  │
│                                    │                                    │
│  ┌─────────────────────────────────┼────────────────────────────────┐  │
│  │                          Core Services                            │  │
│  │                                                                   │  │
│  │  ┌─────────────────┐    ┌─────────────────┐    ┌──────────────┐  │  │
│  │  │    Scheduler    │    │  Agent Manager  │    │   Registry   │  │  │
│  │  │                 │    │                 │    │   Service    │  │  │
│  │  └─────────────────┘    └─────────────────┘    └──────────────┘  │  │
│  │                                                                   │  │
│  │  ┌─────────────────┐    ┌─────────────────┐    ┌──────────────┐  │  │
│  │  │ Result Collector│    │  Git Sync       │    │ Notification │  │  │
│  │  │                 │    │  Service        │    │   Service    │  │  │
│  │  └─────────────────┘    └─────────────────┘    └──────────────┘  │  │
│  │                                                                   │  │
│  └───────────────────────────────────────────────────────────────────┘  │
│                                    │                                    │
│  ┌─────────────────────────────────┼────────────────────────────────┐  │
│  │                         Storage Layer                             │  │
│  │                                                                   │  │
│  │  ┌─────────────────┐    ┌─────────────────┐    ┌──────────────┐  │  │
│  │  │   PostgreSQL    │    │ Artifact Store  │    │    Cache     │  │  │
│  │  │   (primary DB)  │    │   (S3/MinIO)    │    │   (Redis)    │  │  │
│  │  └─────────────────┘    └─────────────────┘    └──────────────┘  │  │
│  │                                                                   │  │
│  └───────────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────┘
```

## Core Services

### Scheduler Service

**Purpose**: Decides when and where tests should run.

**Responsibilities**:
- Receives test run requests (from webhooks, API, schedules)
- Determines which tests to run based on the request (all tests, specific service, specific tags)
- Matches tests to agents based on requirements (network zone, capabilities)
- Manages the work queue
- Handles retries for failed tests
- Enforces concurrency limits

**Key behaviors**:
- Tests requiring specific network access are only assigned to agents in that zone
- If no capable agent is available, work is queued until one becomes available
- Supports priority levels (critical tests run before nightly batch tests)
- Respects test dependencies (if test B requires test A, schedule A first)

**Inputs**:
- Test run requests (which tests, which ref, priority)
- Agent availability from Agent Manager
- Test requirements from Registry

**Outputs**:
- Work assignments sent to Agent Manager for dispatch

### Agent Manager

**Purpose**: Maintains agent connections and dispatches work.

**Responsibilities**:
- Accepts gRPC connections from agents
- Tracks agent state (connected, busy, idle, offline)
- Records agent capabilities (network zones, runtimes, resources)
- Dispatches work assignments from Scheduler to agents
- Receives results from agents and forwards to Result Collector
- Monitors agent health via heartbeats
- Handles agent disconnection and reconnection

**Key behaviors**:
- Agents are considered offline if no heartbeat received within timeout (default 60s)
- Work assigned to an agent that disconnects is marked failed (or re-queued based on policy)
- Maintains a bidirectional gRPC stream per agent
- Can drain an agent (stop sending new work, wait for current work to complete)

**Agent state machine**:
```
CONNECTING → IDLE ←→ BUSY
              ↓
          DRAINING → OFFLINE
              ↓
          OFFLINE
```

### Registry Service

**Purpose**: Maintains the catalog of known tests.

**Responsibilities**:
- Stores test definitions parsed from manifests
- Associates tests with repositories, services, and owners
- Provides query interface (find tests by service, by tag, by repo)
- Tracks manifest versions (which commit the manifest was read from)
- Validates manifest syntax and semantics

**Key behaviors**:
- Registry is the source of truth for what tests exist
- Updated by Git Sync Service when manifests change
- Supports versioning—can retrieve test definitions as they existed at a specific commit

**Data managed**:
- Services (name, repo, owner, network zone)
- Test suites (name, service, type, tags)
- Test cases (name, suite, command, runtime requirements, timeout)

### Result Collector

**Purpose**: Receives, normalizes, and stores test results.

**Responsibilities**:
- Accepts result streams from Agent Manager
- Parses various result formats (JUnit XML, Jest JSON, etc.)
- Normalizes results into canonical schema
- Stores results in database
- Uploads artifacts to artifact storage
- Triggers notifications on completion

**Key behaviors**:
- Handles partial results (streaming) and final results
- Associates results with test run, service, agent, git ref
- Computes aggregate metrics (pass rate, duration)
- Detects regressions (test that passed now fails)

**Result normalization**:
The collector must handle multiple input formats and produce a consistent output. Parsers are pluggable:
- JUnit XML parser
- Jest JSON parser
- Go test JSON parser
- Playwright JSON parser
- TAP format parser
- Generic JSON (configurable schema)

### Git Sync Service

**Purpose**: Discovers and synchronizes test manifests from Git repositories.

**Responsibilities**:
- Periodically scans registered organizations/repositories
- Fetches `.testharness.yaml` files from repositories
- Detects changes to manifests
- Updates Registry Service with new/changed test definitions
- Handles webhook events for immediate sync

**Key behaviors**:
- Uses Git Provider Abstraction to work with GitHub/GitLab/etc.
- Respects rate limits of Git providers
- Caches manifest content to reduce API calls
- Supports manual trigger to sync a specific repo

**Sync process**:
1. List repositories in configured organizations
2. For each repo, check for `.testharness.yaml` at repo root (or configured path)
3. Compare file hash to last known hash
4. If changed, fetch and parse manifest
5. Validate manifest
6. Update Registry Service

### Notification Service

**Purpose**: Sends alerts and notifications based on test results.

**Responsibilities**:
- Evaluates notification rules against completed test runs
- Sends notifications to configured channels (Slack, email, webhooks)
- Supports different notification triggers (failure, recovery, threshold breach)
- Rate limits notifications to avoid spam

**Notification triggers**:
- Test run failed (any test failed)
- Test run recovered (was failing, now passing)
- Flaky test detected (intermittent pass/fail)
- Test duration exceeded threshold
- Agent went offline

**Notification channels**:
- Slack (via incoming webhook or app)
- Email (via SMTP)
- Generic webhook (POST to configured URL)
- PagerDuty (for critical failures)

## API Layer

### gRPC Server (Agent Communication)

Handles all agent communication:
- `Register`: Agent announces itself and capabilities
- `Heartbeat`: Agent confirms liveness
- `WorkStream`: Bidirectional stream for work dispatch and results

This is internal API, not exposed externally.

### HTTP Server (REST API)

Public API for dashboard, CI integrations, and external consumers:

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/api/runs` | POST | Trigger a new test run |
| `/api/runs` | GET | List test runs (with filters) |
| `/api/runs/{id}` | GET | Get test run details |
| `/api/runs/{id}/results` | GET | Get results for a run |
| `/api/services` | GET | List registered services |
| `/api/services/{id}` | GET | Get service details and recent results |
| `/api/agents` | GET | List agents and status |
| `/api/agents/{id}` | GET | Get agent details |
| `/api/agents/{id}/drain` | POST | Drain an agent |
| `/api/webhooks/github` | POST | GitHub webhook receiver |
| `/api/webhooks/gitlab` | POST | GitLab webhook receiver |
| `/api/health` | GET | Health check |

### WebSocket Server (Real-time Updates)

Provides real-time updates to dashboard:
- Test run progress (tests starting, completing)
- Agent status changes
- New results as they arrive

Clients subscribe to channels:
- `runs:{run_id}` - Updates for a specific run
- `agents` - All agent status updates
- `services:{service_id}` - Results for a specific service

## Configuration

The control plane is configured via environment variables and/or a config file.

**Required configuration**:
- Database connection string
- Artifact storage credentials (S3 bucket, access keys)
- Git provider credentials (API tokens)
- Agent authentication secret

**Optional configuration**:
- Bind addresses for gRPC/HTTP/WebSocket servers
- Heartbeat timeout
- Default test timeout
- Notification channel configs
- Redis connection (if used)
- Log level and format

## Startup Sequence

1. Load configuration
2. Connect to database, run migrations if needed
3. Connect to artifact storage
4. Initialize Git provider adapters
5. Load agent authentication credentials
6. Start gRPC server (agent connections)
7. Start HTTP server (REST API)
8. Start WebSocket server (real-time updates)
9. Start Git Sync Service (background)
10. Start Scheduler (begin processing work queue)
11. Report healthy

## Shutdown Sequence

1. Stop accepting new requests
2. Drain scheduler (stop assigning new work)
3. Wait for in-progress work (with timeout)
4. Close agent connections gracefully
5. Close database connections
6. Exit

## Observability

**Metrics** (exposed via Prometheus endpoint):
- `test_runs_total` (counter, by status)
- `test_results_total` (counter, by status, service)
- `agent_connected_count` (gauge)
- `work_queue_depth` (gauge)
- `test_duration_seconds` (histogram, by service)
- `api_request_duration_seconds` (histogram, by endpoint)

**Logs**:
- Structured JSON logging
- Correlation IDs for request tracing
- Log levels: debug, info, warn, error

**Traces**:
- OpenTelemetry instrumentation
- Trace test run from trigger through execution to result storage
