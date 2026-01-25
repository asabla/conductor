# Deployment

## Overview

This document covers deployment strategies for both the control plane and agents, including infrastructure requirements, configuration, and operational considerations.

## Control Plane Deployment

### Infrastructure Requirements

| Component | Minimum | Recommended | Notes |
|-----------|---------|-------------|-------|
| CPU | 2 cores | 4 cores | More for high concurrency |
| Memory | 2 GB | 4 GB | Depends on concurrent connections |
| Disk | 20 GB | 50 GB | Logs, temporary files |
| Database | PostgreSQL 14+ | PostgreSQL 15+ | Managed service recommended |
| Object Storage | S3-compatible | S3/MinIO | For artifacts |

### Deployment Options

#### Option 1: Container (Recommended)

```yaml
# docker-compose.yml (development/small deployments)
version: '3.8'
services:
  control-plane:
    image: testharness/control-plane:latest
    ports:
      - "8080:8080"   # HTTP API
      - "9090:9090"   # gRPC (agents)
    environment:
      - DATABASE_URL=postgres://user:pass@db:5432/testharness
      - ARTIFACT_STORAGE_URL=s3://bucket?endpoint=minio:9000
      - GITHUB_APP_ID=${GITHUB_APP_ID}
      - GITHUB_APP_PRIVATE_KEY_PATH=/secrets/github-app.pem
    volumes:
      - ./secrets:/secrets:ro
    depends_on:
      - db
      - minio

  db:
    image: postgres:15
    environment:
      - POSTGRES_DB=testharness
      - POSTGRES_USER=user
      - POSTGRES_PASSWORD=pass
    volumes:
      - pgdata:/var/lib/postgresql/data

  minio:
    image: minio/minio
    command: server /data --console-address ":9001"
    environment:
      - MINIO_ROOT_USER=minioadmin
      - MINIO_ROOT_PASSWORD=minioadmin
    volumes:
      - miniodata:/data

volumes:
  pgdata:
  miniodata:
```

#### Option 2: Kubernetes

```yaml
# control-plane-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: testharness-control-plane
spec:
  replicas: 2  # HA setup
  selector:
    matchLabels:
      app: testharness-control-plane
  template:
    metadata:
      labels:
        app: testharness-control-plane
    spec:
      containers:
        - name: control-plane
          image: testharness/control-plane:latest
          ports:
            - containerPort: 8080
              name: http
            - containerPort: 9090
              name: grpc
          env:
            - name: DATABASE_URL
              valueFrom:
                secretKeyRef:
                  name: testharness-secrets
                  key: database-url
          resources:
            requests:
              memory: "2Gi"
              cpu: "1"
            limits:
              memory: "4Gi"
              cpu: "2"
          livenessProbe:
            httpGet:
              path: /health
              port: 8080
            initialDelaySeconds: 10
            periodSeconds: 10
          readinessProbe:
            httpGet:
              path: /ready
              port: 8080
            initialDelaySeconds: 5
            periodSeconds: 5
---
apiVersion: v1
kind: Service
metadata:
  name: testharness-control-plane
spec:
  selector:
    app: testharness-control-plane
  ports:
    - name: http
      port: 80
      targetPort: 8080
    - name: grpc
      port: 9090
      targetPort: 9090
```

#### Option 3: Binary on VM

For environments without container orchestration:

1. Download binary for target OS/arch
2. Create systemd service unit
3. Configure via environment file
4. Set up reverse proxy (nginx/caddy) for TLS termination

```ini
# /etc/systemd/system/testharness.service
[Unit]
Description=Test Harness Control Plane
After=network.target

[Service]
Type=simple
User=testharness
EnvironmentFile=/etc/testharness/config.env
ExecStart=/usr/local/bin/testharness-control-plane
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

### High Availability

For production deployments:

**Multiple instances**:
- Run 2+ control plane instances behind load balancer
- HTTP/WebSocket: standard load balancing
- gRPC (agent connections): use sticky sessions or shared state

**Database**:
- Use managed PostgreSQL with replicas
- Or self-hosted with streaming replication

**Shared state** (for multiple instances):
- Redis for session state, real-time pubsub
- Agents can reconnect to any instance if using shared state

### TLS Configuration

**For HTTP/WebSocket**:
- Terminate TLS at load balancer/ingress
- Or use built-in TLS with cert files

**For gRPC (agent connections)**:
- TLS required in production
- Provide cert and key via configuration
- For mTLS: also provide CA for client verification

```yaml
tls:
  enabled: true
  cert_file: /etc/testharness/tls/server.crt
  key_file: /etc/testharness/tls/server.key
  # For mTLS
  client_ca_file: /etc/testharness/tls/ca.crt
  require_client_cert: true
