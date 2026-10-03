#!/usr/bin/env bash
# test/topology/acl/run.sh — F-acl topology + restart-safety test on the host VPP (af_packet rig), slot N.
#   eval "$(tools/lab env <slot>)"; test/topology/acl/run.sh [go test args]
# Opt-in steps: NGFW_ACL_STATS_GLOBALS=1 (switch the VPP-wide ACL counters on under flock -x on the globals lock, D-082;
# never switched off, V7), NGFW_ACL_SCALE=<rules> (10000 first, then 100000 only in a manager window, D-064).
# Builds the agent, ngfw-agentctl and ngfw-vpp-preflight (apps/agent/bin, git-ignored; /run is noexec) and apps/api/dist,
# then runs the test under the shared lab lock (the test takes it itself as well, D-094: only for the run).
set -euo pipefail
HERE="$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")" && pwd)"
ROOT="$(cd "$HERE/../../.." && pwd)"
export PATH="$PATH:$HOME/go/bin:/usr/local/go/bin"
: "${NGFW_TEST_PREFIX:?eval \"\$(tools/lab env <slot>)\" first}"
[[ "$NGFW_TEST_PREFIX" =~ ^w([0-9]{1,2})$ ]] || { echo "run.sh: NGFW_TEST_PREFIX must be w<N>" >&2; exit 1; }
RUN="/run/ngfw-test/$NGFW_TEST_PREFIX"
[[ -d /run/ngfw-test ]] || install -d -m 0755 /run/ngfw-test
[[ -d "$RUN" ]] || install -d -m 0755 "$RUN"
( cd "$ROOT/apps/agent" && go build -o bin/ngfw-agent ./cmd/ngfw-agent && go build -o bin/ngfw-agentctl ./cmd/ngfw-agentctl \
  && go build -o bin/ngfw-vpp-preflight ./cmd/ngfw-vpp-preflight )
( cd "$ROOT/apps/api" && pnpm build >/dev/null )
export NGFW_ACL_AGENT_BIN="$ROOT/apps/agent/bin/ngfw-agent" NGFW_ACL_AGENTCTL_BIN="$ROOT/apps/agent/bin/ngfw-agentctl" \
  NGFW_ACL_PREFLIGHT_BIN="$ROOT/apps/agent/bin/ngfw-vpp-preflight" NGFW_INTEGRATION=1
cd "$HERE"
exec "$ROOT/tools/lab" lock shared go test -count=1 -v -timeout 40m "$@" .
