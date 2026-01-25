# Troubleshooting

This guide helps diagnose and resolve common issues with Conductor.

## Quick Diagnostics

### Health Check Endpoints

All Conductor components expose health endpoints:

```bash
# Control Plane
curl http://localhost:8080/healthz        # Liveness
curl http://localhost:8080/readyz         # Readiness
curl http://localhost:8080/api/v1/health  # Detailed status

# Agent
curl http://localhost:8081/healthz
curl http://localhost:8081/readyz
```

### Detailed Health Response

```json
{
  "status": "healthy",
  "version": "1.0.0",
  "components": {
    "database": {"status": "healthy", "latency_ms": 2},
    "redis": {"status": "healthy", "latency_ms": 1},
    "artifact_storage": {"status": "healthy"},
    "git_provider": {"status": "healthy", "rate_limit_remaining": 4500}
  },
  "agents": {
    "total": 5,
    "online": 4,
    "offline": 1
  }
}
```

---

## Control Plane Issues

### Control Plane Won't Start

#### Database Connection Failed

**Symptoms:**
```
ERROR failed to connect to database: connection refused
```

**Solutions:**

1. **Verify database is running:**
   ```bash
   # Docker
   docker ps | grep postgres
   
   # Kubernetes
   kubectl get pods -l app=postgres
   ```

2. **Check connection string:**
   ```bash
   # Test connection
   psql $DATABASE_URL -c "SELECT 1"
   ```

3. **Verify network access:**
   ```bash
   # From control plane pod
   nc -zv postgres-host 5432
   ```

4. **Check credentials:**
   ```yaml
   # Ensure environment variable is set
   DATABASE_URL: postgres://user:pass@host:5432/conductor?sslmode=disable
   ```

#### Port Already in Use

**Symptoms:**
```
ERROR listen tcp :8080: bind: address already in use
```

**Solutions:**

1. **Find the process:**
   ```bash
   lsof -i :8080
   # or
   netstat -tlnp | grep 8080
   ```

2. **Use a different port:**
   ```yaml
   server:
     http_port: 8090
     grpc_port: 9090
   ```

### High Memory Usage

**Symptoms:**
- OOMKilled pods
- Slow response times

**Solutions:**

1. **Check current usage:**
   ```bash
   kubectl top pod -l app=conductor-control-plane
   ```

2. **Increase limits:**
   ```yaml
   resources:
     limits:
       memory: 2Gi
     requests:
       memory: 512Mi
   ```

3. **Enable query result streaming:**
   ```yaml
   database:
     max_rows_per_query: 1000
     enable_pagination: true
   ```

4. **Reduce connection pool:**
   ```yaml
   database:
     max_open_connections: 25
     max_idle_connections: 5
   ```

### Database Migration Failures

**Symptoms:**
```
ERROR migration failed: relation "services" already exists
```

**Solutions:**

1. **Check migration status:**
   ```bash
   ./control-plane migrate status
   ```

2. **Force migration version (careful!):**
   ```bash
   ./control-plane migrate force VERSION
   ```

3. **Rollback and retry:**
   ```bash
   ./control-plane migrate down 1
   ./control-plane migrate up
   ```

---

## Agent Issues

### Agent Won't Connect

**Symptoms:**
```
ERROR failed to connect to control plane: connection refused
```

**Solutions:**

1. **Verify control plane is running:**
   ```bash
   curl http://control-plane:8080/healthz
   ```

2. **Check gRPC connectivity:**
   ```bash
   grpcurl -plaintext control-plane:9090 list
   ```

3. **Verify agent configuration:**
   ```yaml
   agent:
     control_plane_address: control-plane:9090
     use_tls: false  # or true with proper certs
   ```

4. **Check firewall/network policies:**
   ```bash
   # Test from agent
   nc -zv control-plane 9090
   ```

### Agent Shows Offline

**Symptoms:**
- Agent appears offline in dashboard
- Heartbeat timeout warnings

**Causes & Solutions:**

1. **Network latency:**
   ```yaml
   agent:
     heartbeat_interval: 30s  # Increase from default
   ```

2. **Control plane overloaded:**
   - Check control plane CPU/memory
   - Scale control plane replicas

3. **Agent crashed:**
   ```bash
   # Check agent logs
   kubectl logs -l app=conductor-agent --tail=100
   
   # Check for OOMKill
   kubectl describe pod agent-xyz | grep -A5 "Last State"
   ```

### Test Execution Failures

#### Clone Failed

**Symptoms:**
```
ERROR failed to clone repository: authentication required
```

**Solutions:**

1. **Verify Git credentials:**
   ```bash
   # Test clone manually
   git clone https://x-access-token:TOKEN@github.com/org/repo.git
   ```

2. **Check token permissions:**
   - Ensure token has `repo` scope for private repos
   - Verify token hasn't expired

