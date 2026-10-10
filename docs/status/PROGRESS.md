# Progress

Updated 2026-10-10 from plan/tasks.yaml (estimated hours are the plan's, not actuals).

**Overall: 98.4% by hours (1552.5/1578.5 h), 96.7% by tasks (205/212)**

| state | tasks |
|---|---|
| merged | 205 |
| review | 4 |
| running | 2 |
| ready | 0 |
| parked | 1 |
| failed | 0 |
| todo | 0 |

| stage | merged h / total h | % | tasks merged/total | running | ready | parked |
|---|---|---|---|---|---|---|
| S0 | 6 / 6 | 100.0% | 1/1 | 0 | 0 | 0 |
| S1 | 73 / 73 | 100.0% | 9/9 | 0 | 0 | 0 |
| S2 | 274 / 274 | 100.0% | 29/29 | 0 | 0 | 0 |
| S3 | 16 / 19 | 84.2% | 1/2 | 0 | 0 | 0 |
| S4 | 990.0 / 1003.0 | 98.7% | 146/151 | 2 | 0 | 0 |
| S5 | 145.5 / 155.5 | 93.6% | 15/16 | 0 | 0 | 1 |
| S6 | 48 / 48 | 100.0% | 4/4 | 0 | 0 | 0 |

Merged measures reviewed source completion; deferred lab acceptance is not PASS. Running describes remaining implementation, not verified worker activity.

## Remaining implementation / review

- P12-fib-proof — Prove BGP routes reach the VPP FIB through linux-nl on a private per-slot VPP (review, routing_acceptance; acceptance complete; final CI/integration pending)
- F-global-blocking-host — Global blocking on the lab VPP: topology proof, lookup cost at 200k, real-endpoint screenshot (running, nat46_acceptance; active acceptance and verification)
- F-pppoe-client-host — PPPoE client on the lab: pppd vs an accel-ppp/rp-pppoe server, VPP FIB mirror, reconnect, MSS clamp, screenshot (running, wan_acceptance; active acceptance and verification)
- F-multiwan-host — Multi-WAN on the lab: two WAN netns, failover time, balance split, per-member NAT (review, wan_acceptance; acceptance complete; final CI/integration pending)
- F-nat46-host — F-nat46 host runs: TestNat46OnHost on a slot, vppctl show map domains, rollback, NRestarts, screenshot (review, nat46_acceptance; acceptance complete; final CI/integration pending)
- F-ospf-host — F-ospf host runs: frrtest ospfd + rig FIB evidence (R7/R4/R1 owed lists) (review, routing_acceptance; acceptance complete; final CI/integration pending)

## Parked

- F-ra-vpn — parked_on: Actual both-host private EAP/TLS/VICI/ESP/VRF/ACL tests PASS. 250 genuine artifact ABI preflight FAIL. Supplemental guest5 public Health works but operational=false reason engine-not-ready. Full canonical supplier/session/identity/recovery/API/browser acceptance unverified; underlying initialization stage unresolved. Not Done.
