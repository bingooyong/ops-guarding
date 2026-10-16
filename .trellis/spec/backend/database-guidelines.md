# Database Guidelines

> ORM, schema, query, and transaction conventions for the server side.

---

## Overview

- **ORM: GORM v2** (`gorm.io/gorm v1.25.9`, drivers `gorm.io/driver/mysql`
  and `gorm.io/driver/postgres`). There is no xorm in this repo.
- Only `guarding server` touches the database. agentd persists its local
  state as plain files under `MetaDir` (`meta/<task id>/...`), never via GORM.
- Connection setup: `pkg/appframe/storage.InitDB(storage.Config{...})` builds a
  DSN from `[MySQL]` / `[Postgres]` config and calls `ormx.New`, which sets
  `NamingStrategy{TablePrefix, SingularTable: true}`, `db.Debug()` when
  `Gorm.Debug = true`, and pool limits (`MaxIdleConns`, `MaxOpenConns`,
  `MaxLifetime` seconds). The result is the global `storage.DB`.
- Models access it only through `models.DB()` (`pkg/models/models.go`).
- Schema source of truth is the hand-written **MySQL** script `sql/ibex.sql`
  (`drop database if exists ibex; create database ibex;`, InnoDB, utf8mb4).

---

## Schema and Migrations

- There is **no migration framework and no AutoMigrate call**. The
  `Gorm.EnableAutoMigrate` field exists in `storage.Gorm` but is never read.
  Schema changes are made by editing `sql/ibex.sql` and applying it manually.
- Postgres is accepted by `ormx.New`, but no Postgres DDL exists in the repo;
  treat Postgres as untested.
- Tables:
  - `task_meta` — task definition (auto-increment `id`, `created` defaults to `CURRENT_TIMESTAMP`).
  - `task_action` — current action per task: `start|cancel|kill|pause`. A row's
    absence means the task is **done** (see `taskGet`, `taskGets`, `doneIds`).
  - `task_scheduler` — which server instance (`ip:rpcport`) owns a task; `''` = orphan.
  - `task_scheduler_health` — scheduler heartbeat clock; dead after 10s (`DeadTaskSchedulers`).
  - `task_host_doing` — hosts currently assigned to agents (`action` start/kill, `clock`).
  - `task_host_0` … `task_host_99` — **100 sharded tables** of per-host status
    and stdout/stderr, each with `ii` auto-increment PK and `UNIQUE (id, host)`.
- If you add a column to the host table you must add it to all 100
  `task_host_N` definitions in `sql/ibex.sql`.

---

## Sharded `task_host_N` Access

The shard is chosen by `tht(id)` in `pkg/models/models.go`:

```go
func tht(id int64) string { return fmt.Sprintf("task_host_%d", id%100) }
```

Rules:

- Always query host rows via `DB().Table(tht(id))` (or `tx.Table(tht(id))`),
  never via `DB().Model(&TaskHost{})` — `TaskHost` has no `TableName()`.
- Always filter by `id` as well as the shard (`Where("id=? ...", id)`).
- Preserve submission order with `.Order("ii")` when listing hosts
  (`TaskHostGets`, `WaitingHostList`, `(*TaskMeta).Hosts`).
- `tht` ignores `TablePrefix`; table names are hard-coded, so `TablePrefix`
  must stay `""` in practice.

---

## Query Patterns

- Models with fixed tables declare `TableName()` returning the literal name
  (`TaskMeta`, `TaskAction`, `TaskScheduler`, `TaskSchedulerHealth`, `TaskHostDoing`).
- Lookup functions take a raw `where string, args ...interface{}` and always
  use placeholders: `TaskMetaGet("id = ?", id)`, `DoingHostCount("id=? and host=?", ...)`.
- "Not found" is `(nil, nil)`, not an error:
  - `Find(&slice)` + `len == 0` → `nil, nil` (`TaskMetaGet`, `TaskHostGet`).
  - `First(&obj)` + `errors.Is(err, gorm.ErrRecordNotFound)` → `nil, nil` (`TaskActionGet`).
  Callers must nil-check the result.
- Counts/existence go through `models.Count(tx)` / `models.Exists(tx)`.
- Single-column lists use `Pluck` (`TaskActionExistsIds`, `LongTaskIds`, `HostStrs`).
- Partial reads use `Select(...)` (`TaskHostStatus`, `Stdouts`, `Stderrs`).
- Updates use `Update("col", v)` or `Updates(map[string]interface{}{...})`;
  inserts into sharded tables use `Create(map[string]interface{}{...})`.
- Compare-and-swap ownership uses `RowsAffected`:
  `TakeOverTask(id, pre, current)` updates `scheduler` only `where scheduler = pre`.
- Read-through cache: `TaskMetaGetByID` checks `cache.Get("/cache/task/meta/<id>")`
  (in-memory cache initialised with `cache.InitMemoryCache(time.Hour)`) before hitting DB.
  `task_host_doing` is snapshotted every 400ms into `models.SetDoingCache`
  (`timer/timer_host_doing.go`) and read by `rpc.Report` without DB access.

---

## Transactions

Multi-table writes use `DB().Transaction(func(tx *gorm.DB) error { ... })`
and return the first error: `(*TaskMeta).Save` (meta + scheduler + action +
host rows), `RunWaitingHosts`, `KillTask`, `CleanDoneTask`.

Inside the closure, every statement must use `tx`, not `DB()`.

---

## Naming Conventions

- Tables: singular snake_case (`task_meta`, `task_host_doing`), matching
  `SingularTable: true`.
- Columns: lowercase single words (`id`, `host`, `status`, `clock`, `action`, `scheduler`).
- Timestamps: `task_meta.created` is a SQL `timestamp` (read-only in Go via
  `gorm:"->"`); everything else stores Unix seconds in `bigint clock`.
- Status strings are bare literals: host `waiting|running|success|failed|timeout|killing|killed|killfailed|cancelled|ignored`.

---

## Common Mistakes / Known Issues

- `MarkDoneStatus` (`pkg/models/task_host.go`) opens a transaction but runs the
  `task_host_N` update via `DB()` instead of `tx`, so it is not atomic with the
  `task_host_doing` delete. Don't copy this; use `tx` in new code.
- `TaskMetaTotal` / `TaskMetaGets` build `created > '<formatted time>'` by
  string concatenation. Use `Where("created > ?", before)` in new queries.
- No tests run against a database; `docs/ai-knowledge/task-chain-regression-tests.md`
  requires a disposable DB (never `etc/service/server.conf`'s database) when adding them.
- Server startup fails without a reachable MySQL and a local
  `etc/service/server.conf` (gitignored; copy from `server.conf.example`).
