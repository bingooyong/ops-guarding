# Quality Guidelines

> Validation gates, testing expectations, review standards, and forbidden patterns.

---

## Overview

Quality is enforced twice: locally by scripts and repo-local AI skills, and on
every push / pull request to `main` by GitHub Actions (`.github/workflows/ci.yml`).
CI runs the same commands as the local gates below, so a green local run predicts
a green CI run. CI also enforces a **coverage floor** (`.github/coverage-floor.txt`,
ratchet) and runs the suite plus a server+agentd+task smoke on a real MySQL
(`integration` job); overall coverage is still modest (see Testing Requirements).
The working rules come from `docs/ai-knowledge/ai-agent-rules.md`,
`docs/ai-knowledge/reusable-patterns.md`, and the skills in `skills/`.

---

## Required Gates

Run fresh, read the output, and report the evidence before claiming completion.

| Gate | Command | Expected |
|------|---------|----------|
| Unit tests + build | `./scripts/validate.sh` | last line `validation: PASS` (runs `go test ./...` and `go build -o bin/guarding ./cmd` with repo-local `GOCACHE`/`GOMODCACHE` under `.cache/`) |
| Static checks (any Go change) | `go vet ./...` and `gofmt -l $(git ls-files '*.go')` | exit 0, gofmt prints nothing |
| Race check (logger / concurrency changes) | `go test -race -count=1 ./pkg/appframe/logger` (or the touched package) | PASS |
| Startup smoke (process / config / router changes) | `./scripts/smoke-startup.sh` | `smoke: PASS`; starts agentd + server and curls `/ping`; server may stop at the documented DB blocker without MySQL; requires a local `etc/service/server.conf` (gitignored, copy from `server.conf.example`) |
| Coverage floor (any Go change; CI-enforced) | `./scripts/coverage-check.sh` | `coverage: PASS`; total (`-coverpkg=./...`) >= `.github/coverage-floor.txt`; `--update` only raises the floor (measured - 1.0) |
| Real-process smoke (server/agentd/RPC/schema changes; CI-enforced) | `SMOKE_CONFIRM_DISPOSABLE_DB=1 SMOKE_MYSQL_PASSWORD=... ./scripts/smoke-e2e.sh` against a **disposable** MySQL that has `sql/ibex.sql` loaded | `smoke-e2e: PASS` (server + agentd `/ping`, task reaches `done`, result `success`, stdout `smoke-ok`); normally left to the CI `integration` job |
| Module hygiene (any go.mod / go.sum / import change) | `go mod verify`, then `go mod tidy && git diff --exit-code go.mod go.sum` | `all modules verified`; tidy produces no diff |
| Vulnerabilities (any dependency or toolchain change) | `go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...` | exit 0, `Your code is affected by 0 vulnerabilities.` |
| Hygiene | `git status --short` | only intended paths changed |

### Go toolchain

`go.mod` pins `go 1.26.0` + `toolchain go1.26.10`. Builders need Go >= 1.26.0;
with the default `GOTOOLCHAIN=auto` an older local Go downloads go1.26.10
automatically (into `GOMODCACHE`, i.e. `.cache/go-mod` when run via
`validate.sh`). Do not lower the `go` line: `golang.org/x/net` >= 0.60 and
`golang.org/x/crypto` >= 0.56 require Go 1.26, and Go 1.25 is out of support.
Bump the `toolchain` line when a new 1.26.x patch fixes stdlib advisories
reported by govulncheck.

### CI gates (`.github/workflows/ci.yml`)

Triggers: push to `main`, pull request targeting `main`, manual
`workflow_dispatch`. `permissions: contents: read`, no secrets; a newer run on
the same ref cancels the older one.

| Job | Go | Steps (all blocking) |
|-----|----|----------------------|
| `test (go go.mod)` — required leg | from `go.mod` (`toolchain go1.26.10`) via `setup-go` `go-version-file` | `go mod download` + `go mod verify` → `go build ./...` → `./scripts/validate.sh` → `go vet ./...` → gofmt check (fails if `gofmt -l` lists any tracked `.go` file) → `go test -race -count=1 ./...` → `scripts/coverage-check.sh` (coverage floor) → `go mod tidy` diff check |
| `test (go stable)` — forward-compat leg | latest stable release | same as above except the tidy check (a newer Go may legitimately rewrite the `go`/`toolchain` lines) and the coverage floor (measured once, on the required leg) |
| `govulncheck` | from `go.mod` | `govulncheck@v1.9.0 ./...` — fails if the code calls a known vulnerability; module-only findings with no fix (e.g. GO-2026-5932 `x/crypto/openpgp`, not imported) are informational |
| `integration` | from `go.mod` | MySQL 8.4 service container (throwaway, root password `ci`) → load `sql/ibex.sql` into `ibex` → `go test -tags integration -race -count=1 ./...` with `OPS_MYSQL_DSN` (every `testenv.New` then uses a unique MySQL database; `pkg/models` integration tests use `ibex` directly) → `scripts/smoke-e2e.sh` |

