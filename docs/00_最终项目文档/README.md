# 最终项目文档

## 当前基线

`ops-guarding` 已建立可重复交付基线：

- 源码、配置示例、SQL、README 和项目分析文档已纳入 Git 历史。
- 本地运行态、缓存、构建产物和 IDE 状态已通过 `.gitignore` 排除。
- 标准验证入口为 `./scripts/validate.sh`。
- 启动 smoke 入口为 `./scripts/smoke-startup.sh`。
- AI 长期知识沉淀位于 `docs/ai-knowledge/`。
- 项目本地可复用 Skill 位于 `.codex/skills/`。

## 必读文档

- `README.md`: contributor-facing build, validation, smoke, database, task flow, and troubleshooting instructions.
- `docs/project-analysis.md`: current project analysis.
- `docs/superpowers/specs/2026-07-04-delivery-quality-roadmap-design.md`: delivery quality roadmap spec.
- `docs/superpowers/plans/2026-07-04-engineering-baseline.md`: engineering baseline implementation plan.
- `docs/ai-knowledge/`: reusable AI-agent knowledge.

## 标准验证

Run before claiming completion:

```bash
./scripts/validate.sh
./scripts/smoke-startup.sh
git status --short
```

For logger or concurrency-sensitive changes, also run:

```bash
go test -race -count=1 ./pkg/appframe/logger
go vet ./...
gofmt -l $(git ls-files '*.go')
```

## 风险边界

- Database-backed server behavior requires valid local MySQL configuration.
- Task-chain functional regression coverage is still deferred.
- Production deployment, secrets handling, metrics output, and HA design require later hardening.

## 后续维护规则

- Keep validation scripts executable and documented.
- Update `docs/ai-knowledge/` when new failure modes or delivery patterns are discovered.
- Add or update project-local skills only when the workflow is reusable.
