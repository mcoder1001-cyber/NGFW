# Progress

Updated 2026-10-07 from plan/tasks.yaml (estimated hours are the plan's, not actuals).

**Overall: 98.3% by hours (1551.0/1578.5 h), 96.7% by tasks (205/212)**

| state | tasks |
|---|---|
| merged | 205 |
| review | 0 |
| running | 2 |
| ready | 0 |
| parked | 5 |
| failed | 0 |
| todo | 0 |

| stage | merged h / total h | % | tasks merged/total | running | ready | parked |
|---|---|---|---|---|---|---|
| S0 | 6 / 6 | 100.0% | 1/1 | 0 | 0 | 0 |
| S1 | 73 / 73 | 100.0% | 9/9 | 0 | 0 | 0 |
| S2 | 274 / 274 | 100.0% | 29/29 | 0 | 0 | 0 |
| S3 | 16 / 19 | 84.2% | 1/2 | 0 | 0 | 1 |
| S4 | 992.0 / 1003.0 | 98.9% | 147/151 | 0 | 0 | 4 |
| S5 | 142 / 155.5 | 91.3% | 14/16 | 2 | 0 | 0 |
| S6 | 48 / 48 | 100.0% | 4/4 | 0 | 0 | 0 |

Merged measures reviewed source completion; deferred lab acceptance is not PASS. Running describes remaining implementation, not verified worker activity.

## Remaining implementation / review

- F-ra-vpn — remote-access VPN IKEv2+EAP (running, unassigned; source developer and independent reviewer completed; native acceptance awaiting resume; no verified live native worker)
- TD-19 — Install & lab provisioning from product artifacts (running, td19_developer; verified live developer repairing independent BLOCK B1/B2 alternate-root execution; reviewer completed BLOCK and available for repaired-source verification; source545f118af not approved)

## Parked

- P12-fib-proof — parked_on: PR180 HOLD: current mgmtd startup failed at unchanged 30s deadline before 200-route proof
- F-global-blocking-host — parked_on: lab topology/host acceptance prerequisite; NOTRUN
- F-pppoe-client-host — parked_on: lab topology/host acceptance prerequisite; NOTRUN
- F-nat46-host — parked_on: remaining live packet/FIB/restart/API acceptance
- F-ospf-host — parked_on: remaining live packet/FIB/restart/API acceptance
