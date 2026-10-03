#!/usr/bin/env bash
# Reserved short, host-independent validation lane. Shares heavy slot 3;
# ordinary heavy.sh workers must be capped at 2 before this lane is enabled.
set -euo pipefail
(( $# )) || { echo 'usage: test-fast.sh <finite test/build command> [args...]' >&2; exit 2; }
# Previous namespace flags are refusal conditions, never naming aliases.
previous_prefix=$'\126\122\130'
previous_integration=${previous_prefix}_INTEGRATION
previous_heavy=${previous_prefix}_HEAVY_HELD
[[ ${NGFW_INTEGRATION:-0} != 1 && ${!previous_integration:-0} != 1 ]] || { echo 'fast lane refuses live integration tests' >&2; exit 2; }
[[ -z ${NGFW_HEAVY_HELD:-} && -z ${!previous_heavy:-} ]] || { echo 'fast lane refuses nested heavy execution' >&2; exit 2; }
lock_dir=${NGFW_TEST_LOCK_DIR:-/run/lock}
timeout_seconds=${NGFW_FAST_TIMEOUT_SECONDS:-300}
[[ $timeout_seconds =~ ^[1-9][0-9]*$ ]] || exit 2
export GOTMPDIR=${GOTMPDIR:-/root/.cache/go-tmp}
mkdir -p "$GOTMPDIR"
started=$SECONDS
while :; do
  max=$(cat "$lock_dir/ngfw-heavy-max" 2>/dev/null || true)
  [[ $max == 2 ]] || { echo 'fast lane needs ngfw-heavy-max=2; reservation absent or emergency cap active' >&2; exit 2; }
  available=$(awk '/MemAvailable/{print int($2/1048576)}' /proc/meminfo)
  load=$(awk '{print int($1)}' /proc/loadavg)
  if (( available >= 8 && load < 20 )); then
    exec 8>>"$lock_dir/ngfw-heavy-3.lock"
    if flock -n 8; then
      # Recheck the resource cap after acquiring; never defeat emergency cap=1.
      [[ $(cat "$lock_dir/ngfw-heavy-max") == 2 ]] || exit 2
      export NGFW_HEAVY_HELD=3
      echo "fast: slot 3 after $((SECONDS-started))s; deadline ${timeout_seconds}s" >&2
      exec timeout --signal=TERM --kill-after=10s "${timeout_seconds}s" "$@"
    fi
    exec 8>&-
  fi
  if (( SECONDS - started >= 600 )); then
    echo 'fast: reservation did not become available within 10 minutes; resubmit later' >&2
    exit 75
  fi
  sleep 1
done
