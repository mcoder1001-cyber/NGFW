# Progress

Updated 2026-10-05 from plan/tasks.yaml (estimated hours are the plan's, not actuals).

**Overall: 89.0% by hours (1404.0/1577.5 h), 89.1% by tasks (188/211)**

| state | tasks |
|---|---|
| merged | 188 |
| review | 0 |
| running | 6 |
| ready | 0 |
| parked | 9 |
| failed | 0 |
| todo | 8 |

| stage | merged h / total h | % | tasks merged/total | running | ready | parked |
|---|---|---|---|---|---|---|
| S0 | 6 / 6 | 100.0% | 1/1 | 0 | 0 | 0 |
| S1 | 73 / 73 | 100.0% | 9/9 | 0 | 0 | 0 |
| S2 | 264 / 274 | 96.4% | 28/29 | 0 | 0 | 1 |
| S3 | 16 / 19 | 84.2% | 1/2 | 0 | 0 | 1 |
| S4 | 933.0 / 1002.0 | 93.1% | 138/150 | 3 | 0 | 6 |
| S5 | 112 / 155.5 | 72.0% | 11/16 | 3 | 0 | 1 |
| S6 | 0 / 48 | 0.0% | 0/4 | 0 | 0 | 0 |

Merged measures reviewed source completion; deferred lab acceptance is not PASS. Running describes remaining implementation, not verified worker activity.

## Remaining implementation / review

- F-bfd-redistribution — Wave B (day 10-12): BFD (VPP+FRR), redistribution matrix, route-policy UX (running, unassigned; New source97f67a2c closes R5 scale/history defects; R5 verify APPROVE; affected reviews and full gate pending.)
- F-ra-vpn — remote-access VPN IKEv2+EAP (running, unassigned; Owner-approved independent strongSwan engine; active author upgrade; guarded lifecycle and actual EAP acceptance implementation ongoing.)
- F-ab-upgrade — A/B image upgrade with rollback (running, unassigned; Reviewed sourcebe144811; independent quick9m49 and private-loop acceptance PASS; final current-main integration pending.)
- F-images — VM/cloud image builds (running, unassigned; Reviewed source6bd58ecb; all applicable source approvals; final current-main integration/gates pending.)
- F-ha-state-sync — HA state sync (T2, partial ok): NAT/ACL session sync, IPsec SA sync, failover test automation (running, unassigned; New API source834eed6f:691tests PASS; independent actual API replay active; R6 stale observation defect reproduced awaiting repair.)
- P11-host — Route-based IPsec host evidence: native IKEv2, FIB, ESP, rekey, recovery and rollback (running, unassigned; Reviewed sourcef59c3ea7; independent real native both-role lifecycle acceptance PASS; final integration pending.)

## Parked

- TD-19 — parked_on: PENDING-TD19-repository-trust
- LAB-vpp-per-slot — parked_on: PENDING-vpp-host-hardening
- P12-fib-proof — parked_on: PENDING-vpp-host-hardening (via LAB-vpp-per-slot)
- F-srv6-host — parked_on: lab topology/host acceptance prerequisite; NOTRUN
- F-mpls-srmpls-host — parked_on: lab topology/host acceptance prerequisite; NOTRUN
- F-global-blocking-host — parked_on: lab topology/host acceptance prerequisite; NOTRUN
- F-pppoe-client-host — parked_on: lab topology/host acceptance prerequisite; NOTRUN
- F-nat46-host — parked_on: remaining live packet/FIB/restart/API acceptance
- F-ospf-host — parked_on: remaining live packet/FIB/restart/API acceptance
