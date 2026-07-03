# Engineering Baseline Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `ops-guarding` buildable, testable, runnable enough for smoke checks, documented, and ready for a clean first source baseline commit.

**Architecture:** Keep the existing server-agent architecture. Phase 1 only normalizes entry points, validation scripts, environment-sensitive tests, documentation, and repository hygiene. It does not change scheduler behavior, RPC contracts, database schema, or metrics behavior.

**Tech Stack:** Go 1.22 module, `urfave/cli/v2`, Gin, GORM, MySQL/Postgres drivers, shell scripts, Markdown.

---

## Scope

This plan implements Phase 1 from `docs/superpowers/specs/2026-07-04-delivery-quality-roadmap-design.md`.

Do not implement Phase 2 task-chain tests or Phase 3 production hardening in this plan. Create separate plans for those after Phase 1 passes.

## File Structure

- Modify `.gitignore`: keep generated caches, local runtime state, IDE files, logs, binaries, and local metadata out of the source baseline.
- Modify `pkg/server/server.go`: expose and use the server default config path.
- Create `pkg/server/server_test.go`: lock the server default config path.
- Modify `pkg/agentd/agentd.go`: expose and use the agent default config path.
- Create `pkg/agentd/agentd_test.go`: lock the agent default config path.
- Modify `etc/service/server.conf`: make the local sample avoid outbound IP auto-detection.
- Modify `pkg/appframe/logger/logger_test.go`: skip syslog test when the host cannot open syslog.
- Create `scripts/validate.sh`: run repeatable Go tests and build with repo-local caches.
- Create `scripts/smoke-startup.sh`: build the binary, smoke-check `agentd /ping`, and either check `server /ping` or report the expected database startup blocker.
- Rename `READEME.md` to `README.md`: replace the draft with contributor-facing build, run, validation, and troubleshooting documentation.

## Task 1: Repository Hygiene

**Files:**
- Modify: `.gitignore`

- [ ] **Step 1: Capture current noisy state**

Run:

```bash
git status --short
```

Expected before this task: output includes local or generated paths such as `.codex/`, `.omx/`, `.idea/`, `.DS_Store`, and `meta/`. It also includes source paths because the first source baseline commit has not happened yet.

- [ ] **Step 2: Replace `.gitignore` with source-baseline rules**

Edit `.gitignore` to this complete content:

```gitignore
# macOS and editor noise
.DS_Store
.idea/

# Local agent/runtime state
.codex/
.omx/
/meta/

# Build outputs
/bin/
/ctl
/gostarter
*.exe
*.exe~
*.dll
*.so
*.dylib
*.test
*.out

# Local caches and generated validation state
.cache/
/coverage/
/test.json

# Runtime output
/logs/
/logs-*/
/var/

# Local configuration overrides
cnf.toml
*.local.conf
*.local.toml
*.local.yaml
*.local.json
```

- [ ] **Step 3: Verify ignored paths are no longer candidates for source baseline**

Run:

```bash
git status --short --ignored .codex .omx .idea meta .cache bin logs-server logs-agentd
```

Expected after this task: the listed local/generated paths appear as ignored (`!!`) or do not appear if absent. They must not appear as untracked (`??`).

- [ ] **Step 4: Commit hygiene change**

Run:

```bash
git add .gitignore
git commit -m "chore: ignore local runtime state"
```

Expected: commit succeeds and includes only `.gitignore`.

## Task 2: Normalize Default Configuration Paths

**Files:**
- Modify: `pkg/server/server.go`
- Create: `pkg/server/server_test.go`
- Modify: `pkg/agentd/agentd.go`
- Create: `pkg/agentd/agentd_test.go`
- Modify: `etc/service/server.conf`

- [ ] **Step 1: Add failing tests for default config paths**

Create `pkg/server/server_test.go`:

```go
package server

import (
	"path/filepath"
	"testing"
)

func TestDefaultConfigFile(t *testing.T) {
	want := filepath.Join("etc", "service", "server.conf")
	if got := DefaultConfigFile(); got != want {
		t.Fatalf("DefaultConfigFile() = %q, want %q", got, want)
	}
}
```

Create `pkg/agentd/agentd_test.go`:

