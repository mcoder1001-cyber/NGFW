# Progress

Updated 2026-10-04 from plan/tasks.yaml (estimated hours are the plan's, not actuals).

**Overall: 74.2% by hours (1170.0/1577.5 h), 74.4% by tasks (157/211)**

| state | tasks |
|---|---|
| merged | 157 |
| review | 11 |
| running | 13 |
| ready | 7 |
| parked | 8 |
| failed | 0 |
| todo | 15 |

| stage | merged h / total h | % | tasks merged/total | running | ready | parked |
|---|---|---|---|---|---|---|
| S0 | 6 / 6 | 100.0% | 1/1 | 0 | 0 | 0 |
| S1 | 73 / 73 | 100.0% | 9/9 | 0 | 0 | 0 |
| S2 | 264 / 274 | 96.4% | 28/29 | 0 | 0 | 1 |
| S3 | 16 / 19 | 84.2% | 1/2 | 0 | 0 | 1 |
| S4 | 737.0 / 1002.0 | 73.6% | 110/150 | 11 | 6 | 6 |
| S5 | 74 / 155.5 | 47.6% | 8/16 | 2 | 1 | 0 |
| S6 | 0 / 48 | 0.0% | 0/4 | 0 | 0 | 0 |

Merged measures reviewed source completion; deferred lab acceptance is not PASS. Running describes remaining implementation, not verified worker activity.

## Remaining implementation / review

- F-dataplane-ui — Dataplane screen: VPP plugins, NIC queues/descriptors, workers/corelist, hugepages — startup.conf preview + gated apply (running, cloud session modest-keller; assignment unverified; remaining implementation state, not evidence of a live worker)
- F-management-ui — Management screen: tabbed shell (local users, AAA, API TLS, remote syslog) + apply of management.tls (running, cloud session modest-keller; assignment unverified; remaining implementation state, not evidence of a live worker)
- P11 — Wave B (day 10-12): strongSwan+VPP build (staging sysroot) + IPsec S2S + tunnel dashboards (running, Codex manager delegated worker; assignment unverified; remaining implementation state, not evidence of a live worker)
- F-ikev2-native — Wave B (day 10-12): VPP native IKEv2 responder path (running, unassigned; worker activity unverified)
- F-tunnels — Wave B (day 10-12): GRE, IPIP, VXLAN(-GPE), GTP-U, L2TPv3, PPPoE (running, cloud session modest-keller; assignment unverified; remaining implementation state, not evidence of a live worker)
- F-pki — Wave B (day 10-12): CA, CSR, import/export, CRL/OCSP, expiry alerts (running, unassigned; assignment unverified; remaining implementation state, not evidence of a live worker)
- F-ospf — Wave B (day 10-12): OSPFv2/v3 via FRR (running, cloud session modest-keller; assignment unverified; remaining implementation state, not evidence of a live worker)
- F-isis-rip — Wave B (day 10-12): IS-IS, RIPv2/RIPng via FRR (running, cloud session modest-keller; assignment unverified; remaining implementation state, not evidence of a live worker)
- F-vrrp-config-sync — Wave C (day 13-15): VRRPv3 (VPP plugin + keepalived path), config sync, cluster UI (running, cloud session modest-keller; assignment unverified; remaining implementation state, not evidence of a live worker)
- P10 — Debian packaging + systemd + install (26.04, our VPP debs) (running, unassigned; assignment unverified; remaining implementation state, not evidence of a live worker)
- TD-19 — Install & lab provisioning from product artifacts (running, unassigned; assignment unverified; remaining implementation state, not evidence of a live worker)
- F-setup-wizard — First-boot setup wizard: language/time, admin password, WAN (DHCP/static/PPPoE), LAN + DHCP, safe defaults, one commit (running, setup_build; assignment unverified; remaining implementation state, not evidence of a live worker)
- F-multiwan-host — Multi-WAN on the lab: two WAN netns, failover time, balance split, per-member NAT (running, dashboard_finish; assignment unverified; remaining implementation state, not evidence of a live worker)
- F-mpls-ldp-host — LDP data-plane: FRR mpls ldp renderer section + frrsync/ldp (FRR label state -> VPP MPLS FIB, V5) + neighbour poller (review, Codex F-mpls-ldp-host; worker activity unverified)
- F-default-vpp-nics — Default dataplane ownership: every NIC except the management interface belongs to VPP out of the box; web shows them pre-provisioned and non-deletable (review, developer slot 1; worker activity unverified)
- F-nat46-host — F-nat46 host runs: TestNat46OnHost on a slot, vppctl show map domains, rollback, NRestarts, screenshot (review, developer slot 17; worker activity unverified)
- F-bruteforce-detectors — Brute-force host detectors → auto-block: additive EVENT_KIND_AUTOBLOCK_OBSERVED contract, agent detectors (journald SSH, charon auth failures, nftables port-scan chain), API subscriber → AutoBlockService.observe(), 10-bad-logins topology driver (review, Codex F-bruteforce-detectors; worker activity unverified)
- F-ospf-host — F-ospf host runs: frrtest ospfd + rig FIB evidence (R7/R4/R1 owed lists) (review, developer slot 11; worker activity unverified)
- F-pim-frrsync — PIM via FRR: renderers/frr/pim section + frrsync/pim (PIM → VPP mFIB via seam S1), pimd test-scoped harness (review, Codex F-pim-frrsync; worker activity unverified)
- S-ipclassify-zerofill — Mitigate the VPP ip-classify zero-fill crash (INC-vpp-classify-crash): tools/lab rig + every test fixture resets ip4/ip6 classify on each created interface before any address add; guard test; host check after rig up; optional table-0 sentinel on the globals owner (review, developer slot 16; worker activity unverified)
- S-classify-sentinel — Table-0 sentinel (M2): the globals owner creates a never-deleted classify table at index 0 after each VPP start so an armed classify /32 drops instead of crashing VPP (review, developer slot 5; worker activity unverified)
- F-det44-cnat-fix — CNAT 0/12 on the det44 rig: det44-in2out left on the interface after det44 leaves the document; no cnat client feature (review, developer slot 16; worker activity unverified)
- S-alarms-restart-rebuild — Alarms: API restart forgets active alarms (in-memory) so they never clear; rebuild state from the DB on start (review, developer slot 10; worker activity unverified)
- M-prompts — Generate the missing task prompts (FEATURE/HOST-FOLLOWUP templates) for the remaining board rows (review, developer slot 4; worker activity unverified)

## Parked

- LAB-vpp-per-slot — parked_on: PENDING-vpp-host-hardening
- P12-fib-proof — parked_on: PENDING-vpp-host-hardening (via LAB-vpp-per-slot)
- F-lb-host — parked_on: lab topology/host acceptance prerequisite; NOTRUN
- F-srv6-host — parked_on: lab topology/host acceptance prerequisite; NOTRUN
- F-mpls-srmpls-host — parked_on: lab topology/host acceptance prerequisite; NOTRUN
- F-rule-expiry-host — parked_on: lab topology/host acceptance prerequisite; NOTRUN
- F-global-blocking-host — parked_on: lab topology/host acceptance prerequisite; NOTRUN
- F-pppoe-client-host — parked_on: lab topology/host acceptance prerequisite; NOTRUN
