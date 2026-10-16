# Logging Guidelines

> Which logger to use, levels, format, and what to log.

---

## Overview

Two logging mechanisms coexist; pick by process:

| Where | Logger | Init |
|-------|--------|------|
| `guarding server` (router, rpc, logic, timer) | `ops-guarding/pkg/appframe/logger` (dlog-style) | `logx.Init(config.C.Log)` in `server.initialize()` |
| `guarding agentd` | Go std `log` with manual level prefixes | `log.SetFlags(log.Ldate \| log.Ltime \| log.Lshortfile)` in `agentd.initialize()` |
| Startup / shutdown banners (both) | `fmt.Println` to stdout | `cmd/main.go printEnv`, `config.MustLoad`, `Run` |
| HTTP access log / panics | `aop.Logger()` / `aop.Recovery()` gin middleware | `router.New`; access log only if `HTTP.PrintAccessLog` |

Do not introduce a third library (zap, logrus, slog) without an explicit decision.

---

## appframe/logger (server)

- API: `logger.Debug/Debugf`, `Info/Infof`, `Warning/Warningf`,
  `Error/Errorf`, `Fatal/Fatalf`, plus `LogDepth(s, depth, ...)` for helpers.
- Severities: `DEBUG < INFO < WARNING < ERROR < FATAL`.
- Line format: `2015-06-16 12:00:35 ERROR file.go:12 message` (see
  `pkg/appframe/logger/readme.md`).
- Config (`[Log]` in server.conf, `logx.Config`):
  - `Level` — `DEBUG|INFO|WARNING|ERROR`.
  - `Output` — `stdout` (default backend), `stderr`, or `file`.
  - For `file`: `Dir`, and either `KeepHours` (hourly rotation, files like
    `INFO.log.2016040113`) or `RotateNum` + `RotateSize` (MB). Both zero → init error.
  - File backend writes per-level files (`INFO.log`, `WARNING.log`, `ERROR.log`, ...),
    buffers 256KB and flushes periodically; `logger.Close()` is registered as a
    cleanup func and must run on shutdown.
- Concurrency: logger configuration was hardened against races
  (commit `a5aebc3`); changes in this package need
  `go test -race -count=1 ./pkg/appframe/logger` (see `skills/go-concurrency-review-fix`).

---

## std log prefixes (agentd)

agentd messages start with a level letter:

- `E:` error, `W:` warning, `I:` info, `D:` debug —
  e.g. `log.Println("E: rpc call Server.Report fail:", err)`,
  `log.Printf("D: process of task[%d] done", t.Id)`.
- Metrics code (`metrics_agentd.go`, `inputs/system`) uses telegraf-style
  `E!` / `W!` / `D!`; `D!` lines are gated by `config.C.DebugMode`.

Keep the prefix style of the file you are editing. agentd has no level
filtering or rotation; all output goes to stderr of the process.

---

## Log Levels (as used)

- **Debug**: scheduler internals (`task[%d] scheduling...`, `task[%d] is mine`,
  `orphan task ids:`), agent process lifecycle.
- **Info**: audit-style business events — task creation
  (`task_create_succ: authUser=%s title=%s`), scheduler takeover
  (`%s take over task[%d] of %s`).
- **Warning**: recoverable infra issues (`listener accept error: %v` in rpc).
- **Error**: every failed DB/RPC call in loops. Note: `logic_scheduler.go`
  logs one warning case as `logger.Errorf("[W] no action found ...")`; prefer
  `logger.Warningf` in new code.

---

## Message Style

- Lowercase, `key[value]` or `key=value` style with the task id first:
  `"cannot get task[%d] waiting host count: %v"`,
  `"cannot mark task done, id:%d, hostname:%s, clock:%d, status:%s"`.
- Always include identifiers needed to trace a task: task `id`, `host`,
  `clock`, `action`/`status`, scheduler address.
- Audit events use a stable snake_case tag prefix (`task_create_succ`/`task_create_fail`).

---

## What NOT to Log

- BasicAuth credentials, DB DSN/password (`storage.MySQL.DSN()` contains the password).
- Script content, `stdin`, or full `args` of tasks (may carry secrets);
  `docs/ai-knowledge/production-hardening.md` lists this as a hardening rule.
- Full stdout/stderr payloads (they are stored in DB, truncated to 65535 bytes
  in `LocalTasksT.ReportTasks`).

`aop.Recovery()` already masks the `Authorization` header in panic dumps;
keep that behaviour if you touch it.

---

## Known Gaps

- No structured (JSON/field) logging; production-hardening.md asks for it later.
- `pkg/appframe/cache/serialization.go` uses std `log` even when running in the server.
- agentd logs cannot be rotated or level-filtered from config.
