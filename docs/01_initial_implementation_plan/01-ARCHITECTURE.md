# Architecture Details

## System Context

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
             └──────────┘   └──────────┘   └──────────┘
```

## Component Interaction Patterns

### Pattern 1: Test Run Initiation (Webhook)

```
CI System                Control Plane                Agent
    │                         │                         │
    │  POST /webhook/github   │                         │
    │────────────────────────►│                         │
    │                         │                         │
    │                         │ (validate, parse event) │
    │                         │                         │
    │                         │ (lookup tests for repo) │
    │                         │                         │
    │                         │ (find capable agent)    │
    │                         │                         │
    │                         │   AssignWork(...)       │
    │                         │────────────────────────►│
    │                         │                         │
    │      202 Accepted       │                         │
    │◄────────────────────────│                         │
```

### Pattern 2: Test Execution

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

### Pattern 3: Agent Registration

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

## Technology Stack

### Control Plane

| Component | Technology | Rationale |
|-----------|------------|-----------|
| Language | Go | Concurrent workloads, single binary deployment, strong networking stdlib |
| API Framework | gRPC + grpc-gateway | gRPC for agent communication, auto-generated REST for dashboard/integrations |
| Database | PostgreSQL | Relational model fits test results, JSONB for flexible metadata, mature tooling |
| Artifact Storage | S3-compatible (MinIO for self-hosted) | Standard interface, handles large files (screenshots, traces) |
| Cache | Redis (optional) | Session management, real-time pubsub for dashboard updates, optional for MVP |
| Message Queue | Embedded (or NATS for scale) | Start simple, add NATS if multiple control plane instances needed |

### Agents

| Component | Technology | Rationale |
|-----------|------------|-----------|
| Language | Go | Same as control plane—shared libraries, single binary, cross-compilation |
| Control Plane Connection | gRPC streaming | Bidirectional, efficient, handles reconnection |
| Container Runtime | Docker SDK for Go | Standard container interface, well-maintained SDK |
| Process Execution | os/exec | Standard library sufficient for subprocess management |
| Local State | SQLite | Track local execution state, cached repos, minimal dependencies |

### Dashboard

| Component | Technology | Rationale |
|-----------|------------|-----------|
| Framework | React or SvelteKit | Team familiarity with TypeScript, good ecosystem |
| State Management | TanStack Query | Handles caching, polling, real-time updates cleanly |
| Real-time | WebSocket (or SSE) | Live test progress, agent status updates |
| Charts | Recharts or Apache ECharts | Test trends, pass rates, timing distributions |
| UI Components | shadcn/ui | Accessible, customizable, not overly opinionated |

### Infrastructure

| Concern | Technology |
|---------|------------|
| Secrets | HashiCorp Vault or environment-injected secrets |
| Observability | OpenTelemetry → Prometheus + Grafana |
| Authentication | OIDC for dashboard users, mTLS or bearer tokens for agents |
| Deployment | Kubernetes (control plane), binary distribution (agents) |

## Scalability Considerations

### Horizontal Scaling Points

**Agents**: Naturally horizontal. Add more agents to increase parallel test execution capacity. Agents can be added to specific network zones as needed.

**Control Plane**: For MVP, a single instance is sufficient. For scale:
- Stateless API handlers can be load-balanced
- Database is the bottleneck—standard PostgreSQL scaling patterns apply
- Agent connections need sticky routing or shared state (Redis)
- Consider NATS for distributed work queue

### Capacity Planning

| Metric | Consideration |
|--------|---------------|
| Concurrent test runs | Limited by agent count and agent parallelism settings |
| Results storage | ~1KB per test result, plan for retention policy |
| Artifact storage | Screenshots, traces can be large—set size limits, retention |
| Agent connections | Each agent maintains one gRPC stream—low overhead per agent |

## Security Model

### Agent Authentication

Agents authenticate to control plane using one of:
- Pre-shared tokens (simple, suitable for controlled environments)
- mTLS (mutual TLS with agent certificates)
- OIDC machine credentials (for cloud-native deployments)

### Secrets Distribution

Tests often need credentials (database passwords, API keys). Options:
- Control plane injects secrets into work assignments
- Agents fetch from Vault using their identity
- Secrets baked into agent environment (least flexible)

Recommendation: Start with environment-injected secrets at agent deployment, evolve to Vault integration.

### Network Security

- Agents connect outbound only—no inbound ports required
- gRPC over TLS (mandatory in production)
- API gateway handles rate limiting, authentication for external callers
- Dashboard protected by OIDC/SSO

## Failure Modes and Recovery

| Failure | Impact | Recovery |
|---------|--------|----------|
| Agent loses connection | In-progress tests may be orphaned | Agent reconnects, control plane marks orphaned runs as failed after timeout |
| Control plane restart | Agents reconnect automatically | Work queue replayed from database, in-progress runs recovered or timed out |
| Database unavailable | No new runs can be scheduled | Control plane queues requests in memory (bounded), alerts on prolonged outage |
| Git provider unavailable | Cannot discover new tests or clone repos | Use cached manifests, cached repo clones on agents, alert operators |

## Extension Points

The system should be designed with these extension points:

### Git Provider Adapters
New Git platforms can be added by implementing the Git provider interface.

### Result Parsers
New test result formats can be supported by adding parsers (JUnit XML, Jest JSON, TAP, etc.).

### Notification Channels
New notification destinations (Slack, PagerDuty, email) via a notification interface.

### Execution Drivers
Beyond subprocess and container, future drivers could include: SSH to remote host, Kubernetes Job, cloud functions.
