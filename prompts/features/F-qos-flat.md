# Task: F-qos-flat — policers, marking, QoS record/store/map, DSCP/802.1p (flat QoS; HQoS excluded)   (prepend 00-CONTEXT.md)

## Goal
Flat (non-hierarchical) QoS end to end in FAST MODE: policers applied to interfaces, QoS record/store of the DSCP / PCP / EXP bits, egress
marking through translation maps, and a "shaper" that is honest about what VPP can do. Reference: TNSR "QoS: policers, marking";
VPP `policer` + `qos` (record/store/mark/egress-map). WBS D7.8 (policer, shaper, QoS record/map/mark, DSCP/dot1p, HQoS, per-interface queues; T2)
— **HQoS and per-interface queues are V3 (DPDK HQoS scheduler removed from VPP) → excluded; fallback = policer + marking + flat queues.**

## Inputs to read first
- `packages/schema/src/domains/services.ts` — `services.qos` exists (D-052): `policers{<name>: {type 1r2c|1r3c|2r3c-*, rateUnit kbps|pps, cir/eir,
  cb/eb, round, colorAware, conform/exceed/violate actions + dscp}}`, `shapers{<name>: {rateKbps, burstBytes?}}`, `maps{<name>: {id, rows{ext,vlan,
  mpls,ip}}}`, `interfaces{<vpp if>: {policer{input,output}, shaper, record, store{source,value}, mark{output, map}}}`. Extend only additively,
  as separate `contract(schema|proto): …` commits on your task branch (no own branches; numbers from your envelope), if a field is missing.
  Semantic rules that **already exist** in `packages/schema/src/semantic/services.ts`: `services.qos-references` (policers/shapers/maps/
  interfaces exist) and `services.qos-consistency` (map ids unique, marked values fit the header) — do not duplicate them.
- DF-7 descriptors (merged — use, do not rebuild, D-104; `docs/agent/descriptors/policer.md` and `qos.md`): `policer.policer` (Retrieve via
  per-index `policer_dump_v2`), `policer.interface` (**write-only**, applied once per VPP boot identity — D-063/D-076/D-080; never un-apply on
  an interface that had no policer since VPP start: out-of-bounds write in VPP), `policer.bind` (workers only → skip on this host),
  `policer.classify` (write-only; V20 broken `policer_classify_dump`), `qos.record`, `qos.store` (ip source only in 26.06), `qos.egress-map`,
  `qos.mark`. Binapi `apps/agent/binapi/{policer,qos}` (`policer_add/update/del`, `policer_input/output`, `qos_record_enable_disable`,
  `qos_store_enable_disable`, `qos_egress_map_update/delete`, `qos_mark_enable_disable` — verified present).
- `docs/vpp-code-track.md` V3 (HQoS), V20 (policer_classify dump); D-071 (egress-map ids by id range via `df7.WithIDRange` — tests use the
  slot range <SLOT>000–<SLOT>999; no globals).
- Registration: `policer.Register` + `qos.Register` yourself — **never `df7/registry.Register`** (it registers every DF-7 family: lb, mpls,
  span, lldp, bfd, vrrp, igmp belong to other tasks; duplicates panic, D-030). P08 already installs the persisted DF-7 BootStore
  (`df7.SetBootStore`); `policer.classify` gets DF-2's classify store through `Wiring.ClassifyStore()`.

## Scope — build exactly this
Files you own and shared hotspots: your TASK ENVELOPE is authoritative (the board's old `agent/project_qos_flat*.go` became
`internal/desired/qos*.go` + `internal/subsystems/qos*.go` + `internal/agent/rpc_qos*.go`, wave-A hotspots A2). Shared files: registration
lines under your anchor only.
1. **Schema** (semantic, in your own `semantic/qos-flat.ts`; references and map-id uniqueness exist already): `store.source` = ip only
   (VPP 26.06); `mark.map` required with `mark`; an interface uses either `shaper` or `policer.output`, not both (both become an egress policer).
2. **Agent**: project `services.qos` → policer / qos objects (map → mark dependency; marks removed before maps). **Shaper = egress
   policer** (1r2c, cir = rateKbps, burst = burstBytes or ≈10 ms, exceed → drop) named `shaper:<name>`, documented as drop-based rate
   limiting, not queueing (V3). Retrieve for write-only types follows D-063 (skip in verification, re-apply once per boot identity).
3. **API**: pointer routes; `GET /api/v1/state/services/qos/policers` (conform/exceed/violate counters from the stats segment or `policer_dump_v2`),
   `POST /api/v1/actions/qos/policers/{name}/reset` (`policer_reset`).
4. **UI**: Services → QoS: policers (with a small rate/burst helper), marking maps editor (256-entry rows as a compact grid), interface attachments,
   counters column; en + fa; screenshot against the real endpoint.
5. **Docs**: `docs/user/services/qos-flat.md` (ingress policing, DSCP remark via map, "shaper" caveat + V3 note, CLI equivalent).

## Acceptance (paste the evidence)
- [ ] `vppctl show policer` and `show qos egress map` / `show qos mark` reflect the committed config (pasted)
- [ ] Agent-restart simulation → policers, maps and marks back within 30 s; interface policers applied exactly once (no double feature — D-076 test on the fake)
- [ ] Rollback removes policers/maps/marks (Retrieve) and write-only attachments are removed only if applied in this VPP lifetime
- [ ] `store.source: vlan` → 400 problem+json with `pointer`
- [ ] `tools/ci.sh --base main` green in your worktree

## Out of scope (do not build)
HQoS, per-interface queues/schedulers (V3); classifier-based policing UI beyond the `policer.classify` object (classify tables are F-rpf-adl-pbr's);
ACL-matched marking (F-acl); MPLS EXP on label routes (F-mpls-srmpls); worker handoff `policer.bind` tuning (no workers on the host); traffic
generators for rate verification (F-capture-trace PG).

## Open questions to surface, not to decide silently
Whether the product should expose `shapers` at all given V3 (proposed: keep, render as egress policer, label it "rate limit" in the UI).
