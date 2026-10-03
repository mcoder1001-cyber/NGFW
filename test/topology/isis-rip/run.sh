#!/usr/bin/env bash
# test/topology/isis-rip/run.sh — F-isis-rip host tests (row F-isis-rip-host, RV-A R7/R4/R1 owed lists).
#
#   eval "$(tools/lab env <slot>)"; test/topology/isis-rip/run.sh <test> [extra go test -test.* flags]
#     live-isis  TestISISHostLive  frrtest isisd: golden accepted, DryRun empty, adjacency, parser on real JSON, poller, removal
#     live-rip   TestRIPHostLive   frrtest ripd: golden accepted, DryRun empty, 20 prefixes, withdrawal, removal
#     topology   TestISISRIPTopologyOnHost  the in-process agent + linux-cp pairs on the slot's af_packet rig, two FRR peers:
#                RIP routes in the VPP FIB (VRX_ISISRIP_FIB=root, default), IS-IS without the OSI punt, restart, rollback
#
# The test files live in test/topology/isis-rip/agent-overlay/ (this row's file fence) and are compiled into their target
# packages under apps/agent with `go test -overlay` (they need the agent-internal frrtest harness, the IS-IS parser and the
# P12/F-ospf-host topology helpers, which a module under test/ cannot import). Nothing under apps/ is written: the overlay
# maps a virtual file name in the package directory onto the file here.
#
# The compile is a heavy step (tools/heavy.sh, D-224); the test binary then runs outside the semaphore so no FRR daemon or
# agent child inherits the heavy-slot fd. Each test takes the shared lab lock itself (vpptest.LockLab, D-094) and one test
# runs at a time (D-087). NRestarts is printed before and after (D-175).
set -euo pipefail
HERE="$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")" && pwd)"
ROOT="$(cd "$HERE/../../.." && pwd)"
export PATH="$PATH:$HOME/go/bin:/usr/local/go/bin"
: "${VRX_TEST_PREFIX:?eval \"\$(tools/lab env <slot>)\" first}"
[[ "$VRX_TEST_PREFIX" =~ ^w([0-9]{1,2})$ ]] || { echo "run.sh: VRX_TEST_PREFIX must be w<N>" >&2; exit 1; }
which=${1:?usage: run.sh live-isis|live-rip|topology [go test flags]}; shift
case $which in
  live-isis) pkg=internal/renderers/frr/isis; virt=isis_host_integration_test.go; src=isis_host_integration_test.go; run='^TestISISHostLive$' ;;
  live-rip)  pkg=internal/renderers/frr/rip;  virt=rip_host_integration_test.go;  src=rip_host_integration_test.go;  run='^TestRIPHostLive$' ;;
  topology)  pkg=internal/agent;              virt=isisrip_topology_integration_test.go; src=isisrip_topology_integration_test.go; run='^TestISISRIPTopologyOnHost$'
             export VRX_ISISRIP_TOPOLOGY=1 ;;
  *) echo "run.sh: unknown test '$which' (live-isis|live-rip|topology)" >&2; exit 2 ;;
esac
[[ -d /run/vrx-test ]] || install -d -m 0755 /run/vrx-test
[[ -d "/run/vrx-test/$VRX_TEST_PREFIX" ]] || install -d -m 0755 "/run/vrx-test/$VRX_TEST_PREFIX"
BIN="$HERE/.bin"; mkdir -p "$BIN"
overlay="$BIN/overlay-$which.json"
target="$ROOT/apps/agent/$pkg/$virt"
[[ ! -e $target ]] || { echo "run.sh: $target exists on disk — the overlay would shadow it; refusing" >&2; exit 1; }
printf '{"Replace":{"%s":"%s"}}\n' "$target" "$HERE/agent-overlay/$src" > "$overlay"
testbin="$BIN/$which.test"
echo "run.sh: $(date +%FT%T) compile $pkg + overlay $src (tools/heavy.sh)"
( cd "$ROOT/apps/agent" && "$ROOT/tools/heavy.sh" go test -c -overlay "$overlay" -o "$testbin" "./$pkg" )
if [[ $which == topology ]]; then   # the V19 preflight binary (the test runs it before any packet crosses the rig)
  ( cd "$ROOT/apps/agent" && "$ROOT/tools/heavy.sh" go build -o "$BIN/vrx-vpp-preflight" ./cmd/vrx-vpp-preflight )
  export VRX_PREFLIGHT_BIN="$BIN/vrx-vpp-preflight"
fi
# short temp paths (agent socket, staging dirs) under /tmp/g-<prefix>; every user removes its own files, the dir goes after
export TMPDIR="/tmp/g-$VRX_TEST_PREFIX"; mkdir -p "$TMPDIR"
echo "run.sh: $(date +%FT%T) VPP $(systemctl show vpp -p NRestarts) before $which"
set +e
( cd "$ROOT/apps/agent/$pkg" && VRX_INTEGRATION=1 "$testbin" -test.run "$run" -test.v -test.count=1 -test.timeout 25m "$@" )
rc=$?
set -e
echo "run.sh: $(date +%FT%T) VPP $(systemctl show vpp -p NRestarts) after $which, exit $rc"
rm -f "$testbin" "$BIN/vrx-vpp-preflight"
rmdir "$TMPDIR" 2>/dev/null || echo "run.sh: $TMPDIR not empty: $(ls -A "$TMPDIR" | head -5 | tr '\n' ' ')"
exit $rc
