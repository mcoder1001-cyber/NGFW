#!/usr/bin/env bash
# test/topology/det44/run.sh — F-det44-map-dslite-cnat-host: DET44 / MAP-E / DS-Lite / CNAT topology + packet +
# restart-safety test on the host VPP (af_packet rig), agent-only (gRPC; no API stack).
#   eval "$(tools/lab env <slot>)"; NGFW_FDET44_DET44_HOST=1 test/topology/det44/run.sh [go test args]
# Builds the agent and the V19 preflight (apps/agent/bin, git-ignored; /run is noexec), runs the preflight (D-095: it
# must exit 0 before any packet), then the test under the shared lab lock (the test takes it itself as well and the
# exclusive globals lock for its global fixtures, D-167). NGFW_FDET44_DET44_HOST=1 opts in to the det44 plugin enable
# (irreversible until a VPP restart, V9); NGFW_EVIDENCE_TXT=<file> tees the output there (D-175: evidence as .txt).
set -euo pipefail
HERE="$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")" && pwd)"
ROOT="$(cd "$HERE/../../.." && pwd)"
export PATH="$PATH:$HOME/go/bin:/usr/local/go/bin"
: "${NGFW_TEST_PREFIX:?eval \"\$(tools/lab env <slot>)\" first}"
[[ "$NGFW_TEST_PREFIX" =~ ^w([0-9]{1,2})$ ]] || { echo "run.sh: NGFW_TEST_PREFIX must be w<N>" >&2; exit 1; }
RUN="/run/ngfw-test/$NGFW_TEST_PREFIX"
[[ -d /run/ngfw-test ]] || install -d -m 0755 /run/ngfw-test
[[ -d "$RUN" ]] || install -d -m 0755 "$RUN"
( cd "$ROOT/apps/agent" && go build -o bin/ngfw-agent ./cmd/ngfw-agent && go build -o bin/ngfw-vpp-preflight ./cmd/ngfw-vpp-preflight )
"$ROOT/apps/agent/bin/ngfw-vpp-preflight"
export NGFW_NAT_AGENT_BIN="$ROOT/apps/agent/bin/ngfw-agent" NGFW_PREFLIGHT_BIN="$ROOT/apps/agent/bin/ngfw-vpp-preflight" NGFW_INTEGRATION=1
cd "$HERE"
if [[ -n "${NGFW_EVIDENCE_TXT:-}" ]]; then
  { date; systemctl show vpp -p NRestarts; } | tee "$NGFW_EVIDENCE_TXT"
  "$ROOT/tools/lab" lock shared go test -count=1 -v -timeout 20m "$@" . 2>&1 | tee -a "$NGFW_EVIDENCE_TXT"
  { date; systemctl show vpp -p NRestarts; } | tee -a "$NGFW_EVIDENCE_TXT"
  exit "${PIPESTATUS[0]}"
fi
exec "$ROOT/tools/lab" lock shared go test -count=1 -v -timeout 20m "$@" .