```go
package agentd

import (
	"path/filepath"
	"testing"
)

func TestDefaultConfigFile(t *testing.T) {
	want := filepath.Join("etc", "service", "agentd.conf")
	if got := DefaultConfigFile(); got != want {
		t.Fatalf("DefaultConfigFile() = %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run tests and verify they fail**

Run:

```bash
GOCACHE="$(pwd)/.cache/go-build" GOMODCACHE="$(pwd)/.cache/go-mod" go test ./pkg/server ./pkg/agentd
```

Expected: both packages fail to compile with errors that include `undefined: DefaultConfigFile`.

- [ ] **Step 3: Implement server default config helper**

In `pkg/server/server.go`, add this function near the option helpers:

```go
func DefaultConfigFile() string {
	return filepath.Join("etc", "service", "server.conf")
}
```

In `Run`, change the default config initialization from:

```go
server := Server{
	ConfigFile: filepath.Join("etc", "server.conf"),
	Version:    "not specified",
}
```

to:

```go
server := Server{
	ConfigFile: DefaultConfigFile(),
	Version:    "not specified",
}
```

- [ ] **Step 4: Implement agent default config helper**

In `pkg/agentd/agentd.go`, keep the existing `path/filepath` import and add this function near the option helpers:

```go
func DefaultConfigFile() string {
	return filepath.Join("etc", "service", "agentd.conf")
}
```

In `Run`, change the default config initialization from:

```go
agentd := Agentd{
	ConfigFile: filepath.Join("etc", "agentd.conf"),
	Version:    "not specified",
}
```

to:

```go
agentd := Agentd{
	ConfigFile: DefaultConfigFile(),
	Version:    "not specified",
}
```

- [ ] **Step 5: Make sample server config local-friendly**

In `etc/service/server.conf`, change the heartbeat section from:

```toml
[Heartbeat]
# auto detect if blank
IP = ""
# unit: ms
Interval = 1000
```

to:

```toml
[Heartbeat]
# local sample uses loopback to avoid outbound IP auto-detection
IP = "127.0.0.1"
# unit: ms
Interval = 1000
```

- [ ] **Step 6: Verify config path tests pass**

Run:

```bash
GOCACHE="$(pwd)/.cache/go-build" GOMODCACHE="$(pwd)/.cache/go-mod" go test ./pkg/server ./pkg/agentd
```

Expected: both packages pass.

- [ ] **Step 7: Commit config path normalization**

Run:

```bash
git add pkg/server/server.go pkg/server/server_test.go pkg/agentd/agentd.go pkg/agentd/agentd_test.go etc/service/server.conf
git commit -m "chore: align default config paths"
```

Expected: commit succeeds and includes only the five listed files.

## Task 3: Repeatable Validation Script

**Files:**
- Create: `scripts/validate.sh`

- [ ] **Step 1: Create validation script**

Create `scripts/validate.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

export GOCACHE="${GOCACHE:-$ROOT_DIR/.cache/go-build}"
export GOMODCACHE="${GOMODCACHE:-$ROOT_DIR/.cache/go-mod}"

mkdir -p "$GOCACHE" "$GOMODCACHE" "$ROOT_DIR/bin"

echo "validation: go test ./..."
go test ./...

echo "validation: go build -o bin/guarding ./cmd"
go build -o "$ROOT_DIR/bin/guarding" ./cmd

