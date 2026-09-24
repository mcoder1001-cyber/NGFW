# Progress

Updated 2026-09-24 from plan/tasks.yaml (estimated hours are the plan's, not actuals).

**Overall: 34.9% by hours (322/922 h), 34.5% by tasks (30/87)**

| state | tasks |
|---|---|
| merged | 30 |
| review | 1 |
| running | 3 |
| ready | 1 |
| parked | 0 |
| failed | 0 |
| todo | 52 |

| stage | merged h / total h | % | tasks merged/total | running | ready | parked |
|---|---|---|---|---|---|---|
| S0 | 6 / 6 | 100.0% | 1/1 | 0 | 0 | 0 |
| S1 | 73 / 73 | 100.0% | 9/9 | 0 | 0 | 0 |
| S2 | 215 / 245 | 87.8% | 17/21 | 3 | 0 | 0 |
| S3 | 0 / 16 | 0.0% | 0/1 | 0 | 1 | 0 |
| S4 | 0 / 406 | 0.0% | 0/38 | 0 | 0 | 0 |
| S5 | 28 / 128 | 21.9% | 3/13 | 0 | 0 | 0 |
| S6 | 0 / 48 | 0.0% | 0/4 | 0 | 0 | 0 |

## Running / review

- DF-5 — Descriptors: ipsec, ikev2, wireguard (running, host-agent slot4)
- F-startup-apply — Robust startup.conf apply tooling (detached, watchdog, hung-VPP, ifupdown restore, Go API checks, handover gate) (running, host-agent slot6)
- TD-2 — API follow-ups from P07b/P13/F-sdk: set-password endpoint (plaintext in, argon2id server-side), typed api-client build output, /health schema, reject control chars in free text (P13 H1), per-API-key candidates (D-093) (review, host-agent slot7)
- TD-3 — V19 guard: sanitize inherited per-interface state on interface create; unbind classify before table/interface delete; restart simulations delete dependents; ci.sh full pre-flight (running, host-agent slot2)

## Parked

- none
