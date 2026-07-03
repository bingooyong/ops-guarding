# Delivery Quality Roadmap Design

## Purpose

This spec defines the next workstream for `ops-guarding`. The goal is to turn the current repository into a project that an AI coding agent can run, test, change, and hand over with reliable evidence. Schedule and effort are secondary to delivery quality.

The workstream prioritizes engineering baseline quality before new business capability. The repository already shows a clear server-agent architecture, but it lacks a stable validation path, complete operating documentation, and broad regression coverage.

## Current Evidence

- `cmd/main.go` exposes two CLI modes: `server` and `agentd`.
- `docs/project-analysis.md` documents the current architecture, risks, and data flow.
- `etc/service/server.conf` and `etc/service/agentd.conf` provide sample configuration, while code defaults point to `etc/server.conf` and `etc/agentd.conf`.
- `sql/ibex.sql` defines the MySQL schema, including `task_host_0` through `task_host_99`.
- `go test ./...` does not provide a clean baseline in the current environment. The first run attempted to write outside the sandboxed Go cache. A rerun with `GOCACHE=/tmp/ops-guarding-gocache` still attempted module cache writes under `$HOME/go/pkg/mod` and exposed a `pkg/appframe/logger` syslog test panic.
- The Git repository has no commits yet, and most files are untracked.

## Recommended Direction

Choose the engineering baseline first. This path makes later AI coding work safer because every future change can use the same documented commands, configuration assumptions, and regression checks.

Do not start with state-machine feature work or production deployment hardening. Those areas matter, but they need a reliable test and run baseline first.

## Phase 1: Engineering Baseline

Phase 1 makes the project runnable, testable, and explainable.

Required outcomes:

- A contributor can build the CLI with one documented command.
- A contributor can run `server` and `agentd` with repository-provided configuration paths.
- The repository documents database bootstrap, required services, expected ports, and local smoke checks.
- The test command uses repo-local or temporary Go caches so it does not depend on host-specific cache permissions.
- Known environment-sensitive tests either pass reliably or skip with a clear reason.
- The repository has a first meaningful Git commit that excludes local tool state and generated noise.

Suggested tasks:

1. Normalize configuration entry points.
   - Either copy sample configs to the code's default paths, or change defaults to `etc/service/server.conf` and `etc/service/agentd.conf`.
   - Document the chosen path.
2. Create a practical README.
   - Include build, run, database bootstrap, smoke test, and troubleshooting sections.
   - Explain `server` versus `agentd`.
3. Add a repeatable validation script.
   - Set `GOCACHE` and `GOMODCACHE` to safe local or temporary paths.
   - Run `go test ./...`.
   - Print clear failure causes.
4. Fix or guard the syslog logger test.
   - Skip the test when the platform lacks usable syslog delivery.
   - Keep normal logger tests active.
5. Establish repository hygiene.
   - Add or tighten `.gitignore` for `.DS_Store`, IDE files, local OMX/Codex state, build output, logs, and local metadata.
   - Commit only source, config examples, SQL, docs, and intentional test assets.

Acceptance evidence:

- `go test ./...` runs through the documented validation path.
- `go build -o bin/guarding ./cmd` succeeds.
- README commands match the code's actual defaults.
- `git status --short` shows only intentional changes before commit.

## Phase 2: Core Chain Confidence

Phase 2 adds regression coverage for the actual business chain.

Required outcomes:

- Tests cover task creation validation, host cleanup, action transitions, timeout handling, and done-task cleanup.
- Tests cover agent local task assignment, result persistence, report payload creation, and completed-task cleanup.
- RPC request and response types have focused tests around `GetTaskMeta` and `Report`.
- The project can run a small local smoke flow against a disposable database.

Suggested tasks:

1. Add model and scheduler unit tests with isolated database setup.
2. Add agent task tests that avoid running destructive shell commands.
3. Add route tests for `/ibex/v1/tasks` and task result endpoints.
4. Add a smoke script that creates one harmless task and verifies status/result flow.

Acceptance evidence:

- Focused tests prove task state transitions.
- Smoke flow creates and completes a harmless task.
- Failure output points to the layer that failed: config, DB, RPC, scheduling, agent execution, or result storage.

## Phase 3: Production Readiness

Phase 3 prepares the project for safer deployment.

Required outcomes:

- Secrets no longer live in committed runtime config.
- Deployment docs explain process model, ports, database, logs, and restart behavior.
- Metrics output has a defined target or remains explicitly out of scope.
- Health checks distinguish HTTP availability from scheduler and RPC health.
- Multi-server scheduler takeover behavior has tests or a documented operational model.

Suggested tasks:

1. Move credentials to environment variables or uncommitted local override files.
2. Add service examples for `server` and `agentd`.
3. Define logs, metrics, and alert points.
4. Document high-availability assumptions and failure modes.

Acceptance evidence:

- A new operator can deploy with example commands without reading source code.
- Secret placeholders replace committed passwords.
- Health and logging checks provide useful diagnostics.

## Non-Goals

- Do not redesign the server-agent architecture during Phase 1.
- Do not add new dependencies unless a validation or documentation gap requires one.
- Do not rewrite the task scheduler before tests capture its current behavior.
- Do not implement metrics output until the desired backend is chosen.
- Do not commit local `.codex`, `.omx`, `.idea`, cache, log, or machine-specific files.

## Design Rationale

The current project needs a stable foundation before feature work. AI coding can move quickly, but it needs fast feedback. A clean baseline reduces rework, prevents unverified changes, and makes every later implementation plan easier to review.

The recommended sequence also keeps scope bounded. Phase 1 improves how contributors run and verify the existing code. Phase 2 locks down the core behavior. Phase 3 moves the project toward production operations.

## Open Decisions For Implementation Planning

The implementation plan should decide these items explicitly:

- Whether config defaults should move to `etc/service/*.conf` or sample configs should be copied to `etc/*.conf`.
- Whether validation caches should live under `.cache/go/` or `/tmp/ops-guarding-*`.
- Whether syslog tests should use build tags, environment checks, or `t.Skip`.
- Which database strategy Phase 2 should use: real MySQL, containerized MySQL, or an isolated test database.
- Which files should belong in the first commit.

## Approval State

The user approved the recommended direction on 2026-07-04 and asked to proceed quickly to spec generation. This spec records that recommended direction and stops before implementation planning.
