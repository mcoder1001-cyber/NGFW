# P05 verify — fix round 2 (N1, N2 + CI)

Verifier: independent verify agent · branch `task/P05` @ `24e4cd7` · diff `40c1b43..HEAD` (fix commits `21fb537`,
`7135f5c`) · scope as the re-review asked: N1, N2 and a CI run · run directly on the host, slot 7 (`w7`, tables
7000–7999) · 2026-09-24. VPP `NRestarts=2` before and after every host run. I did not restart VPP. My probe tests were
deleted, and the tree is clean apart from this file.

## Verdicts

| finding | verdict | evidence |
|---|---|---|
| **N1** VPP-generated FIB entries treated as a foreign owner | **FIXED** | host reproduction below; source list checked against the live `show fib source` |
| **N2** an unrevertable owed revert wedges Apply | **FIXED** | fake probes plus the author's regression tests; see the note below the N2 evidence |
| CI | **PASSED** | `tools/ci.sh --base main`: CI GATE PASSED, 1m28s, log `/root/ngfw-wt/logs/ci/P05-20260924-015233-1649852`. The only warnings come from main's `review(P05)` subjects. `go test -race -count=1 ./internal/...`: all ok |

## N1 evidence (host, my probe: owners `w7va`/`w7vb`, table 7070, `/32` source from `ip_route_v2_dump`)
```
N1 agg via nh       APPLIED {Created:2 ...}                  | /32 src=18 (recursive-resolution)
N1 + /32 nh         APPLIED {Created:1 Unchanged:2}          | /32 src=8  (API, ours)
N1 remove /32       APPLIED {Deleted:1 Unchanged:2}          | /32 src=18 (only VPP's RR entry left, no API residue)
N1 re-add /32       APPLIED {Created:1 Unchanged:2}          | /32 src=8
N1 remove both      APPLIED {Deleted:2 Unchanged:1}          | /32 absent
N1 both in one txn  APPLIED {Created:2 Unchanged:1}          | /32 src=8
Plan(desired) after the sequence: empty (no Retrieve drift)
B /32 src=8, B /24 src=8                                      (owner B: table-0 /24 via 10.7.183.1 and the /32 of that next hop)
A claims B's 10.7.183.1/32: ROLLED_BACK … not owned by this agent: table 0 10.7.183.1/32 (fib source 8)
A claims B's 10.7.185.0/24: ROLLED_BACK … not owned by this agent: table 0 10.7.185.0/24 (fib source 8)
after A emptied: B /32 src=8, B /24 src=8                     (B.Plan(theirs) empty: nothing of B deleted)
after B emptied: /32 absent, /24 absent; table 7070 /32 absent; `show ip fib` has no w7 residue
```
The author's `TestRouteOverVPPEntriesOnHost` and `TestClaimRulesOnHost` also pass in my run.
**Source classification** (`route.go` constants against the host's `vppctl show fib source`): the ids match
(special=1, interface=4, SR=5, 6RD=7, API=8, CLI=9, LISP=10, MAP=11, DHCP=12, adjacency=15, RR=18, default-route=20,
interpose=21, plugin ids 22–39). Client sources (SR/6RD/API/CLI/LISP/MAP/DHCP and every plugin id > 21, which includes
lcp-rt, nat and lb) block a claim. VPP-generated sources (special, classify, proxy, interface, BIER, IPv6-nd,
adjacency, mpls, attached_export, RR, urpf-exempt, default-route, interpose) never block. That classification is
sensible.

## N2 evidence (fake VPP)
- A pending txn **before** its deadline still refuses a new Apply: `FailedPrecondition "transaction \"p1\" is pending
  confirmation: confirm it (confirm_txn_id) or let it revert"` (my probe).
- An owed revert that fails deterministically (foreign `w7x` route) arms the timer: `confirm revert of p1 failed,
  retrying in 50ms (and on every resync; a new Apply supersedes it)`. A new Apply is then accepted
  (`new transaction supersedes the owed confirm revert`), and `TestNewApplySupersedesOwedRevert` passes (APPLIED,
  pending cleared, not degraded, a later Apply converges).
- `TestOwedRevertRetriedByTimer`: the obstacle is removed with no reconnect, the timer retry restores the baseline
  route, and pending is cleared. The stale-timer race is safe because `revertLocked` returns when
  `PendingTxnID != txnID`.

## NEW high-severity issues
None.

Two issues below High are recorded for the follow-up and do not block:
- (Medium) **A superseding Apply that itself fails still drops the owed revert.** `service.go` clears
  `PendingTxnID` and `Reverting` and stops the retry timer before `applyLocked` runs. My probe: the revert of p1
  fails, then Apply `bad` returns FAILED (validation). Afterwards `pending=""`, `reverting=false` and there is no
  retry timer. After I removed the obstacle, p1's unconfirmed VRF 7001 was still installed 1 s later and the baseline
  route was not restored. `degraded=true` stays set, but its message ("retrying in …") is stale. Recovery comes at the
  next resync (reconnect or restart), because the persisted desired state is already the baseline. Fix: drop the owed
  revert only when the new Apply returns APPLIED, and otherwise re-arm the retry.
- (Low) `mayHideAPI` compares source ids, not priorities. Plugin sources that outrank API (lcp-rt 16.6, nat-hi,
  path-mtu, cnat, …) have ids > 21. So if such a source is best above our API source, `Delete` sends nothing and
  leaves a stale API route that reappears when the plugin source leaves. This case matters once FRR (lcp-rt) installs
  the same prefix as a static route (D-072, P08/P12).

APPROVE
