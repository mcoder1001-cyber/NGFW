# Task: F-det44-map-dslite-cnat — DET44 CGNAT, MAP-E/T, LW4o6, DS-Lite, 464XLAT, CNAT / PNAT policies   (prepend 00-CONTEXT.md)

## Goal
Carrier-grade and transition translators: deterministic CGNAT (DET44), MAP-E / MAP-T / LW4o6 border relay, DS-Lite AFTR/B4,
464XLAT (documented composition: NAT64 PLAT + MAP-T CLAT, no new object), CNAT translations + SNAT policy, and PNAT policy 1:1.
Reference: TNSR "CGNAT / MAP-T"; VPP plugins `det44`, `map`, `dslite`, `cnat`, `pnat` (WBS D4.4, D4.5, D4.6 in `plan/wbs.csv`).

## Inputs to read first
- `packages/schema/src/domains/nat.ts` + `semantic/nat.ts` + `docs/contracts/schema-nat-objects-acl.md`: `nat.det44{inside, outside,
  insideVrf, outsideVrf, mappings[], timeouts}`, `nat.map{interfaces[], domains[], parameters}` (modes map-e|map-t|lw4o6),
  `nat.dslite{aftr{ipv6,ipv4?}, b4{ipv6,ipv4?}, pools[]}`, `nat.cnat{translations[], snat{policy, addresses, interfaces[], excludePrefixes[]}}`
  exist (D-043 M2/M3, `nat.cnat-valid`, `nat.prefixes-are-networks`). **PNAT has no schema** → contract addition below.
- `docs/agent/descriptors/nat-common.md` first (D-071 globals owner, claims, D-063/D-076 write-only re-apply, D-080 boot identity,
  cnat host lock `/run/lock/vrx-nat-cnat.lock`), then `det44.md`, `map.md`, `cnat.md`, `pnat.md`; packages
  `descriptors/{det44,mapnat,cnat,pnat}` (DF-3) and `natcommon` (shared, read-only for you)
- `apps/agent/binapi/dslite/` — **no DS-Lite descriptor exists** (DF-3 Q5): messages `dslite_set_aftr_addr`/`dslite_get_aftr_addr`,
  `dslite_set_b4_addr`/`dslite_get_b4_addr`, `dslite_add_del_pool_addr_range`/`dslite_address_dump` → build `descriptors/dslite`
- `docs/vpp-code-track.md`: **V9** det44 disable segfaults → det44 is never disabled, host test opt-in `VRX_DF3_DET44=1`;
  **V10** cnat NULL default SNAT entry / `n_paths=0` → guards stay; **V11** pnat lazy-init crash + detach side effects → guarded calls,
  index via `pnat_bindings_get`; D-064 (crash rule, check `systemctl show vpp -p NRestarts` before/after), D-068, D-082

## Contract changes
First, on `contract/F-det44-map-dslite-cnat` (additive, `contract(schema): nat.pnat` + proto + drift guard + `-contract.md`):
`nat.pnat{bindings[]{name, match{proto?, src?, sport?, dst?, dport?}, rewrite{src?, sport?, dst?, dport?}}, attachments[]{binding,
interface, point: input|output}}` (IPv4 only). Tell the manager in the questions file and continue against your branch.

## Scope — build exactly this
Files you own: `apps/agent/internal/descriptors/{det44,mapnat,dslite,cnat,pnat}/**`, `docs/agent/descriptors/{det44,map,dslite,cnat,pnat}.md`,
`apps/agent/internal/agent/project_det44_map_dslite_cnat*.go`, `apps/api/src/features/det44-map-dslite-cnat/**`,
`apps/web/src/domains/firewall/det44-map-dslite-cnat/**`, `apps/web/src/locales/*/det44-map-dslite-cnat.json`,
`docs/user/firewall/det44-map-dslite-cnat.md`, `test/topology/det44-map-dslite-cnat/**`. Shared: one-line appends only.
1. **Agent**: projection `nat.det44` → `det44.{enable,timeouts,interface,map}`; `nat.map` → `map.{domain,rule,params,interface}`
   (LW4o6 = MAP-E domain with ea_bits 0 + per-PSID rules); `nat.dslite` → new `dslite.{aftr,b4,pool}` (aftr/b4 are VPP globals → globals
   owner only, D-071; pool via dump, claim-store ownership like nat64 pools); `nat.cnat` → `cnat.{translation,snat-addresses,snat-policy,
   snat-interface,snat-exclude-prefix,interface-feature}`; `nat.pnat` → `pnat.{binding,attachment}`.
2. **State/actions**: det44 per-user sessions (paged) + close in/out; cnat sessions (paged) + purge (globals owner only);
   det44 forward/reverse lookup as a read-only action (`det44_forward`/`det44_reverse`, confirm in binapi) for CGNAT logging queries.
3. **API**: config via pointer routes; `GET /api/v1/state/nat/det44/sessions?user&page`, `POST /api/v1/actions/nat/det44/lookup`
   (inside↔outside), `GET /api/v1/state/nat/cnat/sessions?page`.
4. **UI**: tabs CGNAT (DET44 + DS-Lite), MAP, CNAT, PNAT on the NAT page (tab hook of F-nat44-ed-sessions); DET44 port-block calculator
   (ports per host from the prefix ratio); en + fa.
5. **Docs**: `docs/user/firewall/det44-map-dslite-cnat.md` — CGNAT sizing, MAP-E vs MAP-T vs LW4o6, DS-Lite, 464XLAT recipe, CNAT
   service VIP, PNAT 1:1; caveats V9/V10/V11; CLI equivalent.

## Acceptance (paste the evidence)
- [ ] DET44: traffic from ns-lan shows the deterministic outside address/port block in ns-wan `tcpdump` (path: af_packet rig); lookup action matches
- [ ] MAP-T (or MAP-E) BR: v4-in-v6 path through the rig translated as configured (path: af_packet rig); `vppctl show map domain` pasted
- [ ] CNAT VIP load-balances to two backends in ns-wan (path: af_packet rig); `show cnat translation` pasted; DS-Lite `show dslite aftr/b4/pool` pasted
- [ ] Agent-restart simulation → all objects back within 30 s; write-only objects re-applied exactly once (fake + host evidence)
- [ ] Rollback removes the owner's objects (Retrieve); det44 stays enabled (V9 policy); VPP `NRestarts` unchanged before/after (pasted)
- [ ] cnat SNAT policy without an SNAT address → 400 with `pointer`; `tools/ci.sh --base main` green; UI screenshot

## Out of scope (do not build)
NAT44-ED/EI, NAT64/66, NPTv6 (F-nat44-ed-sessions, F-nat44-ei-64-66-nptv6); CGNAT IPFIX/syslog export (the exporter is F-ipfix-sflow — only
the det44 lookup API here); load-balancer plugin (F-lb); B4/CE-side DS-Lite beyond the b4 address; MAP pre-resolve / TCP MSS (write-only
in 26.06); HA of any NAT state (F-ha-state-sync); ALGs (V4); changes to `natcommon` (manager).

## Open questions to surface, not to decide silently
DS-Lite AFTR/B4 addresses are VPP globals: confirm they belong to the globals owner only (D-071) — default yes. CNAT and nat44-ed on the
same interface: supported ordering? Default: refuse in semantic validation and document.
