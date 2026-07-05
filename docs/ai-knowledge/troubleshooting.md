# Troubleshooting

## `logger.Errorf` Vet Failure

### Symptom

Validation failed with a logging call that passed arguments without a format directive.

### Cause

`logger.Errorf("LongTaskIds:", err)` supplied `err` but the format string had no `%v`.

### Fix

Use:

```go
logger.Errorf("LongTaskIds: %v", err)
```

### Prevention

Run `./scripts/validate.sh` and treat vet-style formatting issues as real bugs.

## Syslog Test Fails On Host Without Syslog

### Symptom

`TestSysLogBackend` fails on machines where syslog delivery is unavailable.

### Cause

The test assumed host syslog support.

### Fix

Skip when `NewSyslogBackend` returns an environment error:

```go
t.Skipf("syslog backend unavailable on this host: %v", err)
```

### Prevention

Environment-sensitive tests should skip with context when the dependency is unavailable.

## File Logger Data Race

### Symptom

`go test -race ./pkg/appframe/logger` reports races between `flushDaemon`/`rotateByHourDaemon` and configuration setters.

### Cause

`NewFileBackend` started goroutines before later config setters wrote fields such as `flushInterval`, `rotateByHour`, and `keepHours`. Reads and writes did not share synchronization.

### Fix

Guard mutable `FileBackend` configuration fields with `FileBackend.mu`, including daemon reads and setter writes. Add a concurrent configuration test and run:

```bash
go test -race -count=1 ./pkg/appframe/logger
```

### Prevention

Do not start goroutines that read mutable config until config is immutable, or protect every read/write path with the same lock.

## `go vet` Copied Mutex

### Symptom

`go vet ./...` reports `return copies lock value` for `NewLoggerFromConfig`.

### Cause

The function returned `Logger` by value while `Logger` contains `sync.Mutex`.

### Fix

Return `*Logger` and update callers/tests to use the pointer.

### Prevention

Constructors for structs containing locks should return pointers.

## Smoke Startup Fails

### Symptom

`scripts/smoke-startup.sh` fails before `smoke: PASS`.

### Likely Causes

- Port `2090` or `10090` already in use.
- MySQL is unavailable or credentials in `etc/service/server.conf` are invalid.
- The binary failed to build.

### Fix

1. Run `./scripts/validate.sh`.
2. Check whether ports are occupied.
3. Verify database settings or bootstrap with `mysql -u root -p < sql/ibex.sql`.
4. Re-run `./scripts/smoke-startup.sh`.

### Prevention

Keep smoke checks local, time-bounded, and explicit about expected database blockers.