```

## Agent Deployment

### Infrastructure Requirements

| Component | Minimum | Recommended | Notes |
|-----------|---------|-------------|-------|
| CPU | 2 cores | 4+ cores | Per parallel test |
| Memory | 2 GB | 8+ GB | Depends on test requirements |
| Disk | 20 GB | 100 GB | Repo cache, container images |
| Docker | Optional | Recommended | For container execution |
| Network | Outbound to control plane | Same | Plus access to test targets |

### Deployment Options

#### Option 1: Systemd Service

For bare metal or VMs:

```ini
# /etc/systemd/system/testharness-agent.service
[Unit]
Description=Test Harness Agent
After=network.target docker.service

[Service]
Type=simple
User=testharness
EnvironmentFile=/etc/testharness/agent.env
ExecStart=/usr/local/bin/testharness-agent
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

```bash
# /etc/testharness/agent.env
CONTROL_PLANE_URL=grpcs://testharness.example.com:9090
AGENT_TOKEN=secret-token-here
AGENT_NAME=agent-zone-a-1
NETWORK_ZONE=internal-a
MAX_PARALLEL=4
WORKSPACE_DIR=/var/lib/testharness/workspace
CACHE_DIR=/var/lib/testharness/cache
```

#### Option 2: Kubernetes DaemonSet

Run agent on specific nodes:

```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: testharness-agent
spec:
  selector:
    matchLabels:
      app: testharness-agent
  template:
    metadata:
      labels:
        app: testharness-agent
    spec:
      nodeSelector:
        testharness-agent: "true"  # Label nodes that should run agents
      containers:
        - name: agent
          image: testharness/agent:latest
          env:
            - name: CONTROL_PLANE_URL
              value: "grpcs://testharness-control-plane:9090"
            - name: AGENT_TOKEN
              valueFrom:
                secretKeyRef:
                  name: testharness-agent-token
                  key: token
            - name: NETWORK_ZONE
              valueFrom:
                fieldRef:
                  fieldPath: spec.nodeName
          volumeMounts:
            - name: docker-sock
              mountPath: /var/run/docker.sock
            - name: workspace
              mountPath: /var/lib/testharness
          resources:
            requests:
              memory: "2Gi"
              cpu: "2"
      volumes:
        - name: docker-sock
          hostPath:
            path: /var/run/docker.sock
        - name: workspace
          emptyDir:
            sizeLimit: 50Gi
```

#### Option 3: Kubernetes Deployment (with Docker-in-Docker)

For isolated container execution:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: testharness-agent
spec:
  replicas: 3
  selector:
    matchLabels:
      app: testharness-agent
  template:
    spec:
      containers:
        - name: agent
          image: testharness/agent:latest
          env:
            - name: DOCKER_HOST
              value: "tcp://localhost:2375"
          # ... other config
        - name: dind
          image: docker:dind
          securityContext:
            privileged: true
          volumeMounts:
            - name: dind-storage
              mountPath: /var/lib/docker
      volumes:
        - name: dind-storage
          emptyDir: {}
```

### Runtime Dependencies

Agents need access to runtimes for subprocess execution:

**Option A: Install on host**
```bash
# Example for Ubuntu
apt-get install python3.11 nodejs npm openjdk-17-jdk
```

**Option B: Use containers exclusively**
- All tests run in containers
- No host runtime dependencies
- Slower but more isolated

**Option C: Hybrid**
- Common runtimes on host for speed
- Containers for special requirements

### Agent Auto-scaling

#### Kubernetes HPA

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: testharness-agent-hpa
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: testharness-agent
  minReplicas: 2
  maxReplicas: 10
  metrics:
    - type: External
      external:
        metric:
          name: testharness_queue_depth
        target:
          type: AverageValue
          averageValue: 5  # Scale up when >5 pending runs per agent
```

#### Cloud Auto-scaling

For VM-based agents:
- Use cloud auto-scaling groups
- Scale based on queue depth metric from control plane
- Pre-bake agent image with runtime dependencies

### Agent Updates

**Rolling update strategy**:
1. Deploy new agent version to subset of agents
2. Drain old agents (stop accepting new work)
3. Wait for current work to complete
4. Stop old agents
5. Repeat until all updated

**For Kubernetes**:
```yaml
spec:
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxUnavailable: 1
      maxSurge: 1
```

## Network Configuration

### Control Plane Network

| Port | Protocol | Purpose | Exposure |
|------|----------|---------|----------|
| 8080 | HTTP | REST API, Dashboard | Public (behind LB/ingress) |
| 9090 | gRPC | Agent connections | Public (agents connect here) |
| 8081 | HTTP | Metrics (Prometheus) | Internal only |

