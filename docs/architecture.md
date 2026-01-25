# Architecture Overview

This document describes the architecture of Conductor, its components, and how they interact.

## Table of Contents

- [System Overview](#system-overview)
- [Components](#components)
  - [Control Plane](#control-plane)
  - [Agents](#agents)
  - [Dashboard](#dashboard)
- [Data Flow](#data-flow)
- [Communication Protocols](#communication-protocols)
- [Database Schema](#database-schema)
- [Security Model](#security-model)
- [Scalability](#scalability)

## System Overview

Conductor is a distributed test orchestration platform with three main components:

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           External Systems                              │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌───────────────┐  │
│  │   GitHub    │  │   GitLab    │  │  CI System  │  │  Notification │  │
│  │             │  │             │  │ (webhooks)  │  │   (Slack,etc) │  │
│  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘  └───────┬───────┘  │
└─────────┼────────────────┼────────────────┼──────────────────┼──────────┘
          │                │                │                  │
          └────────────────┴────────┬───────┴──────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                           CONTROL PLANE                                 │
│                                                                         │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                         API Gateway                              │   │
│  │              (HTTP REST + gRPC + WebSocket)                      │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                    │                                    │
│       ┌────────────────────────────┼────────────────────────────┐      │
│       │                            │                            │      │
│       ▼                            ▼                            ▼      │
│  ┌─────────────┐           ┌─────────────┐           ┌─────────────┐  │
│  │  Scheduler  │           │   Agent     │           │  Dashboard  │  │
│  │   Service   │◄─────────►│  Manager    │           │     API     │  │
│  └──────┬──────┘           └──────┬──────┘           └──────┬──────┘  │
│         │                         │                         │         │
│         └─────────────────────────┼─────────────────────────┘         │
│                                   │                                    │
│                                   ▼                                    │
│                          ┌─────────────┐                               │
│                          │   Result    │                               │
│                          │   Store     │                               │
│                          └──────┬──────┘                               │
│                                 │                                      │
│  ┌─────────────┐         ┌──────┴──────┐         ┌─────────────┐      │
│  │    Git      │         │  PostgreSQL │         │  Artifact   │      │
│  │  Provider   │         │             │         │   Storage   │      │
│  │  Adapters   │         └─────────────┘         │   (S3/MinIO)│      │
│  └─────────────┘                                 └─────────────┘      │
└─────────────────────────────────────────────────────────────────────────┘
                                    │
                                    │ gRPC (bidirectional stream)
                                    │ (agents connect outbound)
                    ┌───────────────┼───────────────┐
                    │               │               │
                    ▼               ▼               ▼
             ┌──────────┐   ┌──────────┐   ┌──────────┐
             │  Agent   │   │  Agent   │   │  Agent   │
             │ (Zone A) │   │ (Zone B) │   │ (Zone C) │
             └──────────┘   └──────────┘   └──────────┘
```

## Components

### Control Plane

The control plane is the central coordination service that manages all test orchestration.

#### Responsibilities

- **Test Registry** - Maintain knowledge of what tests exist across services
- **Scheduling** - Assign test runs to available agents
- **Agent Management** - Track agent connections, health, and capabilities
- **Result Collection** - Aggregate and store test results
- **API Gateway** - Expose REST, gRPC, and WebSocket APIs
- **Git Sync** - Discover tests from repository manifests
- **Notifications** - Send alerts via Slack, email, Teams, webhooks

#### Internal Services

| Service | Description |
|---------|-------------|
| Scheduler | Matches test runs to agents based on capabilities and availability |
| Agent Manager | Maintains bidirectional streams with agents, tracks health |
| Registry | Stores service and test definitions, handles manifest parsing |
| Result Collector | Receives and normalizes test results from agents |
| Git Syncer | Periodically fetches manifests from repositories |
| Notification Service | Evaluates rules and sends notifications |
| WebSocket Hub | Broadcasts real-time updates to dashboard clients |

#### Technology Stack

| Component | Technology |
|-----------|------------|
| Language | Go 1.22+ |
| API | gRPC + grpc-gateway (REST) |
| Database | PostgreSQL 15+ |
| Object Storage | S3-compatible (MinIO for self-hosted) |
| Cache | Redis (optional) |
| Observability | OpenTelemetry, Prometheus |

### Agents

Agents are lightweight processes that execute tests in private networks.

#### Responsibilities

- **Connect to Control Plane** - Establish and maintain gRPC stream
- **Execute Tests** - Run tests via subprocess or Docker container
- **Stream Results** - Send progress and results back to control plane
- **Manage Workspaces** - Clone repositories and manage test environments
- **Report Health** - Send heartbeats with resource utilization

#### Execution Modes

**Subprocess Mode**
```
Agent Process
     │
     └──► Test Process (go test, npm test, pytest, etc.)
              │
              └──► Test Results
```
- Direct process execution
- Fast startup
- Uses agent's runtime environment
- Best for trusted tests

**Container Mode**
```
Agent Process
     │
     └──► Docker Daemon
              │
              └──► Test Container
                       │
                       └──► Test Results
```
- Isolated execution environment
- Reproducible dependencies
- Container image per test
- Best for untrusted or complex environments

#### Technology Stack

| Component | Technology |
|-----------|------------|
| Language | Go 1.22+ |
| Communication | gRPC streaming |
| Container Runtime | Docker SDK for Go |
| Local State | SQLite |
| Repository Cache | Local filesystem |

### Dashboard

The dashboard provides visibility into test execution and system health.

#### Features

- **Real-time Updates** - Live test progress via WebSocket
- **Run Management** - View, trigger, and cancel test runs
- **Service Management** - Configure services and test definitions
- **Agent Fleet** - Monitor agent status and utilization
- **Analytics** - Trends, pass rates, flakiness detection
- **Configuration** - Notification rules, Git integrations

#### Technology Stack

| Component | Technology |
|-----------|------------|
| Framework | React / SvelteKit |
| Language | TypeScript |
| State Management | TanStack Query |
| Styling | Tailwind CSS |
| Components | shadcn/ui |
| Real-time | WebSocket |

## Data Flow

### Test Run Initiation (Webhook)

```
Git Provider                 Control Plane                Agent
     │                             │                         │
     │  POST /webhooks/github      │                         │
     │────────────────────────────►│                         │
     │                             │                         │
     │                             │ (validate signature)    │
     │                             │ (parse webhook event)   │
     │                             │ (lookup tests)          │
     │                             │ (find capable agent)    │
     │                             │                         │
     │                             │   AssignWork(...)       │
     │                             │────────────────────────►│
     │                             │                         │
     │      202 Accepted           │                         │
     │◄────────────────────────────│                         │
```

### Test Execution

```
Agent                    Control Plane              Dashboard
  │                           │                         │
  │  (clone repo)             │                         │
  │  (run tests)              │                         │
  │                           │                         │
  │  ResultStream(progress)   │                         │
  │──────────────────────────►│                         │
  │                           │  (store result)         │
  │                           │                         │
  │                           │  WebSocket: update      │
  │                           │────────────────────────►│
  │                           │                         │
  │  ResultStream(complete)   │                         │
  │──────────────────────────►│                         │
  │                           │                         │
  │                           │  (finalize run)         │
  │                           │  (send notifications)   │
```

### Agent Registration

```
Agent                         Control Plane
  │                                │
  │  (start up)                    │
  │                                │
  │  Register(capabilities)        │
  │───────────────────────────────►│
  │                                │
  │                                │ (validate agent)
  │                                │ (add to pool)
  │                                │
  │  RegisterAck(agent_id, config) │
  │◄───────────────────────────────│
  │                                │
  │  Heartbeat (every 30s)         │
  │───────────────────────────────►│
  │                                │
```

## Communication Protocols

### gRPC Agent Protocol

The agent-control plane communication uses bidirectional gRPC streaming:

**Agent to Control Plane Messages:**
- `Register` - Initial registration with capabilities
- `Heartbeat` - Periodic health and status
- `WorkAccepted` - Acknowledgement of work assignment
- `WorkRejected` - Rejection with reason
- `ResultStream` - Progress, results, and completion

**Control Plane to Agent Messages:**
- `RegisterResponse` - Registration acknowledgement
- `AssignWork` - Test run assignment
- `CancelWork` - Run cancellation request
- `Drain` - Graceful shutdown request
- `Ack` - Message acknowledgements

### REST API

The control plane exposes REST endpoints via grpc-gateway:

- `POST /api/v1/services` - Create service
- `GET /api/v1/services` - List services
- `POST /api/v1/runs` - Create test run
- `GET /api/v1/runs/{id}` - Get run details
- `GET /api/v1/agents` - List agents

See [API Documentation](api.md) for complete reference.

### WebSocket

Real-time updates for dashboard clients:

```json
// Subscribe to run updates
{"type": "subscribe", "channel": "run", "id": "run-123"}

// Receive updates
{"type": "run.updated", "data": {"id": "run-123", "status": "running"}}
{"type": "test.completed", "data": {"run_id": "run-123", "test_id": "test-1", "status": "passed"}}
```

## Database Schema

### Core Entities

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Service   │────►│    Test     │     │    Agent    │
│             │     │ Definition  │     │             │
└─────────────┘     └─────────────┘     └─────────────┘
       │                   │                   │
       │                   │                   │
       ▼                   ▼                   │
┌─────────────┐     ┌─────────────┐            │
│  Test Run   │────►│Test Result  │◄───────────┘
│             │     │             │
└─────────────┘     └─────────────┘
       │
       ▼
┌─────────────┐
│  Artifact   │
│             │
└─────────────┘
```

**Service** - A registered project with tests
**TestDefinition** - A test or test suite that can be executed
**Agent** - A registered agent instance
**TestRun** - An execution of tests for a service
**TestResult** - The outcome of an individual test
**Artifact** - Files produced during test execution

### Key Relationships

- A Service has many TestDefinitions
- A Service has many TestRuns
- A TestRun has many TestResults
- A TestRun is executed by an Agent
- A TestRun has many Artifacts

## Security Model

### Authentication

| Component | Method |
|-----------|--------|
| Dashboard Users | OIDC/SSO or JWT tokens |
| Agents | Pre-shared tokens or mTLS |
| API Clients | API keys or JWT tokens |
| Webhooks | HMAC signature validation |

### Authorization

- Role-based access control (RBAC) for dashboard
- Service-level permissions for API operations
- Agent capabilities restrict allowed operations

### Network Security

- Agents connect **outbound** only - no inbound firewall rules needed
- gRPC over TLS mandatory in production
- Secrets injected at runtime, not stored in manifests

## Scalability

### Horizontal Scaling

**Agents** - Add more agents to increase test execution capacity. Agents can be deployed in specific network zones as needed.

**Control Plane** - For scale:
- Stateless API handlers can be load-balanced
- Database is the bottleneck - standard PostgreSQL scaling applies
- Agent connections need sticky routing or Redis-backed state
- Consider NATS for distributed work queue

### Capacity Planning

| Metric | Consideration |
|--------|---------------|
| Concurrent runs | Limited by agent count and parallelism settings |
| Results storage | ~1KB per test result, plan retention policy |
| Artifact storage | Screenshots/traces can be large, set size limits |
| Agent connections | Each agent maintains one gRPC stream, low overhead |

### Resource Requirements

**Control Plane (Recommended)**
- CPU: 2 cores
- Memory: 1GB
- Storage: 10GB (for database)

**Agent (Recommended)**
- CPU: 2+ cores (depends on test workload)
- Memory: 2GB+ (depends on test workload)
- Storage: 20GB+ (for workspaces and cache)

**PostgreSQL**
- CPU: 2 cores
- Memory: 1GB
- Storage: Based on retention policy

**MinIO/S3**
- Storage: Based on artifact volume and retention
