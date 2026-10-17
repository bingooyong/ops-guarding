# Directory Structure

> How the ops-guarding Go module is laid out and where new code belongs.

---

## Overview

`ops-guarding` is a single Go module (`module ops-guarding`, `go 1.26.0`, `toolchain go1.26.9`) that
builds one binary, `bin/guarding`, from `cmd/main.go`. The binary has two
sub-commands (urfave/cli v2): `guarding server` and `guarding agentd`. The
design follows Nightingale **ibex**: the server accepts shell tasks over HTTP,
schedules them in batches, and agentd pulls work via heartbeat RPC, executes
locally, and reports results back.

Imports always use the module path (`ops-guarding/pkg/...`); there is no
`internal/` directory.

---

## Directory Layout

```
cmd/
  main.go            # CLI entry: newServerCmd / newAgentdCmd, VERSION via -ldflags
  README.md          # planned middleware-management roadmap (not implemented)
pkg/
  server/            # control plane (needs MySQL/Postgres)
    server.go        # Run(opts...) + initialize(): config → i18n → logx → cache → DB → timers → HTTP → RPC
    config/          # config.C global, MustLoad (multiconfig: TOML/.conf/JSON/YAML + env)
    router/          # gin engine (router.go), handlers (handler.go), helpers (funcs.go)
    rpc/             # net/rpc + msgpack server; methods on type Server (method.go)
    logic/           # scheduling / timeout decisions per task id
    timer/           # long-running loops: Heartbeat, Schedule, CleanLong, CacheHostDoing
  agentd/            # host-side agent (no database)
    agentd.go        # Run(opts...) + initialize(): std log flags → config → i18n → HTTP → timer.Heartbeat(ctx)
    config/          # config.C global for agentd, GetHost, GetOutboundIP, Duration type
    client/          # RPC client to server (picks fastest server), Meta()
    timer/           # heartbeat loop, Task (exec.Cmd wrapper), Locals task table, cmd_nix/cmd_windows
    router/          # gin engine with /ping,/pid,/addr,/version only
    inputs/ outputs/ # metrics plugin skeleton (see Known Gaps)
    metrics_agentd.go
  models/            # GORM models + query functions (server side only)
  types/             # RPC wire types (ReportRequest, AssignTask...) and Sample/SampleList
  services/          # empty placeholder (metrics_status.go has only `package services`)
  appframe/          # in-repo shared framework, copied from n9e/ibex-style toolkits
    aop/ ginx/ errorx/ i18n/   # gin middleware, param/render helpers, panic-based errors
    logger/ logx/              # logging backend + config adapter
    storage/ ormx/             # DB config → *gorm.DB (storage.DB global)
    cache/ httpx/ net/ runner/ sys/ file/ str/ slice/ runtimex/ cfg/
etc/service/         # agentd.conf (tracked), server.conf.example (tracked); server.conf is gitignored
sql/ibex.sql         # schema (MySQL); creates DB `ibex`
scripts/             # validate.sh, smoke-startup.sh
skills/              # repo-local AI skills (validation, quality gates, Go concurrency)
docs/                # project-analysis.md, ai-knowledge/, superpowers/{specs,plans}, 00_最终项目文档/
```

---

## Module Organization

- **Process entry packages** (`pkg/server`, `pkg/agentd`) expose `Run(opts ...XxxOption)`
  with functional options (`SetConfigFile`, `SetVersion`) and a private
  `initialize() (func(), error)` that collects cleanup funcs in a local
  `Functions` slice. Follow the same shape for any new process type.
- **Config** lives in a per-process `config` package with a package-level
  `C = new(Config)` loaded once via `sync.Once` in `MustLoad`. Handlers and
  timers read `config.C.*` directly; do not thread config through arguments.
- **Server layering** is `router` (HTTP) / `rpc` (agent wire) → `logic` /
  `timer` → `models` → `appframe/storage`. Handlers call `models.*` directly;
  there is no service layer in practice (`pkg/services` is empty).
- **Agent layering** is `timer` (heartbeat + task execution) → `client` (RPC)
  → `config`. agentd never imports `pkg/models` or `pkg/appframe/storage`.
- **Wire types shared by server and agentd** go in `pkg/types`
  (`types.ReportRequest`, `types.AssignTask`, `types.TaskMetaResponse`), never
  in `models`.
- **Platform-specific code** uses filename suffixes / build tags:
  `pkg/agentd/timer/cmd_nix.go` vs `cmd_windows.go`,
  `pkg/appframe/runner/limits_windows.go`, `logger/syslog_unix.go`.
- **Generic helpers** go in the matching `pkg/appframe/<area>` package
  (e.g. `str.IdsInt64`, `slice.SubInt64`, `file.WriteString`) rather than new
  util packages under `server/` or `agentd/`.

---

## Naming Conventions

- Lowercase single-word package names matching the directory (`logic`, `timer`, `ginx`).
- Server files are prefixed by role inside their package:
  `timer/timer_heartbeat.go`, `timer/timer_scheduler.go`,
  `logic/logic_scheduler.go`, `logic/logic_timeout.go`.
- Model files are named after the table: `models/task_meta.go`,
  `task_host.go`, `task_host_doing.go`, `task_scheduler_health.go`.
- Model query functions are `<Model><Verb>` package functions
  (`TaskMetaGet`, `TaskMetaGetByID`, `TaskActionExistsIds`, `DoingHostCount`)
  plus methods on the model for per-row operations (`(*TaskMeta).Save`, `KillHost`).
- HTTP handlers are unexported camelCase funcs named after the route
  (`taskAdd`, `taskGets`, `taskHostStdout`) registered in `router.configRoute`.
- RPC methods are exported methods on `rpc.Server` with the net/rpc
  signature `func (*Server) Name(req T, resp *U) error` (`Ping`, `GetTaskMeta`, `Report`).

---

## Examples

- Process bootstrap: `pkg/server/server.go` (`Run`, `initialize`, `Functions`).
- HTTP route table: `pkg/server/router/router.go` (`/ibex/v1` group behind `gin.BasicAuth`).
- Agent loop: `pkg/agentd/timer/heartbeat.go` (ctx-cancelled `Heartbeat`).

---

## Known Gaps (document, don't copy)

- `pkg/services/` and `pkg/agentd/outputs` are placeholders; `outputs.WriteSamples` has an empty body.
- `MetricsAgent` / input readers in `pkg/agentd/metrics_agentd.go` are never started from `agentd.initialize()`.
- Middleware management described in `cmd/README.md` (start/stop/autostart/log cleanup) has no code yet.
- `meta/`, `bin/`, `.cache/`, `logs*/` are runtime output and gitignored.
