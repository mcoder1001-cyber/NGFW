# Progress

Updated 2026-10-10 from plan/tasks.yaml (estimated hours are the plan's, not actuals).

**Overall: 99.0% by hours (1562.5/1578.5 h), 98.1% by tasks (208/212)**

| state | tasks |
|---|---|
| merged | 208 |
| review | 2 |
| running | 2 |
| ready | 0 |
| parked | 0 |
| failed | 0 |
| todo | 0 |

| stage | merged h / total h | % | tasks merged/total | running | ready | parked |
|---|---|---|---|---|---|---|
| S0 | 6 / 6 | 100.0% | 1/1 | 0 | 0 | 0 |
| S1 | 73 / 73 | 100.0% | 9/9 | 0 | 0 | 0 |
| S2 | 274 / 274 | 100.0% | 29/29 | 0 | 0 | 0 |
| S3 | 19 / 19 | 100.0% | 2/2 | 0 | 0 | 0 |
| S4 | 997.0 / 1003.0 | 99.4% | 148/151 | 1 | 0 | 0 |
| S5 | 145.5 / 155.5 | 93.6% | 15/16 | 1 | 0 | 0 |
| S6 | 48 / 48 | 100.0% | 4/4 | 0 | 0 | 0 |

Merged measures reviewed source completion; deferred lab acceptance is not PASS. Running describes remaining implementation, not verified worker activity.

## Remaining implementation / review

- F-ra-vpn — remote-access VPN IKEv2+EAP (running, routing_acceptance; active bounded RA signal contract implementation and review)
- F-global-blocking-host — Global blocking on the lab VPP: topology proof, lookup cost at 200k, real-endpoint screenshot (review, nat46_acceptance; dual-host acceptance complete; final mandatory CI and integration pending)
- F-pppoe-client-host — PPPoE client on the lab: pppd vs an accel-ppp/rp-pppoe server, VPP FIB mirror, reconnect, MSS clamp, screenshot (running, wan_acceptance; active acceptance and verification)
- F-multiwan-host — Multi-WAN on the lab: two WAN netns, failover time, balance split, per-member NAT (review, wan_acceptance; acceptance complete; final CI/integration pending)

## Parked

- none
