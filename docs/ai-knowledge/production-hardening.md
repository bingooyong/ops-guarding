# Production Hardening

## Purpose

Use this document when preparing `ops-guarding` for deployment beyond local development. It identifies security, configuration, runtime, observability, and availability work that is intentionally outside the current baseline.

## Secrets And Configuration

### Current Risk

Sample config files include database credentials and BasicAuth values for local use.

### Hardening Rule

- Move secrets to environment variables, secret files, or a deployment secret manager.
- Keep committed config files as examples only.
- Document all required variables and defaults.
- Never log database passwords, BasicAuth credentials, script stdin, or secret command arguments.

### Suggested Shape

```text
etc/service/server.example.conf
etc/service/agentd.example.conf
deploy/systemd/
deploy/docker-compose.local.yml
```

## Network Exposure

### Current Risk

Sample configs bind HTTP/RPC to `0.0.0.0`.

### Hardening Rule

- Bind local development to loopback unless remote access is required.
- Protect RPC with network policy, mTLS, or a private network.
- Keep `/debug/pprof` disabled by default.
- Confirm `/ibex/v1` remains authenticated and `/ping` remains safe.

## Database Operations

### Current Risk

Server startup depends on a reachable MySQL/Postgres database with matching schema.

### Hardening Rule

- Add schema migration ownership and versioning.
- Define backup/restore procedures.
- Monitor connection pool saturation.
- Validate max connection settings against database capacity.

## Agent Execution Safety

### Current Risk

Agents execute scripts provided by the server.

### Hardening Rule

- Run agent scripts under a restricted OS account.
- Set resource limits for CPU, memory, process count, and runtime.
- Sanitize script storage permissions under `meta/`.
- Define safe behavior for long-running, stuck, or killed processes.
- Audit command output truncation and retention.

## Observability

### Required Signals

- Server HTTP request rate, latency, and error counts.
- RPC report count, latency, and failure count.
- Scheduler queue depth and running host count.
- Task status transitions by state.
- Agent heartbeat freshness.
- Script execution duration and exit status.
- Database connection pool metrics.

### Logging Rules

- Use structured logs for task id, host, clock, action, status, and duration.
- Do not log secrets or full script content by default.
- Keep log rotation enabled for file output.

## Health Checks

### Current Baseline

`/ping` proves process-level reachability.

### Production Requirement

Add deeper health checks:

- Database connectivity.
- Scheduler loop freshness.
- RPC listener availability.
- Agent heartbeat to selected server.
- Disk write permission for `meta/` and log directories.

## High Availability

### Server HA

- Define scheduler ownership semantics before running multiple active servers.
- Ensure only one scheduler claims a host/task transition at a time.
- Make heartbeat and doing-cache behavior deterministic under failover.

### Agent HA

- Keep multiple RPC server addresses in `Heartbeat.Servers`.
- Verify fastest-server selection recovers when the selected server fails.
- Add tests for reconnect and report retry behavior.

## Deployment Checklist

Before production:

1. Replace committed sample secrets.
2. Add migration workflow.
3. Add task-chain regression tests.
4. Add deployment manifests.
5. Add metrics and dashboards.
6. Add log retention and redaction rules.
7. Add HA/failover tests.
8. Document rollback and emergency stop procedures.
