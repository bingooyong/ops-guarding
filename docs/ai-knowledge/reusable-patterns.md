# Reusable Patterns

## Spec-Driven Delivery Loop

Use when a task has a written spec or plan and quality is more important than speed.

1. Extract task checklist from the plan.
2. Implement one task at a time.
3. Run the smallest relevant validation.
4. Commit the task.
5. Run spec compliance review.
6. Run code quality review.
7. Fix findings only after reproducing them.
8. Run final validation and smoke checks.

Why this works: spec review catches under/over-building; quality review catches runtime and maintainability issues.

## Source Baseline Commit Pattern

Use when a repository has many untracked source files and local runtime state mixed in.

1. Confirm `.gitignore` excludes generated/local state.
2. Run validation before staging.
3. Inspect `git status --short`.
4. Stage only source/config/docs/module/schema paths.
5. Confirm staged paths do not include ignored state.
6. Commit source baseline.
7. Confirm `git status --short` is clean.

Avoid: `git add .` when `.cache/`, `bin/`, `meta/`, `.codex/`, `.omx/`, or IDE state exists.

## Review Finding Fix Pattern

Use when final review reports a bug or risk.

1. Reproduce the exact command failure.
2. Read the failure output completely.
3. Trace the root cause through code paths.
4. Make a minimal fix.
5. Add or strengthen focused regression coverage.
6. Run focused validation.
7. Run project-level validation.
8. Commit and request/re-run final review.

Avoid: changing code based only on a reviewer summary.

## Go Concurrency Fix Pattern

Use when `go test -race` reports shared mutable state.

1. Identify read and write goroutines in the race report.
2. Locate shared fields.
3. Ensure every read and write uses the same lock, or move all writes before goroutines start.
4. Add a focused concurrent test.
5. Run `go test -race -count=1 <package>`.

Avoid: locking only setters while background goroutines keep reading unlocked fields.

## Script Evidence Pattern

Use when reporting completion to a user or AI agent.

- State the claim.
- Name the command that proves it.
- Run the command fresh.
- Read the output and exit code.
- Report the evidence, not assumptions.

Completion claims without fresh command output are not acceptable for this repository.
