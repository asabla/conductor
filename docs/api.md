# API Documentation

This document describes the Conductor REST API, gRPC API, and WebSocket API.

## Table of Contents

- [REST API Overview](#rest-api-overview)
- [Authentication](#authentication)
- [Services API](#services-api)
- [Runs API](#runs-api)
- [Agents API](#agents-api)
- [Results API](#results-api)
- [Notifications API](#notifications-api)
- [gRPC API](#grpc-api)
- [WebSocket API](#websocket-api)

## REST API Overview

The REST API is available at `http://localhost:8080/api/v1/` by default.

### Base URL

```
https://your-conductor-instance.com/api/v1
```

### Content Type

All requests and responses use JSON:
```
Content-Type: application/json
```

### Pagination

List endpoints support pagination:

```bash
GET /api/v1/services?page_size=20&page_token=abc123
```

Response includes pagination info:
```json
{
  "services": [...],
  "pagination": {
    "total_count": 100,
    "page_size": 20,
    "next_page_token": "xyz789"
  }
}
```

### Error Responses

Errors return appropriate HTTP status codes with details:

```json
{
  "error": {
    "code": "NOT_FOUND",
    "message": "Service not found",
    "details": {
      "service_id": "abc123"
    }
  }
}
```

## Authentication

### API Keys

Include the API key in the `Authorization` header:

```bash
curl -H "Authorization: Bearer your-api-key" \
  https://conductor.example.com/api/v1/services
```

### JWT Tokens

For dashboard users, authenticate via the login endpoint:

```bash
# Login
curl -X POST https://conductor.example.com/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email": "user@example.com", "password": "secret"}'

# Response
{
  "token": "eyJhbGciOiJIUzI1NiIs...",
  "expires_at": "2024-01-16T12:00:00Z"
}

# Use token
curl -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIs..." \
  https://conductor.example.com/api/v1/services
```

## Services API

### Create Service

```http
POST /api/v1/services
```

Request:
```json
{
  "name": "my-service",
  "git_url": "https://github.com/org/my-service.git",
  "default_branch": "main",
  "network_zones": ["default"],
  "owner": "platform-team",
  "contact": {
    "email": "platform@example.com",
    "slack": "#platform-alerts"
  },
  "default_execution_type": "SUBPROCESS",
  "config_path": ".testharness.yaml",
  "labels": {
    "environment": "production",
    "team": "platform"
  }
}
```

Response:
```json
{
  "service": {
    "id": "svc_abc123",
    "name": "my-service",
    "git_url": "https://github.com/org/my-service.git",
    "default_branch": "main",
    "network_zones": ["default"],
    "owner": "platform-team",
    "active": true,
    "created_at": "2024-01-15T10:00:00Z",
    "test_count": 0
  }
}
```

### Get Service

```http
GET /api/v1/services/{service_id}
```

Query parameters:
- `include_tests` - Include test definitions (boolean)
- `include_recent_runs` - Include recent runs (boolean)

Response:
```json
{
  "service": {
    "id": "svc_abc123",
    "name": "my-service",
    "git_url": "https://github.com/org/my-service.git",
    "default_branch": "main",
    "network_zones": ["default"],
    "owner": "platform-team",
    "active": true,
    "created_at": "2024-01-15T10:00:00Z",
    "updated_at": "2024-01-15T12:00:00Z",
    "last_synced_at": "2024-01-15T12:00:00Z",
    "test_count": 5
  },
  "tests": [...],
  "recent_runs": [...]
}
```

### List Services

```http
GET /api/v1/services
```

Query parameters:
- `owner` - Filter by owner
- `network_zone` - Filter by network zone
- `query` - Search query
- `page_size` - Results per page (default: 20)
- `page_token` - Pagination token

Response:
```json
{
  "services": [
    {
      "id": "svc_abc123",
      "name": "my-service",
      ...
    }
  ],
  "pagination": {
    "total_count": 50,
    "page_size": 20,
    "next_page_token": "token123"
  }
}
```

### Update Service

```http
PATCH /api/v1/services/{service_id}
```

Request (only include fields to update):
```json
{
  "name": "new-name",
  "owner": "new-owner",
  "active": false
}
```

### Delete Service

```http
DELETE /api/v1/services/{service_id}
```

Query parameters:
- `delete_history` - Also delete run history (boolean)

### Sync Service

Trigger manifest discovery from repository:

```http
POST /api/v1/services/{service_id}/sync
```

Request:
```json
{
  "branch": "main",
  "delete_missing": true
}
```

Response:
```json
{
  "tests_added": 3,
  "tests_updated": 1,
  "tests_removed": 0,
  "errors": [],
  "synced_at": "2024-01-15T12:00:00Z"
}
```

## Runs API

### Create Run

```http
POST /api/v1/runs
```

Request:
```json
{
  "service_id": "svc_abc123",
  "branch": "main",
  "commit_sha": "abc123def456",
  "test_ids": ["test_1", "test_2"],
  "tags": ["unit"],
  "priority": 10,
  "environment": {
    "DEBUG": "true"
  }
}
```

Response:
```json
{
  "run": {
    "id": "run_xyz789",
    "service_id": "svc_abc123",
    "status": "PENDING",
    "branch": "main",
    "commit_sha": "abc123def456",
    "created_at": "2024-01-15T12:00:00Z"
  }
}
```

### Get Run

```http
GET /api/v1/runs/{run_id}
```

Query parameters:
- `include_results` - Include test results (boolean)
- `include_logs` - Include log output (boolean)

Response:
```json
{
  "run": {
    "id": "run_xyz789",
    "service_id": "svc_abc123",
    "status": "COMPLETED",
    "branch": "main",
    "commit_sha": "abc123def456",
    "agent_id": "agent_001",
    "started_at": "2024-01-15T12:00:05Z",
    "completed_at": "2024-01-15T12:05:00Z",
    "duration_ms": 295000,
    "summary": {
      "total": 10,
      "passed": 8,
      "failed": 1,
      "skipped": 1
    }
  },
  "results": [...],
  "logs": "..."
}
```

### List Runs

```http
GET /api/v1/runs
```

Query parameters:
- `service_id` - Filter by service
- `status` - Filter by status (PENDING, RUNNING, COMPLETED, FAILED, CANCELLED)
- `branch` - Filter by branch
- `from` - Start time (ISO 8601)
- `to` - End time (ISO 8601)
- `page_size` - Results per page
- `page_token` - Pagination token

### Cancel Run

```http
POST /api/v1/runs/{run_id}/cancel
```

Request:
```json
{
  "reason": "User requested cancellation"
}
```

### Retry Run

```http
POST /api/v1/runs/{run_id}/retry
```

Request:
```json
{
  "failed_only": true
}
```

## Agents API

### List Agents

```http
GET /api/v1/agents
```

Query parameters:
- `status` - Filter by status (ONLINE, OFFLINE, DRAINING)
- `network_zone` - Filter by network zone
- `labels` - Filter by labels (key=value)

Response:
```json
{
  "agents": [
    {
      "id": "agent_001",
      "name": "agent-zone-a-1",
      "status": "ONLINE",
      "network_zones": ["zone-a"],
      "capabilities": {
        "max_parallel": 4,
        "docker_available": true,
        "runtimes": ["node18", "python3.11", "go1.22"],
        "os": "linux",
        "arch": "amd64"
      },
      "resource_usage": {
        "cpu_percent": 45.2,
        "memory_percent": 62.1,
        "disk_percent": 35.8
      },
      "active_runs": ["run_xyz789"],
      "last_heartbeat": "2024-01-15T12:04:30Z",
      "connected_at": "2024-01-15T08:00:00Z"
    }
  ]
}
```

### Get Agent

```http
GET /api/v1/agents/{agent_id}
```

### Drain Agent

Request agent to stop accepting new work:

```http
POST /api/v1/agents/{agent_id}/drain
```

Request:
```json
{
  "reason": "Maintenance scheduled",
  "cancel_active": false,
  "deadline": "2024-01-15T14:00:00Z"
}
```

## Results API

### Get Results for Run

```http
GET /api/v1/runs/{run_id}/results
```

Response:
```json
{
  "results": [
    {
      "id": "result_001",
      "test_id": "test_unit",
      "test_name": "unit-tests",
      "status": "PASSED",
      "duration_ms": 5230,
      "retry_attempt": 0,
      "timestamp": "2024-01-15T12:01:00Z"
    },
    {
      "id": "result_002",
      "test_id": "test_integration",
      "test_name": "integration-tests",
      "status": "FAILED",
      "duration_ms": 12500,
      "error_message": "Connection refused",
      "stack_trace": "...",
      "retry_attempt": 0,
      "timestamp": "2024-01-15T12:02:30Z"
    }
  ]
}
```

### Get Artifacts for Run

```http
GET /api/v1/runs/{run_id}/artifacts
```

Response:
```json
{
  "artifacts": [
    {
      "id": "art_001",
      "name": "screenshot.png",
      "path": "test-results/screenshot.png",
      "content_type": "image/png",
      "size": 125000,
      "download_url": "https://storage.example.com/artifacts/art_001",
      "created_at": "2024-01-15T12:03:00Z"
    }
  ]
}
```

## Notifications API

### List Notification Channels

```http
GET /api/v1/notifications/channels
```

### Create Channel

```http
POST /api/v1/notifications/channels
```

Request (Slack):
```json
{
  "name": "slack-alerts",
  "type": "SLACK",
  "config": {
    "webhook_url": "https://hooks.slack.com/services/...",
    "channel": "#test-alerts",
    "username": "Conductor"
  }
}
```

### Test Channel

```http
POST /api/v1/notifications/channels/{channel_id}/test
```

### List Notification Rules

```http
GET /api/v1/notifications/rules
```

### Create Rule

```http
POST /api/v1/notifications/rules
```

Request:
```json
{
  "name": "notify-on-failure",
  "service_id": "svc_abc123",
  "channel_id": "ch_001",
  "event_types": ["RUN_FAILED", "RUN_ERRORED"],
  "conditions": {
    "branch": "main"
  },
  "enabled": true
}
```

## gRPC API

The gRPC API is available on port 9090 by default.

### Proto Definitions

Proto files are located in `api/proto/conductor/v1/`:

- `services.proto` - Service registry
- `runs.proto` - Test runs
- `agents.proto` - Agent management
- `results.proto` - Test results
- `notifications.proto` - Notifications
- `agent_service.proto` - Agent streaming protocol
- `health.proto` - Health checks

### Example: Creating a Run (Go)

```go
import (
    "context"
    pb "github.com/conductor/conductor/api/gen/conductor/v1"
    "google.golang.org/grpc"
)

conn, err := grpc.Dial("localhost:9090", grpc.WithInsecure())
if err != nil {
    log.Fatal(err)
}
defer conn.Close()

client := pb.NewRunServiceClient(conn)

resp, err := client.CreateRun(context.Background(), &pb.CreateRunRequest{
    ServiceId: "svc_abc123",
    Branch:    "main",
    Tags:      []string{"unit"},
})
```

## WebSocket API

Connect to `/ws` for real-time updates.

### Connection

```javascript
const ws = new WebSocket('wss://conductor.example.com/ws');

ws.onopen = () => {
  // Authenticate
  ws.send(JSON.stringify({
    type: 'auth',
    token: 'your-jwt-token'
  }));
};
```

### Subscribe to Updates

```javascript
// Subscribe to a specific run
ws.send(JSON.stringify({
  type: 'subscribe',
  channel: 'run',
  id: 'run_xyz789'
}));

// Subscribe to all runs for a service
ws.send(JSON.stringify({
  type: 'subscribe',
  channel: 'service',
  id: 'svc_abc123'
}));

// Subscribe to agent updates
ws.send(JSON.stringify({
  type: 'subscribe',
  channel: 'agents'
}));
```

### Receive Updates

```javascript
ws.onmessage = (event) => {
  const message = JSON.parse(event.data);
  
  switch (message.type) {
    case 'run.updated':
      console.log('Run updated:', message.data);
      break;
    case 'test.completed':
      console.log('Test completed:', message.data);
      break;
    case 'log.chunk':
      console.log('Log:', message.data.content);
      break;
    case 'agent.status':
      console.log('Agent status:', message.data);
      break;
  }
};
```

### Message Types

| Type | Description |
|------|-------------|
| `run.created` | New run created |
| `run.updated` | Run status changed |
| `run.completed` | Run finished |
| `test.started` | Individual test started |
| `test.completed` | Individual test finished |
| `log.chunk` | Log output chunk |
| `artifact.uploaded` | Artifact available |
| `agent.connected` | Agent came online |
| `agent.disconnected` | Agent went offline |
| `agent.status` | Agent status update |

### Unsubscribe

```javascript
ws.send(JSON.stringify({
  type: 'unsubscribe',
  channel: 'run',
  id: 'run_xyz789'
}));
```
