# Execution Protocol

## Overview

The execution protocol defines how agents and the control plane communicate. It covers connection establishment, work assignment, result reporting, and health monitoring. The protocol uses gRPC with bidirectional streaming for efficiency and real-time updates.

## Connection Model

```
┌─────────────────┐                         ┌─────────────────┐
│      Agent      │                         │  Control Plane  │
│                 │                         │                 │
│  ┌───────────┐  │     TLS Connection      │  ┌───────────┐  │
│  │  gRPC     │  │ ───────────────────────►│  │  gRPC     │  │
│  │  Client   │  │  (agent initiates)      │  │  Server   │  │
│  └───────────┘  │                         │  └───────────┘  │
│                 │                         │                 │
│                 │◄═══════════════════════►│                 │
│                 │  Bidirectional Stream   │                 │
│                 │                         │                 │
└─────────────────┘                         └─────────────────┘

Agent always initiates connection (outbound from private network).
Control plane never initiates connection to agent.
```

## Protocol Messages

### Agent → Control Plane

**RegisterRequest**
```
RegisterRequest {
  agent_id: string (optional, assigned if empty)
  agent_name: string
  version: string
  capabilities: Capabilities
}

Capabilities {
  network_zones: []string
  runtimes: []Runtime
  max_parallel: int
  docker_available: bool
  resources: Resources
}

Runtime {
  name: string (e.g., "python", "node", "java")
  version: string (e.g., "3.11", "20", "17")
}

Resources {
  cpu_cores: int
  memory_mb: int
  disk_mb: int
}
```

Sent immediately after connection established. Agent announces itself and its capabilities.

---

**Heartbeat**
```
Heartbeat {
  agent_id: string
  timestamp: Timestamp
  status: AgentStatus (IDLE, BUSY, DRAINING)
  active_runs: []string (run IDs currently executing)
  resources: ResourceUsage
}

ResourceUsage {
  cpu_percent: float
  memory_percent: float
  disk_percent: float
  running_containers: int
}
```

Sent periodically (every 30 seconds) to confirm agent is alive and report status.

---

**WorkAccepted**
```
WorkAccepted {
  run_id: string
  agent_id: string
  accepted_at: Timestamp
}
```

Sent when agent receives work assignment and begins execution.

---

**WorkRejected**
```
WorkRejected {
  run_id: string
  agent_id: string
  reason: string
}
```

Sent if agent cannot accept work (resources exhausted, capability mismatch).

---

**ResultStream**
```
ResultStream {
  run_id: string
  agent_id: string
  
  oneof payload {
    LogChunk log_chunk
    TestResult test_result
    Artifact artifact
    RunComplete run_complete
  }
}

LogChunk {
  stream: LogStream (STDOUT, STDERR)
  data: bytes
  timestamp: Timestamp
}

TestResult {
  test_id: string
  name: string
  suite: string
  status: TestStatus (PASS, FAIL, SKIP, ERROR)
  duration_ms: int
  error_message: string (if failed)
  stack_trace: string (if failed)
  metadata: map<string, string>
}

Artifact {
  name: string
  path: string
  content_type: string
  data: bytes (if small) or upload_url: string (if large)
}

RunComplete {
  status: RunStatus (SUCCESS, FAILURE, ERROR, TIMEOUT, CANCELLED)
  summary: RunSummary
  completed_at: Timestamp
}

RunSummary {
  total_tests: int
  passed: int
  failed: int
  skipped: int
  errors: int
  duration_ms: int
}
```

Streamed during and after test execution. Multiple messages per run.

---

### Control Plane → Agent

**RegisterResponse**
```
RegisterResponse {
  agent_id: string (assigned or confirmed)
  config: AgentConfig
}

AgentConfig {
  heartbeat_interval_seconds: int
  default_timeout_seconds: int
  max_artifact_size_bytes: int
}
```

Sent in response to RegisterRequest.

---

**AssignWork**
```
AssignWork {
  run_id: string
  priority: int
  repository: Repository
  ref: string
  tests: []TestSpec
  environment: map<string, string>
  secrets: map<string, string>
  timeout_seconds: int
}

Repository {
  clone_url: string
  credentials: Credentials
}

Credentials {
  type: CredentialType (TOKEN, SSH_KEY)
  token: string
  ssh_private_key: string
}

TestSpec {
  test_id: string
  name: string
  suite: string
  execution: ExecutionSpec
  result_config: ResultConfig
}

ExecutionSpec {
  type: ExecutionType (SUBPROCESS, CONTAINER)
  
  # For subprocess
  command: string
  args: []string
  working_dir: string
  
  # For container
  image: string
  entrypoint: []string
  mounts: []Mount
  network_mode: string
  resource_limits: ResourceLimits
}

ResultConfig {
  result_file: string
  result_format: ResultFormat (JUNIT, JEST, PLAYWRIGHT, TAP, JSON)
  artifact_paths: []string
}
```

Sent to assign work to an agent.

---

**CancelWork**
```
CancelWork {
  run_id: string
  reason: string
}
```

Sent to cancel an in-progress run.

---

**Drain**
```
Drain {
  drain_id: string
}
```

Tells agent to stop accepting new work and report when current work is complete.

---

**DrainComplete**
```
DrainComplete {
  drain_id: string
}
```

Agent response when draining is complete (all work finished).

---

## Message Flow Examples

### Normal Test Execution

