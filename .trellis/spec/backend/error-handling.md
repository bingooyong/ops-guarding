# Error Handling

> How errors are created, propagated, logged, and returned in ops-guarding.

---

## Overview

There are three distinct error styles, one per layer. Use the one that
matches where your code runs:

| Layer | Style | Reference |
|-------|-------|-----------|
| HTTP handlers (server) | panic with `errorx.PageError`, recovered by `aop.Recovery()` | `pkg/server/router/handler.go`, `pkg/appframe/errorx/errorx.go` |
| models / logic / timers / agentd | plain `error` returns; loops log and `return`/`continue` | `pkg/models/*.go`, `pkg/server/logic/logic_scheduler.go` |
| RPC methods | return `nil` and put the failure in `resp.Message` | `pkg/server/rpc/method.go` |
| Process startup | print to stdout and `os.Exit(1)` | `config.MustLoad`, `server.Run`, `timer.CacheHostDoing` |

---

## Error Types

- `errorx.PageError{Code int, Message string}` is the only custom error type.
  It is created via:
  - `errorx.Bomb(code, format, args...)` — always panics.
  - `errorx.Dangerous(v, code...)` — panics if `v` is a non-nil `error` or a
    non-empty `string`; default code is **200**.
  - `ginx.Bomb` / `ginx.Dangerous` are thin aliases.
- Everything else uses `fmt.Errorf(...)` with lowercase messages, often with
  an `arg(<name>)` prefix for validation (`TaskMeta.CleanFields`:
  `"arg(title) is required"`, `"arg(timeout) longer than one day"`).
- Error wrapping: the codebase does **not** use `%w`. `github.com/pkg/errors`
  is used only in `pkg/appframe/logx` (`errors.WithMessage`). `errors.Is` is
  used only for `gorm.ErrRecordNotFound` in `TaskActionGet`.
- "Not found" in models is `(nil, nil)`, not an error (see database guidelines).

---

## Error Handling Patterns

### HTTP handlers (`pkg/server/router`)

- Parse inputs with `ginx` helpers, which `Bomb(400, ...)` on bad input:
  `ginx.UrlParamInt64(c, "id")`, `ginx.QueryInt(c, "limit", 20)`, `ginx.BindJSON(c, &f)`.
- Load the task with `router.TaskMeta(id)`, which `Dangerous(err)`s on DB error
  and `Bomb(404, "no such task meta")` when missing.
- Fail fast on errors with `errorx.Dangerous(err)`; reserve `errorx.Bomb(code, ...)`
  for explicit business errors (`Bomb(http.StatusBadRequest, "arg(hosts) empty")`).
- For the final result, pass the error to the renderer instead of panicking:
  `ginx.NewRender(c).Data(obj, err)` or `ginx.NewRender(c).Message(err)`.

### Models / logic / timers

- Return `error` up; do not log inside `pkg/models`.
- Long-running loops (`timer.Schedule`, `timer.Heartbeat`, `logic.ScheduleTask`)
  log with `logger.Errorf("cannot <verb> task[%d] ...: %v", id, err)` and then
  `return` (skip this round) or `continue` (skip this id). They never panic.

### agentd

- agentd has no recovery middleware for its loops. Errors are logged with the
  std `log` package (`log.Printf("E: ... %v", err)`) and the function returns;
  the next heartbeat retries (`timer/heartbeat.go`, `timer/task.go`).
- After an RPC failure, call `client.CloseCli()` so the next call reconnects.
- Metrics input goroutines guard with `defer recover()` + `runtimex.Stack(3)`
  (`InputReader.gatherOnce`). Do the same for any new goroutine that runs
  plugin/third-party code.

### RPC (`pkg/server/rpc`)

`net/rpc` methods return `nil` and report failures in the response struct, so
agents see a message instead of a transport error:

```go
if meta == nil {
    resp.Message = fmt.Sprintf("task %d not found", id)
    return nil
}
```

agentd checks `resp.Message != ""` and logs it (`client.Meta`, `heartbeat()`).

---

## API Error Responses

All `/ibex/v1` JSON responses use the `{dat, err}` envelope from `ginx.Render`:

- Success: `{"dat": <data>, "err": ""}`; `Message(nil)` → `{"err": ""}`.
- Error via `Render` with code 200 (default): HTTP **200** with `{"err": "<msg>"}`.
  Callers must check `err`, not the status code.
- Error via `Render` with non-200 code: plain-text body.
- `PageError` recovered by `aop.Recovery()`: HTTP `e.Code` with `{"err": "<msg>"}`.
  Messages are passed through `i18n.Sprintf(c.GetHeader("X-Language"), ...)`.
- Any other panic: logged with stack, HTTP 500 (Authorization header masked).
- `*.txt` endpoints (`taskStdoutTxt`) write `c.String(500|404, msg)` directly.

---

## Common Mistakes / Known Issues

- Handlers must nil-check model results (`(nil, nil)` = not found) and render an
  error instead of dereferencing; see `taskHostFromDB` in `handler.go`
  (fixed: unknown host on `/task/:id/host/:host/stdout|stderr` used to panic).
- `errorx.Dangerous(err)` defaults to HTTP 200; pass a code
  (`errorx.Dangerous(err, 500)`) when the client should see a failure status.
- `taskState` aggregates `task_action` + per-host counts via
  `models.AggregateState`; unknown task id → `{"err":"task N not found"}`.
- `cleanDoneTask` logs a `CleanDoneTask` error but still logs "done".
- Config errors still call `os.Exit(1)` inside `config.MustLoad`. Reusable
  helpers must return errors instead (pattern: `netx.LocalIP` returns
  `(ip, source, err)`; only the config loader exits, with a config hint).
- Do not log raw secrets in error messages (BasicAuth, DB password, script stdin).