### Agent Network

| Direction | Destination | Purpose |
|-----------|-------------|---------|
| Outbound | Control plane:9090 | gRPC connection |
| Outbound | Git providers | Clone repositories |
| Outbound | Container registries | Pull images |
| Outbound | Internal services | Test targets |

**Firewall rules for agents**:
- Outbound to control plane: required
- Outbound to internet: for public Git/registries (or use mirrors)
- Inbound: none required

### Network Zones

Map network zones to deployment locations:

| Zone | Location | Agents | Reachable Services |
|------|----------|--------|-------------------|
| internal-a | VPC-A | agent-a-1, agent-a-2 | service-a, db-a |
| internal-b | VPC-B | agent-b-1 | service-b, service-c |
| public | Cloud | agent-cloud-1 | public APIs only |

## Secrets Management

### Control Plane Secrets

| Secret | Purpose | Rotation |
|--------|---------|----------|
| Database password | PostgreSQL access | Quarterly |
| Git provider tokens | API access, cloning | Per provider policy |
| Agent auth secret | Validate agent tokens | Annually |
| TLS certificates | HTTPS, gRPC | Before expiry |

### Agent Secrets

| Secret | Purpose | Rotation |
|--------|---------|----------|
| Agent token | Authenticate to control plane | Annually |
| (Test-specific) | Injected at runtime | Varies |

### Secret Injection Options

**Environment variables**: Simple, works everywhere
```bash
GITHUB_TOKEN=ghp_xxx testharness-control-plane
```

**Kubernetes secrets**: Native K8s integration
```yaml
env:
  - name: GITHUB_TOKEN
    valueFrom:
      secretKeyRef:
        name: github-token
        key: token
```

**Vault integration**: Dynamic secrets
```yaml
vault:
  enabled: true
  address: https://vault.example.com
  auth_method: kubernetes
  secrets:
    - path: secret/data/testharness/github
      key: token
      env: GITHUB_TOKEN
```

## Monitoring and Alerting

### Metrics to Monitor

**Control Plane**:
- Request rate and latency (HTTP, gRPC)
- Active agent connections
- Queue depth (pending test runs)
- Test run completion rate
- Error rate

**Agents**:
- Connection status
- Resource utilization (CPU, memory, disk)
- Test execution rate
- Execution errors

**Infrastructure**:
- Database connections and query latency
- Object storage availability
- Network connectivity

### Alerting Rules

| Alert | Condition | Severity |
|-------|-----------|----------|
| No agents connected | agent_count == 0 for 5m | Critical |
| High queue depth | queue_depth > 100 for 10m | Warning |
| Agent offline | agent_status == offline for 5m | Warning |
| High error rate | error_rate > 5% for 5m | Warning |
| Database unavailable | db_up == 0 | Critical |
| Disk space low | disk_usage > 90% | Warning |

### Dashboards

**System Overview**:
- Active runs, queue depth
- Agent status distribution
- Pass/fail rate (last hour, day)
- Error rate

**Agent Fleet**:
- Agents by status
- Resource utilization heatmap
- Execution throughput

**Test Health**:
- Pass rate trends
- Slowest tests
- Flaky test count

## Backup and Recovery

### What to Back Up

| Data | Method | Frequency | Retention |
|------|--------|-----------|-----------|
| PostgreSQL | pg_dump or managed backup | Daily | 30 days |
| Artifacts (S3) | S3 versioning or cross-region replication | Continuous | Per retention policy |
| Configuration | Git (infrastructure as code) | On change | Forever |

### Recovery Procedures

**Database recovery**:
1. Stop control plane
2. Restore database from backup
3. Start control plane
4. Verify connectivity

**Control plane failure**:
1. Agents will reconnect automatically
2. Deploy new instance
3. Agents re-register

**Agent failure**:
1. In-progress runs marked failed (after timeout)
2. Deploy replacement agent
3. Work re-queued automatically

## Upgrade Procedures

### Control Plane Upgrade

1. Review changelog for breaking changes
2. Back up database
3. Run database migrations (if any)
4. Deploy new version (rolling update)
5. Verify health
6. Monitor for issues

### Agent Upgrade

1. Ensure control plane supports new agent version
2. Drain agents one at a time
3. Update agent binary/image
4. Restart agent
5. Verify connection and execution
6. Repeat for all agents

### Database Migrations

- Migrations run automatically on startup
- Or run manually before deployment:
  ```bash
  testharness-control-plane migrate
  ```
- Always back up before migration
- Test migrations in staging first
