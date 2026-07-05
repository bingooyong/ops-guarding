# Project Context

## Product Role

`ops-guarding` runs shell scripts across many machines through a central server and lightweight agents.

## Runtime Components

- `server`: owns task metadata, scheduling, HTTP APIs, RPC, heartbeat state, and result storage.
- `agentd`: runs on target hosts, polls server RPC, executes assigned scripts, stores local task artifacts under `meta/`, and reports output/status.
- `pkg/appframe`: shared infrastructure for logging, HTTP, storage, runtime, system utilities, and config helpers.
- `sql/ibex.sql`: local database bootstrap schema.

## Key Commands

```bash
go build -o bin/guarding ./cmd
./bin/guarding server -c etc/service/server.conf
./bin/guarding agentd -c etc/service/agentd.conf
./scripts/validate.sh
./scripts/smoke-startup.sh
```

## Configuration Decisions

- Default runtime configs point to `etc/service/server.conf` and `etc/service/agentd.conf`.
  - Applicable when local scripts or README examples need stable paths.
- Server heartbeat sample IP uses `127.0.0.1`.
  - Applicable for local smoke testing without binding to an external address.
- Runtime artifacts such as `meta/`, `bin/`, `var/`, `logs/`, `.cache/`, `.codex/`, `.omx/`, and IDE state are ignored.
  - Applicable when staging commits; these paths should not enter source history.

## Validation Contract

`scripts/validate.sh` is the standard project gate:

- Sets Go caches under `.cache/`.
- Runs `go test ./...`.
- Builds `bin/guarding`.
- Ends with `validation: PASS` on success.

`scripts/smoke-startup.sh` is the runtime sanity gate:

- Builds the binary.
- Starts `agentd` and checks `http://127.0.0.1:2090/ping`.
- Starts `server` and checks `http://127.0.0.1:10090/ping` when database dependencies allow.
- Ends with `smoke: PASS` on success.

## Deferred Roadmap

- Add task creation, scheduling, agent execution, RPC report, timeout, and result retrieval regression tests.
- Replace committed sample secrets with safe deployment patterns.
- Document production service management, metrics output, health checks, and HA topology.
