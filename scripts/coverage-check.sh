#!/usr/bin/env bash
# Coverage gate: total statement coverage (all packages, cross-package via
# -coverpkg=./...) must not fall below the floor in .github/coverage-floor.txt.
#
#   scripts/coverage-check.sh            run tests, print per-package table, enforce floor
#   scripts/coverage-check.sh --update   raise the floor to (measured - 1.0); never lowers it
#   scripts/coverage-check.sh --update --force   set the floor even if lower (needs review)
#
# The floor is a ratchet: it is only edited by hand/--update in a reviewed commit,
# CI never changes it. Env: COVERAGE_PROFILE (default .cache/coverage.out),
# COVERAGE_FLOOR_FILE (default .github/coverage-floor.txt).
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

FLOOR_FILE="${COVERAGE_FLOOR_FILE:-.github/coverage-floor.txt}"
PROFILE="${COVERAGE_PROFILE:-.cache/coverage.out}"
MARGIN="1.0"

update=0
force=0
for arg in "$@"; do
	case "$arg" in
	--update) update=1 ;;
	--force) force=1 ;;
	-h | --help)
		sed -n '2,11p' "$0"
		exit 0
		;;
	*)
		echo "coverage: unknown argument: $arg" >&2
		exit 2
		;;
	esac
done

mkdir -p "$(dirname "$PROFILE")"

echo "coverage: go test -coverpkg=./... ./..."
go test -count=1 -coverpkg=./... -coverprofile="$PROFILE" ./... >/dev/null

total="$(go tool cover -func="$PROFILE" | awk '/^total:/ {gsub("%", "", $NF); print $NF}')"
if [[ -z "$total" ]]; then
	echo "coverage: could not read total from $PROFILE" >&2
	exit 1
fi

# Per-package table. With -coverpkg every test binary writes the same blocks;
# a block counts as covered if any binary hit it.
table="$(awk -F'[: ]' '
	NR == 1 { next }
	{
		key = $1 ":" $2; n = $3; c = $4
		split($1, parts, "/"); pkg = ""
		for (i = 1; i < length(parts); i++) pkg = pkg (i > 1 ? "/" : "") parts[i]
		stmts[key] = n; pk[key] = pkg
		if (c > 0) hit[key] = 1
	}
	END {
		for (k in stmts) { tot[pk[k]] += stmts[k]; if (k in hit) cov[pk[k]] += stmts[k] }
		for (p in tot) if (tot[p] > 0) { p0 = p; sub(/^[^\/]*\//, "", p0); printf "%-34s %6.1f%%  %5d/%-5d\n", p0, 100 * cov[p] / tot[p], cov[p], tot[p] }
	}' "$PROFILE" | sort)"

echo
echo "coverage by package (statements covered/total):"
echo "$table"
echo
echo "coverage: total = ${total}%"

if [[ -n "${GITHUB_STEP_SUMMARY:-}" ]]; then
	{
		echo "### Coverage (go test -coverpkg=./...)"
		echo
		echo "Total: **${total}%**, floor: **$(cat "$FLOOR_FILE" 2>/dev/null || echo n/a)%**"
		echo
		echo '```'
		echo "$table"
		echo '```'
	} >>"$GITHUB_STEP_SUMMARY"
fi

ge() { awk -v a="$1" -v b="$2" 'BEGIN { exit !(a + 0 >= b + 0) }'; }

if ((update)); then
	new="$(awk -v t="$total" -v m="$MARGIN" 'BEGIN { v = int((t - m) * 10) / 10; if (v < 0) v = 0; printf "%.1f", v }')"
	old="$(cat "$FLOOR_FILE" 2>/dev/null || echo 0)"
	if ge "$old" "$new" && ((force == 0)); then
		echo "coverage: floor stays ${old}% (measured-${MARGIN} = ${new}% would not raise it)"
		exit 0
	fi
	echo "$new" >"$FLOOR_FILE"
	echo "coverage: floor ${old}% -> ${new}% in $FLOOR_FILE (commit it)"
	exit 0
fi

if [[ ! -f "$FLOOR_FILE" ]]; then
	echo "coverage: $FLOOR_FILE missing; run scripts/coverage-check.sh --update" >&2
	exit 1
fi
floor="$(tr -d '[:space:]' <"$FLOOR_FILE")"

if ge "$total" "$floor"; then
	echo "coverage: PASS (${total}% >= floor ${floor}%)"
else
	echo "coverage: FAIL (${total}% < floor ${floor}%). Add tests; do not lower the floor to make CI pass." >&2
	exit 1
fi
