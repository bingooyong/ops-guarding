# Backend Development Guidelines

> Best practices for backend development in this project.

---

## Overview

These guides describe the conventions actually used in the ops-guarding Go
module: the `guarding server` control plane (gin HTTP + net/rpc + GORM) and
the database-free `guarding agentd`. Each rule cites real files under `pkg/`,
`sql/`, `scripts/`, `skills/`, or `docs/ai-knowledge/`. Known gaps are listed
explicitly so they are not copied as patterns.

---

## Guidelines Index

| Guide | Description | Status |
|-------|-------------|--------|
| [Directory Structure](./directory-structure.md) | cmd / pkg layout, layering, naming | Documented |
| [Database Guidelines](./database-guidelines.md) | GORM, sql/ibex.sql, sharded task_host_N, transactions | Documented |
| [Error Handling](./error-handling.md) | errorx panics, {dat,err} envelope, RPC resp.Message | Documented |
| [Quality Guidelines](./quality-guidelines.md) | validate.sh, vet/gofmt/race gates, tests, review | Documented |
| [Logging Guidelines](./logging-guidelines.md) | appframe/logger (server), std log prefixes (agentd) | Documented |

---

## How to Maintain These Guidelines

- Update the matching guide when a task establishes a new repository-wide
  convention (use the `trellis-update-spec` skill at the end of the task).
- Move items out of the "Known Gaps" sections once the gap is fixed.
- Keep every rule backed by a real file path; remove rules whose code is deleted.

---

**Language**: All documentation should be written in **English**.
