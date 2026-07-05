# AI Agent Rules

## Completion Rules

- Do not claim work is complete without fresh command output.
- Report exact commands and results.
- Keep final reports concise: changed assets, validation evidence, and remaining risks.
- Prefer local repo evidence over memory.

## Editing Rules

- Keep diffs small and scoped to the requested task.
- Do not revert unrelated user changes.
- Do not stage local runtime state or ignored files.
- Use existing project scripts before inventing new validation commands.
- Use `apply_patch` for manual file edits.

## Review Rules

- Treat reviewer findings as hypotheses.
- Reproduce failures before fixing.
- Fix one root cause at a time.
- Re-run the failing command after the fix.
- Re-run project-level validation before reporting success.

## Subagent Rules

- Use subagents for independent implementation, spec review, quality review, or final verification when it improves quality.
- Give each subagent a bounded scope and explicit write permissions.
- Do not trust subagent success reports without checking git diff/status and validation output.

## Git Rules

- Commit task-level changes with focused messages.
- Before source baseline commits, inspect staged file names.
- Never use destructive commands such as `git reset --hard` unless explicitly requested.
- Keep generated and local paths ignored: `.cache/`, `.codex/`, `.omx/`, `.idea/`, `bin/`, `meta/`, `var/`, `logs/`.

## Go-Specific Rules

- Run `go test ./...` through `./scripts/validate.sh`.
- Run `go vet ./...` when code changes affect shared packages.
- Run `go test -race -count=1` for logger or concurrency-sensitive changes.
- Constructors for types containing `sync.Mutex` should return pointers.
- Mutable config read by goroutines must be immutable before start or synchronized.
