# Configuration Reference

This document provides a complete reference for configuring Conductor components.

## Table of Contents

- [Control Plane Configuration](#control-plane-configuration)
- [Agent Configuration](#agent-configuration)
- [Dashboard Configuration](#dashboard-configuration)
- [Docker Compose Configuration](#docker-compose-configuration)
- [Kubernetes Configuration](#kubernetes-configuration)

## Control Plane Configuration

The control plane is configured via environment variables with the `CONDUCTOR_` prefix.

### Server Settings

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `CONDUCTOR_HTTP_PORT` | HTTP API port | `8080` | No |
| `CONDUCTOR_GRPC_PORT` | gRPC port | `9090` | No |
| `CONDUCTOR_METRICS_PORT` | Prometheus metrics port | `9091` | No |
| `CONDUCTOR_SHUTDOWN_TIMEOUT` | Graceful shutdown timeout | `30s` | No |

### Database Settings

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `CONDUCTOR_DATABASE_URL` | PostgreSQL connection string | - | Yes |
| `CONDUCTOR_DATABASE_MAX_OPEN_CONNS` | Maximum open connections | `25` | No |
| `CONDUCTOR_DATABASE_MAX_IDLE_CONNS` | Maximum idle connections | `5` | No |
| `CONDUCTOR_DATABASE_CONN_MAX_LIFETIME` | Connection max lifetime | `5m` | No |
| `CONDUCTOR_DATABASE_CONN_MAX_IDLE_TIME` | Connection max idle time | `1m` | No |
| `CONDUCTOR_DATABASE_QUERY_TIMEOUT` | Default query timeout | `30s` | No |

Example connection string:
```
postgres://user:password@host:5432/conductor?sslmode=require
```

### Storage Settings (S3/MinIO)

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `CONDUCTOR_STORAGE_ENDPOINT` | S3/MinIO endpoint URL | - | No* |
| `CONDUCTOR_STORAGE_BUCKET` | Artifact bucket name | - | Yes |
| `CONDUCTOR_STORAGE_REGION` | AWS region | `us-east-1` | No |
| `CONDUCTOR_STORAGE_ACCESS_KEY_ID` | Access key | - | Yes |
| `CONDUCTOR_STORAGE_SECRET_ACCESS_KEY` | Secret key | - | Yes |
| `CONDUCTOR_STORAGE_USE_SSL` | Enable SSL for MinIO | `true` | No |
| `CONDUCTOR_STORAGE_PATH_STYLE` | Use path-style addressing | `true` | No |

*Required for MinIO, leave empty for AWS S3.

### Redis Settings (Optional)

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `CONDUCTOR_REDIS_URL` | Redis connection URL | - | No |
| `CONDUCTOR_REDIS_POOL_SIZE` | Connection pool size | `10` | No |
| `CONDUCTOR_REDIS_MIN_IDLE_CONNS` | Minimum idle connections | `2` | No |
| `CONDUCTOR_REDIS_DIAL_TIMEOUT` | Connection timeout | `5s` | No |
| `CONDUCTOR_REDIS_READ_TIMEOUT` | Read timeout | `3s` | No |
| `CONDUCTOR_REDIS_WRITE_TIMEOUT` | Write timeout | `3s` | No |

### Authentication Settings

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `CONDUCTOR_AUTH_JWT_SECRET` | JWT signing secret (32+ chars) | - | Yes |
| `CONDUCTOR_AUTH_JWT_EXPIRATION` | JWT token expiration | `24h` | No |
| `CONDUCTOR_AUTH_OIDC_ENABLED` | Enable OIDC authentication | `false` | No |
| `CONDUCTOR_AUTH_OIDC_ISSUER_URL` | OIDC provider issuer URL | - | If OIDC |
| `CONDUCTOR_AUTH_OIDC_CLIENT_ID` | OIDC client ID | - | If OIDC |
| `CONDUCTOR_AUTH_OIDC_CLIENT_SECRET` | OIDC client secret | - | If OIDC |
| `CONDUCTOR_AUTH_OIDC_REDIRECT_URL` | OIDC callback URL | - | If OIDC |

### Agent Management Settings

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `CONDUCTOR_AGENT_HEARTBEAT_TIMEOUT` | Time before agent marked offline | `90s` | No |
| `CONDUCTOR_AGENT_DEFAULT_TEST_TIMEOUT` | Default test timeout | `30m` | No |
| `CONDUCTOR_AGENT_MAX_TEST_TIMEOUT` | Maximum allowed test timeout | `4h` | No |
| `CONDUCTOR_AGENT_RESULT_STREAM_BUFFER_SIZE` | Result streaming buffer | `100` | No |

### Git Provider Settings

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `CONDUCTOR_GIT_PROVIDER` | Provider type (github, gitlab, bitbucket) | `github` | No |
| `CONDUCTOR_GIT_TOKEN` | Personal access token | - | No |
| `CONDUCTOR_GIT_BASE_URL` | API base URL (for enterprise) | - | No |
| `CONDUCTOR_GIT_WEBHOOK_SECRET` | GitHub webhook secret | - | No |
| `CONDUCTOR_GITLAB_WEBHOOK_SECRET` | GitLab webhook secret | - | No |
| `CONDUCTOR_BITBUCKET_WEBHOOK_SECRET` | Bitbucket webhook secret | - | No |
| `CONDUCTOR_GIT_APP_ID` | GitHub App ID | - | No |
| `CONDUCTOR_GIT_APP_PRIVATE_KEY_PATH` | GitHub App private key path | - | No |
| `CONDUCTOR_GIT_APP_INSTALLATION_ID` | GitHub App installation ID | - | No |

GitHub App authentication is enabled when the app ID, installation ID, and private key path are all set. Otherwise Conductor uses the personal access token if provided.

### Webhook Settings

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `CONDUCTOR_WEBHOOK_ENABLED` | Enable webhook handling | `true` | No |
| `CONDUCTOR_WEBHOOK_BASE_URL` | External URL for status links | - | No |

### Logging Settings

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `CONDUCTOR_LOG_LEVEL` | Log level (debug, info, warn, error) | `info` | No |
| `CONDUCTOR_LOG_FORMAT` | Log format (json, console) | `json` | No |

### Observability Settings

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `CONDUCTOR_TRACING_ENABLED` | Enable OpenTelemetry tracing | `false` | No |
| `CONDUCTOR_TRACING_ENDPOINT` | OTLP collector endpoint | - | No |
| `CONDUCTOR_TRACING_INSECURE` | Disable TLS for tracing | `true` | No |
| `CONDUCTOR_TRACING_SAMPLE_RATE` | Sampling rate (0.0-1.0) | `1.0` | No |
| `CONDUCTOR_ENVIRONMENT` | Deployment environment name | `development` | No |

## Agent Configuration

The agent is configured via environment variables with the `CONDUCTOR_AGENT_` prefix.

### Identity Settings

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `CONDUCTOR_AGENT_ID` | Unique agent identifier | hostname | No |
| `CONDUCTOR_AGENT_NAME` | Human-readable name | hostname | No |
| `CONDUCTOR_AGENT_NETWORK_ZONES` | Network zones (comma-separated) | `default` | No |
| `CONDUCTOR_AGENT_RUNTIMES` | Available runtimes (comma-separated) | - | No |
| `CONDUCTOR_AGENT_LABELS` | Labels (key=value,key=value) | - | No |

### Control Plane Connection

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `CONDUCTOR_AGENT_CONTROL_PLANE_URL` | Control plane gRPC address | - | Yes |
| `CONDUCTOR_AGENT_TOKEN` | Authentication token | - | Yes |
| `CONDUCTOR_AGENT_HEARTBEAT_INTERVAL` | Heartbeat interval | `30s` | No |
| `CONDUCTOR_AGENT_RECONNECT_MIN_INTERVAL` | Min reconnect delay | `1s` | No |
| `CONDUCTOR_AGENT_RECONNECT_MAX_INTERVAL` | Max reconnect delay | `60s` | No |

### TLS Settings

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `CONDUCTOR_AGENT_TLS_ENABLED` | Enable TLS | `false` | No |
| `CONDUCTOR_AGENT_TLS_CERT_FILE` | TLS certificate path | - | If TLS |
| `CONDUCTOR_AGENT_TLS_KEY_FILE` | TLS key path | - | If TLS |
| `CONDUCTOR_AGENT_TLS_CA_FILE` | CA certificate path | - | No |
| `CONDUCTOR_AGENT_TLS_INSECURE_SKIP_VERIFY` | Skip cert verification | `false` | No |

### Execution Settings

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `CONDUCTOR_AGENT_MAX_PARALLEL` | Max concurrent test runs | `4` | No |
| `CONDUCTOR_AGENT_DEFAULT_TIMEOUT` | Default test timeout | `30m` | No |
| `CONDUCTOR_AGENT_WORKSPACE_DIR` | Test workspace directory | `/tmp/conductor/workspaces` | No |
| `CONDUCTOR_AGENT_CACHE_DIR` | Repository cache directory | `/tmp/conductor/cache` | No |
| `CONDUCTOR_AGENT_STATE_DIR` | Persistent state directory | `/var/lib/conductor` | No |

### Docker Settings

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `CONDUCTOR_AGENT_DOCKER_ENABLED` | Enable Docker execution | `true` | No |
| `CONDUCTOR_AGENT_DOCKER_HOST` | Docker daemon socket | `unix:///var/run/docker.sock` | No |

### Storage Settings (for artifact uploads)

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `CONDUCTOR_AGENT_STORAGE_ENDPOINT` | S3/MinIO endpoint | - | No |
| `CONDUCTOR_AGENT_STORAGE_ACCESS_KEY` | Access key | - | No |
| `CONDUCTOR_AGENT_STORAGE_SECRET_KEY` | Secret key | - | No |
| `CONDUCTOR_AGENT_STORAGE_BUCKET` | Bucket name | - | No |
| `CONDUCTOR_AGENT_STORAGE_REGION` | Region | `us-east-1` | No |
| `CONDUCTOR_AGENT_STORAGE_USE_SSL` | Enable SSL | `true` | No |

### Secrets Settings

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `CONDUCTOR_AGENT_SECRETS_PROVIDER` | Secrets provider (`vault`) | - | No |
| `CONDUCTOR_AGENT_SECRETS_VAULT_ADDR` | Vault API address | - | If vault |
| `CONDUCTOR_AGENT_SECRETS_VAULT_TOKEN` | Vault token | - | If vault |
| `CONDUCTOR_AGENT_SECRETS_VAULT_NAMESPACE` | Vault namespace header | - | No |
| `CONDUCTOR_AGENT_SECRETS_VAULT_MOUNT` | Vault KV v2 mount path | `secret` | No |
| `CONDUCTOR_AGENT_SECRETS_VAULT_TIMEOUT` | Vault request timeout | `10s` | No |

### Resource Thresholds

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `CONDUCTOR_AGENT_RESOURCE_CHECK_INTERVAL` | Resource check interval | `10s` | No |
| `CONDUCTOR_AGENT_CPU_THRESHOLD` | CPU threshold (%) | `90` | No |
| `CONDUCTOR_AGENT_MEMORY_THRESHOLD` | Memory threshold (%) | `90` | No |
| `CONDUCTOR_AGENT_DISK_THRESHOLD` | Disk threshold (%) | `90` | No |

### Logging Settings

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `CONDUCTOR_AGENT_LOG_LEVEL` | Log level | `info` | No |
| `CONDUCTOR_AGENT_LOG_FORMAT` | Log format | `json` | No |

## Dashboard Configuration

The dashboard is configured via environment variables at build or runtime.

| Variable | Description | Default |
|----------|-------------|---------|
| `VITE_API_URL` | Control plane API URL | `http://localhost:8080` |
| `VITE_WS_URL` | WebSocket URL | `ws://localhost:8080` |
| `VITE_ENABLE_ANALYTICS` | Enable analytics features | `true` |

## Docker Compose Configuration

The `docker-compose.yml` uses environment variables that can be set in a `.env` file:

```bash
# .env file

# Database
POSTGRES_USER=conductor
POSTGRES_PASSWORD=your_secure_password
POSTGRES_DB=conductor

# MinIO
MINIO_ROOT_USER=conductor
MINIO_ROOT_PASSWORD=your_secure_password
MINIO_BUCKET=conductor-artifacts

# Control Plane
CONDUCTOR_ENV=production
LOG_LEVEL=info
JWT_SECRET=your-32-character-minimum-secret
AGENT_TOKEN_SECRET=your-agent-token-secret

# Git Integration (optional)
GITHUB_APP_ID=12345
GITHUB_WEBHOOK_SECRET=your-webhook-secret

# Ports (optional overrides)
POSTGRES_PORT=5432
REDIS_PORT=6379
CONTROL_PLANE_HTTP_PORT=8080
CONTROL_PLANE_GRPC_PORT=9090
DASHBOARD_PORT=3000

# Agent scaling
AGENT_REPLICAS=2
AGENT_MAX_CONCURRENT_TESTS=4
```

## Kubernetes Configuration

### ConfigMap

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: conductor-config
data:
  CONDUCTOR_HTTP_PORT: "8080"
  CONDUCTOR_GRPC_PORT: "9090"
  CONDUCTOR_METRICS_PORT: "9091"
  CONDUCTOR_LOG_LEVEL: "info"
  CONDUCTOR_LOG_FORMAT: "json"
  CONDUCTOR_STORAGE_REGION: "us-east-1"
  CONDUCTOR_STORAGE_USE_SSL: "true"
  CONDUCTOR_GIT_PROVIDER: "github"
```

### Secret

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: conductor-secrets
type: Opaque
stringData:
  CONDUCTOR_DATABASE_URL: "postgres://user:pass@postgres:5432/conductor"
  CONDUCTOR_AUTH_JWT_SECRET: "your-32-character-minimum-secret"
  CONDUCTOR_STORAGE_ACCESS_KEY_ID: "access-key"
  CONDUCTOR_STORAGE_SECRET_ACCESS_KEY: "secret-key"
  CONDUCTOR_GIT_TOKEN: "ghp_xxxxxxxxxxxx"
  CONDUCTOR_GIT_WEBHOOK_SECRET: "webhook-secret"
```

### Control Plane Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: conductor-control-plane
spec:
  replicas: 2
  selector:
    matchLabels:
      app: conductor-control-plane
  template:
    metadata:
      labels:
        app: conductor-control-plane
    spec:
      containers:
      - name: control-plane
        image: conductor/control-plane:latest
        ports:
        - containerPort: 8080
          name: http
        - containerPort: 9090
          name: grpc
        - containerPort: 9091
          name: metrics
        envFrom:
        - configMapRef:
            name: conductor-config
        - secretRef:
            name: conductor-secrets
        resources:
          requests:
            cpu: 500m
            memory: 512Mi
          limits:
            cpu: 2000m
            memory: 1Gi
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
```

### Agent DaemonSet

```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: conductor-agent
spec:
  selector:
    matchLabels:
      app: conductor-agent
  template:
    metadata:
      labels:
        app: conductor-agent
    spec:
      containers:
      - name: agent
        image: conductor/agent:latest
        env:
        - name: CONDUCTOR_AGENT_CONTROL_PLANE_URL
          value: "conductor-control-plane:9090"
        - name: CONDUCTOR_AGENT_TOKEN
          valueFrom:
            secretKeyRef:
              name: conductor-secrets
              key: AGENT_TOKEN
        - name: CONDUCTOR_AGENT_NETWORK_ZONES
          value: "kubernetes"
        - name: CONDUCTOR_AGENT_DOCKER_ENABLED
          value: "true"
        volumeMounts:
        - name: docker-sock
          mountPath: /var/run/docker.sock
        - name: workspace
          mountPath: /workspace
        resources:
          requests:
            cpu: 500m
            memory: 512Mi
          limits:
            cpu: 2000m
            memory: 2Gi
      volumes:
      - name: docker-sock
        hostPath:
          path: /var/run/docker.sock
      - name: workspace
        emptyDir: {}
```

### Services

```yaml
apiVersion: v1
kind: Service
metadata:
  name: conductor-control-plane
spec:
  selector:
    app: conductor-control-plane
  ports:
  - name: http
    port: 8080
    targetPort: 8080
  - name: grpc
    port: 9090
    targetPort: 9090
---
apiVersion: v1
kind: Service
metadata:
  name: conductor-dashboard
spec:
  selector:
    app: conductor-dashboard
  type: LoadBalancer
  ports:
  - port: 80
    targetPort: 80
```
