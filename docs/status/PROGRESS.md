# Progress

Updated 2026-09-25 from plan/tasks.yaml (estimated hours are the plan's, not actuals).

**Overall: 34.3% by hours (389/1135.0 h), 35.3% by tasks (42/119)**

| state | tasks |
|---|---|
| merged | 42 |
| review | 3 |
| running | 10 |
| ready | 21 |
| parked | 1 |
| failed | 0 |
| todo | 42 |

| stage | merged h / total h | % | tasks merged/total | running | ready | parked |
|---|---|---|---|---|---|---|
| S0 | 6 / 6 | 100.0% | 1/1 | 0 | 0 | 0 |
| S1 | 73 / 73 | 100.0% | 9/9 | 0 | 0 | 0 |
| S2 | 253 / 268 | 94.4% | 24/27 | 0 | 0 | 1 |
| S3 | 16 / 16 | 100.0% | 1/1 | 0 | 0 | 0 |
| S4 | 13 / 576.5 | 2.3% | 4/62 | 10 | 20 | 0 |
| S5 | 28 / 147.5 | 19.0% | 3/15 | 0 | 1 | 0 |
| S6 | 0 / 48 | 0.0% | 0/4 | 0 | 0 | 0 |

## Running / review

- F-vlan-qinq — Wave A (day 7-9): 802.1q sub-interfaces + QinQ stacking (review, ngfw-46 slot5)
- F-bonding — Wave A (day 7-9): LACP/XOR/RR/active-backup bonds (running, ngfw-46 slot6)
- F-bridge-l2 — Wave A (day 7-9): bridge domains, L2XC/L3XC, split-horizon, MAC aging, time-range MAC filter (running, ngfw-46 slot7)
- F-vrf-static-ecmp — Wave A (day 7-9): VRF mgmt, static routes, ECMP, FIB browser (paged), ping/traceroute actions (running, ngfw-46 slot2)
- F-neighbors-ra — Wave A (day 7-9): ARP/ND table, proxy-ND, IPv6 RA, DAD (running, ngfw-46 slot9)
- F-rpf-adl-pbr — Wave A (day 7-9): uRPF strict/loose, ADL, ABF policy-based routing (running, ngfw-46 slot10)
- F-object-model — Wave A (day 7-9): addresses, groups, FQDN (agent-resolved), services, schedules, zones, tags (running, ngfw-46 slot3)
- F-nat44-ed-sessions — Wave A (day 7-9): NAT44-ED outbound/1:1/port-forward + session browser/kill (running, ngfw-46 slot4)
- TD-4 — Auth hardening follow-ups from TD-2 (D-100): account disable bumps the credential generation; API-key creation from a JWT session requires the current password; login gets the same transport check as password set (review, ngfw-46 slot8)
- TD-7 — apply-startup.sh follow-ups from the TD-6 review: F1 concurrent manual rollback of one apply (per-apply lock + cancel the dead-man), F2 scenario 40 bound independent of load (count ip neigh calls) (review, ngfw-46 slot6)
- WEB-1 — ui-kit SchemaForm gaps: presence toggle, port/ip-range, datetime/time/timezone/color widgets, LTR identifiers in RTL, per-path i18n, itemKey summaries + rule-editor table view (running, ngfw-46 slot11)
- WEB-2 — Config screen kit (generic list+drawer+live-status over any candidate path) + data widgets + Secrets page (running, ngfw-46 slot1)
- TD-10a — API commit engine correctness (+ running vs Health.last_txn_id check on boot/reconnect) (running, ngfw-46 slot5)

## Parked

- LAB-vpp-per-slot — parked_on: PENDING-vpp-host-hardening
