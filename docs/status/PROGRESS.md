# Progress

Updated 2026-10-08 from plan/tasks.yaml (estimated hours are the plan's, not actuals).

**Overall: 98.3% by hours (1551.0/1578.5 h), 96.7% by tasks (205/212)**

| state | tasks |
|---|---|
| merged | 205 |
| review | 1 |
| running | 1 |
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
| S4 | 992.0 / 1003.0 | 98.9% | 147/151 | 1 | 0 | 3 |
| S5 | 142 / 155.5 | 91.3% | 14/16 | 0 | 0 | 1 |
| S6 | 48 / 48 | 100.0% | 4/4 | 0 | 0 | 0 |

Merged measures reviewed source completion; deferred lab acceptance is not PASS. Running describes remaining implementation, not verified worker activity.

## Remaining implementation / review

- TD-19 — Install & lab provisioning from product artifacts (review, unassigned; Reviewed complete Python release pins/provenance and offline wheel materialization in PR212; staged in completion PR214. Final aggregate CI/integration pending; real Ubuntu26.04 install/boot remains deferred. No live developer implied.)
- F-pppoe-client-host — PPPoE client on the lab: pppd vs an accel-ppp/rp-pppoe server, VPP FIB mirror, reconnect, MSS clamp, screenshot (running, unassigned; Kernel-carrier, exclusive raw-parent transport, distinct logical transit identity, PD LAN application and WAN readiness integration actively being implemented on published isolated checkpoints. PR210 lifecycle source staged in PR214; not lab-only and not complete.)

## Parked

- F-ra-vpn — parked_on: native supplier/session/identity and EAP TLS acceptance; source integrated, operational negative receipts require diagnosis
- P12-fib-proof — parked_on: PR180 HOLD: current mgmtd startup failed at unchanged 30s deadline before 200-route proof
- F-global-blocking-host — parked_on: lab topology/host acceptance prerequisite; NOTRUN
- F-nat46-host — parked_on: remaining live packet/FIB/restart/API acceptance
- F-ospf-host — parked_on: remaining live packet/FIB/restart/API acceptance
