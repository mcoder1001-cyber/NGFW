# Progress

Updated 2026-09-24 from plan/tasks.yaml (estimated hours are the plan's, not actuals).

**Overall: 37.0% by hours (349/942 h), 36.2% by tasks (34/94)**

| state | tasks |
|---|---|
| merged | 34 |
| review | 2 |
| running | 4 |
| ready | 0 |
| parked | 0 |
| failed | 0 |
| todo | 54 |

| stage | merged h / total h | % | tasks merged/total | running | ready | parked |
|---|---|---|---|---|---|---|
| S0 | 6 / 6 | 100.0% | 1/1 | 0 | 0 | 0 |
| S1 | 73 / 73 | 100.0% | 9/9 | 0 | 0 | 0 |
| S2 | 242 / 256 | 94.5% | 21/25 | 2 | 0 | 0 |
| S3 | 0 / 16 | 0.0% | 0/1 | 1 | 0 | 0 |
| S4 | 0 / 415 | 0.0% | 0/41 | 1 | 0 | 0 |
| S5 | 28 / 128 | 21.9% | 3/13 | 0 | 0 | 0 |
| S6 | 0 / 48 | 0.0% | 0/4 | 0 | 0 | 0 |

## Running / review

- P08 — Vertical slice: interfaces end to end (af_packet rig) (running, ngfw-46 slot1)
- W-seed — Wave-A seed commit: hotspot anchors (A1,A2,A4,C1-C3,C5,P1,P4-P6,W1-W3), one-entry-per-line lists, Env event-publish + Resync hooks (A5), subsystems.SlotIDRange(), vpn/services page shells, proto.md "Feature RPCs" heading — no behaviour change (running, ngfw-46 slot3)
- TD-2 — API follow-ups from P07b/P13/F-sdk: set-password endpoint (plaintext in, argon2id server-side), typed api-client build output, /health schema, reject control chars in free text (P13 H1), per-API-key candidates (D-093) (review, ngfw-46 slot7)
- TD-4 — Auth hardening follow-ups from TD-2 (D-100): account disable bumps the credential generation; API-key creation from a JWT session requires the current password; login gets the same transport check as password set (running, ngfw-46 slot8)
- TD-6 — apply-startup.sh hardening before the first real --apply (D-103): stale dead-man vs newer commit, rollback write failure, systemctl-show failure, NRestarts compare; lows V5–V9; fake-host harness + shellcheck wired into tools/ci.sh (review, ngfw-46 slot6)
- TD-7 — apply-startup.sh follow-ups from the TD-6 review: F1 concurrent manual rollback of one apply (per-apply lock + cancel the dead-man), F2 scenario 40 bound independent of load (count ip neigh calls) (running, ngfw-46 slot6)

## Parked

- none
