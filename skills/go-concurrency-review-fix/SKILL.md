---
name: go-concurrency-review-fix
description: Reproduce, root-cause, and fix Go race detector, go vet, mutex-copy, and concurrent configuration findings. Use when reviews or tests report data races, copied locks, unsafe goroutine initialization, or static-analysis failures in Go code.
---

# Go Concurrency Review Fix

## Skill 名称

`go-concurrency-review-fix`

## 适用场景

Use this skill when Go review findings mention `DATA RACE`, `go vet`, `return copies lock value`, goroutines reading mutable configuration, or logger/backend initialization order.

## 输入参数

- `package`: Go package to inspect, for example `./pkg/appframe/logger`.
- `finding`: reviewer finding or failing command output.
- `suspect_files`: optional file list.
- `verification_level`: `focused` or `full`; default `focused` before final validation.

## 输出结果

- Reproduced failure command and key stack/file lines.
- Root-cause explanation.
- Minimal code fix.
- Regression test or focused race/static check.
- Verification commands and final status.

## 执行步骤

1. Reproduce the finding before editing:
   ```bash
   go test -race <package>
   go vet ./...
   ```
2. Read the exact files and line numbers from the failure. For races, identify both the read goroutine and write goroutine.
3. Trace shared mutable state:
   - Which fields are read by goroutines?
   - Which setters or constructors write those fields?
   - Is the same lock used on every read/write path?
4. Fix root cause, not the symptom:
   - Configure objects before starting goroutines when practical.
   - Or guard every read/write of mutable shared fields with the same lock.
   - Avoid returning structs containing `sync.Mutex` by value; return pointers instead.
5. Add or strengthen focused tests. For this repo, use a concurrent configuration test plus:
   ```bash
   go test -race -count=1 ./pkg/appframe/logger
   ```
6. Run static and formatting checks:
   ```bash
   go vet ./...
   gofmt -l $(git ls-files '*.go')
   ```
7. Run project-level validation before completion:
   ```bash
   ./scripts/validate.sh
   ./scripts/smoke-startup.sh
   ```

## 依赖工具

- Go test and race detector.
- `go vet`.
- `gofmt`.
- Git diff/status for checking scoped changes.

## 使用示例

```text
Use $go-concurrency-review-fix with package=./pkg/appframe/logger and finding="go test -race reports FileBackend flushInterval race".
```

## 注意事项

- A passing normal test does not prove a race fix; run the race detector.
- Locking only setters is insufficient if daemon reads remain unlocked.
- Returning a struct with embedded mutex by value can copy lock state; use pointer constructors.
- If `go vet` exits 0 with third-party C warnings, record the warnings but do not treat them as repository failures.
- Do not bundle unrelated refactors with concurrency fixes.

## 变更记录

- 2026-07-05: Initial project-local skill created from the logger race and copied-mutex remediation flow.