echo "validation: PASS"
```

- [ ] **Step 2: Make script executable**

Run:

```bash
chmod +x scripts/validate.sh
```

Expected: command exits with code 0.

- [ ] **Step 3: Check shell syntax**

Run:

```bash
bash -n scripts/validate.sh
```

Expected: command exits with code 0 and prints no output.

- [ ] **Step 4: Run validation and capture current blocker**

Run:

```bash
./scripts/validate.sh
```

Expected before Task 4: validation may fail in `pkg/appframe/logger` because `TestSysLogBackend` panics on hosts without syslog delivery. If it fails for that reason, continue to Task 4.

- [ ] **Step 5: Commit validation script**

Run:

```bash
git add scripts/validate.sh
git commit -m "chore: add repeatable validation script"
```

Expected: commit succeeds and includes only `scripts/validate.sh`.

## Task 4: Guard Environment-Sensitive Syslog Test

**Files:**
- Modify: `pkg/appframe/logger/logger_test.go`

- [ ] **Step 1: Verify current syslog failure**

Run:

```bash
GOCACHE="$(pwd)/.cache/go-build" GOMODCACHE="$(pwd)/.cache/go-mod" go test ./pkg/appframe/logger -run TestSysLogBackend -v
```

Expected before this task on hosts without syslog delivery: test fails or panics with an error similar to `Unix syslog delivery error`.

- [ ] **Step 2: Replace panic with skip**

In `pkg/appframe/logger/logger_test.go`, change `TestSysLogBackend` from:

```go
func TestSysLogBackend(t *testing.T) {
	b, err := NewSyslogBackend("local3", "mohawk")
	if err != nil {
		panic(err)
	}
	defer Close()
	SetLogging("DEBUG", b)
	Debug("Debug")
	Errorf("%d %s", 123, "error")
	Info("Info")
	time.Sleep(time.Second * 2)
}
```

to:

```go
func TestSysLogBackend(t *testing.T) {
	b, err := NewSyslogBackend("local3", "mohawk")
	if err != nil {
		t.Skipf("syslog backend unavailable on this host: %v", err)
	}
	defer Close()
	SetLogging("DEBUG", b)
	Debug("Debug")
	Errorf("%d %s", 123, "error")
	Info("Info")
	time.Sleep(time.Second * 2)
}
```

- [ ] **Step 3: Verify logger tests pass or skip syslog cleanly**

Run:

```bash
GOCACHE="$(pwd)/.cache/go-build" GOMODCACHE="$(pwd)/.cache/go-mod" go test ./pkg/appframe/logger -v
```

Expected: package passes. `TestSysLogBackend` either passes or reports `SKIP: syslog backend unavailable on this host`.

- [ ] **Step 4: Verify full validation passes**

Run:

```bash
./scripts/validate.sh
```

Expected: output ends with `validation: PASS`.

- [ ] **Step 5: Commit syslog test guard**

Run:

```bash
git add pkg/appframe/logger/logger_test.go
git commit -m "test: guard syslog logger test"
```

Expected: commit succeeds and includes only `pkg/appframe/logger/logger_test.go`.

## Task 5: Startup Smoke Script

**Files:**
- Create: `scripts/smoke-startup.sh`

- [ ] **Step 1: Create startup smoke script**

Create `scripts/smoke-startup.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

export GOCACHE="${GOCACHE:-$ROOT_DIR/.cache/go-build}"
export GOMODCACHE="${GOMODCACHE:-$ROOT_DIR/.cache/go-mod}"

mkdir -p "$GOCACHE" "$GOMODCACHE" "$ROOT_DIR/bin" "$ROOT_DIR/.cache/smoke"

go build -o "$ROOT_DIR/bin/guarding" ./cmd

agent_log="$ROOT_DIR/.cache/smoke/agentd.log"
server_log="$ROOT_DIR/.cache/smoke/server.log"
rm -f "$agent_log" "$server_log"

cleanup() {
	if [[ -n "${AGENT_PID:-}" ]] && kill -0 "$AGENT_PID" 2>/dev/null; then
		kill "$AGENT_PID" 2>/dev/null || true
		wait "$AGENT_PID" 2>/dev/null || true
	fi
	if [[ -n "${SERVER_PID:-}" ]] && kill -0 "$SERVER_PID" 2>/dev/null; then
		kill "$SERVER_PID" 2>/dev/null || true
		wait "$SERVER_PID" 2>/dev/null || true
	fi
}
trap cleanup EXIT

"$ROOT_DIR/bin/guarding" agentd -c etc/service/agentd.conf >"$agent_log" 2>&1 &
AGENT_PID="$!"
sleep 2

if ! kill -0 "$AGENT_PID" 2>/dev/null; then
	echo "smoke: agentd exited unexpectedly"
	cat "$agent_log"
	exit 1
fi

curl -fsS http://127.0.0.1:2090/ping >/dev/null
echo "smoke: agentd /ping PASS"

"$ROOT_DIR/bin/guarding" server -c etc/service/server.conf >"$server_log" 2>&1 &
SERVER_PID="$!"
sleep 3

if kill -0 "$SERVER_PID" 2>/dev/null; then
	curl -fsS http://127.0.0.1:10090/ping >/dev/null
	echo "smoke: server /ping PASS"
else
	if grep -Eiq 'connect|connection|database|mysql|postgres|access denied|refused|no such host' "$server_log"; then
		echo "smoke: server startup blocked by documented database dependency"
		cat "$server_log"
	else
		echo "smoke: server exited for an unexpected reason"
		cat "$server_log"
		exit 1
	fi
fi

