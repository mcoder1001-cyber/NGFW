# Progress

Updated 2026-09-25 from plan/tasks.yaml (estimated hours are the plan's, not actuals).

**Overall: 38.6% by hours (452/1172.5 h), 40.0% by tasks (52/130)**

| state | tasks |
|---|---|
| merged | 52 |
| review | 32 |
| running | 6 |
| ready | 2 |
| parked | 2 |
| failed | 0 |
| todo | 36 |

| stage | merged h / total h | % | tasks merged/total | running | ready | parked |
|---|---|---|---|---|---|---|
| S0 | 6 / 6 | 100.0% | 1/1 | 0 | 0 | 0 |
| S1 | 73 / 73 | 100.0% | 9/9 | 0 | 0 | 0 |
| S2 | 260 / 274 | 94.9% | 27/29 | 0 | 0 | 1 |
| S3 | 16 / 19 | 84.2% | 1/2 | 0 | 0 | 1 |
| S4 | 59 / 605.0 | 9.8% | 10/70 | 5 | 1 | 0 |
| S5 | 38 / 147.5 | 25.8% | 4/15 | 1 | 1 | 0 |
| S6 | 0 / 48 | 0.0% | 0/4 | 0 | 0 | 0 |

## Running / review

- F-vlan-qinq — Wave A (day 7-9): 802.1q sub-interfaces + QinQ stacking (review, ngfw-46 slot5)
- F-bonding — Wave A (day 7-9): LACP/XOR/RR/active-backup bonds (review, ngfw-46 slot6)
- F-bridge-l2 — Wave A (day 7-9): bridge domains, L2XC/L3XC, split-horizon, MAC aging, time-range MAC filter (review, ngfw-46 slot7)
- F-loopback-bvi-gso-lldp-span — Wave A (day 7-9): loopback/BVI, GSO/offload flags, LLDP, SPAN/ERSPAN, nsim (review, unassigned)
- F-vrf-static-ecmp — Wave A (day 7-9): VRF mgmt, static routes, ECMP, FIB browser (paged), ping/traceroute actions (review, ngfw-46 slot2)
- F-neighbors-ra — Wave A (day 7-9): ARP/ND table, proxy-ND, IPv6 RA, DAD (review, ngfw-46 slot9)
- F-rpf-adl-pbr — Wave A (day 7-9): uRPF strict/loose, ADL, ABF policy-based routing (review, ngfw-46 slot10)
- F-object-model — Wave A (day 7-9): addresses, groups, FQDN (agent-resolved), services, schedules, zones, tags (review, ngfw-46 slot3)
- F-acl — Wave A (day 7-9): MACIP/L3/L4 ACLs, attachments, hit counters, 100k-rule editor, ADL/Auto-SDL (review, unassigned)
- F-host-acl-nftables — Wave A (day 7-9): local-in ACL + nftables host policy renderer (review, unassigned)
- F-nat44-ed-sessions — Wave A (day 7-9): NAT44-ED outbound/1:1/port-forward + session browser/kill (review, ngfw-46 slot4)
- F-nat44-ei-64-66-nptv6 — Wave A (day 7-9): NAT44-EI, NAT64, NAT66, NPTv6 (npt66 skip-unless-loaded) (review, unassigned)
- F-wireguard — Wave B (day 10-12): WireGuard peers/keys (review, unassigned)
- P12 — Wave B (day 10-12): FRR + linux-cp framework, BGP (review, unassigned)
- F-kea-dhcp-relay — Wave B (day 10-12): Kea DHCPv4/v6 server + VPP DHCP relay/client (review, unassigned)
- F-unbound-chrony-syslog — Wave B (day 10-12): Unbound DNS, chrony NTP, syslog export + log explorer (review, unassigned)
- F-mpls-srmpls — Wave C (day 13-15): static MPLS + SR-MPLS (LDP split to F-mpls-ldp, D-085/D-109) (running, unassigned)
- F-srv6 — Wave C (day 13-15): SRv6 policies, network programming, service chaining proxies, SRv6-mobile (running, unassigned)
- F-host-stack — Wave C (day 13-15): expose host-stack config: session layer, TCP/UDP tuning, TLS, QUIC, HTTP static, HTTP/3 proxy (running, unassigned)
- F-lb — Wave C (day 13-15): Load Balancer plugin (GRE/NAT/L3DSR/maglev) (running, unassigned)
- F-qos-flat — Wave C (day 13-15): policer, marking, QoS record/map, DSCP/dot1p (HQoS = V3, excluded) (running, unassigned)
- F-snmp — Wave C (day 13-15): SNMP v2c/v3 via snmpd renderer + private MIB (review, unassigned)
- P10 — Debian packaging + systemd + install (26.04, our VPP debs) (running, unassigned)
- WEB-1 — ui-kit SchemaForm gaps: presence toggle, port/ip-range, datetime/time/timezone/color widgets, LTR identifiers in RTL, per-path i18n, itemKey summaries + rule-editor table view (review, ngfw-46 slot11)
- WEB-2 — Config screen kit (generic list+drawer+live-status over any candidate path) + data widgets + Secrets page (review, ngfw-46 slot1)
- WEB-3 — Committed browser harness: e2e lib, shots.mjs, screens/_example.mjs (P08 screenshot script was lost, F6) (review, unassigned)
- TD-9 — Agent core: bounded VPP calls + txn semantics (+ failed state save answers DEGRADED) (review, unassigned)
- TD-10a — API commit engine correctness (+ running vs Health.last_txn_id check on boot/reconnect) (review, ngfw-46 slot5)
- TD-10b — API auth, session, audit (review, unassigned)
- TD-11c — Untagged NICs, alias-aware delete order, claim-store scale (review, unassigned)
- TD-13 — Scheduler tier-3 Validator + VPP→daemon stage (validate daemon config before any VPP write) (review, unassigned)
- TD-15 — API hygiene: z.output response DTOs, ConfigTx userHashes/secretVersions + pool timeout, secret refs from the schema (review, unassigned)
- UI-domain-editor — Advanced configuration editor: generic schema-driven page for any domain path (review, unassigned)
- TD-23 — Shared test seams: fake-agent action dispatch table + coretest fakevpp extension registry (no feature handlers) (review, unassigned)
- TD-8b — Agent seams follow-up: quarantine only the failing dynamic object (not the whole source), SyncFunc doc for goroutines, ID-range flip to refuse start-up (tools/app VRX_VPP_TABLE_BASE=13000, topology harness passthrough, P10 unit `all`, df7.WithIDs + feature-prompt rule) (review, unassigned)
- TD-22 — Small follow-ups from reviews: dhcp.client claim-before-write (TD-11b Q3), P08 InterfaceDrawer saveSub stale-dialog l2:null (F-bridge-l2 review #3), CommitDialog reuses applyOutcome() on a lost commit answer (TD-10a Q), examples.test.ts SIBLING regex widened for feature examples (review, unassigned)
- TD-24 — interface-ip reconcile must not delete a DHCP-leased address (Retrieve skips the dhcp_client_dump lease) (review, unassigned)
- TD-25 — ifsanitize: stop the classify-pool hole ratchet (resurrect only indices a binding names; never pop fresh indices) (review, unassigned)

## Parked

- LAB-vpp-per-slot — parked_on: PENDING-vpp-host-hardening
- P12-fib-proof — parked_on: PENDING-vpp-host-hardening (via LAB-vpp-per-slot)
