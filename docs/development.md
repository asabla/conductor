# Development Guide

This guide covers setting up a development environment for contributing to Conductor.

## Prerequisites

### Required Tools

| Tool | Version | Purpose |
|------|---------|---------|
| Go | 1.21+ | Backend development |
| Node.js | 20+ | Dashboard development |
| Docker | 24+ | Containerization |
| Docker Compose | 2.20+ | Local development |
| PostgreSQL | 15+ | Database (or via Docker) |
| protoc | 3.21+ | Protocol buffer compilation |
| buf | 1.28+ | Protobuf tooling |

### Optional Tools

| Tool | Purpose |
|------|---------|
| golangci-lint | Go linting |
| pre-commit | Git hooks |
| k3d/kind | Local Kubernetes |
| grpcurl | gRPC debugging |

## Quick Setup

### 1. Clone the Repository

```bash
git clone https://github.com/conductor/conductor.git
cd conductor
```

### 2. Install Dependencies

```bash
# Go dependencies
go mod download

# Dashboard dependencies
cd web && npm install && cd ..

# Install development tools
make tools
```

### 3. Start Infrastructure

```bash
# Start PostgreSQL, Redis, MinIO
docker-compose up -d postgres redis minio

# Wait for services to be ready
docker-compose ps
```

### 4. Set Up Database

```bash
# Run migrations
go run ./cmd/control-plane migrate up

# Seed development data (optional)
go run ./cmd/control-plane seed
```

### 5. Run the Services

```bash
# Terminal 1: Control Plane
go run ./cmd/control-plane

# Terminal 2: Agent
go run ./cmd/agent

# Terminal 3: Dashboard
cd web && npm run dev
```

### 6. Verify Setup

```bash
# Health check
curl http://localhost:8080/healthz

# Dashboard
open http://localhost:5173
```

---

## Project Structure

```
conductor/
├── api/
│   ├── proto/              # Protocol buffer definitions
│   │   └── conductor/v1/   # API v1 definitions
│   └── gen/                # Generated Go code
├── cmd/
│   ├── control-plane/      # Control plane entrypoint
│   └── agent/              # Agent entrypoint
├── internal/
│   ├── agent/              # Agent implementation
│   ├── config/             # Configuration loading
│   ├── database/           # Database layer
│   ├── git/                # Git provider integration
│   ├── notification/       # Notification service
│   ├── registry/           # Test registry service
│   ├── result/             # Result parsing
│   ├── scheduler/          # Test scheduling
│   └── server/             # HTTP/gRPC servers
├── pkg/
│   ├── metrics/            # Prometheus metrics
│   ├── testutil/           # Test utilities
│   └── tracing/            # OpenTelemetry tracing
├── web/                    # Dashboard frontend
│   ├── src/
│   │   ├── components/     # React components
│   │   ├── hooks/          # Custom hooks
│   │   ├── pages/          # Page components
│   │   └── api/            # API client
│   └── public/             # Static assets
├── migrations/             # Database migrations
├── docs/                   # Documentation
└── scripts/                # Development scripts
```

---

## Building

### Build All

```bash
make build
```

### Build Individual Components

```bash
# Control Plane
go build -o bin/control-plane ./cmd/control-plane

# Agent
go build -o bin/agent ./cmd/agent

# Dashboard
cd web && npm run build
```

### Build Docker Images

```bash
# All images
make docker-build

# Individual images
docker build -t conductor/control-plane -f Dockerfile.control-plane .
docker build -t conductor/agent -f Dockerfile.agent .
docker build -t conductor/dashboard -f Dockerfile.dashboard .
```

### Build with Version Info

```bash
VERSION=$(git describe --tags --always)
go build -ldflags "-X main.version=$VERSION" -o bin/control-plane ./cmd/control-plane
```

---

## Testing

### Run All Tests

```bash
# Unit tests
go test ./...

# With coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Integration tests (requires Docker)
go test -tags=integration ./...
```

