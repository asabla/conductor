# Getting Started with Conductor

This guide walks you through setting up Conductor and running your first test.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Quick Start with Docker Compose](#quick-start-with-docker-compose)
- [Create Your First Service](#create-your-first-service)
- [Trigger Your First Test Run](#trigger-your-first-test-run)
- [View Results in Dashboard](#view-results-in-dashboard)
- [Next Steps](#next-steps)

## Prerequisites

Before you begin, ensure you have:

- **Docker** 20.10+ and **Docker Compose** v2
- **Git** for cloning repositories
- A GitHub, GitLab, or Bitbucket account (optional, for Git integration)

For development from source:
- **Go** 1.22+
- **Node.js** 20+ (for dashboard)
- **PostgreSQL** 15+

## Quick Start with Docker Compose

### 1. Clone the Repository

```bash
git clone https://github.com/conductor/conductor.git
cd conductor
```

### 2. Configure Environment (Optional)

Create a `.env` file for custom configuration:

```bash
# .env
POSTGRES_PASSWORD=your_secure_password
MINIO_ROOT_PASSWORD=your_secure_password
JWT_SECRET=your-32-character-minimum-secret
```

### 3. Start Services

```bash
docker compose up -d
```

This starts:
- **PostgreSQL** - Database for test registry and results
- **MinIO** - S3-compatible storage for artifacts
- **Redis** - Caching and real-time updates
- **Control Plane** - Central coordination service
- **Agent** - Test execution worker
- **Dashboard** - Web interface

### 4. Verify Services

```bash
# Check service health
docker compose ps

# View logs
docker compose logs -f control-plane
```

### 5. Access the Dashboard

Open http://localhost:3000 in your browser.

Other endpoints:
- **REST API**: http://localhost:8080
- **gRPC**: localhost:9090
- **MinIO Console**: http://localhost:9001 (user: `conductor`, password: `conductor_secret`)

## Create Your First Service

### Option A: Using the CLI

Install the CLI:

```bash
go install github.com/conductor/conductor/cmd/conductor-ctl@latest
```

Configure the CLI:

```bash
conductor-ctl config set api-url http://localhost:8080
```

Create a service:

```bash
conductor-ctl services create \
  --name my-service \
  --git-url https://github.com/your-org/your-repo.git \
  --default-branch main
```

### Option B: Using the API

```bash
curl -X POST http://localhost:8080/api/v1/services \
  -H "Content-Type: application/json" \
  -d '{
    "name": "my-service",
    "git_url": "https://github.com/your-org/your-repo.git",
    "default_branch": "main",
    "owner": "platform-team"
  }'
```

### Option C: Using the Dashboard

1. Navigate to http://localhost:3000
2. Click **Services** in the sidebar
3. Click **Add Service**
4. Fill in the service details and click **Create**

## Add a Test Manifest

Add a `.testharness.yaml` file to your repository root:

```yaml
version: "1"

service:
  name: my-service
  description: My example microservice
  owner: platform-team
  contact:
    email: platform@example.com
    slack: "#platform-alerts"

defaults:
  execution_type: subprocess
  timeout_seconds: 300

tests:
  - name: unit-tests
    description: Run unit tests
    command: npm test
    result_format: jest
    tags:
      - unit
      - fast

  - name: lint
    description: Run linter
    command: npm run lint
    result_format: json
    tags:
      - lint
      - fast

  - name: integration-tests
    description: Run integration tests
    command: npm run test:integration
    result_format: jest
    timeout_seconds: 600
    tags:
      - integration
    depends_on:
      - unit-tests
```

Commit and push this file to your repository.

## Trigger Your First Test Run

### Sync the Service

First, sync the service to discover tests from the manifest:

```bash
conductor-ctl services sync my-service
```

Or via API:

```bash
curl -X POST http://localhost:8080/api/v1/services/{service_id}/sync
```

### Trigger a Run

Using CLI:

```bash
conductor-ctl runs create \
  --service my-service \
  --branch main
```

Using API:

```bash
curl -X POST http://localhost:8080/api/v1/runs \
  -H "Content-Type: application/json" \
  -d '{
    "service_id": "your-service-id",
    "branch": "main"
  }'
```

### Monitor Progress

Watch the run in real-time:

```bash
conductor-ctl runs watch {run_id}
```

Or view in the dashboard at http://localhost:3000/runs/{run_id}

## View Results in Dashboard

### Run Details

Navigate to the run in the dashboard to see:

- **Summary** - Pass/fail counts, duration, status
- **Test Results** - Individual test outcomes with details
- **Logs** - Real-time and historical log output
- **Artifacts** - Screenshots, reports, and other files

### Service Overview

The service page shows:

- **Recent Runs** - Latest test executions
- **Test Health** - Pass rate trends over time
- **Test Definitions** - All discovered tests from the manifest
- **Configuration** - Service settings

### Analytics

The analytics dashboard provides:

- **Pass Rate Trends** - Historical success rates
- **Duration Trends** - Test execution time over time
- **Flaky Tests** - Tests that fail intermittently
- **Agent Utilization** - Resource usage across agents

## Next Steps

Now that you have Conductor running:

1. **Set up Git webhooks** - Automatically trigger tests on push. See [Git Integration](git-integration.md).

2. **Configure notifications** - Get alerts on Slack, email, or Teams. See [Notifications](notifications.md).

3. **Deploy more agents** - Scale test execution capacity. See [Agent Deployment](agent-deployment.md).

4. **Customize test manifests** - Add more tests and configure execution. See [Test Manifest Reference](test-manifest.md).

5. **Integrate with CI/CD** - Connect Conductor to your existing pipelines. See [API Documentation](api.md).

## Troubleshooting

### Services not starting

Check Docker logs:
```bash
docker compose logs control-plane
docker compose logs agent
```

### Agent not connecting

Verify the agent can reach the control plane:
```bash
docker compose exec agent curl -v control-plane:9090
```

### Tests not running

1. Ensure the manifest is valid: `conductor-ctl services validate .testharness.yaml`
2. Check agent status: `conductor-ctl agents list`
3. Review agent logs: `docker compose logs agent`

For more help, see the [Troubleshooting Guide](troubleshooting.md).
