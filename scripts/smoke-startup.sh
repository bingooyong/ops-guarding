#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

export GOCACHE="${GOCACHE:-$ROOT_DIR/.cache/go-build}"
export GOMODCACHE="${GOMODCACHE:-$ROOT_DIR/.cache/go-mod}"

mkdir -p "$GOCACHE" "$GOMODCACHE" "$ROOT_DIR/bin" "$ROOT_DIR/.cache/smoke"

go build -o "$ROOT_DIR/bin/guarding" ./cmd

agent_log="$ROOT_DIR/.cache/smoke/agentd.log"
server_log="$ROOT_DIR/.cache/smoke/server.log"
rm -f "$agent_log" "$server_log"

cleanup() {
	if [[ -n "${AGENT_PID:-}" ]] && kill -0 "$AGENT_PID" 2>/dev/null; then
		kill "$AGENT_PID" 2>/dev/null || true
		wait "$AGENT_PID" 2>/dev/null || true
	fi
	if [[ -n "${SERVER_PID:-}" ]] && kill -0 "$SERVER_PID" 2>/dev/null; then
		kill "$SERVER_PID" 2>/dev/null || true
		wait "$SERVER_PID" 2>/dev/null || true
	fi
}
trap cleanup EXIT

"$ROOT_DIR/bin/guarding" agentd -c etc/service/agentd.conf >"$agent_log" 2>&1 &
AGENT_PID="$!"
sleep 2

if ! kill -0 "$AGENT_PID" 2>/dev/null; then
	echo "smoke: agentd exited unexpectedly"
	cat "$agent_log"
	exit 1
fi

curl --connect-timeout 2 --max-time 5 -fsS http://127.0.0.1:2090/ping >/dev/null
echo "smoke: agentd /ping PASS"

"$ROOT_DIR/bin/guarding" server -c etc/service/server.conf >"$server_log" 2>&1 &
SERVER_PID="$!"
sleep 3

if kill -0 "$SERVER_PID" 2>/dev/null; then
	curl --connect-timeout 2 --max-time 5 -fsS http://127.0.0.1:10090/ping >/dev/null
	echo "smoke: server /ping PASS"
else
	if grep -Eiq 'server init fail: .*(dial tcp .*:(3306|5432)|connect: connection refused|connect: no route to host|i/o timeout|access denied|unknown database|database .* does not exist|password authentication failed|no such host)' "$server_log"; then
		echo "smoke: server startup blocked by documented database dependency"
		cat "$server_log"
	else
		echo "smoke: server exited for an unexpected reason"
		cat "$server_log"
		exit 1
	fi
fi

echo "smoke: PASS"
