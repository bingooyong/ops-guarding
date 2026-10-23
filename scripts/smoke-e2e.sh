#!/usr/bin/env bash
# Startup + task smoke test against a REAL, DISPOSABLE MySQL that already has
# sql/ibex.sql loaded. Starts `guarding server` and `guarding agentd` as real
# processes, creates a task through the HTTP API and waits until it is `done`.
#
# It inserts rows into the target database, so it refuses to run unless
# SMOKE_CONFIRM_DISPOSABLE_DB=1 is set. CI sets it for the throwaway MySQL
# service container; do NOT point it at a shared or production database.
#
# Env:
#   SMOKE_CONFIRM_DISPOSABLE_DB=1   required
#   SMOKE_MYSQL_PASSWORD            required
#   SMOKE_MYSQL_ADDR (127.0.0.1:3306)  SMOKE_MYSQL_USER (root)  SMOKE_MYSQL_DB (ibex)
#   GUARDING_BIN                    prebuilt binary (default: build ./cmd into bin/guarding)
#   SMOKE_TIMEOUT                   seconds to wait for the task (default 90)
# Fixed local ports: server HTTP 10090, server RPC 20090, agentd HTTP 2090.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

if [[ "${SMOKE_CONFIRM_DISPOSABLE_DB:-}" != "1" ]]; then
	echo "smoke-e2e: set SMOKE_CONFIRM_DISPOSABLE_DB=1 to confirm the database is disposable" >&2
	exit 2
fi
: "${SMOKE_MYSQL_PASSWORD:?smoke-e2e: SMOKE_MYSQL_PASSWORD is required}"
MYSQL_ADDR="${SMOKE_MYSQL_ADDR:-127.0.0.1:3306}"
MYSQL_USER="${SMOKE_MYSQL_USER:-root}"
MYSQL_DB="${SMOKE_MYSQL_DB:-ibex}"
TIMEOUT="${SMOKE_TIMEOUT:-90}"
API_USER="ibex"
API_PASS="smoke-$$-secret"
HOST_ID="smoke-agent"
SERVER_URL="http://127.0.0.1:10090"
AGENT_URL="http://127.0.0.1:2090"

if [[ -z "${GUARDING_BIN:-}" ]]; then
	mkdir -p "$ROOT_DIR/bin"
	go build -o "$ROOT_DIR/bin/guarding" ./cmd
	GUARDING_BIN="$ROOT_DIR/bin/guarding"
fi

WORK="$(mktemp -d)"
# The binaries chdir to their own directory (logs, meta, task scripts land there),
# so run a private copy from the temp dir to keep the repo clean.
cp "$GUARDING_BIN" "$WORK/guarding"
SERVER_PID=""
AGENT_PID=""

cleanup() {
	local rc=$?
	for pid in "$AGENT_PID" "$SERVER_PID"; do
		if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
			kill "$pid" 2>/dev/null || true
			wait "$pid" 2>/dev/null || true
		fi
	done
	if ((rc != 0)); then
		echo "smoke-e2e: FAILED (rc=$rc); logs follow" >&2
		for f in server agentd; do
			echo "----- $f.log (tail) -----" >&2
			tail -n 60 "$WORK/$f.log" >&2 || true
		done
	fi
	rm -rf "$WORK"
}
trap cleanup EXIT

# Configs are derived from the shipped samples, so sample drift breaks the smoke.
sed \
	-e "s|^Host = \"0.0.0.0\"|Host = \"127.0.0.1\"|" \
	-e "s|^ibex = \"CHANGE_ME\"|ibex = \"$API_PASS\"|" \
	-e "s|^Listen = \"0.0.0.0:20090\"|Listen = \"127.0.0.1:20090\"|" \
	-e "s|^Address = \"127.0.0.1:3306\"|Address = \"$MYSQL_ADDR\"|" \
	-e "s|^User = \"root\"|User = \"$MYSQL_USER\"|" \
	-e "s|^Password = \"CHANGE_ME\"|Password = \"$SMOKE_MYSQL_PASSWORD\"|" \
	-e "s|^DBName = \"ibex\"|DBName = \"$MYSQL_DB\"|" \
	etc/service/server.conf.example >"$WORK/server.conf"

