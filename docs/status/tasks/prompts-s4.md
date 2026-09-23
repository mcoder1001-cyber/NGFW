# prompts-s4 — 42 S4/S5 feature prompts + files_owned

Manager-assistant task (branch `task/prompts-s4`, docs-only). No product code, no VPP, no daemons touched.

## Files generated (prompts/features/, from FEATURE-TEMPLATE per MANAGER-PROMPT §6)
- `prompts/features/F-aaa.md` (58 lines)
- `prompts/features/F-ab-upgrade.md` (50 lines)
- `prompts/features/F-acl.md` (62 lines)
- `prompts/features/F-backup-restore.md` (57 lines)
- `prompts/features/F-bfd-redistribution.md` (53 lines)
- `prompts/features/F-bonding.md` (59 lines)
- `prompts/features/F-bridge-l2.md` (58 lines)
- `prompts/features/F-capture-trace.md` (54 lines)
- `prompts/features/F-dashboard-prom-alarms.md` (57 lines)
- `prompts/features/F-det44-map-dslite-cnat.md` (60 lines)
- `prompts/features/F-hardening-lite.md` (50 lines)
- `prompts/features/F-ha-state-sync.md` (60 lines)
- `prompts/features/F-host-acl-nftables.md` (58 lines)
- `prompts/features/F-host-stack.md` (59 lines)
- `prompts/features/F-igmp-mfib.md` (50 lines)
- `prompts/features/F-ikev2-native.md` (59 lines)
- `prompts/features/F-images.md` (47 lines)
- `prompts/features/F-ipfix-sflow.md` (52 lines)
- `prompts/features/F-isis-rip.md` (52 lines)
- `prompts/features/F-kea-dhcp-relay.md` (55 lines)
- `prompts/features/F-lb.md` (52 lines)
- `prompts/features/F-licensing.md` (49 lines)
- `prompts/features/F-lisp.md` (54 lines)
- `prompts/features/F-loopback-bvi-gso-lldp-span.md` (63 lines)
- `prompts/features/F-mpls-srmpls.md` (54 lines)
- `prompts/features/F-nat44-ei-64-66-nptv6.md` (55 lines)
- `prompts/features/F-neighbors-ra.md` (60 lines)
- `prompts/features/F-object-model.md` (61 lines)
- `prompts/features/F-ospf.md` (56 lines)
- `prompts/features/F-pki.md` (54 lines)
- `prompts/features/F-qos-flat.md` (50 lines)
- `prompts/features/F-ra-vpn.md` (55 lines)
- `prompts/features/F-restconf-yang.md` (52 lines)
- `prompts/features/F-rpf-adl-pbr.md` (59 lines)
- `prompts/features/F-sdk-terraform-ansible.md` (49 lines)
- `prompts/features/F-snmp.md` (53 lines)
- `prompts/features/F-srv6.md` (50 lines)
- `prompts/features/F-tunnels.md` (56 lines)
- `prompts/features/F-unbound-chrony-syslog.md` (56 lines)
- `prompts/features/F-vrf-static-ecmp.md` (63 lines)
- `prompts/features/F-vrrp-config-sync.md` (54 lines)
- `prompts/features/F-wireguard.md` (57 lines)

Each prompt names: the existing schema path vs the additive `contract/<id>` fields it needs; the descriptors/renderers to reuse by
path (main, or read-only from task/DF-5, task/DF-7, task/RF-2, task/RF-4); the relevant V-items with their fallback; the D-ids that
constrain it; a "Files you own" line identical to the board's `files_owned` (plus docs/status/tasks/<id>*.md); a mandatory, id-specific
"Out of scope" fence. Packet-level acceptance lines only in NAT (nat44-ei-64-66-nptv6, det44-map-dslite-cnat, ha-state-sync), IPsec
(ikev2-native, ra-vpn), BGP→FIB-like (ospf, isis-rip, bfd-redistribution) and VRRP (vrrp-config-sync).

