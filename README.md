# Conductor

A distributed test orchestration platform that coordinates test execution across polyglot microservices and frontend applications.

```
                             ┌─────────────────────────────────────┐
                             │          CONTROL PLANE              │
                             │  ┌─────────┐ ┌─────────┐ ┌───────┐  │
                             │  │Scheduler│ │Registry │ │Results│  │
                             │  └─────────┘ └─────────┘ └───────┘  │
                             │  ┌─────────┐ ┌─────────┐ ┌───────┐  │
                             │  │   Git   │ │Dashboard│ │Notify │  │
                             │  │ Sync    │ │   API   │ │Service│  │
                             │  └─────────┘ └─────────┘ └───────┘  │
                             └──────────────────┬──────────────────┘
                                               │
                              gRPC (agents connect outbound)
                                               │
             ┌─────────────────────────────────┼─────────────────────────────────┐
             │                                 │                                 │
             ▼                                 ▼                                 ▼
       ┌──────────┐                     ┌──────────┐                     ┌──────────┐
       │  Agent   │                     │  Agent   │                     │  Agent   │
       │ (Zone A) │                     │ (Zone B) │                     │ (Zone C) │
       └────┬─────┘                     └────┬─────┘                     └────┬─────┘
            │                                │                                │
       ┌────┴─────┐                     ┌────┴─────┐                     ┌────┴─────┐
       │ Internal │                     │ Internal │                     │ Internal │
       │ Services │                     │ Services │                     │ Services │
       └──────────┘                     └──────────┘                     └──────────┘
```

## Features

- **Distributed Test Execution** - Run tests across multiple network zones and environments
- **Language Agnostic** - Execute tests written in any language or framework
- **Git Integration** - Automatic test discovery from GitHub, GitLab, and Bitbucket
- **Real-time Dashboard** - Monitor test runs, view results, and analyze trends
- **Flexible Execution** - Run tests as subprocesses or in Docker containers
- **Rich Notifications** - Slack, email, Microsoft Teams, and webhook integrations
- **Result Aggregation** - Unified view of test results with support for JUnit, Jest, Playwright, and more

## Quick Start

### Prerequisites

- Go 1.24+
- Node.js 20+ (for dashboard)
- Docker and Docker Compose
- PostgreSQL 15+ (or use Docker Compose)

### Start with Docker Compose

```bash
# Clone the repository
git clone https://github.com/conductor/conductor.git
cd conductor

# (Optional) Copy default environment config
cp .env.example .env

# Start all services
docker compose up -d

# Or via Makefile
make docker-up

# Dev auth bypass (optional)
export VITE_AUTH_DISABLED=true

# Access the dashboard
open http://localhost:3000
```

Services:
- **Dashboard**: http://localhost:3000
- **Control Plane API**: http://localhost:8080
- **Control Plane gRPC**: localhost:9090
- **MinIO Console**: http://localhost:9001

### Build from Source

```bash
# Build control plane
go build -o bin/control-plane ./cmd/control-plane

# Build agent
go build -o bin/agent ./cmd/agent

# Build CLI
go build -o bin/conductor-ctl ./cmd/conductor-ctl

# Run control plane
export CONDUCTOR_DATABASE_URL="postgres://conductor:conductor_secret@localhost:5432/conductor?sslmode=disable"
export CONDUCTOR_STORAGE_ENDPOINT="http://localhost:9000"
export CONDUCTOR_STORAGE_BUCKET="conductor-artifacts"
export CONDUCTOR_STORAGE_ACCESS_KEY_ID="conductor"
export CONDUCTOR_STORAGE_SECRET_ACCESS_KEY="conductor_secret"
export CONDUCTOR_AUTH_JWT_SECRET="your-32-character-secret-key-here"
./bin/control-plane

# Run agent (in another terminal)
export CONDUCTOR_AGENT_CONTROL_PLANE_URL="localhost:9090"
export CONDUCTOR_AGENT_TOKEN="your-agent-token"
./bin/agent
```

