# Progress

Updated 2026-09-24 from plan/tasks.yaml (estimated hours are the plan's, not actuals).

**Overall: 36.6% by hours (340/930 h), 35.6% by tasks (32/90)**

| state | tasks |
|---|---|
| merged | 32 |
| review | 2 |
| running | 2 |
| ready | 0 |
| parked | 0 |
| failed | 0 |
| todo | 54 |

| stage | merged h / total h | % | tasks merged/total | running | ready | parked |
|---|---|---|---|---|---|---|
| S0 | 6 / 6 | 100.0% | 1/1 | 0 | 0 | 0 |
| S1 | 73 / 73 | 100.0% | 9/9 | 0 | 0 | 0 |
| S2 | 233 / 253 | 92.1% | 19/24 | 2 | 0 | 0 |
| S3 | 0 / 16 | 0.0% | 0/1 | 0 | 0 | 0 |
| S4 | 0 / 406 | 0.0% | 0/38 | 0 | 0 | 0 |
| S5 | 28 / 128 | 21.9% | 3/13 | 0 | 0 | 0 |
| S6 | 0 / 48 | 0.0% | 0/4 | 0 | 0 | 0 |

## Running / review

- P08 — Vertical slice: interfaces end to end (af_packet rig) (review, host-agent slot1)
- TD-2 — API follow-ups from P07b/P13/F-sdk: set-password endpoint (plaintext in, argon2id server-side), typed api-client build output, /health schema, reject control chars in free text (P13 H1), per-API-key candidates (D-093) (review, host-agent slot7)
- TD-3 — V19 guard: sanitize inherited per-interface state on interface create; unbind classify before table/interface delete; restart simulations delete dependents; ci.sh full pre-flight (running, host-agent slot2)
- TD-6 — apply-startup.sh hardening before the first real --apply (D-103): stale dead-man vs newer commit, rollback write failure, systemctl-show failure, NRestarts compare; lows V5–V9; fake-host harness + shellcheck wired into tools/ci.sh (running, host-agent slot6)

## Parked

- none