## Ownership layout used for `files_owned`
Per feature `<slug>` (id without `F-`) in nav group `<G>`: `apps/agent/internal/agent/project_<slug_>*.go` (projection registered from
init()/P08 hook), `apps/agent/internal/actions/<slug>/**` (only features with actions), `apps/api/src/features/<slug>/**`,
`apps/web/src/domains/<G>/<slug>/**`, `apps/web/src/locales/*/<slug>.json`, `docs/user/<G>/<slug>.md`, `test/topology/<slug>/**`,
`docs/status/tasks/<id>*.md`, plus each feature's descriptor/renderer dirs (exactly one owner each).
**Owned by no feature** (shared, contract or generated): `packages/schema/**`, `packages/proto/**` (contract branches only),
`packages/api-client/**` (regenerated), web frame (shell, nav.ts, router.tsx), apps/api generic controllers, `apps/agent/binapi/**`,
helper packages `descriptors/{dfkit,df2,df6,df7,natcommon,interface}`, `core/{loopback,ifaddr}`, frr framework files,
`renderers/{renderer.go,helpers_*,ALLOWLIST.md}` — features make one-line appends there, resolved by the manager at merge.
Deviations from the first draft found by the prompt writers: `ip6_dad/` dropped from F-neighbors-ra (DAD is `ip6-nd.dad` in DF-2's ip6_nd);
`actions/<slug>/**` added for F-ikev2-native, F-wireguard, F-ra-vpn; `test/topology/<slug>/**` added for the S5 rows.

## Overlap check (pairwise, brace-expanded globs, witness-string matching; script in the manager-assistant scratchpad)
Among the 42 rows:
```
rows checked: 42  globs: 450  pairs: 861  sequential(dep-chain) overlaps ignored: 0  concurrent overlaps: 0
skipped (placeholder/except globs): 
```
All non-merged rows with concrete globs (overlaps along a dependency chain are sequential and ignored):
```
OVERLAP P07b <-> F-restconf-yang: [('apps/web/**', 'apps/web/src/domains/system/restconf-yang/**'), ('apps/web/**', 'apps/web/src/locales/*/restconf-yang.json')]
OVERLAP P07b <-> F-aaa: [('apps/web/**', 'apps/web/src/domains/system/aaa/**'), ('apps/web/**', 'apps/web/src/locales/*/aaa.json')]
OVERLAP P07b <-> F-licensing: [('apps/web/**', 'apps/web/src/domains/system/licensing/**'), ('apps/web/**', 'apps/web/src/locales/*/licensing.json')]
OVERLAP P08 <-> F-restconf-yang: [('apps/**', 'apps/api/src/features/restconf-yang/**'), ('apps/**', 'apps/web/src/domains/system/restconf-yang/**'), ('apps/**', 'apps/web/src/locales/*/restconf-yang.json')]
OVERLAP P08 <-> F-aaa: [('apps/**', 'apps/api/src/features/aaa/**'), ('apps/**', 'apps/web/src/domains/system/aaa/**'), ('apps/**', 'apps/web/src/locales/*/aaa.json')]
OVERLAP P08 <-> F-backup-restore: [('apps/**', 'apps/api/src/features/backup-restore/**'), ('apps/**', 'apps/web/src/domains/system/backup-restore/**'), ('apps/**', 'apps/web/src/locales/*/backup-restore.json')]
OVERLAP P08 <-> F-licensing: [('apps/**', 'apps/api/src/features/licensing/**'), ('apps/**', 'apps/web/src/domains/system/licensing/**'), ('apps/**', 'apps/web/src/locales/*/licensing.json')]
rows checked: 46  globs: 458  pairs: 1035  sequential(dep-chain) overlaps ignored: 109  concurrent overlaps: 7
skipped (placeholder/except globs): DF-5 DF-7 RF-2 RF-4
```
The 7 remaining hits are S5 rows (F-aaa, F-licensing, F-restconf-yang, F-backup-restore) that depend only on P06 (+P07b) while P07b owns
`apps/web/**` and P08 owns `apps/**`. Fix either by the deps below or by narrowing P08's `files_owned` (it is broader than P08 needs:
`apps/agent/internal/agent/** apps/api/src/{state,…}/** apps/web/src/domains/interfaces/basics/**` would do). Placeholder rows
(DF-5, DF-7, RF-2, RF-4: `<plugins of DF-n>`) cannot be checked; they are all predecessors of the features that take over their dirs.

## Suggested dependency changes (not applied — manager decides; also noted in each row's `notes:`)
| task | add dep | why |
|---|---|---|
| F-loopback-bvi-gso-lldp-span | F-bridge-l2 (soft: F-tunnels) | BVI membership uses F-bridge-l2's l2 descriptor; ERSPAN rides a GRE tunnel |
| F-tunnels | F-bridge-l2 | TEB GRE / VXLAN bridge-domain membership |
| F-rpf-adl-pbr | DF-4 | ABF policies resolve ACLs by name |
| F-acl | F-rpf-adl-pbr (soft) | ADL/Auto-SDL screens it links to (D5.5 duplicated with D2.4 — F-rpf-adl-pbr owns the descriptors) |
| F-host-acl-nftables | F-object-model | host rules expand objects through `internal/objects` |
| F-nat44-ei-64-66-nptv6 | F-nat44-ed-sessions | shared NAT page; ED and EI are mutually exclusive on one VPP |
| F-det44-map-dslite-cnat | F-nat44-ed-sessions | NAT page tab hook |
| F-ha-state-sync | F-nat44-ei-64-66-nptv6 | only `nat44_ei_ha` exists (V2) |
| F-ikev2-native | P11 (soft: F-pki) | ipsec itf / tunnel-protect / SA state are P11/DF-5's; cert auth |
| F-bfd-redistribution | F-ospf | acceptance redistributes static into OSPF |
| F-vrrp-config-sync | P12 | keepalived path needs linux-cp pairs (RF-4 renderer rejects instances without them) |
| F-host-stack | F-startup-gen | TCP/UDP buffer tuning exists only in startup.conf |
| F-capture-trace | F-vpp-debs | tracedump/tracenode not built (V18); binapi regen by manager afterwards |
| F-dashboard-prom-alarms | F-startup-gen (soft) | only if the VPP prom plugin path is chosen over the agent exporter |
| F-backup-restore | F-ab-upgrade, P08 | upgrade UI (D8.7) drives `vrx-upgrade`; overlap check |
| F-aaa, F-licensing, F-restconf-yang | P07b, P08 | overlap check (web frame / `apps/**`) |
| F-ab-upgrade, F-images | P14 | rootB partition layout / autoinstall + offline pool |
| F-ipfix-sflow | (soft) hsflowd packaging in P10 or a follow-up | VPP sflow samples but does not export to collectors |