### Run Specific Tests

```bash
# Single package
go test ./internal/scheduler/...

# Single test
go test -run TestScheduler_AssignWork ./internal/scheduler

# Verbose output
go test -v -run TestScheduler ./internal/scheduler
```

### Test with Race Detection

```bash
go test -race ./...
```

### Dashboard Tests

```bash
cd web

# Run tests
npm test

# Watch mode
npm test -- --watch

# Coverage
npm test -- --coverage
```

### E2E Tests

```bash
# Start test environment
docker-compose -f docker-compose.test.yml up -d

# Run E2E tests
go test -tags=e2e ./test/e2e/...

# Cleanup
docker-compose -f docker-compose.test.yml down -v
```

---

## Code Generation

### Protocol Buffers

```bash
# Generate all protos
buf generate

# Lint protos
buf lint

# Check breaking changes
buf breaking --against '.git#branch=main'
```

### Wire (Dependency Injection)

```bash
# Generate wire files
go generate ./internal/wire/...
```

### Mocks

```bash
# Generate mocks
go generate ./...

# Or specific package
mockgen -source=internal/scheduler/scheduler.go -destination=internal/scheduler/mock_scheduler.go
```

---

## Linting and Formatting

### Go

```bash
# Format
gofmt -w .
goimports -w .

# Lint
golangci-lint run ./...

# Fix lint issues
golangci-lint run --fix ./...
```

### TypeScript/React

```bash
cd web

# Lint
npm run lint

# Fix lint issues
npm run lint -- --fix

# Format
npm run format
```

### Pre-commit Hooks

```bash
# Install hooks
pre-commit install

# Run manually
pre-commit run --all-files
```

---

## Database Development

### Create Migration

```bash
go run ./cmd/control-plane migrate create add_new_feature
```

This creates:
- `migrations/TIMESTAMP_add_new_feature.up.sql`
- `migrations/TIMESTAMP_add_new_feature.down.sql`

### Run Migrations

```bash
# Up
go run ./cmd/control-plane migrate up

# Down (1 step)
go run ./cmd/control-plane migrate down 1

# Status
go run ./cmd/control-plane migrate status
```

### Database Console

```bash
# Connect to dev database
docker-compose exec postgres psql -U conductor -d conductor

# Or use psql directly
psql $DATABASE_URL
```

### Reset Database

```bash
# Drop and recreate
docker-compose exec postgres psql -U conductor -c "DROP DATABASE conductor"
docker-compose exec postgres psql -U conductor -c "CREATE DATABASE conductor"
go run ./cmd/control-plane migrate up
```

---

## API Development

### gRPC Development

```bash
# List available services
grpcurl -plaintext localhost:9090 list

# Describe a service
grpcurl -plaintext localhost:9090 describe conductor.v1.AgentService

# Call a method
grpcurl -plaintext -d '{"service_id": "abc"}' localhost:9090 conductor.v1.TestService/ListTests
```

### REST API Testing

```bash
# Using curl
curl http://localhost:8080/api/v1/services

# Using httpie
http GET localhost:8080/api/v1/services

# With authentication
curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/v1/services
```

### OpenAPI/Swagger

Access the API documentation at:
- Swagger UI: http://localhost:8080/swagger/
- OpenAPI spec: http://localhost:8080/api/v1/openapi.json

---

## Debugging

### VS Code Launch Configuration

```json
// .vscode/launch.json
{
  "version": "0.2.0",
  "configurations": [
    {
      "name": "Control Plane",
      "type": "go",
      "request": "launch",
      "mode": "auto",
      "program": "${workspaceFolder}/cmd/control-plane",
      "env": {
        "DATABASE_URL": "postgres://conductor:conductor@localhost:5432/conductor?sslmode=disable"
      }
    },
    {
      "name": "Agent",
      "type": "go",
      "request": "launch",
      "mode": "auto",
      "program": "${workspaceFolder}/cmd/agent",
      "env": {
        "CONTROL_PLANE_ADDRESS": "localhost:9090"
      }
    }
  ]
}
```

