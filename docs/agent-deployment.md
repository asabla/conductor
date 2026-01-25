# Agent Deployment Guide

This guide covers deploying Conductor agents in various environments.

## Table of Contents

- [Overview](#overview)
- [Deployment Options](#deployment-options)
  - [Binary Deployment](#binary-deployment)
  - [Docker Deployment](#docker-deployment)
  - [Kubernetes Deployment](#kubernetes-deployment)
- [Network Requirements](#network-requirements)
- [Resource Requirements](#resource-requirements)
- [Docker Socket Access](#docker-socket-access)
- [Security Considerations](#security-considerations)
- [Multi-Zone Deployment](#multi-zone-deployment)
- [Troubleshooting](#troubleshooting)

## Overview

Conductor agents are lightweight processes that execute tests in private networks. Key characteristics:

- **Outbound connections only** - Agents connect to the control plane; no inbound firewall rules needed
- **Stateless** - Agents can be restarted without losing work
- **Scalable** - Deploy multiple agents to increase capacity
- **Zone-aware** - Agents can be deployed in specific network zones

## Deployment Options

### Binary Deployment

#### Download

```bash
# Download latest release
curl -LO https://github.com/conductor/conductor/releases/latest/download/conductor-agent-linux-amd64
chmod +x conductor-agent-linux-amd64
sudo mv conductor-agent-linux-amd64 /usr/local/bin/conductor-agent
```

#### Systemd Service

Create `/etc/systemd/system/conductor-agent.service`:

```ini
[Unit]
Description=Conductor Test Agent
After=network.target docker.service
Wants=docker.service

[Service]
Type=simple
User=conductor
Group=conductor
Environment=CONDUCTOR_AGENT_CONTROL_PLANE_URL=control-plane.example.com:9090
Environment=CONDUCTOR_AGENT_TOKEN=your-agent-token
Environment=CONDUCTOR_AGENT_NETWORK_ZONES=zone-a
Environment=CONDUCTOR_AGENT_MAX_PARALLEL=4
Environment=CONDUCTOR_AGENT_WORKSPACE_DIR=/var/lib/conductor/workspaces
Environment=CONDUCTOR_AGENT_CACHE_DIR=/var/lib/conductor/cache
Environment=CONDUCTOR_AGENT_LOG_LEVEL=info
ExecStart=/usr/local/bin/conductor-agent
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

Setup and start:

```bash
# Create user and directories
sudo useradd -r -s /bin/false conductor
sudo mkdir -p /var/lib/conductor/{workspaces,cache}
sudo chown -R conductor:conductor /var/lib/conductor

# Add to docker group (for container execution)
sudo usermod -aG docker conductor

# Enable and start
sudo systemctl daemon-reload
sudo systemctl enable conductor-agent
sudo systemctl start conductor-agent

# Check status
sudo systemctl status conductor-agent
sudo journalctl -u conductor-agent -f
```

### Docker Deployment

#### Docker Run

```bash
docker run -d \
  --name conductor-agent \
  --restart unless-stopped \
  -e CONDUCTOR_AGENT_CONTROL_PLANE_URL=control-plane.example.com:9090 \
  -e CONDUCTOR_AGENT_TOKEN=your-agent-token \
  -e CONDUCTOR_AGENT_NETWORK_ZONES=zone-a \
  -e CONDUCTOR_AGENT_MAX_PARALLEL=4 \
  -e CONDUCTOR_AGENT_DOCKER_ENABLED=true \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  -v conductor-workspace:/workspace \
  -v conductor-cache:/cache \
  conductor/agent:latest
```

#### Docker Compose

```yaml
# docker-compose.agent.yml
version: '3.8'

services:
  agent:
    image: conductor/agent:latest
    container_name: conductor-agent
    restart: unless-stopped
    environment:
      CONDUCTOR_AGENT_CONTROL_PLANE_URL: control-plane.example.com:9090
      CONDUCTOR_AGENT_TOKEN: ${AGENT_TOKEN}
      CONDUCTOR_AGENT_NAME: ${HOSTNAME:-agent}
      CONDUCTOR_AGENT_NETWORK_ZONES: zone-a
      CONDUCTOR_AGENT_MAX_PARALLEL: 4
      CONDUCTOR_AGENT_DOCKER_ENABLED: "true"
      CONDUCTOR_AGENT_LOG_LEVEL: info
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - workspace:/workspace
      - cache:/cache
    deploy:
      resources:
        limits:
          cpus: '2'
          memory: 2G

volumes:
  workspace:
  cache:
```

Run with:
```bash
AGENT_TOKEN=your-token docker compose -f docker-compose.agent.yml up -d
```

#### Scaling Agents

```bash
# Scale to 3 agents
docker compose -f docker-compose.agent.yml up -d --scale agent=3
```

### Kubernetes Deployment

#### DaemonSet (One agent per node)

```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: conductor-agent
  namespace: conductor
spec:
  selector:
    matchLabels:
      app: conductor-agent
  template:
    metadata:
      labels:
        app: conductor-agent
    spec:
      serviceAccountName: conductor-agent
      containers:
      - name: agent
        image: conductor/agent:latest
        env:
        - name: CONDUCTOR_AGENT_CONTROL_PLANE_URL
          value: "conductor-control-plane.conductor.svc:9090"
        - name: CONDUCTOR_AGENT_TOKEN
          valueFrom:
            secretKeyRef:
              name: conductor-agent
              key: token
        - name: CONDUCTOR_AGENT_NAME
          valueFrom:
            fieldRef:
              fieldPath: spec.nodeName
        - name: CONDUCTOR_AGENT_NETWORK_ZONES
          value: "kubernetes"
        - name: CONDUCTOR_AGENT_MAX_PARALLEL
          value: "4"
        - name: CONDUCTOR_AGENT_DOCKER_ENABLED
          value: "true"
        - name: DOCKER_HOST
          value: "unix:///var/run/docker.sock"
        volumeMounts:
        - name: docker-sock
          mountPath: /var/run/docker.sock
          readOnly: true
        - name: workspace
          mountPath: /workspace
        - name: cache
          mountPath: /cache
        resources:
          requests:
            cpu: 500m
            memory: 512Mi
          limits:
            cpu: 2000m
            memory: 2Gi
        livenessProbe:
          httpGet:
            path: /health
            port: 8081
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /ready
            port: 8081
          initialDelaySeconds: 5
          periodSeconds: 5
      volumes:
      - name: docker-sock
        hostPath:
          path: /var/run/docker.sock
          type: Socket
      - name: workspace
        emptyDir: {}
      - name: cache
        emptyDir: {}
```

#### Deployment (Scalable replicas)

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: conductor-agent
  namespace: conductor
spec:
  replicas: 3
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
          value: "conductor-control-plane.conductor.svc:9090"
        - name: CONDUCTOR_AGENT_TOKEN
          valueFrom:
            secretKeyRef:
              name: conductor-agent
              key: token
        - name: CONDUCTOR_AGENT_NETWORK_ZONES
          value: "kubernetes"
        - name: CONDUCTOR_AGENT_DOCKER_ENABLED
          value: "false"  # Subprocess only in k8s pods
        resources:
          requests:
            cpu: 500m
            memory: 512Mi
          limits:
            cpu: 2000m
            memory: 2Gi
```

#### RBAC (if needed for cluster access)

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: conductor-agent
  namespace: conductor
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: conductor-agent
rules:
- apiGroups: [""]
  resources: ["pods", "services"]
  verbs: ["get", "list"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: conductor-agent
subjects:
- kind: ServiceAccount
  name: conductor-agent
  namespace: conductor
roleRef:
  kind: ClusterRole
  name: conductor-agent
  apiGroup: rbac.authorization.k8s.io
```

## Network Requirements

### Outbound Connections

Agents require outbound access to:

| Destination | Port | Protocol | Purpose |
|-------------|------|----------|---------|
| Control Plane | 9090 | gRPC/TCP | Work stream, results |
| Git Provider | 443 | HTTPS | Clone repositories |
| Container Registry | 443 | HTTPS | Pull test images |
| Artifact Storage | 443/9000 | HTTPS | Upload artifacts |

### No Inbound Connections Required

Agents initiate all connections outbound, making deployment in restricted networks straightforward.

### Firewall Rules Example

```bash
# Allow outbound to control plane
iptables -A OUTPUT -p tcp -d control-plane.example.com --dport 9090 -j ACCEPT

# Allow outbound to GitHub
iptables -A OUTPUT -p tcp -d github.com --dport 443 -j ACCEPT

# Allow outbound to artifact storage
iptables -A OUTPUT -p tcp -d minio.example.com --dport 9000 -j ACCEPT
```

## Resource Requirements

### Minimum Requirements

| Resource | Minimum | Recommended |
|----------|---------|-------------|
| CPU | 1 core | 2+ cores |
| Memory | 512 MB | 2+ GB |
| Disk | 10 GB | 50+ GB |

### Sizing Guidelines

| Workload | CPU | Memory | Disk |
|----------|-----|--------|------|
| Light (unit tests) | 1 core | 1 GB | 20 GB |
| Medium (integration tests) | 2 cores | 2 GB | 50 GB |
| Heavy (E2E, containers) | 4+ cores | 4+ GB | 100+ GB |

### Resource Thresholds

Configure agents to stop accepting work when resources are low:

```bash
CONDUCTOR_AGENT_CPU_THRESHOLD=90       # Stop at 90% CPU
CONDUCTOR_AGENT_MEMORY_THRESHOLD=90    # Stop at 90% memory
CONDUCTOR_AGENT_DISK_THRESHOLD=90      # Stop at 90% disk
```

## Docker Socket Access

For container execution mode, agents need access to the Docker daemon.

### Docker Socket Mount

```bash
# Binary
export DOCKER_HOST=unix:///var/run/docker.sock

# Docker
-v /var/run/docker.sock:/var/run/docker.sock:ro

# Kubernetes
volumes:
- name: docker-sock
  hostPath:
    path: /var/run/docker.sock
```

### Security Considerations

Docker socket access provides significant privileges. Mitigate risks:

1. **Read-only mount** - Use `:ro` when possible
2. **Dedicated user** - Run as a dedicated user in the docker group
3. **Network isolation** - Use Docker networks to isolate test containers
4. **Resource limits** - Set CPU/memory limits on test containers

### Rootless Docker

For enhanced security, use rootless Docker:

```bash
# Install rootless Docker
dockerd-rootless-setuptool.sh install

# Configure agent
export DOCKER_HOST=unix:///run/user/1000/docker.sock
```

### Podman Alternative

Agents can use Podman instead of Docker:

```bash
export DOCKER_HOST=unix:///run/user/1000/podman/podman.sock
```

## Security Considerations

### Agent Authentication

Agents authenticate using tokens. Best practices:

1. **Unique tokens per agent** - Don't share tokens across agents
2. **Rotate regularly** - Implement token rotation
3. **Store securely** - Use secret management (Vault, K8s secrets)

### TLS Configuration

Enable TLS for production:

```bash
CONDUCTOR_AGENT_TLS_ENABLED=true
CONDUCTOR_AGENT_TLS_CA_FILE=/etc/conductor/ca.crt
```

For mTLS:
```bash
CONDUCTOR_AGENT_TLS_CERT_FILE=/etc/conductor/agent.crt
CONDUCTOR_AGENT_TLS_KEY_FILE=/etc/conductor/agent.key
```

### Workspace Isolation

Each test run gets an isolated workspace:

```
/workspace/
├── run-abc123/
│   └── repo/
├── run-def456/
│   └── repo/
```

Workspaces are cleaned up after completion.

## Multi-Zone Deployment

Deploy agents in different network zones to reach isolated services.

### Zone Configuration

```yaml
# Zone A agent
CONDUCTOR_AGENT_NETWORK_ZONES=zone-a,shared

# Zone B agent
CONDUCTOR_AGENT_NETWORK_ZONES=zone-b,shared

# DMZ agent
CONDUCTOR_AGENT_NETWORK_ZONES=dmz
```

### Service Zone Requirements

In `.testharness.yaml`:

```yaml
service:
  name: internal-service
  network_zones:
    - zone-a    # Tests will only run on zone-a agents
```

### Architecture Example

```
                    Control Plane
                         │
         ┌───────────────┼───────────────┐
         │               │               │
    ┌────┴────┐    ┌────┴────┐    ┌────┴────┐
    │ Agent   │    │ Agent   │    │ Agent   │
    │ Zone A  │    │ Zone B  │    │  DMZ    │
    └────┬────┘    └────┬────┘    └────┬────┘
         │               │               │
    ┌────┴────┐    ┌────┴────┐    ┌────┴────┐
    │Internal │    │Internal │    │External │
    │Services │    │Services │    │Services │
    │  (A)    │    │  (B)    │    │         │
    └─────────┘    └─────────┘    └─────────┘
```

## Troubleshooting

### Agent Not Connecting

1. **Check network connectivity**
   ```bash
   curl -v control-plane.example.com:9090
   telnet control-plane.example.com 9090
   ```

2. **Verify token**
   ```bash
   # Check token is set
   echo $CONDUCTOR_AGENT_TOKEN
   ```

3. **Check logs**
   ```bash
   journalctl -u conductor-agent -f
   # or
   docker logs conductor-agent -f
   ```

### Tests Not Running

1. **Check agent status**
   ```bash
   conductor-ctl agents list
   ```

2. **Verify network zones match**
   - Agent zones must include service required zones

3. **Check resource thresholds**
   - Agent may be at capacity

### Container Execution Failing

1. **Verify Docker access**
   ```bash
   docker ps
   ```

2. **Check Docker socket permissions**
   ```bash
   ls -la /var/run/docker.sock
   ```

3. **Test image pull**
   ```bash
   docker pull the-test-image:tag
   ```

### High Resource Usage

1. **Check running tests**
   ```bash
   conductor-ctl agents get {agent-id}
   ```

2. **Review test timeouts**
   - Long-running tests may be stuck

3. **Clean up workspaces**
   ```bash
   rm -rf /workspace/run-*
   ```

### Connection Drops

1. **Check heartbeat settings**
   ```bash
   CONDUCTOR_AGENT_HEARTBEAT_INTERVAL=30s
   ```

2. **Review control plane logs**
   - Look for timeout messages

3. **Check for network issues**
   ```bash
   ping control-plane.example.com
   ```