Other board fixes: F-ha-state-sync `template:` was `FEATURE-TEMPLATE.md` → `prompts/FEATURE-TEMPLATE.md` (applied).
Doc inconsistencies seen: P11's out-of-scope says `F-pki-basic` (board id F-pki), RF-2's docs say `F-pki-basic`/`F-remote-access`;
`docs/agent/descriptors/lisp.md` numbers bugs V9/V10 which are V13/V14 in docs/vpp-code-track.md (prompts use the code-track ids).
New V-items proposed by prompts (to be added by the worker/manager when hit): npt66 has no dump (write-only), IPsec SA state sync impossible via
API (no seq/replay in `ipsec_sad_entry_update`), SRv6-mobile (D-074), traceroute has no VPP API, ikev2 ip-id truncation (DF-5 Q2).

## Scope that looks infeasible in FAST MODE / 10 h
| task | problem | what the prompt does |
|---|---|---|
| F-host-stack | D7.10 is 130 PD, T3 | scoped to session enable, app namespaces, session rules, TCP source addrs; http_static opt-in; VCL/TLS/QUIC/HTTP3/SRTP/HSI fenced → have-not in STATUS-FINAL |
| F-mpls-srmpls | no MPLS schema (big contract), new LDP→VPP sync (V5), MPLS table 0 global, ldpd needs kernel MPLS modules not loadable on the shared host | suggest split: static MPLS/SR-MPLS vs LDP sync; LDP evidence fake-client only |
| F-vrrp-config-sync | two VRRP engines + API↔API config sync + cluster UI with two instances | 14–16 h, or split D9.2/D9.5 into their own task |
| F-det44-map-dslite-cnat | five families, new dslite descriptor, `nat.pnat` contract, V9/V10/V11 guards | ~14–16 h; suggest splitting CNAT+PNAT |
| F-ha-state-sync | NAT HA is VPP-global (one host cannot be both nodes); IPsec SA sync impossible via API | partial: listener/failover config + re-key-on-failover fallback; two-node test written, deferred until vrx-b |
| F-srv6 | SRv6-mobile not built (D-074); proxies need manager binapi | mobile out, End.AD only |
| F-lisp | V14 leaks → host test opt-in only | mostly fake-client evidence unless a manager VPP window |
| F-capture-trace | classic trace and PG have no binary API (only `cli_inband`); Trace Path no API | pcap + BPF filter; `cli_inband` use is a manager decision |
| F-restconf-yang | NETCONF | RESTCONF + generated YANG only; NETCONF have-not |
| F-aaa | 6 methods + MFA | order RADIUS → TOTP → LDAP → OIDC → TACACS+ → SAML, stop at time box |
| F-sdk-terraform-ansible | three SDKs | Python SDK → Terraform (generic `vrx_config` + `vrx_interface`) → Ansible likely left over |
| F-backup-restore | five sub-features | bulk provisioning fenced as have-not |
| F-ab-upgrade, F-images | no KVM/cloud on this host (D-002) | loop-device / build-only evidence; boots deferred |
| F-igmp-mfib | PIM→mFIB sync (V5) new code, no multicast schema, BIER T3 | BIER optional last step; expect partial |
| F-isis-rip | IS-IS needs CLNS frames through linux-cp (V1) | may ship without FIB proof, skip-with-reason |
| F-tunnels | seven tunnel types; V8/V14 | GTP-U/L2TPv3/PPPoE last / drop first, fake-only tests |
| F-loopback-bvi-gso-lldp-span | five WBS items, LLDP/SPAN still on task/DF-7 | tight; LLDP neighbours may not be readable via API |

## Verification
- `python3 tools/board.py` → `board ok: 82 tasks; progress 23.9% by hours, 20/82 merged; ready=0 running=6 parked=0` (PROGRESS.md unchanged)
- `tools/ci.sh --base main` → see below
