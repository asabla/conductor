# AGENTS.md - Conductor Test Harness

## Project Overview

Conductor is a distributed test orchestration platform that coordinates test execution across polyglot microservices:

- **Control Plane**: Central coordination service (Go) - manages test registry, schedules work, coordinates agents
- **Agents**: Lightweight processes (Go) deployed in private networks to execute tests
- **Dashboard**: Web interface (React/TypeScript) for visibility into test execution and system health

See `docs/01_initial_implementation_plan/` for detailed architecture and implementation plans.

## Technology Stack

| Component | Technology |
|-----------|------------|
| Control Plane | Go, gRPC + grpc-gateway, PostgreSQL, S3/MinIO, Redis (optional) |
| Agent | Go, gRPC, Docker SDK, SQLite |
| Dashboard | React or SvelteKit, TypeScript, TanStack Query, Tailwind CSS, shadcn/ui |
| Protocol | gRPC with bidirectional streaming, Protocol Buffers |
| Observability | OpenTelemetry, Prometheus, structured JSON logging |

## Build, Lint, and Test Commands

### Go (Control Plane & Agent)

```bash
# Build
go build -o bin/control-plane ./cmd/control-plane
go build -o bin/agent ./cmd/agent

# Run all tests
go test ./...

# Run single test
go test -run TestFunctionName ./path/to/package
go test -run "TestSuite/TestCase" ./path/to/package

# Lint and format
golangci-lint run ./...
gofmt -w . && goimports -w .

# Generate protobuf
buf generate
```

### Dashboard (TypeScript/React)

```bash
cd web/

npm install          # Install dependencies
npm run dev          # Development server
npm run build        # Production build
npm test             # Run all tests
npm test -- path/to/file.test.ts           # Single test file
npm test -- -t "test name pattern"         # Tests matching pattern
npm run lint         # Lint
npm run format       # Format
```

### Database Migrations

```bash
go run ./cmd/control-plane migrate up
go run ./cmd/control-plane migrate create migration_name
```

## Code Style Guidelines

### Go

**Naming**: `camelCase` for unexported, `PascalCase` for exported. Use `-er` suffix for single-method interfaces. Avoid stuttering (`agent.Agent` not `agent.AgentStruct`).

**Imports**: Group as stdlib → external → internal. Use goimports.
```go
import (
    "context"
    "fmt"

    "google.golang.org/grpc"

    "github.com/org/conductor/internal/scheduler"
)
```

**Error Handling**: Always check errors. Wrap with context: `fmt.Errorf("failed to connect: %w", err)`. Return early to reduce nesting.

**Context**: Pass `context.Context` as first parameter for cancellation and timeouts.

**Concurrency**: Prefer channels for communication, mutexes for state. Always handle goroutine lifecycle. Use `errgroup` for coordination.

**Logging**: Structured JSON logging with levels (debug, info, warn, error). Include correlation IDs.

### TypeScript/React

**Naming**: Components `PascalCase` (`TestRunList.tsx`), hooks `useCamelCase` (`useTestRuns.ts`), utilities `camelCase`.

**Types**: Prefer `interface` for object shapes, `type` for unions. Avoid `any`; use `unknown` and narrow.

**Components**: Functional components with hooks. Extract logic to custom hooks. Use TanStack Query for server state.

**Imports**: Group as react → external → internal → relative.

## Architecture Patterns

### Service Interfaces

```go
type Scheduler interface {
    ScheduleRun(ctx context.Context, req ScheduleRequest) (*TestRun, error)
    CancelRun(ctx context.Context, runID string) error
}
```

### gRPC Streaming

Bidirectional streams between agents and control plane:
- Agent → CP: Register, Heartbeat, WorkAccepted, ResultStream
- CP → Agent: RegisterResponse, AssignWork, CancelWork, Drain

### Configuration

Use environment variables: `DATABASE_URL`, `ARTIFACT_STORAGE_URL`, Git provider credentials. Validate at startup.

## Testing Guidelines

**Unit Tests**: Test in isolation, mock dependencies. Place in same package with `_test.go` suffix.

**Integration Tests**: Use testcontainers for database tests.

```go
func TestScheduler_ScheduleRun(t *testing.T) {
    sched := NewScheduler(mockRegistry, mockAgentMgr)
    run, err := sched.ScheduleRun(ctx, req)
    require.NoError(t, err)
    assert.Equal(t, "pending", run.Status)
}
```

## Project Structure

```
conductor/
├── cmd/
│   ├── control-plane/    # Control plane entrypoint
│   └── agent/            # Agent entrypoint
├── internal/
│   ├── scheduler/        # Test scheduling logic
│   ├── agent/            # Agent manager
│   ├── registry/         # Test registry service
│   ├── result/           # Result collection and parsing
│   ├── git/              # Git provider abstraction
│   └── notification/     # Notification service
├── api/proto/            # Protocol buffer definitions
├── web/                  # Dashboard frontend
├── migrations/           # Database migrations
└── docs/                 # Documentation
```

## Key Implementation Notes

- **Agents connect outbound**: No inbound firewall rules required in private networks
- **Language agnostic**: Tests invoked via CLI; harness doesn't care about test language
- **Git provider abstraction**: Support GitHub initially, designed for GitLab/Bitbucket later
- **Execution modes**: Subprocess (fast) or Container (isolated)
- **Heartbeat timeout**: 90 seconds to declare agent offline
- **Default test timeout**: 30 minutes
