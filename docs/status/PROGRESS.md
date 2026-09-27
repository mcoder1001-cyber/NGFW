# Progress

Updated 2026-09-27 from plan/tasks.yaml (estimated hours are the plan's, not actuals).

**Overall: 61.1% by hours (803.0/1313.5 h), 62.1% by tasks (90/145)**

| state | tasks |
|---|---|
| merged | 90 |
| review | 1 |
| running | 3 |
| ready | 26 |
| parked | 2 |
| failed | 0 |
| todo | 23 |

| stage | merged h / total h | % | tasks merged/total | running | ready | parked |
|---|---|---|---|---|---|---|
| S0 | 6 / 6 | 100.0% | 1/1 | 0 | 0 | 0 |
| S1 | 73 / 73 | 100.0% | 9/9 | 0 | 0 | 0 |
| S2 | 264 / 274 | 96.4% | 28/29 | 0 | 0 | 1 |
| S3 | 16 / 19 | 84.2% | 1/2 | 0 | 0 | 1 |
| S4 | 406.0 / 746.0 | 54.4% | 47/85 | 2 | 25 | 0 |
| S5 | 38 / 147.5 | 25.8% | 4/15 | 1 | 1 | 0 |
| S6 | 0 / 48 | 0.0% | 0/4 | 0 | 0 | 0 |

## Running / review

- P10 — Debian packaging + systemd + install (26.04, our VPP debs) (running, unassigned)
- TD-8c — Two-phase resync for dynamic sources (config first, then dynamic objects) — per-key reruns must not re-create the whole transaction (running, cloud session charming-johnson)
- TD-26 — Core VRF tolerant delete: a table VPP keeps locked (nat64 never releases its FIB locks) is left, recorded and warned, not a failed transaction (running, cloud session modest-keller)
- TD-27 — ifsanitize: clear inherited SPAN source state and LLDP entries on interface create (V19 family) (review, cloud session modest-keller)

## Parked

- LAB-vpp-per-slot — parked_on: PENDING-vpp-host-hardening
- P12-fib-proof — parked_on: PENDING-vpp-host-hardening (via LAB-vpp-per-slot)
