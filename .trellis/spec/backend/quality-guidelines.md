# Quality Guidelines

> Validation gates, testing expectations, review standards, and forbidden patterns.

---

## Overview

Quality is enforced by local scripts and repo-local AI skills, not by CI.
**There is no CI** (no `.github/workflows`) and **test coverage is minimal**.
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
| Hygiene | `git status --short` | only intended paths changed |

These gates are packaged as `skills/ops-guarding-validation/SKILL.md`
(scopes `focused` / `full` / `final`). Warnings from `github.com/shoenig/go-m1cpu`
C code are non-blocking if the command exits 0.

---

## Testing Requirements

Current state (don't overstate it):

- Only three test files: `pkg/server/server_test.go` and
  `pkg/agentd/agentd_test.go` (`TestDefaultConfigFile`), and
  `pkg/appframe/logger/logger_test.go` (backends, concurrency).
- No tests touch models, HTTP handlers, RPC, scheduler logic, or agent execution.

Rules for new work:

- Use stdlib `testing` with plain assertions via `t.Fatalf("X() = %q, want %q", got, want)`
  (style of the existing tests); no testify dependency is present.
- Behaviour changes in `pkg/server/logic`, `pkg/models`, `pkg/server/rpc`,
  or `pkg/agentd/timer` should come with a focused test. Follow the layer plan
  in `docs/ai-knowledge/task-chain-regression-tests.md` (model-level, HTTP,
  RPC, agent-execution tests).
- Agent execution tests use `t.TempDir()` as `MetaDir` and small shell scripts.
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

- No CI pipeline; gates run only locally.
- No functional regression test for the task chain (create → schedule → agent → report → result).
- `outputs.WriteSamples` is empty; metrics inputs are not started by agentd.
- `taskState` TODO at `pkg/server/router/handler.go:32` (state ignores host results).
- `GetOutboundIP()` dials `8.8.8.8` in both `pkg/server/config` and `pkg/agentd/config`.
- Server↔agent RPC (`net/rpc` + msgpack) has no TLS or authentication.
- Dependabot PR #1 (`golang.org/x/crypto` 0.14.0 → 0.52.0) is pending.
