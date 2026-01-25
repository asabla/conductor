# Agents

## Overview

Agents are lightweight processes that run inside private networks and execute tests on behalf of the control plane. They connect outbound to the control plane, receive work assignments, execute tests, and report results.

## Agent Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                              AGENT                                      │
│                                                                         │
│  ┌──────────────────────────────────────────────────────────────────┐  │
│  │                    Control Plane Client                           │  │
│  │                   (gRPC bidirectional stream)                     │  │
│  └──────────────────────────────────────────────────────────────────┘  │
│                                    │                                    │
│       ┌────────────────────────────┼────────────────────────────────┐  │
│       │                            │                            │      │
│       ▼                            ▼                            ▼      │
│  ┌─────────────┐           ┌─────────────┐           ┌─────────────┐  │
│  │    Work     │           │  Executor   │           │   Result    │  │
│  │   Manager   │──────────►│   Engine    │──────────►│  Reporter   │  │
│  └─────────────┘           └─────────────┘           └─────────────┘  │
│                                    │                                    │
│                    ┌───────────────┼───────────────┐                   │
│                    │               │               │                   │
│                    ▼               ▼               ▼                   │
│             ┌───────────┐   ┌───────────┐   ┌───────────┐             │
│             │Subprocess │   │ Container │   │  Future   │             │
│             │  Driver   │   │  Driver   │   │  Drivers  │             │
│             └───────────┘   └───────────┘   └───────────┘             │
│                                                                         │
│  ┌──────────────────────────────────────────────────────────────────┐  │
│  │                       Local Services                              │  │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────┐   │  │
│  │  │  Repo Cache │  │ State Store │  │    Resource Monitor     │   │  │
│  │  │   (git)     │  │  (SQLite)   │  │   (CPU, memory, disk)   │   │  │
│  │  └─────────────┘  └─────────────┘  └─────────────────────────┘   │  │
│  └──────────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────┘
```

## Core Components

### Control Plane Client

**Purpose**: Maintains connection to control plane and handles protocol.

**Responsibilities**:
- Establishes and maintains gRPC connection to control plane
- Handles authentication (token, mTLS, or OIDC)
- Sends registration message on connect
- Sends periodic heartbeats
- Receives work assignments
- Sends result streams
- Handles reconnection on connection loss

**Connection lifecycle**:
```
START
  │
  ▼
CONNECTING ──(success)──► REGISTERED ──► IDLE ◄───┐
  │                                        │      │
  │(failure)                               │      │
  │                                        ▼      │
  ▼                                     BUSY ─────┘
BACKOFF ──(retry)──► CONNECTING              │
                                             │
                                       (work complete)
```

**Reconnection behavior**:
- On disconnect, immediately attempt reconnect
- Use exponential backoff: 1s, 2s, 4s, 8s, ... up to max (60s)
- Add jitter to avoid thundering herd
- On reconnect, re-register and report any in-progress work

### Work Manager

**Purpose**: Receives work from control plane and coordinates execution.

**Responsibilities**:
- Accepts work assignments from Control Plane Client
- Validates work can be executed (required resources available)
- Queues work for Executor Engine
- Manages concurrent execution (respects parallelism limit)
- Handles work cancellation
- Tracks work status (pending, running, completed, failed)

**Concurrency control**:
- Agent has configurable max concurrent executions (default: 4)
- Work manager maintains a semaphore to limit parallelism
- Heavy tests can request exclusive execution (parallelism = 1 while running)

### Executor Engine

**Purpose**: Runs tests using the appropriate execution driver.

**Responsibilities**:
- Selects execution driver based on test configuration
- Prepares execution environment (clone repo, set up directories)
- Invokes driver to run test
- Captures output (stdout, stderr, exit code)
- Collects artifacts (result files, screenshots)
- Enforces timeouts
- Cleans up after execution

**Execution flow**:
1. Create workspace directory
2. Clone repository (or update if cached)
3. Checkout specified ref (commit, branch, tag)
4. Prepare environment (set env vars, inject secrets)
5. Invoke driver (subprocess or container)
6. Stream output to Result Reporter
7. Wait for completion or timeout
8. Collect result files and artifacts
9. Clean up workspace

### Result Reporter

**Purpose**: Sends execution results back to control plane.

**Responsibilities**:
- Streams real-time output (logs) to control plane
- Parses result files into structured results
- Uploads artifacts to control plane
- Reports final status (pass, fail, error, timeout)

**Streaming behavior**:
- Logs are streamed in chunks (every 1s or 4KB, whichever first)
- Final result sent when execution completes
- Artifacts uploaded after execution completes
- If connection lost during reporting, queue locally and retry

## Execution Drivers

### Subprocess Driver

Runs tests as a subprocess on the agent's host.

**When to use**:
- Fast startup required
- Runtime dependencies already on agent
- Need direct access to host resources
- Debugging/development

**How it works**:
1. Build command from test configuration
2. Set working directory to repo checkout
3. Set environment variables (PATH, test-specific vars, secrets)
4. Execute command via `os/exec`
5. Capture stdout/stderr
6. Wait for exit
7. Read result file from configured path

**Requirements**:
- Required runtimes must be installed on agent (Python, Node, Java, etc.)
- Agent must have network access to systems under test

### Container Driver

Runs tests inside a Docker container.

**When to use**:
- Isolation required
- Reproducible environment needed
- Dependencies are complex or conflict with agent host
- Running untrusted test code

**How it works**:
1. Pull image (or use cached)
2. Create container with:
   - Repo checkout mounted as volume
   - Environment variables set
   - Network mode configured
   - Resource limits applied
3. Start container
4. Stream logs from container
5. Wait for container exit
6. Copy result files from container
7. Remove container

**Container configuration** (from test manifest):
- Image name and tag
- Environment variables
- Volume mounts
- Network mode (host, bridge, custom)
- Resource limits (CPU, memory)
- Entrypoint override (if needed)

## Local Services

### Repo Cache

**Purpose**: Caches Git repositories to speed up repeated checkouts.

**Behavior**:
- First clone: full clone to cache directory
- Subsequent runs: fetch latest, checkout requested ref
- Supports shallow clones for large repos (configurable)
- Periodic cleanup of unused repos (LRU eviction)

**Directory structure**:
```
/var/lib/testharness/repos/
  github.com/
    org/
      repo-a/
        .git/
      repo-b/
        .git/