3. **Network access:**
   ```bash
   # From agent
   curl -I https://github.com
   ```

#### Container Pull Failed

**Symptoms:**
```
ERROR failed to pull image: unauthorized
```

**Solutions:**

1. **Configure registry credentials:**
   ```yaml
   agent:
     docker:
       registry_auth:
         "ghcr.io":
           username: ${GITHUB_USERNAME}
           password: ${GITHUB_TOKEN}
   ```

2. **For Kubernetes:**
   ```yaml
   imagePullSecrets:
     - name: ghcr-secret
   ```

3. **Check image exists:**
   ```bash
   docker pull ghcr.io/org/image:tag
   ```

#### Test Timeout

**Symptoms:**
```
ERROR test timed out after 30m0s
```

**Solutions:**

1. **Increase timeout in manifest:**
   ```yaml
   # .testharness.yaml
   tests:
     - name: integration
       timeout: 1h
   ```

2. **Check for hung processes:**
   ```bash
   # On agent
   ps aux | grep test
   ```

3. **Resource constraints:**
   ```yaml
   agent:
     resources:
       cpu_limit: "2"
       memory_limit: "4Gi"
   ```

---

## Git Integration Issues

### Webhook Not Triggering Tests

**Solutions:**

1. **Check webhook delivery:**
   - GitHub: Settings > Webhooks > Recent Deliveries
   - Look for response codes and errors

2. **Verify webhook URL:**
   ```bash
   curl -X POST https://conductor.example.com/api/v1/webhooks/github \
     -H "Content-Type: application/json" \
     -d '{"test": true}'
   # Should return 401 (signature invalid) not 404
   ```

3. **Check webhook secret:**
   ```bash
   # Verify secret matches
   kubectl get secret conductor-secrets -o jsonpath='{.data.GITHUB_WEBHOOK_SECRET}' | base64 -d
   ```

4. **Review control plane logs:**
   ```bash
   kubectl logs -l app=conductor-control-plane | grep webhook
   ```

### Commit Status Not Updating

**Solutions:**

1. **Check token permissions:**
   - GitHub: `repo:status` scope required
   - GitLab: `api` scope required

2. **Verify API access:**
   ```bash
   # GitHub
   curl -H "Authorization: token $GITHUB_TOKEN" \
     https://api.github.com/repos/OWNER/REPO/statuses/SHA
   ```

3. **Check rate limits:**
   ```bash
   curl -H "Authorization: token $GITHUB_TOKEN" \
     https://api.github.com/rate_limit
   ```

### Rate Limit Exceeded

**Symptoms:**
```
ERROR GitHub API rate limit exceeded, retry after: 2024-01-15T11:00:00Z
```

**Solutions:**

1. **Use GitHub App** (higher limits):
   ```yaml
   git:
     providers:
       - name: github
         app_id: 12345
         app_private_key_path: /path/to/key.pem
   ```

2. **Enable caching:**
   ```yaml
   git:
     caching:
       enabled: true
       repository_ttl: 5m
   ```

3. **Reduce polling frequency:**
   ```yaml
   git:
     discovery:
       scan_interval: 1h  # Increase from default
   ```

---

## Notification Issues

### Notifications Not Sending

**Solutions:**

1. **Verify channel is enabled:**
   ```bash
   curl https://conductor.example.com/api/v1/notification-channels/ID
   # Check "enabled": true
   ```

2. **Test channel directly:**
   ```bash
   curl -X POST https://conductor.example.com/api/v1/notification-channels/ID/test
   ```

3. **Check rule configuration:**
   ```bash
   curl https://conductor.example.com/api/v1/notification-rules?service_id=SERVICE_ID
   # Verify trigger_on includes the event type
   ```

4. **Check throttling:**
   - Duplicate notifications within throttle window are suppressed
   - Default: 5 minutes

### Slack Webhook Errors

| Error | Solution |
|-------|----------|
| `invalid_token` | Regenerate webhook URL |
| `channel_not_found` | Verify channel exists and bot has access |
| `rate_limited` | Reduce notification frequency |
| `request_timeout` | Check Slack status, retry later |

### Email Delivery Issues

1. **Test SMTP connection:**
   ```bash
   # Using telnet
   telnet smtp.example.com 587
   
   # Using openssl for TLS
   openssl s_client -starttls smtp -connect smtp.example.com:587
   ```

2. **Check spam folder:**
   - Add conductor email to allowlist
   - Configure SPF/DKIM for your domain

3. **Review SMTP errors:**
   ```bash
   kubectl logs -l app=conductor-control-plane | grep -i smtp
   ```

---

## Performance Issues

### Slow Dashboard

**Solutions:**

1. **Check database queries:**
   ```sql
   -- Enable slow query logging
   ALTER SYSTEM SET log_min_duration_statement = 1000;
   SELECT pg_reload_conf();
   ```