sed \
	-e "s|^Host = \"0.0.0.0\"|Host = \"127.0.0.1\"|" \
	-e "s|^Host = \"categraf01\"|Host = \"$HOST_ID\"|" \
	-e "s|^MetaDir = .*|MetaDir = \"$WORK/meta\"|" \
	etc/service/agentd.conf >"$WORK/agentd.conf"

grep -q "$API_PASS" "$WORK/server.conf" || { echo "smoke-e2e: server.conf sample drifted (BasicAuth)" >&2; exit 1; }
grep -q "$SMOKE_MYSQL_PASSWORD" "$WORK/server.conf" || { echo "smoke-e2e: server.conf sample drifted (MySQL password)" >&2; exit 1; }
grep -q "\"$HOST_ID\"" "$WORK/agentd.conf" || { echo "smoke-e2e: agentd.conf sample drifted (Heartbeat.Host)" >&2; exit 1; }

wait_ping() { # url name pid
	local i
	for ((i = 0; i < 30; i++)); do
		if ! kill -0 "$3" 2>/dev/null; then
			echo "smoke-e2e: $2 exited during startup" >&2
			return 1
		fi
		if curl --connect-timeout 1 --max-time 2 -fsS "$1/ping" >/dev/null 2>&1; then
			echo "smoke-e2e: $2 /ping PASS"
			return 0
		fi
		sleep 1
	done
	echo "smoke-e2e: $2 did not answer /ping in 30s" >&2
	return 1
}

(cd "$WORK" && exec "$WORK/guarding" server -c "$WORK/server.conf") >"$WORK/server.log" 2>&1 &
SERVER_PID="$!"
wait_ping "$SERVER_URL" server "$SERVER_PID"

(cd "$WORK" && exec "$WORK/guarding" agentd -c "$WORK/agentd.conf") >"$WORK/agentd.log" 2>&1 &
AGENT_PID="$!"
wait_ping "$AGENT_URL" agentd "$AGENT_PID"

api() { # method path [json-body]
	curl --connect-timeout 2 --max-time 10 -fsS -u "$API_USER:$API_PASS" \
		-H 'Content-Type: application/json' -X "$1" ${3:+-d "$3"} "$SERVER_URL/ibex/v1$2"
}
dat() { python3 -c '
import json, sys
env = json.load(sys.stdin)
if env.get("err"):
    sys.exit("api error: %s" % env["err"])
d = env["dat"]
print(d if isinstance(d, str) else json.dumps(d, sort_keys=True))'; }

body="$(python3 - "$(id -un)" "$HOST_ID" <<'PY'
import json, sys
print(json.dumps({
    "title": "smoke", "account": sys.argv[1], "creator": "smoke", "action": "start",
    "batch": 0, "tolerance": 0, "timeout": 60,
    "script": "#!/bin/sh\necho smoke-ok\n", "args": "", "stdin": "",
    "hosts": [sys.argv[2]],
}))
PY
)"

task_id="$(api POST /tasks "$body" | dat)"
echo "smoke-e2e: created task $task_id on host $HOST_ID"

state=""
for ((i = 0; i < TIMEOUT; i++)); do
	state="$(api GET "/task/$task_id/state" | dat)"
	case "$state" in
	done) break ;;
	failed)
		echo "smoke-e2e: task failed: $(api GET "/task/$task_id/result" | dat)" >&2
		exit 1
		;;
	esac
	sleep 1
done
if [[ "$state" != "done" ]]; then
	echo "smoke-e2e: task $task_id not done after ${TIMEOUT}s (state=$state, result=$(api GET "/task/$task_id/result" | dat))" >&2
	exit 1
fi
echo "smoke-e2e: task state PASS (done)"

result="$(api GET "/task/$task_id/result" | dat)"
if [[ "$result" != "{\"success\": [\"$HOST_ID\"]}" ]]; then
	echo "smoke-e2e: unexpected result: $result" >&2
	exit 1
fi

stdout="$(api GET "/task/$task_id/host/$HOST_ID/stdout" | dat)"
if [[ "$stdout" != "smoke-ok" ]]; then
	echo "smoke-e2e: unexpected stdout: '$stdout'" >&2
	exit 1
fi
echo "smoke-e2e: result + stdout PASS"
echo "smoke-e2e: PASS"