Rules:

- Run the same commands locally before pushing; CI is a backstop, not the first check.
- Keep `ci.yml` and this table in sync. When adding a local gate, add the CI step
  in the same change (and vice versa).
- Changing `ci.yml`: lint with `actionlint` (with `shellcheck` on `PATH`) and
  YAML-parse it before committing.
- Actions are pinned to major tags (`actions/checkout@v7`, `actions/setup-go@v7`);
  govulncheck is pinned to an exact version. Bump them deliberately.
- `.github/dependabot.yml` opens weekly PRs for `gomod` (all `golang.org/x/*`
  grouped into one PR, because they require each other) and `github-actions`.
  Merge them only when CI is green.
- Branch protection is a repo-admin setting, not code. Required checks to enable on
  `main`: `test (go go.mod)`, `test (go stable)`, `govulncheck`, `integration`
  (steps in README "CI Quality Gates"). Until enabled, check the run before and after merging.
- Coverage floor is a ratchet: raise it with `scripts/coverage-check.sh --update` in the same
  PR that adds tests; never lower it to get CI green. CI never edits the floor file.
- The `integration` job and `smoke-e2e.sh` write to their database: only the CI service
  container or another disposable MySQL. `smoke-e2e.sh` requires
  `SMOKE_CONFIRM_DISPOSABLE_DB=1` for that reason.

These gates are packaged as `skills/ops-guarding-validation/SKILL.md`
(scopes `focused` / `full` / `final`). Warnings from `github.com/shoenig/go-m1cpu`
C code are non-blocking if the command exits 0.

---

## Testing Requirements

