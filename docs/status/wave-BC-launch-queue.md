# Wave B/C (+S5) launch queue (prep-rest critic, 2026-09-24; main@e310add, P08@e1587c9, W-seed@0083590)
Scope: the 31 wave-B/C/S5 envelopes of this branch plus the wave-A rows that are still queued (batch-2 follow-ons, P11, P12, F-wireguard,
F-kea-dhcp-relay, F-unbound-chrony-syslog). Batch 1 is wave-A-launch-plan.md §2. Numbers: wave-BC-numbers.md (collision audit at its end).

## 1. Critic findings (fixed in place unless marked M = manager action, §4)
- **Glob overlaps.** I intersected the owned globs of all 49 unmerged envelopes (brace-expanded; `**`/`*` sampled both ways). Only three
  real pairs remain; every other shared file is a declared hotspot or dep-chained hunk:
  (1) P11 `renderers/strongswan/**` ⊃ F-ra-vpn's `ra*` files: dep-chained, F-ra-vpn spawns after P11 merges, OK (board row → narrowed list, M6);
  (2) TD-4 `apps/api/src/auth/**` (running) ⊃ F-aaa's auth.controller/auth.service hunks → **F-aaa +TD-4** (M5);
  (3) P10 owned `apps/api/package.json`, which F-aaa, F-pki and F-restconf-yang also change → moved to P10's hotspots (D4/SY8). **Fixed.**
- **Missing hotspots.** (a) ED's `desired/nat.go` dispatch and natTabs had no anchors, but EI and F-det44 may run in parallel there. ED now
  seeds an EI group and a CGNAT group, each under its own anchor (new ED obligation; EI and F-det44 hunks point to them). **Fixed.**
  (b) F-vrrp-config-sync's peer sync endpoint: add SY1 route-guard.test.ts `PUBLIC` if it authenticates by cluster key. **Fixed.**
  (c) The pack rule "seed wave-BC anchors below the wave-A ones" would put each anchor exactly where a still-running wave-A or batch-2 task
  inserts, which conflicts at rebase. Rule changed to: seed directly above the first `wave-A:` anchor. **Fixed** in wave-BC-numbers.md.
  (d) Shared new `Domains` keys (`Tunnels` F-tunnels×F-lisp, `Management` F-unbound×F-dashboard): a duplicate key is a compile error;
  the second lander merges into the existing entry (noted there).
- **Numbers.** No collision across wave-A §2, wave-BC-numbers.md and the envelopes. The risk was in the P11/P12/F-wireguard/F-kea/F-unbound
  envelopes: they still said "from the manager's wave-B table", and P11's "EventKind ×1–2" could have taken F-wireguard's 13. They now cite
  §2's binding numbers (D-109 e). Also: F-kea DhcpRelay 9–10 is reserved-if-gap, and F-loopback is marked confirmed. **Fixed.**
- **Daemons.** The five FRR envelopes said parallel FRR was "manager-confirmed", but nothing is logged yet. They now say: parallel only
  after M3, and a root-netns zebra is always one slot at a time (zebra deletes FRR-protocol kernel routes it finds at startup; P12
  coordination (4) updated). The strongSwan pair F-ikev2-native × F-ra-vpn is already conditional. keepalived (F-vrrp), snmpd (F-snmp),
  kea (F-kea) and unbound/chrony/rsyslog (F-unbound) each have a single owner.
