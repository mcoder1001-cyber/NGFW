# Progress

Updated 2026-09-24 from plan/tasks.yaml (estimated hours are the plan's, not actuals).

**Overall: 36.7% by hours (358/976 h), 35.6% by tasks (36/101)**

| state | tasks |
|---|---|
| merged | 36 |
| review | 5 |
| running | 11 |
| ready | 0 |
| parked | 0 |
| failed | 0 |
| todo | 49 |

| stage | merged h / total h | % | tasks merged/total | running | ready | parked |
|---|---|---|---|---|---|---|
| S0 | 6 / 6 | 100.0% | 1/1 | 0 | 0 | 0 |
| S1 | 73 / 73 | 100.0% | 9/9 | 0 | 0 | 0 |
| S2 | 251 / 258 | 97.3% | 23/26 | 1 | 0 | 0 |
| S3 | 0 / 16 | 0.0% | 0/1 | 0 | 0 | 0 |
| S4 | 0 / 447 | 0.0% | 0/47 | 10 | 0 | 0 |
| S5 | 28 / 128 | 21.9% | 3/13 | 0 | 0 | 0 |
| S6 | 0 / 48 | 0.0% | 0/4 | 0 | 0 | 0 |

## Running / review

- P08 — Vertical slice: interfaces end to end (af_packet rig) (review, ngfw-46 slot1)
- W-seed — Wave-A seed commit: hotspot anchors (A1,A2,A4,C1-C3,C5,P1,P4-P6,W1-W3), one-entry-per-line lists, Env event-publish + Resync hooks (A5), subsystems.SlotIDRange(), vpn/services page shells, proto.md "Feature RPCs" heading — no behaviour change (review, ngfw-46 slot3)
- TD-8 — Agent seams: Env.Publish/Resync wiring (W-seed Q1), S1 dynamic desired source, metrics collector hook (running, ngfw-46 (no slot, unit only))
- F-vlan-qinq — Wave A (day 7-9): 802.1q sub-interfaces + QinQ stacking (running, ngfw-46 slot5)
- F-bridge-l2 — Wave A (day 7-9): bridge domains, L2XC/L3XC, split-horizon, MAC aging, time-range MAC filter (running, ngfw-46 slot7)
- F-vrf-static-ecmp — Wave A (day 7-9): VRF mgmt, static routes, ECMP, FIB browser (paged), ping/traceroute actions (running, ngfw-46 slot2)
- F-neighbors-ra — Wave A (day 7-9): ARP/ND table, proxy-ND, IPv6 RA, DAD (running, ngfw-46 slot9)
- F-rpf-adl-pbr — Wave A (day 7-9): uRPF strict/loose, ADL, ABF policy-based routing (running, ngfw-46 slot10)
- F-object-model — Wave A (day 7-9): addresses, groups, FQDN (agent-resolved), services, schedules, zones, tags (running, ngfw-46 slot3)
- F-nat44-ed-sessions — Wave A (day 7-9): NAT44-ED outbound/1:1/port-forward + session browser/kill (running, ngfw-46 slot4)
- TD-4 — Auth hardening follow-ups from TD-2 (D-100): account disable bumps the credential generation; API-key creation from a JWT session requires the current password; login gets the same transport check as password set (review, ngfw-46 slot8)
- TD-7 — apply-startup.sh follow-ups from the TD-6 review: F1 concurrent manual rollback of one apply (per-apply lock + cancel the dead-man), F2 scenario 40 bound independent of load (count ip neigh calls) (review, ngfw-46 slot6)
- TD-12 — De-flake load-sensitive tests that break merge gates (TestGRPCRoundTrip, TestPendingSurvives…, strongswan TestWatchResync, api vitest fetch timeout) (running, ngfw-46 slot6)
- WEB-1 — ui-kit SchemaForm gaps: presence toggle, port/ip-range, datetime/time/timezone/color widgets, LTR identifiers in RTL, per-path i18n, itemKey summaries + rule-editor table view (running, ngfw-46 slot11)
- WEB-2 — Config screen kit (generic list+drawer+live-status over any candidate path) + data widgets + Secrets page (running, ngfw-46 slot1)
- ui-nav-collapse — Web nav: collapsible groups (closed by default, current group opens, useId ids, shared open state) — product-owner request in session «وب» (review, «وب»)

## Parked

- none
