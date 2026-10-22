# ops-guarding

`ops-guarding` runs shell scripts across many machines through a central server and a lightweight agent.

The `server` process owns task metadata, scheduling, HTTP APIs, RPC, and result storage. The `agentd` process runs on target hosts, polls the server through RPC, executes assigned scripts, stores local task artifacts under `meta/`, and reports stdout, stderr, and final status.

## Project Structure

- `cmd/`: CLI entry point for `server` and `agentd`.
- `pkg/server/`: server configuration, HTTP routes, RPC methods, schedulers, and timers.
- `pkg/agentd/`: agent configuration, RPC client, heartbeat loop, task execution, and metrics input framework.
- `pkg/models/`: GORM models and task state operations.
- `pkg/appframe/`: shared HTTP, logging, storage, system, and utility code.
- `etc/service/`: sample runtime configuration.
- `sql/`: database bootstrap SQL.
- `scripts/`: local validation and smoke-test helpers.
- `docs/`: project analysis, specs, and plans.

## Requirements

- Go 1.26 or newer (`go.mod`: `go 1.26.0`, `toolchain go1.26.10`; with the default `GOTOOLCHAIN=auto` an older Go downloads go1.26.10 automatically).
- MySQL for `server` runtime. The bootstrap schema lives in `sql/ibex.sql`.
- `curl` for startup smoke checks.

## Build

```bash
go build -o bin/guarding ./cmd
```

The binary exposes two subcommands:

```bash
./bin/guarding server -c etc/service/server.conf
./bin/guarding agentd -c etc/service/agentd.conf
```

The default config paths also point to `etc/service/server.conf` and `etc/service/agentd.conf`.

## Configuration

`etc/service/server.conf` is git-ignored because it carries live database
and BasicAuth credentials. The repository ships a sanitized template at
`etc/service/server.conf.example`. After cloning, copy it locally and fill
in your values:

```bash
cp etc/service/server.conf.example etc/service/server.conf
${EDITOR:-vi} etc/service/server.conf
```

At minimum, update these fields:

- `[MySQL]` → `Address`, `User`, `Password`, `DBName`
- `[BasicAuth]` → `ibex` (username / shared-secret pair used by HTTP APIs)

`[Postgres]` only needs to be filled in if you switch `[Gorm] DBType` to
`"postgres"`. `[Heartbeat].IP` defaults to `127.0.0.1`, which is fine for
local samples; replace it when the `server` listens on a routable address.

`agentd` reads `etc/service/agentd.conf` (tracked in git). Adjust
`[Heartbeat].Host` and `[Heartbeat].Servers` to match your environment.

Local IP auto-detection (server when `[Heartbeat].IP` is blank, agentd when
`[Heartbeat].Host = "$ip"`) works offline. Order: `Interface` (NIC name) →
UDP route probe to `ProbeAddr` (no packet sent; server default `8.8.8.8:80`,
agentd default its first `Servers` entry) → first up, non-loopback,
non-link-local IPv4 on a non-virtual NIC (skips lo/docker/veth/br-/cni/
flannel/cali/virbr). IPv4 only. The chosen source is printed at startup.

## Task state API

`GET /ibex/v1/task/:id/state` returns `{"dat": "<state>", "err": ""}`:

| state | meaning |
|---|---|
| `start` / `pause` / `cancel` / `kill` | some hosts still waiting/running/killing; value is the current action |
| `failed` | all hosts finished, at least one `failed`/`timeout`/`killfailed` |
| `done` | all hosts finished without failures |

Unknown task id → `{"dat": "", "err": "task <id> not found"}` (previously `done`).
`?detail=1` returns `{"state","action","total","counts":{"<host status>":n}}`.

## Validate

Run the standard validation script:

```bash
./scripts/validate.sh
```

The script sets `GOCACHE` and `GOMODCACHE` under `.cache/`, runs `go test ./...`, and builds `bin/guarding`.

## Startup Smoke Check

Run:

```bash
./scripts/smoke-startup.sh
```

The smoke script builds the binary, starts `agentd`, and checks `http://127.0.0.1:2090/ping`.

For `server`, the script checks `http://127.0.0.1:10090/ping` when the configured database is available. If MySQL is not available, the script reports the database dependency as the expected startup blocker.

## Database Bootstrap

Create the local database with:

```bash
mysql -u root -p < sql/ibex.sql
```

Then update `etc/service/server.conf` with the local MySQL address, user, password, and database name.

## Key Runtime Ports

- `server` HTTP: `10090`
- `server` RPC: `20090`
- `agentd` HTTP: `2090`

## Task Flow

1. A client creates a task through `POST /ibex/v1/tasks`.
2. The server writes task metadata, action state, scheduler ownership, and target hosts.
3. The scheduler moves waiting hosts into running state according to batch, tolerance, pause, and timeout settings.
4. `agentd` reports completed tasks and receives assigned work through `Server.Report`.
5. `agentd` caches scripts and inputs under `meta/<task-id>/`, runs the script, and persists stdout, stderr, and done flags.
6. The server stores final host status and output.

## Troubleshooting

- If validation fails while downloading modules, remove `.cache/go-mod` and rerun `./scripts/validate.sh`.
- If `TestSysLogBackend` is skipped, the host does not expose usable syslog delivery. The rest of the logger tests still run.
- If the server does not start, check the database settings in `etc/service/server.conf`.
- If `agentd` cannot reach RPC, check `Heartbeat.Servers` in `etc/service/agentd.conf`.

## Follow-Up Work

Phase 2 should add regression tests for task creation, scheduling, agent execution, RPC reporting, timeout handling, and result retrieval. Phase 3 should address secrets, deployment, metrics output, health checks, and high-availability behavior.