```

### State Store

**Purpose**: Persists agent state across restarts.

**Stored data**:
- Agent ID (assigned by control plane)
- In-progress work (for recovery after crash)
- Execution history (recent runs, for debugging)
- Configuration cache

**Implementation**: SQLite database in agent data directory.

### Resource Monitor

**Purpose**: Tracks agent resource usage and availability.

**Monitored resources**:
- CPU usage (percentage)
- Memory usage (percentage, absolute)
- Disk space (workspace directory)
- Running containers (count, resource usage)

**Used for**:
- Reporting capabilities to control plane
- Rejecting work when resources exhausted
- Alerting on resource pressure

## Agent Configuration

Configuration via environment variables and/or config file.

**Required**:
- `CONTROL_PLANE_URL`: gRPC endpoint of control plane
- `AGENT_TOKEN`: Authentication token (or path to cert for mTLS)

**Optional**:
- `AGENT_ID`: Persistent agent ID (auto-generated if not set)
- `AGENT_NAME`: Human-readable name
- `NETWORK_ZONE`: Zone identifier for routing (e.g., "internal-a")
- `MAX_PARALLEL`: Maximum concurrent test executions (default: 4)
- `WORKSPACE_DIR`: Directory for test execution (default: /tmp/testharness)
- `CACHE_DIR`: Directory for repo cache (default: /var/lib/testharness)
- `DOCKER_HOST`: Docker daemon socket (default: unix:///var/run/docker.sock)
- `LOG_LEVEL`: Logging verbosity (default: info)

**Runtime capabilities** (auto-detected or configured):
- Available runtimes: python3, node, java, go, etc.
- Docker available: true/false
- Reachable network zones (if multiple)

## Agent Lifecycle

### Startup

1. Load configuration
2. Initialize local state store
3. Detect runtime capabilities
4. Start resource monitor
5. Connect to control plane
6. Register with capabilities
7. Enter idle state, wait for work

### During Operation

- Receive work → execute → report → return to idle
- Send heartbeat every 30 seconds
- Monitor resources, report changes
- Handle cancellation requests

### Shutdown

**Graceful** (SIGTERM):
1. Stop accepting new work
2. Wait for in-progress work (with timeout)
3. Report final results
4. Disconnect from control plane
5. Exit

**Immediate** (SIGKILL):
- Process terminates
- In-progress work will be detected by control plane via missed heartbeat
- On restart, agent can attempt to recover/report orphaned work

### Draining

Control plane can put agent in draining mode:
1. Agent stops accepting new work
2. Current work continues to completion
3. Agent reports "draining" status
4. When all work complete, agent reports "drained"
5. Operator can then safely stop agent

## Security Considerations

### Secrets Handling

- Secrets injected via environment variables
- Never logged (redacted in output)
- Never written to disk (except via controlled mechanisms)
- Cleared from memory after use (best effort)

### Container Isolation

- Tests in containers run as non-root user (unless explicitly required)
- Resource limits enforced (prevent runaway tests)
- Network access can be restricted per-test
- Host filesystem access limited to repo mount

### Code Execution

- Agent executes arbitrary code from repositories
- Trust model: only run tests from trusted repositories
- Consider: signing test manifests, allowlisting repos

## Deployment Patterns

### Bare Metal / VM

- Deploy agent binary directly
- Install required runtimes (Python, Node, Java)
- Install Docker for container execution
- Configure as systemd service

### Kubernetes

- Deploy agent as DaemonSet or Deployment
- Use node selectors to place in specific network zones
- Mount Docker socket (or use Docker-in-Docker)
- Configure resource requests/limits

### Auto-scaling

- Agents can be added dynamically based on queue depth
- Control plane API provides queue metrics
- External autoscaler (K8s HPA, cloud autoscaling) adds/removes agents
- New agents register automatically
