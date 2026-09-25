#!/usr/bin/env bash
# test/topology/unbound-chrony-syslog/run.sh — F-unbound-chrony-syslog end-to-end run on the slot (real agent + API +
# slot-local unbound/chronyd/rsyslogd instances; no VPP object besides the prefixed loopback the document names).
#   eval "$(tools/lab env <slot>)"; test/topology/unbound-chrony-syslog/run.sh [go test args]
# Builds the agent binary (apps/agent/bin, git-ignored) and apps/api/dist, then runs the test under the shared lab lock.
set -euo pipefail
HERE="$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")" && pwd)"
ROOT="$(cd "$HERE/../../.." && pwd)"
export PATH="$PATH:$HOME/go/bin:/usr/local/go/bin"
: "${VRX_TEST_PREFIX:?eval \"\$(tools/lab env <slot>)\" first}"
[[ "$VRX_TEST_PREFIX" =~ ^w([0-9]{1,2})$ ]] || { echo "run.sh: VRX_TEST_PREFIX must be w<N>" >&2; exit 1; }
RUN="/run/vrx-test/$VRX_TEST_PREFIX"
[[ -d /run/vrx-test ]] || install -d -m 0755 /run/vrx-test
[[ -d "$RUN" ]] || install -d -m 0755 "$RUN"
( cd "$ROOT/apps/agent" && go build -o bin/vrx-agent ./cmd/vrx-agent )
( cd "$ROOT/apps/api" && pnpm build >/dev/null )
export VRX_UCS_AGENT_BIN="$ROOT/apps/agent/bin/vrx-agent" VRX_INTEGRATION=1
cd "$HERE"
exec "$ROOT/tools/lab" lock shared go test -count=1 -v -timeout 20m "$@" .
