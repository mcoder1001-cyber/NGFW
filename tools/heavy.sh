#!/usr/bin/env bash
# Heavy-step semaphore (D-224): at most THREE heavy commands (fewer while /run/lock/ngfw-heavy-max holds 1 or 2, D-228) run at once on this host — go build/test/vet, tsc, vitest,
# vite build, pnpm install, and each CI gate (tools/ci-slot.sh takes one of these slots itself). Takes one of
# /run/lock/ngfw-heavy-{1,2,3}.lock (waits while all three are taken), then execs the command in the current directory;
# the slot is released when the command and every child that inherited the lock fd have exited.
# Usage:  tools/heavy.sh go test ./internal/foo/...      tools/heavy.sh pnpm --filter @ngfw/api exec vitest run x.test.ts
# Only for finite build/test commands — never wrap a server, a daemon or anything left running in the background.
set -euo pipefail
export GOTMPDIR=${GOTMPDIR:-/root/.cache/go-tmp}; mkdir -p "$GOTMPDIR"   # go's build work dirs on disk, not tmpfs /tmp (D-224)
(( $# )) || { echo "usage: tools/heavy.sh <command> [args…]" >&2; exit 2; }
[[ -n ${NGFW_HEAVY_HELD:-} ]] && exec "$@"      # nested call inside a step that already holds a slot
waited=0
while :; do
  max=$(cat /run/lock/ngfw-heavy-max 2>/dev/null || echo 3); [[ $max =~ ^[1-3]$ ]] || max=3   # manager lever (D-228)
  for n in $(seq 1 "$max"); do
    exec 8>>"/run/lock/ngfw-heavy-$n.lock"
    if flock -n 8; then
      export NGFW_HEAVY_HELD=$n
      if (( waited > 0 )); then echo "heavy: slot $n after ${waited}s" >&2; fi
      exec "$@"
    fi
    exec 8>&-
  done
  if (( waited % 300 == 0 )); then echo "heavy: all 3 heavy slots busy, waiting (${waited}s): $*" >&2; fi
  sleep 10; waited=$((waited + 10))
done
