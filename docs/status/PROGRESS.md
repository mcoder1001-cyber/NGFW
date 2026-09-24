# Progress

Updated 2026-09-24 from plan/tasks.yaml (estimated hours are the plan's, not actuals).

**Overall: 36.0% by hours (334/927 h), 34.8% by tasks (31/89)**

| state | tasks |
|---|---|
| merged | 31 |
| review | 3 |
| running | 1 |
| ready | 0 |
| parked | 0 |
| failed | 0 |
| todo | 54 |

| stage | merged h / total h | % | tasks merged/total | running | ready | parked |
|---|---|---|---|---|---|---|
| S0 | 6 / 6 | 100.0% | 1/1 | 0 | 0 | 0 |
| S1 | 73 / 73 | 100.0% | 9/9 | 0 | 0 | 0 |
| S2 | 227 / 250 | 90.8% | 18/23 | 0 | 0 | 0 |
| S3 | 0 / 16 | 0.0% | 0/1 | 1 | 0 | 0 |
| S4 | 0 / 406 | 0.0% | 0/38 | 0 | 0 | 0 |
| S5 | 28 / 128 | 21.9% | 3/13 | 0 | 0 | 0 |
| S6 | 0 / 48 | 0.0% | 0/4 | 0 | 0 | 0 |

## Running / review

- P08 — Vertical slice: interfaces end to end (af_packet rig) (running, host-agent slot1)
- F-startup-apply — Robust startup.conf apply tooling (detached, watchdog, hung-VPP, ifupdown restore, Go API checks, handover gate) (review, host-agent slot6)
- TD-2 — API follow-ups from P07b/P13/F-sdk: set-password endpoint (plaintext in, argon2id server-side), typed api-client build output, /health schema, reject control chars in free text (P13 H1), per-API-key candidates (D-093) (review, host-agent slot7)
- TD-3 — V19 guard: sanitize inherited per-interface state on interface create; unbind classify before table/interface delete; restart simulations delete dependents; ci.sh full pre-flight (review, host-agent slot2)

## Parked

- none
