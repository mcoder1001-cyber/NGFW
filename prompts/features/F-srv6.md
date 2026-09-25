# Task: F-srv6 — SRv6 policies, local SIDs, steering   (prepend 00-CONTEXT.md)

> Refreshed 2026-09-24 on `task/prep-rest` against main (DF-6 merged), `task/P08`, `task/W-seed` and the VPP 26.06 source: the
> service-chaining proxies are **not buildable** (no binary API, see below). Your TASK ENVELOPE (`docs/status/tasks/F-srv6.envelope.md`)
> wins for branch, files, numbers, anchors and process.

## Goal
Implement **SRv6** end to end in FAST MODE: local SIDs (END, END.X, END.T, END.DX2/DX4/DX6, END.DT4/DT6), SR policies (encap/insert,
weighted SID lists), L2/L3 steering into a BSID, and the encap source / hop limit globals. Reference: TNSR "Segment Routing"; VPP `sr`
(WBS D6.7 in `plan/wbs.csv`; T3 — "only large mobile operators need this").

## Inputs to read first
- Contract first: there is **no** `srv6` config path in `packages/schema` today. Commit it **first on your task branch** as separate
  `contract(schema): routing srv6` / `contract(proto): routing srv6, Srv6State` commits (schema + proto + drift guard, additive after
  contracts-v1; never a `contract/` branch): `routing.srv6{encapSource, encapHopLimit, localSids{<sid>: {behavior, psp?, vrf?, interface?,
  nextHop?, lookupVrf?}}, policies{<bsid>: {type default|spray|tef, encap: bool, vrf?, sidLists[{sids[≤16], weight}]}}, steering[{l2Interface|
  {prefix, vrf}}, bsid}]}`. The config home is decided (`docs/status/wave-BC-numbers.md`): `routing.srv6`, RoutingConfig **17** — say so
  in F-srv6-questions.md and continue.
- Merged DF-6: `apps/agent/internal/descriptors/sr/` + `docs/agent/descriptors/sr.md` — `sr.localsid/<sid>`, `sr.policy/<bsid>`,
  `sr.steering/…`, globals `sr.encap-source`, `sr.encap-hop-limit` (write-only, **globals owner only**; `sr.Register` builds the setters
  only with `df6.WithGlobalsOwner(true)`); ClaimStore ownership (untagged objects, `df6.WithClaims(<Wiring.IfaceClaims()>)`); every encap
  policy carries its own `encap_src` — the projection fills it from `routing.srv6.encapSource` (or a per-policy override if you model one).
- `apps/agent/binapi/sr/`, `sr_types` — only source of names. Counters come from `sr_localsids_with_packet_stats_dump`.
- **SRv6 proxies (End.AD / End.AM / End.AS) cannot be configured**: in VPP 26.06 `src/plugins/srv6-{ad,am,as}` have no `.api` file, their
  behaviours are not in `sr_types.api`'s behaviour enum and exist only as CLI (`sr localsid … behavior end.ad …`). There is nothing for
  `binapi-generator` to generate (the D-074 note "the manager generates srv6_ad/am/as binapi" cannot be done), and CLI is forbidden (only
  D-090's fixed-string lb cleanup is allowed). Not built: write `### V-new (F-srv6)` and list them as have-not in F-srv6.md.
- `docs/decisions/LOG.md` D-074 (every encap policy names its source; delete checks existence first; srv6-mobile not built), D-071
  (encap source/hop limit are globals), D-076/D-080 (write-only globals applied once per boot identity), D-082 (globals lock in tests)
- `docs/vpp-code-track.md` V15 (FIB table delete leaks routes — delete own routes/steering before the VRF), V19

## Scope — build exactly this
1. **Schema**: semantic rules — SIDs are IPv6 host addresses, unique; behaviour-specific required fields (END.X needs interface + nextHop,
   END.DT4/DT6 need lookupVrf); encap policies require `encapSource` (D-074), insert policies forbid it; steering BSID must exist; ≤ 16 SIDs.
2. **Agent**: projection → DF-6 `sr.*` descriptors. Globals only on the globals owner. Retrieve covers every object (the globals are
   write-only). ONE integration check on the host VPP (`VRX_INTEGRATION=1`, shared lock, slot table range, prefixed objects).
3. **API**: config via pointer routes; `GET /api/v1/state/srv6` (localsids with counters from `sr_localsids_with_packet_stats_dump`,
   policies, steering) through the `Srv6State` RPC. OpenAPI; regenerate `packages/api-client`.
4. **UI**: SRv6 tab on the VPN page (`vpnTabs` registry, W-seed shell) with Local SIDs / Policies / Steering sub-tabs, SchemaForm,
   SID-list editor, counters column; en + fa; screenshot.
5. **Docs**: `docs/user/vpn/srv6.md` — L3VPN over SRv6 (END.DT4 + encap policy + steering) example, CLI equivalent, the proxy limit.
Files you own: see the envelope (`descriptors/sr/**` gap-only, `desired/srv6*.go`, `subsystems/srv6*.go`, `agent/rpc_srv6*.go`,
`ext/srv6*.ts`, `semantic/srv6*.ts`, `features/srv6/**`, `domains/vpn/srv6/**`, `srv6.json`, `docs/user/vpn/srv6.md`, `test/topology/srv6/**`).
Shared files: one line under your `// wave-BC: F-srv6` anchor only; `descriptors/df6/**` read-only.

## Acceptance (paste the evidence)
- [ ] After commit `Retrieve()` == desired; `vppctl show sr localsids`, `show sr policies`, `show sr steering-policies` contain them
- [ ] Agent-restart simulation → objects back within 30 s; claimed objects not re-added (log excerpt)
- [ ] Rollback removes steering → policies → SIDs in order, no leaked routes in the slot tables (Retrieve + `show ip6 fib table <t>`)
- [ ] Encap policy without `encapSource` → 400 problem+json with a `pointer`
- [ ] `tools/ci.sh --base main` green in your worktree

## Out of scope (do not build)
**SRv6 proxies** End.AD/AM/AS (no binary API in 26.06 — V-new, have-not). **SRv6-mobile** (GTP4.D/E, GTP6.*): D-074 — not built (no
delete message, clashes with `sr.policy`); record it as a vpp-code-track candidate in your status file, do not model it. uSID (`un`/`ua`,
`sr_localsid_add_del_v2` locators) and path tracing (`sr_pt`); SR-MPLS and MPLS (F-mpls-srmpls); BGP/IS-IS SRv6 signalling (P12,
F-isis-rip); tunnels (F-tunnels); LISP (F-lisp).

## Open questions to surface, not to decide silently
Whether the product needs service chaining at all (it would need a VPP API patch — a V-item, not FAST MODE work); whether a per-policy
encap source override is worth modelling beside the global `encapSource`.
