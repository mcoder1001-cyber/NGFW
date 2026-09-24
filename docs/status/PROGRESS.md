# Progress

Updated 2026-09-24 from plan/tasks.yaml (estimated hours are the plan's, not actuals).

**Overall: 37.7% by hours (355/942 h), 37.2% by tasks (35/94)**

| state | tasks |
|---|---|
| merged | 35 |
| review | 2 |
| running | 10 |
| ready | 0 |
| parked | 0 |
| failed | 0 |
| todo | 47 |

| stage | merged h / total h | % | tasks merged/total | running | ready | parked |
|---|---|---|---|---|---|---|
| S0 | 6 / 6 | 100.0% | 1/1 | 0 | 0 | 0 |
| S1 | 73 / 73 | 100.0% | 9/9 | 0 | 0 | 0 |
| S2 | 248 / 256 | 96.9% | 22/25 | 2 | 0 | 0 |
| S3 | 0 / 16 | 0.0% | 0/1 | 1 | 0 | 0 |
| S4 | 0 / 415 | 0.0% | 0/41 | 7 | 0 | 0 |
| S5 | 28 / 128 | 21.9% | 3/13 | 0 | 0 | 0 |
| S6 | 0 / 48 | 0.0% | 0/4 | 0 | 0 | 0 |

## Running / review

- P08 — Vertical slice: interfaces end to end (af_packet rig) (running, ngfw-46 slot1)
- W-seed — Wave-A seed commit: hotspot anchors (A1,A2,A4,C1-C3,C5,P1,P4-P6,W1-W3), one-entry-per-line lists, Env event-publish + Resync hooks (A5), subsystems.SlotIDRange(), vpn/services page shells, proto.md "Feature RPCs" heading — no behaviour change (review, ngfw-46 slot3)
- F-vlan-qinq — Wave A (day 7-9): 802.1q sub-interfaces + QinQ stacking (running, ngfw-46 slot5)
- F-bridge-l2 — Wave A (day 7-9): bridge domains, L2XC/L3XC, split-horizon, MAC aging, time-range MAC filter (running, ngfw-46 slot7)
- F-vrf-static-ecmp — Wave A (day 7-9): VRF mgmt, static routes, ECMP, FIB browser (paged), ping/traceroute actions (running, ngfw-46 slot2)
- F-neighbors-ra — Wave A (day 7-9): ARP/ND table, proxy-ND, IPv6 RA, DAD (running, ngfw-46 slot9)
- F-rpf-adl-pbr — Wave A (day 7-9): uRPF strict/loose, ADL, ABF policy-based routing (running, ngfw-46 slot10)
- F-object-model — Wave A (day 7-9): addresses, groups, FQDN (agent-resolved), services, schedules, zones, tags (running, ngfw-46 slot3)
- F-nat44-ed-sessions — Wave A (day 7-9): NAT44-ED outbound/1:1/port-forward + session browser/kill (running, ngfw-46 slot4)
- TD-4 — Auth hardening follow-ups from TD-2 (D-100): account disable bumps the credential generation; API-key creation from a JWT session requires the current password; login gets the same transport check as password set (running, ngfw-46 slot8)
- TD-6 — apply-startup.sh hardening before the first real --apply (D-103): stale dead-man vs newer commit, rollback write failure, systemctl-show failure, NRestarts compare; lows V5–V9; fake-host harness + shellcheck wired into tools/ci.sh (review, ngfw-46 slot6)
- TD-7 — apply-startup.sh follow-ups from the TD-6 review: F1 concurrent manual rollback of one apply (per-apply lock + cancel the dead-man), F2 scenario 40 bound independent of load (count ip neigh calls) (running, ngfw-46 slot6)

## Parked

- none
