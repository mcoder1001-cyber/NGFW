# Progress

Updated 2026-09-25 from plan/tasks.yaml (estimated hours are the plan's, not actuals).

**Overall: 35.3% by hours (406/1151.5 h), 37.1% by tasks (46/124)**

| state | tasks |
|---|---|
| merged | 46 |
| review | 8 |
| running | 16 |
| ready | 19 |
| parked | 1 |
| failed | 0 |
| todo | 34 |

| stage | merged h / total h | % | tasks merged/total | running | ready | parked |
|---|---|---|---|---|---|---|
| S0 | 6 / 6 | 100.0% | 1/1 | 0 | 0 | 0 |
| S1 | 73 / 73 | 100.0% | 9/9 | 0 | 0 | 0 |
| S2 | 260 / 270 | 96.3% | 27/28 | 0 | 0 | 1 |
| S3 | 16 / 16 | 100.0% | 1/1 | 0 | 0 | 0 |
| S4 | 23 / 591.0 | 3.9% | 5/66 | 16 | 17 | 0 |
| S5 | 28 / 147.5 | 19.0% | 3/15 | 0 | 2 | 0 |
| S6 | 0 / 48 | 0.0% | 0/4 | 0 | 0 | 0 |

## Running / review

- F-vlan-qinq — Wave A (day 7-9): 802.1q sub-interfaces + QinQ stacking (review, ngfw-46 slot5)
- F-bonding — Wave A (day 7-9): LACP/XOR/RR/active-backup bonds (review, ngfw-46 slot6)
- F-bridge-l2 — Wave A (day 7-9): bridge domains, L2XC/L3XC, split-horizon, MAC aging, time-range MAC filter (review, ngfw-46 slot7)
- F-loopback-bvi-gso-lldp-span — Wave A (day 7-9): loopback/BVI, GSO/offload flags, LLDP, SPAN/ERSPAN, nsim (running, unassigned)
- F-vrf-static-ecmp — Wave A (day 7-9): VRF mgmt, static routes, ECMP, FIB browser (paged), ping/traceroute actions (running, ngfw-46 slot2)
- F-neighbors-ra — Wave A (day 7-9): ARP/ND table, proxy-ND, IPv6 RA, DAD (review, ngfw-46 slot9)
- F-rpf-adl-pbr — Wave A (day 7-9): uRPF strict/loose, ADL, ABF policy-based routing (review, ngfw-46 slot10)
- F-object-model — Wave A (day 7-9): addresses, groups, FQDN (agent-resolved), services, schedules, zones, tags (review, ngfw-46 slot3)
- F-acl — Wave A (day 7-9): MACIP/L3/L4 ACLs, attachments, hit counters, 100k-rule editor, ADL/Auto-SDL (running, unassigned)
- F-host-acl-nftables — Wave A (day 7-9): local-in ACL + nftables host policy renderer (running, unassigned)
- F-nat44-ed-sessions — Wave A (day 7-9): NAT44-ED outbound/1:1/port-forward + session browser/kill (running, ngfw-46 slot4)
- F-nat44-ei-64-66-nptv6 — Wave A (day 7-9): NAT44-EI, NAT64, NAT66, NPTv6 (npt66 skip-unless-loaded) (running, unassigned)
- F-wireguard — Wave B (day 10-12): WireGuard peers/keys (running, unassigned)
- P12 — Wave B (day 10-12): FRR + linux-cp framework, BGP (running, unassigned)
- F-kea-dhcp-relay — Wave B (day 10-12): Kea DHCPv4/v6 server + VPP DHCP relay/client (running, unassigned)
- F-unbound-chrony-syslog — Wave B (day 10-12): Unbound DNS, chrony NTP, syslog export + log explorer (running, unassigned)
- WEB-1 — ui-kit SchemaForm gaps: presence toggle, port/ip-range, datetime/time/timezone/color widgets, LTR identifiers in RTL, per-path i18n, itemKey summaries + rule-editor table view (running, ngfw-46 slot11)
- WEB-2 — Config screen kit (generic list+drawer+live-status over any candidate path) + data widgets + Secrets page (review, ngfw-46 slot1)
- WEB-3 — Committed browser harness: e2e lib, shots.mjs, screens/_example.mjs (P08 screenshot script was lost, F6) (running, unassigned)
- TD-9 — Agent core: bounded VPP calls + txn semantics (+ failed state save answers DEGRADED) (running, unassigned)
- TD-10a — API commit engine correctness (+ running vs Health.last_txn_id check on boot/reconnect) (review, ngfw-46 slot5)
- TD-10b — API auth, session, audit (running, unassigned)
- TD-11c — Untagged NICs, alias-aware delete order, claim-store scale (running, unassigned)
- TD-23 — Shared test seams: fake-agent action dispatch table + coretest fakevpp extension registry (no feature handlers) (running, unassigned)

## Parked

- LAB-vpp-per-slot — parked_on: PENDING-vpp-host-hardening
