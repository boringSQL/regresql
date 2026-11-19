#!/usr/bin/env bash
#
# Build pg_regresql in throwaway postgres containers and run the extended
# statistics restore tests (round trip, invalid input, extension lifecycle).
#
#   pg_ext/test/ext_stats/run.sh              # 16 and 17
#   VERSIONS="16" pg_ext/test/ext_stats/run.sh
#   KEEP=1 pg_ext/test/ext_stats/run.sh       # keep the containers
#
# Needs docker and network access (image pull, apt). Exits non-zero on failure.

set -uo pipefail

EXT=$(cd "$(dirname "$0")/../.." && pwd)
HERE="$EXT/test/ext_stats"
VERSIONS=${VERSIONS:-"16 17"}
KEEP=${KEEP:-0}
LOGDIR=$(mktemp -d "${TMPDIR:-/tmp}/regresql-ext-stats.XXXXXX")
FAILED=0

tag() { case $1 in 16) echo 16.15 ;; *) echo "$1" ;; esac; }
fail() { printf '  FAIL  %s\n' "$*"; FAILED=1; }
pass() { printf '  ok    %s\n' "$*"; }

for v in $VERSIONS; do
	c=regresql-ext-build$v
	printf '== PostgreSQL %s (%s)\n' "$v" "$c"
	docker rm -f "$c" >/dev/null 2>&1
	docker run -d --name "$c" -e POSTGRES_PASSWORD=pw -p "54$((30 + v)):5432" "postgres:$(tag "$v")" >/dev/null || { fail "start $c"; continue; }

	docker exec "$c" bash -c "apt-get update -qq && apt-get install -y -qq build-essential postgresql-server-dev-$v" >"$LOGDIR/apt$v.log" 2>&1 \
		|| { fail "apt install"; continue; }

	docker exec "$c" rm -rf /build
	docker exec "$c" mkdir /build
	(cd "$EXT" && tar --no-xattrs --exclude='*.zip' --exclude=test --exclude=prototypes --exclude='*.md' \
		--exclude='*.o' --exclude='*.so' --exclude='*.bc' -cf - .) | docker exec -i "$c" tar -xf - -C /build

	docker exec "$c" bash -c "cd /build && make USE_PGXS=1 && make USE_PGXS=1 install" >"$LOGDIR/build$v.log" 2>&1 \
		|| { fail "build (see $LOGDIR/build$v.log)"; continue; }
	if grep 'warning:' "$LOGDIR/build$v.log" | grep -v 'overriding recipe\|ignoring old recipe' | grep -q .; then
		fail "compiler warnings:"; grep 'warning:' "$LOGDIR/build$v.log" | grep -v 'overriding recipe\|ignoring old recipe'
	else
		pass "build, no compiler warnings"
	fi

	# apt may upgrade the server packages under the running postmaster
	docker restart "$c" >/dev/null

	# wait for the server (the image restarts once after initdb)
	for _ in $(seq 1 60); do
		docker exec "$c" pg_isready -U postgres -q 2>/dev/null && sleep 2 && docker exec "$c" pg_isready -U postgres -q 2>/dev/null && break
		sleep 1
	done

	printf '  server: %s\n' "$(docker exec "$c" psql -X -At -U postgres -c 'select version()' | cut -c1-60)"
	psqlc() { docker exec -i "$c" psql -X -q -U postgres -v ON_ERROR_STOP=1 "$@"; }
	for f in 00_setup 10_roundtrip 20_compare 30_invalid 40_lifecycle; do
		out="$LOGDIR/$v-$f.log"
		psqlc <"$HERE/$f.sql" >"$out" 2>&1
		rc=$?
		cat "$out" | sed 's/^/        /' | grep -E 'PASS|NOTE|FAIL|ERROR|restored values|WARNING' | head -80
		if [ $rc -ne 0 ] || grep -q 'FAIL' "$out"; then fail "$f"; fi
		case $f in
		10_roundtrip|20_compare|00_setup|40_lifecycle)
			if grep -q WARNING "$out"; then fail "$f emitted WARNING"; else pass "$f: no WARNING"; fi ;;
		30_invalid)
			n=$(grep -c WARNING "$out"); pass "$f: $n WARNINGs, no crash" ;;
		esac
	done
	docker exec "$c" pg_isready -U postgres -q || fail "server not running at the end"
	[ "$KEEP" = 1 ] || docker rm -f "$c" >/dev/null 2>&1
done

printf 'logs: %s\n' "$LOGDIR"
exit $FAILED
