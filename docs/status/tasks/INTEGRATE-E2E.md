# INTEGRATE-E2E — freeze campaign

The freeze runner assembles existing cross-component checks and records exact
source, private logs, failures and explicit unexecuted live cases. Offline success
cannot certify release acceptance. Independent process review found stale-result
and interruption hazards; reporting now invalidates previous outcomes before
preflight, persists atomically and reaps the owned child process group.

## Actual verification, 2026-10-05

- `python3 -B -m unittest discover -s test/acceptance/freeze`: 5 checks PASS,
  including stale-success invalidation and owned interruption cleanup.
- `GOMAXPROCS=2 GOFLAGS=-p=2 python3 -B test/acceptance/freeze/run.py --run-offline
  --output .scratch/freeze-offline-final`: all four groups PASS. Contract builds,
  reachability regressions, 46 commit-engine/service checks and 11 authentication
  checks. This is fake-agent/offline regression, not real appliance acceptance.
- Isolated real VPP, slot31, `go -C test/integration/smoke test -json -count=1
  -timeout 5m ./...`: two tests PASS, zero SKIP/FAIL. Ping traversed af_packet;
  interface receive counters increased (LAN 2→10, WAN 0→8). Disposable VPP
  stopped; no w31 namespace or shared interface remained. Shared VPP observed
  MainPID1014/NRestarts0 afterward. This does not exercise product API/browser.

The first full quick attempt hit schema/UI timeout and scale checks under load45
while other gates ran. That attempt was stopped with failure logs retained;
no green result is claimed. The final unchanged complete local/hosted gate and
current-main integration remain required before merge.

## Acceptance boundary

Real wave B/C product packet chains, browser en/fa transactions, appliance
installation/boot and two-node HA remain NOT RUN for this campaign. Track the
exact next commands in the central deferred acceptance ledger. The owner's
AGENTS.md permits deferring laboratory-only acceptance; code gaps and failed
mandatory quick checks remain blockers. No shared-VPP restart was attempted.

P11-pkg closes only as superseded by DEC-ipsec-route-based. Native IKEv2 packet
acceptance belongs to P11-host; no obsolete strongSwan package is certified.
