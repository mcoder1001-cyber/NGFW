# Progress

Updated 2026-10-05 from plan/tasks.yaml (estimated hours are the plan's, not actuals).

**Overall: 91.9% by hours (1450.0/1577.5 h), 91.5% by tasks (193/211)**

| state | tasks |
|---|---|
| merged | 193 |
| review | 0 |
| running | 1 |
| ready | 3 |
| parked | 9 |
| failed | 0 |
| todo | 5 |

| stage | merged h / total h | % | tasks merged/total | running | ready | parked |
|---|---|---|---|---|---|---|
| S0 | 6 / 6 | 100.0% | 1/1 | 0 | 0 | 0 |
| S1 | 73 / 73 | 100.0% | 9/9 | 0 | 0 | 0 |
| S2 | 264 / 274 | 96.4% | 28/29 | 0 | 0 | 1 |
| S3 | 16 / 19 | 84.2% | 1/2 | 0 | 0 | 1 |
| S4 | 959.0 / 1002.0 | 95.7% | 141/150 | 0 | 2 | 6 |
| S5 | 132 / 155.5 | 84.9% | 13/16 | 1 | 1 | 1 |
| S6 | 0 / 48 | 0.0% | 0/4 | 0 | 0 | 0 |

Merged measures reviewed source completion; deferred lab acceptance is not PASS. Running describes remaining implementation, not verified worker activity.

## Remaining implementation / review

- F-ra-vpn — remote-access VPN IKEv2+EAP (running, unassigned; Owner-approved independent strongSwan engine; active author upgrade; guarded lifecycle and actual EAP acceptance implementation ongoing.)

## Parked

- TD-19 — parked_on: PENDING-TD19-repository-trust
- LAB-vpp-per-slot — parked_on: PENDING-vpp-host-hardening
- P12-fib-proof — parked_on: PR180 HOLD: current mgmtd startup failed at unchanged 30s deadline before 200-route proof
- F-srv6-host — parked_on: lab topology/host acceptance prerequisite; NOTRUN
- F-mpls-srmpls-host — parked_on: lab topology/host acceptance prerequisite; NOTRUN
- F-global-blocking-host — parked_on: lab topology/host acceptance prerequisite; NOTRUN
- F-pppoe-client-host — parked_on: lab topology/host acceptance prerequisite; NOTRUN
- F-nat46-host — parked_on: remaining live packet/FIB/restart/API acceptance
- F-ospf-host — parked_on: remaining live packet/FIB/restart/API acceptance