### Delve Debugging

```bash
# Debug control plane
dlv debug ./cmd/control-plane

# Attach to running process
dlv attach $(pgrep control-plane)

# Debug test
dlv test ./internal/scheduler -- -test.run TestScheduler
```

### Logging

Enable debug logging:

```bash
LOG_LEVEL=debug go run ./cmd/control-plane
```

Component-specific logging:

```bash
LOG_LEVEL=info LOG_COMPONENT_LEVELS="scheduler=debug,git=debug" go run ./cmd/control-plane
```

---

## Dashboard Development

### Component Development

```bash
cd web

# Start Storybook (if available)
npm run storybook

# Generate component
npm run generate:component MyComponent
```

### API Client Generation

```bash
# Generate TypeScript API client from OpenAPI
npm run generate:api
```

### Environment Variables

Create `.env.local` for local development:

```bash
VITE_API_URL=http://localhost:8080
VITE_WS_URL=ws://localhost:8080
```

---

## Performance Profiling

### CPU Profiling

```bash
# Enable pprof
go run ./cmd/control-plane -pprof

# Collect CPU profile
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30

# Analyze
go tool pprof -http=:8081 profile.pb.gz
```

### Memory Profiling

```bash
# Heap profile
go tool pprof http://localhost:6060/debug/pprof/heap

# Allocations
go tool pprof http://localhost:6060/debug/pprof/allocs
```

### Trace Analysis

```bash
# Collect trace
curl -o trace.out http://localhost:6060/debug/pprof/trace?seconds=5

# Analyze
go tool trace trace.out
```

---

## Local Kubernetes Development

### Using k3d

```bash
# Create cluster
k3d cluster create conductor

# Load images
k3d image import conductor/control-plane:dev conductor/agent:dev

# Deploy
kubectl apply -k deploy/kubernetes/overlays/dev/
```

### Using kind

```bash
# Create cluster
kind create cluster --name conductor

# Load images
kind load docker-image conductor/control-plane:dev --name conductor

# Deploy
kubectl apply -k deploy/kubernetes/overlays/dev/
```

### Port Forwarding

```bash
# Control plane
kubectl port-forward svc/conductor-control-plane 8080:8080 9090:9090

# Dashboard
kubectl port-forward svc/conductor-dashboard 5173:80
```

---

## Useful Make Targets

```bash
make help          # Show all targets
make build         # Build all binaries
make test          # Run all tests
make lint          # Run linters
make fmt           # Format code
make docker-build  # Build Docker images
make proto         # Generate protobuf code
make migrate-up    # Run database migrations
make clean         # Clean build artifacts
make dev           # Start development environment
```

---

## Environment Variables Reference

### Control Plane

| Variable | Default | Description |
|----------|---------|-------------|
| `DATABASE_URL` | - | PostgreSQL connection string |
| `REDIS_URL` | - | Redis connection string |
| `ARTIFACT_STORAGE_URL` | - | S3/MinIO URL |
| `HTTP_PORT` | 8080 | HTTP server port |
| `GRPC_PORT` | 9090 | gRPC server port |
| `LOG_LEVEL` | info | Logging level |

### Agent

| Variable | Default | Description |
|----------|---------|-------------|
| `CONTROL_PLANE_ADDRESS` | - | Control plane gRPC address |
| `AGENT_NAME` | hostname | Agent identifier |
| `WORK_DIR` | /tmp/conductor | Test execution directory |
| `LOG_LEVEL` | info | Logging level |

---

## Next Steps

- [Contributing Guidelines](../CONTRIBUTING.md) - How to submit changes
- [Architecture](architecture.md) - System design overview
- [API Reference](api.md) - API documentation