### Add Your First Service

1. Add a `.testharness.yaml` manifest to your repository:

```yaml
version: "1"

service:
  name: my-service
  owner: platform-team

tests:
  - name: unit-tests
    command: go test ./...
    result_format: go_test
    timeout_seconds: 300
```

2. Register the service:

```bash
conductor-ctl services create \
  --name my-service \
  --git-url https://github.com/org/my-service.git
```

3. Trigger a test run:

```bash
conductor-ctl runs create --service my-service --branch main
```

## Configuration

### Control Plane Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `CONDUCTOR_HTTP_PORT` | HTTP API port | `8080` |
| `CONDUCTOR_GRPC_PORT` | gRPC port | `9090` |
| `CONDUCTOR_DATABASE_URL` | PostgreSQL connection string | Required |
| `CONDUCTOR_STORAGE_ENDPOINT` | S3/MinIO endpoint | Required |
| `CONDUCTOR_STORAGE_BUCKET` | Artifact storage bucket | Required |
| `CONDUCTOR_AUTH_JWT_SECRET` | JWT signing secret (32+ chars) | Required |
| `CONDUCTOR_NOTIFICATIONS_EMAIL_SMTP_HOST` | SMTP host for email notifications | Optional |
| `CONDUCTOR_GIT_TOKEN` | GitHub/GitLab personal access token | Optional |
| `CONDUCTOR_GIT_WEBHOOK_SECRET` | Webhook signature secret | Optional |

### Agent Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `CONDUCTOR_AGENT_CONTROL_PLANE_URL` | Control plane gRPC address | Required |
| `CONDUCTOR_AGENT_TOKEN` | Authentication token | Required |
| `CONDUCTOR_AGENT_MAX_PARALLEL` | Max concurrent test runs | `4` |
| `CONDUCTOR_AGENT_NETWORK_ZONES` | Network zones (comma-separated) | `default` |
| `CONDUCTOR_AGENT_DOCKER_ENABLED` | Enable Docker execution mode | `true` |

See [Configuration Reference](docs/configuration.md) for complete documentation.

## Documentation

- [Getting Started Guide](docs/getting-started.md)
- [Architecture Overview](docs/architecture.md)
- [Configuration Reference](docs/configuration.md)
- [API Documentation](docs/api.md)
- [Test Manifest Reference](docs/test-manifest.md)
- [Agent Deployment Guide](docs/agent-deployment.md)
- [Git Integration](docs/git-integration.md)
- [Notifications Setup](docs/notifications.md)
- [Troubleshooting](docs/troubleshooting.md)
- [Development Guide](docs/development.md)

## Project Structure

```
conductor/
├── cmd/
│   ├── control-plane/    # Control plane entrypoint
│   ├── agent/            # Agent entrypoint
│   └── conductor-ctl/    # CLI tool
├── internal/
│   ├── scheduler/        # Test scheduling logic
│   ├── agentmgr/         # Agent manager
│   ├── registry/         # Test registry service
│   ├── result/           # Result collection and parsing
│   ├── git/              # Git provider abstraction
│   ├── notification/     # Notification service
│   └── database/         # Database models and queries
├── api/proto/            # Protocol buffer definitions
├── web/                  # Dashboard frontend
├── migrations/           # Database migrations
└── docs/                 # Documentation
```

## Contributing

We welcome contributions! Please see our [Contributing Guide](CONTRIBUTING.md) for details.

### Development Setup

```bash
# Install dependencies
go mod download

# Run tests
go test ./...

# Run linter
golangci-lint run ./...

# Generate protobuf
buf generate
```

## License

Conductor is licensed under the Apache License 2.0. See [LICENSE](LICENSE) for details.

## Support

- [Documentation](docs/)
- [GitHub Issues](https://github.com/conductor/conductor/issues)
- [Discussions](https://github.com/conductor/conductor/discussions)
