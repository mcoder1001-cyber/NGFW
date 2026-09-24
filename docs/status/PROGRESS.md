# Progress

Updated 2026-09-24 from plan/tasks.yaml (estimated hours are the plan's, not actuals).

**Overall: 31.7% by hours (290/916 h), 31.4% by tasks (27/86)**

| state | tasks |
|---|---|
| merged | 27 |
| review | 1 |
| running | 5 |
| ready | 0 |
| parked | 0 |
| failed | 0 |
| todo | 53 |

| stage | merged h / total h | % | tasks merged/total | running | ready | parked |
|---|---|---|---|---|---|---|
| S0 | 6 / 6 | 100.0% | 1/1 | 0 | 0 | 0 |
| S1 | 73 / 73 | 100.0% | 9/9 | 0 | 0 | 0 |
| S2 | 203 / 239 | 84.9% | 16/20 | 3 | 0 | 0 |
| S3 | 0 / 16 | 0.0% | 0/1 | 0 | 0 | 0 |
| S4 | 0 / 406 | 0.0% | 0/38 | 0 | 0 | 0 |
| S5 | 8 / 128 | 6.2% | 1/13 | 2 | 0 | 0 |
| S6 | 0 / 48 | 0.0% | 0/4 | 0 | 0 | 0 |

## Running / review

- P07b — UI flows: login, pending-change bar + commit dialog, revisions, users, Playwright login→commit→rollback (review, host-agent slot1)
- DF-5 — Descriptors: ipsec, ikev2, wireguard (running, host-agent slot4)
- P13 — CLI basic (running, host-agent slot3)
- F-sdk-terraform-ansible — Terraform provider, Ansible collection, Python SDK (running, host-agent slot5)
- F-startup-apply — Robust startup.conf apply tooling (detached, watchdog, hung-VPP, ifupdown restore, Go API checks, handover gate) (running, host-agent slot6)
- TD-2 — API follow-ups from P07b/P13/F-sdk: set-password endpoint (plaintext in, argon2id server-side), typed api-client build output, /health schema, reject control chars in free text (P13 H1), per-API-key candidates (D-093) (running, host-agent slot7)

## Parked

- none