- **Board deps vs prompts/envelopes** (M5): F-igmp-mfib +P12 (lcpmap, pimd over P12's LCP pairs). F-det44, F-ikev2-native, F-ra-vpn,
  F-capture-trace and F-backup-restore +F-vrf-static-ecmp (they reuse its API Action bridge / generic Action stream method). F-aaa +TD-4.
  F-dashboard-prom-alarms +F-unbound-chrony-syslog (adds `Domains["management"]`; or accept the union). P10 +TD-7 (packages the gated
  apply-startup.sh). Every wave-B/C row +W-seed-BC (anchors). F-mpls-ldp, F-igmp-mfib, F-dashboard +TD-8 (seams). Soft only: F-hardening-lite→P11.
- **Prompts:** grep for stale `task/…`/`git show`/`contract/…`/`project_*.go`/`dpdk { disable }`/apt-install → clean (the pack refresh holds).

## 2. Queue (spawn in this order when a slot frees and deps are merged; prio from the board, ↑ = critical-path bump)
Critical paths after batch 1 starts: F-vrf 15 → P12 24 → F-vrrp 24 → F-ha 18 = **81 h** · F-vrf → P12 → F-ospf → F-bfd = 69 h ·
P10 → P14 → F-ab-upgrade → F-backup-restore = 60 h · P11 → F-pki → F-ra-vpn = 54 h.
| # | task | prio | deps still open at batch 1 | daemon-owner | batch | note |
|---|---|---|---|---|---|---|
| 0 | W-seed-BC (manager anchor pass, 3 h) | – | W-seed | – | during 1 | gate for every wave-B/C row; placement rule above |
| 1 | F-acl | 3 | F-object-model | none | 2 | 100k-rule step never beside F-vrf's 100k-prefix step |
| 2 | F-host-acl-nftables | 3 | F-object-model | none | 2 | |
| 3 | F-nat44-ei-64-66-nptv6 | 3 | ED | none | 2 | runs beside #7 through ED's anchors |
| 4 | F-loopback-bvi-gso-lldp-span | 3 | F-bridge-l2 | none | 2 | |
| 5 | P12 | 4↑ | F-vrf-static-ecmp | frr | 2 | head of both longest chains; S2/S3 if M4 |
| 6 | TD-8 agent seams (4 h, new) | 4 | P08, W-seed | none | 2 | Env.Publish/Resync wiring (W-seed Q1), S1, metrics hook |
| 7 | F-det44-map-dslite-cnat | 4 | ED, F-vrf, W-seed-BC | none | 2 | 24 h; det44 enable only in a window |
| 8 | F-tunnels | 4 | F-bridge-l2, W-seed-BC | none | 2 | |
| 9 | F-unbound-chrony-syslog | 4 | – | unbound+chrony+rsyslog | 2 | if not in batch 1 |
| 10 | F-wireguard | 4 | – | none | 2 | |
| 11 | P10 | 6↑ | TD-7 | none (own nspawn) | 2 | network; heads the 60 h S5 chain |
| 12 | F-vrrp-config-sync | 5↑ | P12 | keepalived | 3 | 24 h; VPP-engine steps alone in a window (V22b) |
| 13 | F-pki | 4 | P11 | none | 3 | npm X.509 lib (network) |
| 14 | F-ikev2-native | 4 | P11, F-vrf | strongswan | 3 | |
| 15 | F-ospf | 4 | P12 | frr | 3 | beside P12's other successors only after M3 |
| 16 | F-isis-rip | 4 | P12 | frr | 3 | OSI punt enable is irreversible → window |
| 17 | P14 | 6↑ | P10 | none | 3 | network (ISO ~3 GB), ~10 GB disk |
| 18 | F-mpls-srmpls | 5 | W-seed-BC | none | 3 | table-0 steps only in a window; seeds F-mpls-ldp anchors |
| 19–25 | F-lb, F-qos-flat, F-host-stack, F-snmp (snmpd), F-ipfix-sflow (after M7), F-capture-trace (+F-vrf), F-srv6 | 5 | W-seed-BC | see task | 3 | fill free slots in this order |
| 26 | F-lisp | 5 | W-seed-BC | none | 3 | beside F-tunnels through anchors; V14 → window |
| 27 | F-bfd-redistribution | 4 | F-ospf | frr | 4 | FRR-bfdd step needs no VPP BFD session anywhere |
| 28 | F-ra-vpn | 6 | F-pki, F-vrf | strongswan | 4 | not beside F-ikev2-native unless M3 |
| 29 | F-mpls-ldp | 5 | F-mpls-srmpls, P12, TD-8 | frr | 4 | kernel MPLS absent |
| 30 | F-igmp-mfib | 5 | P12, TD-8 | frr (pimd) | 4 | IGMP host steps alone in a window (V22b) |
| 31 | F-dashboard-prom-alarms | 5 | F-unbound, TD-8 | none | 4 | DB migration regenerated at merge |
| 32 | F-hardening-lite | 6 | P10, F-host-acl-nftables | none (own nspawn) | 4 | network |
| 33–35 | F-aaa (+TD-4), F-licensing (after ui-nav-collapse), F-restconf-yang | 6 | – | none | 4 | npm registry; fill order |
| 36 | F-ha-state-sync | 6 | F-vrrp, EI | none | 5 | EI HA globals in a window, with ED idle |
| 37–38 | F-ab-upgrade, F-images | 6 | P14 | none | 5 | not both building images if free disk < 60 G |
| 39 | F-backup-restore | 6 | F-ab-upgrade | none | 6 | last in the S5 chain |

## 3. Must not run together
- **Daemons:** FRR {P12, F-ospf, F-isis-rip, F-bfd, F-mpls-ldp, F-igmp} one at a time until M3 is logged; a root-netns zebra is always one
  at a time. strongSwan: F-ikev2-native × F-ra-vpn until M3 (P11 comes before both by dep).
- **VPP windows** (the manager serializes them; never two at once): V22b F-vrrp VPP engine and F-igmp IGMP run alone with VPP idle.
  F-det44 det44 enable (V9) · F-isis-rip OSI punt · F-mpls-srmpls/F-mpls-ldp table 0 · F-lb GC (V20 leaks) · F-lisp (V14) · F-ha EI HA
  (never while ED/F-det44 host tests hold nat44-ed) · F-ipfix exporter 0 · F-bfd FRR-bfdd (no VPP BFD session, not during `ci.sh full`) ·
  F-srv6 encap globals · F-capture-trace BPF filter (one pcap per VPP). F-host-stack never changes the session layer.
- **Merge unions** (parallel is allowed only with seeded anchors, and the manager unions at merge): EI × F-det44 (ED's groups) ·
  F-tunnels × F-lisp (`Tunnels` key, TunnelsSchema/TunnelsConfig) · F-isis-rip × F-bfd (IsisInterface blocks) · F-unbound × F-dashboard
  (`Management` key unless dep) · SY4 management.ts: F-aaa, F-backup-restore, F-dashboard, F-unbound · SY3 migrations: F-aaa, F-backup-restore,
  F-dashboard, F-licensing (regenerate, never hand-merge) · SY8 lockfile: P10, F-aaa, F-pki, F-restconf-yang, F-backup-restore.
- F-licensing × ui-nav-collapse (AppShell.tsx) → start F-licensing after ui-nav-collapse merges.

## 4. Manager actions (log each with options in LOG.md; then `python3 tools/board.py`)
- **M1** New row W-seed-BC (manager anchor pass, 3 h, dep W-seed). It seeds every `wave-BC` site in wave-BC-numbers.md, incl. the A4 cases
  (det44 ×2, ikev2_sa, remote_access_disconnect, ha_sync, capture, upgrade/support_bundle) and SY1–SY5. Every wave-B/C row gets it as a dep.
- **M2** New row TD-8 "agent seams" (4 h, deps P08 + W-seed, owns A5 hunks in agent/{agent,service,metrics}.go + subsystems/seams*.go):
  Env.Publish/Resync wiring (W-seed Q1; needed before any EventKind feature merges), S1 dynamic desired source, metrics collector hook.
  Options: (a) TD-8 row (b) the manager writes it (c) first consumer as an A5 exception.
- **M3** Daemon ownership per slot instance (frrtest/swantest: own pathspace, netns, sockets, lock; units stay disabled). Options:
  (a) per instance, root-netns zebra one at a time (recommended) (b) one owner per daemon. With (b), FRR serializes P12→…→F-igmp (~100 h).
- **M4** P12 scope: S3 table-driven routing warning (with wave-BC anchors) + S2 `frr.RegisterInterfaceLines`. Options: (a) in P12
  (coordination 6) (b) TD-8 (c) fallbacks, and the manager edits at merge.
- **M5** Board deps as in §1. **M6** Board `files_owned` ← envelope globs for all 31 rows: stale `project_*.go`; F-ra-vpn narrowed; F-mpls-srmpls
  without the LDP paths; F-mpls-ldp filled; F-capture-trace without `descriptors/bpf_trace_filter`; P10 without apps/api/package.json.
  F-srv6 title/scope without proxies and SRv6-mobile (D-074 binapi note void).
- **M7** NAT44-ED IPFIX logging owner. Options: (a) F-ipfix-sflow gap descriptor in `descriptors/ipfix` (recommended) (b) follow-up row (c) have-not.
- **M8** PENDING-secret-channel parked steps += F-ikev2-native, F-pki, F-ra-vpn, F-snmp, F-ospf, F-isis-rip, F-bfd, F-mpls-ldp, F-host-stack,
  F-vrrp (keepalived auth). New PENDING-cluster-secret-sync (F-vrrp, decision-policy #4).
- **M9** Log: P10 agent unit AF_NETLINK + ReadWritePaths (#4 agent privileges) · vrx-upgrade trigger (`vrx-upgrade@<op>.service`,
  P10/F-ab-upgrade/F-backup-restore) · BFD engine per box (F-bfd default) · critical-path bumps P10/P14/F-vrrp.
- **M10** Before the S5/network rows: `/.scratch/` in .gitignore (SY7); reachability of the mirror, deb.frrouting.org, nodesource, PyPI +
  DPDK tarball, npm, the Go proxy and download.strongswan.org (else PENDING-network); `/srv/vrx-artifacts/{vpp,apt}`; SY9 ci.sh steps;
  `packages/yang/modules` → GEN_PATHS when F-restconf-yang merges.