Current state (don't overstate it):

- Task-chain regression: `pkg/server/e2e/chain_test.go` drives create task (HTTP,
  BasicAuth) → schedule (`logic`) → agent RPC `Report`/`GetTaskMeta` → state /
  result / host output (HTTP), incl. batch, tolerance→pause, timeout + late
  report, unknown task/host, auth. `pkg/agentd/timer/loopback_test.go` runs the
  real agentd heartbeat/RPC client/script execution against an in-process
  `rpc.Serve` listener. Both use `pkg/internal/testenv`.
- `pkg/internal/testenv.New(t)`: isolated DB + memory cache + `config.C`
  (restored in Cleanup). Default backend is in-memory sqlite (test-only dep
  `github.com/glebarez/sqlite`, pure Go, never linked into `./cmd`); with
  `-tags integration` and `OPS_MYSQL_DSN` it creates a unique MySQL database
  from `sql/ibex.sql` and drops it afterwards. `TestSchemaMatchesIbexSQL`
  fails if the hand-written sqlite DDL drifts from `sql/ibex.sql` columns.
- Unit tests now cover `models.AggregateState`, `router` task-state / host
  output handlers (gin `httptest` + injectable `task*Fn` vars, no DB), and
  `appframe/netx.LocalIP` (injected `Dial` / `Interfaces`). MySQL tests live
  behind the `integration` build tag.
- Focused unit tests: `pkg/server/rpc` (`method_test.go`, `serve_test.go` incl.
  the msgpack wire via `rpc.Serve`), `pkg/server/logic` (batch, tolerance→pause,
  pause hosts, non-start actions, missing meta/action, done-task cleanup,
  timeout grace), `pkg/server/timer` (doing cache, orphan/mine scheduling,
  long-task cancel, dead-scheduler takeover), `pkg/agentd/timer`
  (`task_exec_test.go`: success/failed/stdin/args, duplicate+new-clock
  assignment, replay of finished results, unknown action, report truncation
  at 65535, `Clean`; `task_kill_test.go`: kill, grace fallback, alive flag).
  Shared helpers: `testenv.Env.SeedTask/HostStatuses/Doing/Action`.
- agentd kill/exec fixes (task `agentd-kill-fixes`) are covered by regression
  tests that run under `-race` (no `!race` exclusion any more):
  `TestKillWhileOutputIsStreaming` (race witness), `TestKillTaskWithoutProcess`
  (nil/unstarted `Cmd`, replayed result), `TestKillGraceFallbackRecordsResult`,
  `TestRerunResetsOutputAndStdin`, `TestStartFailureReportsFailed`,
  `TestStartMarksTaskAliveSynchronously`, and `prepare_failure_test.go`
  (prepare error -> reported `failed`, redo with a new clock recovers). When changing `pkg/agentd/timer/task.go`
  run `go test -race -count=3 ./pkg/agentd/...`.
- Agent execution tests pre-populate `MetaDir/<id>/{script,args,account,stdin,.write}`
  so `Task.prepare` reads from disk and needs no RPC server; use the current
  OS user as account (root runs directly, others always run via `sh -c`).
  `testenv.New` costs ~0.2s under `-race` (sqlite DDL); prefer table-driven
  subtests that share one env only when state does not leak.
- Coverage (Go 1.26.10): per-package default mode 22.6% total; with
  `-coverpkg=./...` (counts cross-package reach of e2e/loopback tests) 37.4%.
  Use the `-coverpkg=./...` figure for any coverage gate.

Rules for new work:

- Use stdlib `testing` with plain assertions via `t.Fatalf("X() = %q, want %q", got, want)`
  (style of the existing tests); no testify dependency is present.
- Behaviour changes in `pkg/server/logic`, `pkg/models`, `pkg/server/rpc`,
  or `pkg/agentd/timer` should come with a focused test. Follow the layer plan
  in `docs/ai-knowledge/task-chain-regression-tests.md` (model-level, HTTP,
  RPC, agent-execution tests).
- Agent execution tests use `t.TempDir()` as `MetaDir` and small shell scripts
  (`newAgentEnv` in `pkg/agentd/timer/task_exec_test.go`); poll for status with a
  deadline, never fixed long sleeps.
- Database tests must use a disposable database and skip with a clear message
  when none is configured; never use `etc/service/server.conf`'s DB or edit
  that file in place.
- For concurrency fixes, reproduce with `-race` first, then add a concurrent test
  (`skills/go-concurrency-review-fix`).

---

## Code Standards

- `gofmt`-clean, `go vet`-clean (both currently pass on `main`).
- Follow the existing layering (see `directory-structure.md`), error style
  (`error-handling.md`), and logger choice per process (`logging-guidelines.md`).
- Types containing `sync.Mutex` are passed/returned by pointer
  (`agentd/timer.Task` embeds `sync.Mutex`; `Locals.M` stores `*Task`).
- Mutable config read by goroutines must be set before the goroutine starts
  or guarded by the same lock on every read and write.
- Long-running loops must be stoppable or at least must not panic: agentd uses
  `ctx.Done()` (`timer.Heartbeat(ctx)`); server timers currently loop forever
  with `time.Sleep`.
- Keep diffs small and scoped; prefer existing `appframe` helpers over new
  packages; add dependencies only when explicitly requested.
- Secrets stay out of the repo: `etc/service/server.conf` is gitignored, only
  `server.conf.example` with `CHANGE_ME` placeholders is tracked.

---

## Forbidden Patterns

- Staging ignored/runtime paths: `.cache/`, `bin/`, `meta/`, `logs*/`, `.codex/`,
  `.omx/`, `.idea/`, `.trellis/`, `.cursor/`, `.grok/`. Never `git add .`.
- String-concatenated SQL; use `?` placeholders (see `database-guidelines.md`).
- Using `DB()` instead of `tx` inside `DB().Transaction(...)`.
- Dereferencing a model lookup result without a nil check.
- Hard-coded outbound network dependencies at startup (the existing
  `GetOutboundIP()` dialing `8.8.8.8:80` is a known gap, not a pattern).
- Destructive git commands (`git reset --hard`, force push) without explicit request.
- Claiming success without fresh command output.

---

## Code Review Checklist

Based on `skills/ai-delivery-quality-gates/SKILL.md`:

1. **Spec compliance** — the change does what the PRD/plan asks, no more, no less.
2. **Code quality** — error handling per layer, nil checks, tx usage, logging
   without secrets, goroutine lifecycle, lock discipline.
3. Treat reviewer findings as hypotheses: reproduce, fix one root cause, re-run
   the failing command, then re-run `./scripts/validate.sh`.
4. Final report: changed files, commands run with results, remaining risks.

---

## Known Gaps (tracked, not yet fixed)

- Branch protection does not yet require the CI checks (admin must enable; see above).
- The `integration` job was only verifiable on GitHub (no local MySQL); watch its first runs.
- `outputs.WriteSamples` is empty; metrics inputs are not started by agentd.
- `taskState` TODO at `pkg/server/router/handler.go:32` (state ignores host results).
- `GetOutboundIP()` dials `8.8.8.8` in both `pkg/server/config` and `pkg/agentd/config`.
- Server↔agent RPC (`net/rpc` + msgpack) has no TLS or authentication.