2. **Add indexes:**
   ```sql
   -- Check for missing indexes
   SELECT * FROM pg_stat_user_indexes 
   WHERE idx_scan = 0 AND idx_tup_read = 0;
   ```

3. **Increase connection pool:**
   ```yaml
   database:
     max_open_connections: 50
   ```

4. **Enable Redis caching:**
   ```yaml
   cache:
     enabled: true
     redis_url: redis://redis:6379
   ```

### Slow Test Execution

**Solutions:**

1. **Check agent resources:**
   ```bash
   kubectl top pod -l app=conductor-agent
   ```

2. **Increase parallelism:**
   ```yaml
   agent:
     max_concurrent_tests: 4
   ```

3. **Use container caching:**
   ```yaml
   agent:
     docker:
       cache_images: true
   ```

4. **Local artifact cache:**
   ```yaml
   agent:
     artifact_cache:
       enabled: true
       path: /var/cache/conductor
       max_size_gb: 10
   ```

---

## Debug Mode

Enable debug logging for detailed diagnostics:

### Control Plane

```yaml
logging:
  level: debug
  format: json
```

Or via environment:
```bash
LOG_LEVEL=debug ./control-plane
```

### Agent

```yaml
agent:
  log_level: debug
```

### Specific Components

```yaml
logging:
  level: info
  component_levels:
    scheduler: debug
    git: debug
    notification: debug
```

---

## Log Locations

### Docker Compose

```bash
# All logs
docker-compose logs -f

# Specific service
docker-compose logs -f control-plane
docker-compose logs -f agent
```

### Kubernetes

```bash
# Control plane logs
kubectl logs -l app=conductor-control-plane -f

# Agent logs
kubectl logs -l app=conductor-agent -f

# All conductor logs
kubectl logs -l app.kubernetes.io/name=conductor -f

# Previous container logs (after restart)
kubectl logs POD_NAME --previous
```

### Systemd

```bash
journalctl -u conductor-control-plane -f
journalctl -u conductor-agent -f
```

---

## Common Error Messages

### "context deadline exceeded"

**Cause:** Operation timed out

**Solutions:**
- Increase timeout for the operation
- Check network connectivity
- Reduce load on the system

### "connection refused"

**Cause:** Target service not running or not accessible

**Solutions:**
- Verify service is running
- Check firewall/network policies
- Verify correct host:port

### "permission denied"

**Cause:** Insufficient permissions

**Solutions:**
- Check file permissions
- Verify token/credential scopes
- Check Kubernetes RBAC

### "resource exhausted"

**Cause:** Out of resources (memory, connections, etc.)

**Solutions:**
- Increase resource limits
- Reduce concurrency
- Scale horizontally

### "unavailable"

**Cause:** Service temporarily unavailable

**Solutions:**
- Check service health
- Wait and retry
- Check for ongoing deployments

---

## Collecting Diagnostics

### Generate Support Bundle

```bash
# Create diagnostic bundle
./conductor-support-bundle.sh

# Or manually collect:
mkdir -p /tmp/conductor-diagnostics

# Logs
kubectl logs -l app=conductor-control-plane --tail=1000 > /tmp/conductor-diagnostics/control-plane.log
kubectl logs -l app=conductor-agent --tail=1000 > /tmp/conductor-diagnostics/agent.log

# Configuration (redacted)
kubectl get configmap conductor-config -o yaml > /tmp/conductor-diagnostics/config.yaml

# Resource status
kubectl get pods -l app.kubernetes.io/name=conductor -o wide > /tmp/conductor-diagnostics/pods.txt
kubectl describe pods -l app.kubernetes.io/name=conductor > /tmp/conductor-diagnostics/pod-details.txt

# Events
kubectl get events --sort-by='.lastTimestamp' > /tmp/conductor-diagnostics/events.txt

# Package
tar -czf conductor-diagnostics.tar.gz /tmp/conductor-diagnostics/
```

### Metrics Export

```bash
# Prometheus metrics
curl http://localhost:8080/metrics > metrics.txt

# Key metrics to check
grep conductor_test_runs_total metrics.txt
grep conductor_agent_connected metrics.txt
grep conductor_errors_total metrics.txt
```

---

## Getting Help

### Resources

1. **Documentation:** https://conductor.example.com/docs
2. **GitHub Issues:** https://github.com/org/conductor/issues
3. **Community Slack:** https://conductor-community.slack.com

### Filing a Bug Report

Include:
1. Conductor version (`conductor --version`)
2. Deployment type (Docker, Kubernetes, Binary)
3. Error messages and logs
4. Steps to reproduce
5. Expected vs actual behavior
6. Diagnostic bundle (if possible)

### Security Issues

Report security vulnerabilities privately to security@conductor.example.com

---

## Next Steps

- [Configuration Reference](configuration.md) - All configuration options
- [Architecture](architecture.md) - System design and components
- [API Reference](api.md) - REST and gRPC API documentation
