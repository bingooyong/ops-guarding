---
name: ops-guarding-validation
description: Run the ops-guarding repository validation gates. Use when verifying local changes, preparing completion reports, checking source baseline health, or confirming README build/smoke instructions remain actionable.
---

# Ops Guarding Validation

## Skill 名称

`ops-guarding-validation`

## 适用场景

Use this skill when a Codex agent must prove that `ops-guarding` still builds, tests, starts locally, and has no accidental source or runtime-state drift.

## 输入参数

- `repo_root`: repository root, default current working directory.
- `scope`: `full`, `focused`, or `final`; default `full`.
- `focused_package`: optional Go package, for example `./pkg/appframe/logger`.
- `commit`: optional commit or `HEAD` to inspect after validation.

## 输出结果

- Command evidence with exit codes.
- PASS/FAIL summary for tests, build, smoke startup, git status, and optional static checks.
- List of remaining risks, including environment-only warnings.

## 执行步骤

1. Confirm workspace state:
   ```bash
   git status --short
   ```
2. For focused code changes, run the smallest proof first:
   ```bash
   go test <focused_package>
   ```
3. For logger or concurrency-sensitive changes, run race detection:
   ```bash
   go test -race -count=1 ./pkg/appframe/logger
   ```
4. Run static checks when code changed:
   ```bash
   go vet ./...
   gofmt -l $(git ls-files '*.go')
   ```
5. Run the project validation script:
   ```bash
   ./scripts/validate.sh
   ```
   Expected final line: `validation: PASS`.
6. Run startup smoke:
   ```bash
   ./scripts/smoke-startup.sh
   ```
   Expected final line: `smoke: PASS`.
7. Re-check repository cleanliness:
   ```bash
   git status --short
   git log --oneline -n 8
   ```

## 依赖工具

- Go toolchain compatible with `go.mod`.
- `curl` for `scripts/smoke-startup.sh`.
- Local shell with executable project scripts.
- Optional: `go vet`, Go race detector.

## 使用示例

```text
Use $ops-guarding-validation with scope=final before reporting this task complete.
```

Expected evidence summary:

```text
validation: PASS
smoke: PASS
go vet ./...: exit 0
git status --short: empty
```

## 注意事项

- Treat dependency C warnings from `github.com/shoenig/go-m1cpu` as non-blocking only when the command exits 0.
- Do not claim completion from cached memory; run fresh commands in the current turn.
- `scripts/smoke-startup.sh` starts local `agentd` and `server` temporarily and expects `/ping` checks to pass or an explicitly handled database blocker.
- Ignored paths such as `.cache/`, `bin/`, `meta/`, `.codex/`, `.omx/`, and `.idea/` must not be staged as source.

## 变更记录

- 2026-07-05: Initial project-local skill created from the engineering baseline validation workflow.