```
Agent                                         Control Plane
  │                                                │
  │  RegisterRequest(capabilities)                 │
  │───────────────────────────────────────────────►│
  │                                                │
  │  RegisterResponse(agent_id, config)            │
  │◄───────────────────────────────────────────────│
  │                                                │
  │  Heartbeat(IDLE)                               │
  │───────────────────────────────────────────────►│
  │                                                │
  │  AssignWork(run_id=123, tests=[...])           │
  │◄───────────────────────────────────────────────│
  │                                                │
  │  WorkAccepted(run_id=123)                      │
  │───────────────────────────────────────────────►│
  │                                                │
  │  Heartbeat(BUSY, active=[123])                 │
  │───────────────────────────────────────────────►│
  │                                                │
  │  ResultStream(log_chunk)                       │
  │───────────────────────────────────────────────►│
  │  ResultStream(log_chunk)                       │
  │───────────────────────────────────────────────►│
  │  ResultStream(test_result: test_1 PASS)        │
  │───────────────────────────────────────────────►│
  │  ResultStream(test_result: test_2 FAIL)        │
  │───────────────────────────────────────────────►│
  │  ResultStream(artifact: screenshot.png)        │
  │───────────────────────────────────────────────►│
  │  ResultStream(run_complete: FAILURE)           │
  │───────────────────────────────────────────────►│
  │                                                │
  │  Heartbeat(IDLE)                               │
  │───────────────────────────────────────────────►│
```

### Work Cancellation

```
Agent                                         Control Plane
  │                                                │
  │  (executing run_id=123)                        │
  │                                                │
  │  CancelWork(run_id=123, reason="user request") │
  │◄───────────────────────────────────────────────│
  │                                                │
  │  (terminate execution)                         │
  │                                                │
  │  ResultStream(run_complete: CANCELLED)         │
  │───────────────────────────────────────────────►│
```

### Agent Draining

```
Agent                                         Control Plane
  │                                                │
  │  (executing run_id=123)                        │
  │                                                │
  │  Drain(drain_id=abc)                           │
  │◄───────────────────────────────────────────────│
  │                                                │
  │  Heartbeat(DRAINING, active=[123])             │
  │───────────────────────────────────────────────►│
  │                                                │
  │  (finish run 123)                              │
  │                                                │
  │  ResultStream(run_complete)                    │
  │───────────────────────────────────────────────►│
  │                                                │
  │  DrainComplete(drain_id=abc)                   │
  │───────────────────────────────────────────────►│
```

### Connection Recovery

```
Agent                                         Control Plane
  │                                                │
  │  (connection lost)                             │
  │                                                │
  │  (backoff: 1s, 2s, 4s...)                      │
  │                                                │
  │  RegisterRequest(agent_id=previous_id)         │
  │───────────────────────────────────────────────►│
  │                                                │
  │  RegisterResponse(same agent_id)               │
  │◄───────────────────────────────────────────────│
  │                                                │
  │  (if work was in progress, continue reporting) │
  │                                                │
  │  ResultStream(run_id=123, partial results)     │
  │───────────────────────────────────────────────►│
```

## Authentication

### Token-Based

Simple pre-shared token for agent authentication.

```
Agent connects with:
  Authorization: Bearer <agent-token>
```

Control plane validates token against known agent tokens.

Suitable for: controlled environments, getting started.

### mTLS

Mutual TLS with agent certificates.

```
Agent presents client certificate.
Control plane validates against CA.
Agent identity extracted from certificate CN/SAN.
```

Suitable for: high-security environments, automated agent provisioning.

### OIDC Machine Credentials

For cloud-native environments (Kubernetes, cloud VMs).

```
Agent obtains token from identity provider (e.g., workload identity).
Control plane validates token with identity provider.
```

Suitable for: Kubernetes deployments, cloud environments.

## Error Handling

### Connection Errors

| Error | Agent Behavior |
|-------|----------------|
| Connection refused | Backoff and retry |
| TLS handshake failure | Check certs, backoff and retry |
| Authentication failure | Log error, exit (configuration issue) |
| Connection dropped | Immediate reconnect with backoff |

### Protocol Errors

| Error | Behavior |
|-------|----------|
| Unknown message type | Log warning, ignore message |
| Invalid message format | Log error, ignore message |
| Missing required field | Log error, reject message |

### Execution Errors

| Error | Agent Behavior |
|-------|----------------|
| Clone failure | Report WorkRejected or RunComplete(ERROR) |
| Execution timeout | Kill process, report RunComplete(TIMEOUT) |
| Out of disk space | Report WorkRejected(resource_exhausted) |
| Container pull failure | Report RunComplete(ERROR) |

## Flow Control

### Work Queue Backpressure

Control plane tracks:
- How many runs assigned to each agent
- Agent's reported max_parallel

Control plane will not assign work beyond agent capacity.

### Result Stream Backpressure

If control plane cannot keep up with results:
- gRPC flow control kicks in
- Agent's send blocks
- Agent may buffer locally

For large artifacts:
- Control plane returns upload URL
- Agent uploads directly to artifact storage
- Reports artifact reference in message

## Timeouts

| Timeout | Default | Purpose |
|---------|---------|---------|
| Connection timeout | 10s | Initial connection establishment |
| Heartbeat interval | 30s | Agent liveness check |
| Heartbeat timeout | 90s | Declare agent offline if no heartbeat |
| Default run timeout | 30m | Kill run if not complete |
| Graceful shutdown | 60s | Wait for in-progress work on shutdown |

## Versioning

Protocol version included in RegisterRequest.

```
RegisterRequest {
  protocol_version: "1.0"
  ...
}
```

Control plane can reject incompatible agents or adapt behavior for older versions.

Plan for backward compatibility:
- New optional fields are safe
- New required fields require version bump
- Removed fields require version bump
