# Task Chain Regression Tests

## Purpose

Use this document when adding functional regression tests for the full task lifecycle: task creation, scheduling, agent assignment, execution, reporting, timeout, kill, and result retrieval.

## Current Gaps

The current baseline validates compilation, package tests, and local `/ping` startup. It does not yet prove that a task can move end to end through server, database, scheduler, RPC, agent execution, and output retrieval.

## Recommended Test Layers

### Model-Level Tests

Applicable when testing state transitions without running HTTP/RPC servers.

- Create `TaskMeta`, `TaskAction`, `TaskHost`, and scheduler records in a disposable database.
- Verify waiting, running, done, failed, timeout, and killing states.
- Verify output truncation rules for stdout/stderr over `65535` bytes.
- Verify `MarkDoneStatus` persists status and outputs idempotently.

### Server HTTP Tests

Applicable when testing API contracts.

- `POST /ibex/v1/tasks`: creates metadata, action, hosts, and scheduler rows.
- `GET /ibex/v1/task/:id/state`: returns action-derived state.
- `GET /ibex/v1/task/:id/result`: groups hosts by status.
- `GET /ibex/v1/task/:id/stdout.json` and `stderr.json`: return host output data.
- Auth tests must include missing, invalid, and valid BasicAuth credentials.

### RPC Tests

Applicable when testing agent/server integration without shell execution.

- `Server.Ping` returns `pong`.
- `Server.GetTaskMeta` returns script, args, account, and stdin for an existing task.
- `Server.Report` accepts completed task reports and returns assigned tasks from doing-cache.
- Missing task metadata returns a message instead of panicking.

### Agent Execution Tests

Applicable when testing local execution behavior.

- Use `t.TempDir()` as `MetaDir`.
- Use small shell scripts that exit `0`, exit non-zero, write stdout/stderr, and read stdin.
- Verify `.write`, `script`, `args`, `account`, `stdin`, `stdout`, `stderr`, and `<clock>.done` files.
- Verify repeated assignment with same id/clock/action is ignored.
- Verify `kill` for unknown local task is ignored.

## Disposable Database Rule

Do not run task-chain tests against the developer's default database. Use one of:

- A Docker Compose MySQL service with disposable schema.
- A temporary test database name generated per test run.
- A transactional test harness that truncates all model tables after each test.

## Suggested Test Command

After adding the database harness, expose a separate command:

```bash
./scripts/test-task-chain.sh
```

Expected behavior:

- Skip with a clear message when no disposable database is configured.
- Never modify `etc/service/server.conf` in place.
- Print final `task-chain: PASS` only after all lifecycle assertions pass.

## Risks To Cover

- Scheduler moves too many hosts into running state.
- Timeout marks server state done while agent later reports success.
- Agent local `meta/` cache replays stale results for a new clock.
- Output retrieval path differs between `database` and `remote` modes.
- BasicAuth accidentally protects `/ping` or fails to protect `/ibex/v1`.

## Acceptance Criteria

A future regression suite is sufficient when it proves:

1. A task can be created through HTTP.
2. The scheduler assigns at least one host.
3. Agent RPC report receives assigned work.
4. Agent executes script and persists local artifacts.
5. Server records final host status and output.
6. HTTP result/stdout/stderr endpoints return the final data.
7. Timeout and kill paths have deterministic assertions.
