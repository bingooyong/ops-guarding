---
name: ai-delivery-quality-gates
description: Execute spec-driven AI coding delivery with implementation, spec compliance review, code quality review, final verification, and source baseline control. Use when a user asks to continue a plan, fix all review issues, or complete high-quality AI coding delivery.
---

# AI Delivery Quality Gates

## Skill 名称

`ai-delivery-quality-gates`

## 适用场景

Use this skill for multi-step AI coding delivery where quality matters more than speed: spec generation, plan execution, subagent reviews, bug fixing, and final evidence reporting.

## 输入参数

- `spec_path`: optional requirements/spec document.
- `plan_path`: implementation plan with task checklist.
- `task_scope`: current task or `all`.
- `review_mode`: `spec`, `quality`, or `final`; default `final` after all tasks.
- `commit_policy`: whether each task should create a small commit; default yes for this repo.

## 输出结果

- Task-by-task completion evidence.
- Spec compliance result for each task.
- Code quality findings and fixes.
- Final validation evidence and clean git status.
- Commit list for completed delivery.

## 执行步骤

1. Read the current spec and plan; extract concrete files, commands, and expected outputs.
2. Create or update a todo list with one active item at a time.
3. For each task:
   - Implement only the scoped change.
   - Run the task's smallest validation command.
   - Commit the task with a focused message.
   - Run a spec compliance review against the plan.
   - Run a code quality review after spec compliance passes.
   - Fix review findings by reproducing the issue first.
4. Before source-baseline commits, inspect `git status --short` and ensure ignored/local paths are absent.
5. Stage only planned source paths. For this repo, the baseline whitelist was:
   ```bash
   git add AGENTS.md cmd docs/project-analysis.md etc go.mod go.sum pkg sql
   ```
6. Run final commands:
   ```bash
   ./scripts/validate.sh
   ./scripts/smoke-startup.sh
   git status --short
   git log --oneline -n 8
   ```
7. Report only verified results, remaining risks, and changed assets.

## 依赖工具

- Git.
- Project validation scripts.
- Native subagents for independent implementation/review slices when available.
- `rg`, `git diff`, `git show`, and targeted Go commands.

## 使用示例

```text
Use $ai-delivery-quality-gates with plan_path=docs/superpowers/plans/2026-07-04-engineering-baseline.md and task_scope=all.
```

## 注意事项

- Never skip the spec compliance review or the code quality review for task commits.
- Treat reviewer output as hypotheses to verify. Reproduce failures before patching.
- Do not stage ignored local state, generated binaries, runtime data, or editor metadata.
- Keep commits small enough that review failures can be isolated and fixed.
- If a quality issue is caused by a later planned baseline commit, complete the dependency and re-review rather than weakening documentation.

## 变更记录

- 2026-07-05: Initial project-local skill created from the subagent-driven delivery and final-review loop used in this session.
