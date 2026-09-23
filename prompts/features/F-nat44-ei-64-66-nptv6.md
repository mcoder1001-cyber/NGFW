# Task: F-nat44-ei-64-66-nptv6 — NAT44-EI, NAT64, NAT66, NPTv6   (prepend 00-CONTEXT.md)

## Goal
The remaining stateful/stateless translators next to NAT44-ED: endpoint-independent NAT44 (`nat.mode: ei`), NAT64 (incl. the
PLAT side of 464XLAT), NAT66 and NPTv6 (RFC 6296, `npt66` plugin — loaded since D-060). Reference: TNSR "NAT44 (EI), NAT64,
NPTv6"; VPP plugins `nat44_ei`, `nat64`, `nat66`, `npt66` (WBS D4.2, D4.3 in `plan/wbs.csv`).

## Inputs to read first
- `packages/schema/src/domains/nat.ts` + `semantic/nat.ts` (P02b): the top level of `nat` is NAT44 (`mode: ed|ei`, inside, outside,
  pools, staticMappings, identityMappings, timeouts, ipfix); `nat.nat64{prefixes, pools, staticBibs, inside/outside, timeouts}`,
  `nat.nat66{inside/outside, staticMappings}`, `nat.nptv6.bindings[]{interface, internal, external}` all exist; D-043, D-062
  (`isNat44Enabled()`, never read `nat.enabled` raw), rule `nat.prefixes-are-networks`
- `docs/agent/descriptors/nat-common.md` (**read first** — D-071 globals owner, claims, write-only re-apply, fixtures), `nat44-ei.md`,
  `nat64.md`, `nat66.md`; packages `descriptors/{nat44ei,nat64,nat66}` and `natcommon` (shared, **read-only** for you)
- `apps/agent/binapi/npt66/` — only `npt66_binding_add_del` exists: **no dump** → the new descriptor is write-only (D-063); check whether
  a second add errors or duplicates and, if so, use the D-076 applied-once record keyed by the D-080 boot identity (`natcommon.BootIdentity`)
- `prompts/features/F-nat44-ed-sessions.md` — sibling task that owns the NAT page, `nat44ed` and the ED projection; ED and EI are
  mutually exclusive on one VPP (`ErrOtherVariant`) — the integration tests serialise on the slot's nat44 lock
- D-060 (npt66 loaded; NPTv6 no longer skipped), D-064 (crash rule), D-082 (globals lock)

## Contract changes
Likely none. If needed (e.g. `nat.nat64.interfaceAddress`), additive on `contract/F-nat44-ei-64-66-nptv6`; tell the manager and continue.

## Scope — build exactly this
Files you own: `apps/agent/internal/descriptors/{nat44ei,nat64,nat66,npt66}/**`, `docs/agent/descriptors/{nat44-ei,nat64,nat66,npt66}.md`,
`apps/agent/internal/agent/project_nat44_ei_64_66_nptv6*.go`, `apps/api/src/features/nat44-ei-64-66-nptv6/**`,
`apps/web/src/domains/firewall/nat44-ei-64-66-nptv6/**`, `apps/web/src/locales/*/nat44-ei-64-66-nptv6.json`,
`docs/user/firewall/nat44-ei-64-66-nptv6.md`, `test/topology/nat44-ei-64-66-nptv6/**`. Shared: one-line appends only (app.module,
router/nav — as extra tabs registered into F-nat44-ed-sessions' NAT page through its tab hook, or a sibling route if it has none).
1. **Agent**: projection for `nat.mode: ei` → `nat44-ei.{enable,timeouts,forwarding,interface-feature,output-feature,
   interface-address,address-pool,static-mapping,identity-mapping}`; `nat.nat64` → `nat64.{enable,timeouts,prefix,pool,interface,
   static-bib}`; `nat.nat66` → `nat66.{enable,interface,static-mapping}`; new `descriptors/npt66` → `npt66.binding/<interface>/<internal>`
   (write-only, idempotency proven in a fake that models duplicate add). Globals only by the globals owner (D-071); slots require them.
2. **State/actions**: EI users + sessions (`nat44ei` `Users`/`UserSessions`, paged), `DeleteSession`; NAT64 sessions (`nat64.Sessions`, paged).
3. **API**: config via pointer routes; `GET /api/v1/state/nat/ei/sessions?page&filter`, `DELETE …/ei/sessions/{id}`,
   `GET /api/v1/state/nat/nat64/sessions?page`, `GET /api/v1/state/nat/nptv6` (desired bindings, write-only marker).
4. **UI**: tabs EI (shown when `mode: ei`), NAT64, NAT66, NPTv6 with SchemaForm + session grids; en + fa.
5. **Docs**: `docs/user/firewall/nat44-ei-64-66-nptv6.md` — EI vs ED, NAT64 + DNS64 note, 464XLAT PLAT, NPTv6 example; CLI equivalent.

## Acceptance (paste the evidence)
- [ ] EI: `ip netns exec ns-<p>-lan` traffic → `tcpdump` in ns-wan shows the pool address (path: af_packet rig); `vppctl show nat44 ei sessions` pasted
- [ ] NAT64: v6 client in ns-lan reaches a v4 host in ns-wan via `64:ff9b::/96` (path: af_packet rig); `show nat64 session table all` pasted
- [ ] NPTv6: `tcpdump` in ns-wan shows the translated external prefix (path: af_packet rig); NAT66 static mapping `show nat66 static mappings` pasted
- [ ] Agent-restart simulation → all four translators back within 30 s; npt66 binding not duplicated (evidence from the fake + host)
- [ ] Rollback removes interfaces/pools/prefixes/bindings (Retrieve); plugins stay enabled if any other owner has objects (nat-common rule)
- [ ] `nat.mode: ei` together with ED-only fields (twice-NAT, LB mappings) → 400 with `pointer`; `tools/ci.sh --base main` green; UI screenshot

## Out of scope (do not build)
NAT44-ED, its session browser and the NAT page frame (F-nat44-ed-sessions); DET44, MAP-E/T, DS-Lite, LW4o6, CNAT, PNAT
(F-det44-map-dslite-cnat); NAT HA / `nat44_ei_ha_*` (F-ha-state-sync); NAT IPFIX exporter object (F-ipfix-sflow); ALGs (V4, unsupported);
DNS64 in Unbound (F-unbound-chrony-syslog); changes to `natcommon` (manager).

## Open questions to surface, not to decide silently
npt66 has no dump: is a write-only NPTv6 binding acceptable to the product owner for restart claims, or should it be a V-item
(`npt66_binding_dump`, ~0.5 day)? Default: write-only + propose a V22 entry in the questions file.