echo "smoke: PASS"
```

- [ ] **Step 2: Make script executable**

Run:

```bash
chmod +x scripts/smoke-startup.sh
```

Expected: command exits with code 0.

- [ ] **Step 3: Check shell syntax**

Run:

```bash
bash -n scripts/smoke-startup.sh
```

Expected: command exits with code 0 and prints no output.

- [ ] **Step 4: Run startup smoke**

Run:

```bash
./scripts/smoke-startup.sh
```

Expected in a local environment without MySQL: output includes `smoke: agentd /ping PASS`, `smoke: server startup blocked by documented database dependency`, and `smoke: PASS`.

Expected in an environment with MySQL configured from `etc/service/server.conf`: output includes `smoke: agentd /ping PASS`, `smoke: server /ping PASS`, and `smoke: PASS`.

- [ ] **Step 5: Commit startup smoke script**

Run:

```bash
git add scripts/smoke-startup.sh
git commit -m "chore: add startup smoke script"
```

Expected: commit succeeds and includes only `scripts/smoke-startup.sh`.

## Task 6: Contributor README

**Files:**
- Rename: `READEME.md` to `README.md`
- Modify: `README.md`

- [ ] **Step 1: Rename misspelled README**

Run:

```bash
mv READEME.md README.md
```

Expected: command exits with code 0. `READEME.md` is currently untracked, so use `mv` rather than `git mv`.

- [ ] **Step 2: Replace README content**

Replace `README.md` with this complete content:

```markdown
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

- Go 1.22 or newer.
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
```

- [ ] **Step 3: Verify README references existing paths**

Run:

```bash
test -f README.md
test -f etc/service/server.conf
test -f etc/service/agentd.conf
test -f scripts/validate.sh
test -f scripts/smoke-startup.sh
```

Expected: all commands exit with code 0.

- [ ] **Step 4: Commit README**

Run:

```bash
git add README.md
git commit -m "docs: add contributor README"
```

Expected: commit succeeds and includes `README.md`. `READEME.md` was untracked before the rename, so it should not appear as a tracked deletion.

## Task 7: Final Validation And Source Baseline Commit

**Files:**
- Stage source baseline files that are still untracked after previous task commits.

- [ ] **Step 1: Run full validation**

Run:

```bash
./scripts/validate.sh
```

Expected: output ends with `validation: PASS`.

- [ ] **Step 2: Run startup smoke**

Run:

```bash
./scripts/smoke-startup.sh
```

Expected: output ends with `smoke: PASS`.

- [ ] **Step 3: Inspect remaining untracked files**

Run:

```bash
git status --short
```

Expected: remaining untracked source files include paths such as `AGENTS.md`, `cmd/`, `etc/`, `go.mod`, `go.sum`, `pkg/`, `sql/`, and `docs/project-analysis.md`. Local paths such as `.codex/`, `.omx/`, `.idea/`, `.DS_Store`, `.cache/`, `bin/`, and `meta/` must not appear as untracked.

- [ ] **Step 4: Stage source baseline**

Run:

```bash
git add AGENTS.md cmd docs/project-analysis.md etc go.mod go.sum pkg sql
```

Expected: command exits with code 0.

- [ ] **Step 5: Verify staged source baseline**

Run:

```bash
git diff --cached --name-only
```

Expected: staged files are limited to project source, docs, config examples, SQL, and module files. The list must not include `.codex/`, `.omx/`, `.idea/`, `.DS_Store`, `.cache/`, `bin/`, or `meta/`.

- [ ] **Step 6: Commit source baseline**

Run:

```bash
git commit -m "chore: add source baseline"
```

Expected: commit succeeds.

- [ ] **Step 7: Verify clean source status**

Run:

```bash
git status --short
```

Expected: no untracked source files remain. Ignored local state may exist but should not appear without `--ignored`.

## Final Verification

Run these commands after all tasks:

```bash
./scripts/validate.sh
./scripts/smoke-startup.sh
git log --oneline -n 8
git status --short
```

Expected final evidence:

- Validation ends with `validation: PASS`.
- Smoke check ends with `smoke: PASS`.
- Git log contains commits for hygiene, config defaults, validation, syslog guard, startup smoke, README, and source baseline.
- `git status --short` does not show source files that should have been committed.

## Deferred Work

- Phase 2: add task-chain regression tests and local disposable database flow.
- Phase 3: remove committed secrets from runtime config, define deployment/service examples, and document production health and metrics behavior.
