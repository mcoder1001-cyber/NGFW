#!/usr/bin/env bash
# test/topology/vlan-qinq/run.sh — F-vlan-qinq topology + restart-safety test on the host VPP (af_packet rig).
#   eval "$(tools/lab env <slot>)"; test/topology/vlan-qinq/run.sh [go test args]
# Builds the agent binary (apps/agent/bin, git-ignored; /run is noexec) and apps/api/dist, then runs the test under
# the shared lab lock (the test takes it itself as well, D-094: only for the run). With NGFW_QINQ_SHOTS set it also
# builds apps/web/dist for the screenshot run (TestVlanQinqScreenshots).
set -euo pipefail
HERE="$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")" && pwd)"
ROOT="$(cd "$HERE/../../.." && pwd)"
export PATH="$PATH:$HOME/go/bin:/usr/local/go/bin"
: "${NGFW_TEST_PREFIX:?eval \"\$(tools/lab env <slot>)\" first}"
[[ "$NGFW_TEST_PREFIX" =~ ^w([0-9]{1,2})$ ]] || { echo "run.sh: NGFW_TEST_PREFIX must be w<N>" >&2; exit 1; }
# the slot run dir stays 0755 and is never re-moded (D-106/D-107): create it only when missing
RUN="/run/ngfw-test/$NGFW_TEST_PREFIX"
[[ -d /run/ngfw-test ]] || install -d -m 0755 /run/ngfw-test
[[ -d "$RUN" ]] || install -d -m 0755 "$RUN"
( cd "$ROOT/apps/agent" && go build -o bin/ngfw-agent ./cmd/ngfw-agent )
( cd "$ROOT/apps/api" && pnpm build >/dev/null )
if [[ -n "${NGFW_QINQ_SHOTS:-}" ]]; then ( cd "$ROOT/apps/web" && pnpm build >/dev/null ); fi
export NGFW_QINQ_AGENT_BIN="$ROOT/apps/agent/bin/ngfw-agent" NGFW_INTEGRATION=1
cd "$HERE"
exec "$ROOT/tools/lab" lock shared go test -count=1 -v -timeout 15m "$@" .
