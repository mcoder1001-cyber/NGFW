# Task: F-igp-followups-a — IGP + VRRP-state contract: ospf6, OSPF/IS-IS/RIP auth refs, ripng, rip.version, IS-IS families, EventKind 17/20/21, VrrpState RPC, cluster.syncExclude   (prepend 00-CONTEXT.md)

> Split 1 of 6 of board row F-igp-followups (D-173; split by M-prompts — the manager names the board rows). Contract only:
> -b/-c (FRR agent), -d (VRRP state), -e (API state + UI), -f (config sync) build on it.

## Goal
Add, **additively**, every schema/proto field the RV-A scope cuts need, with the numbers already allocated in
`docs/status/wave-BC-numbers.md` (§ F-ospf, § F-isis-rip, § F-vrrp-config-sync). No behaviour: the agent/API/UI rows consume it.
Reference: TNSR "OSPFv3", "RIPng", "IS-IS", "VRRP status"; FRR 10.7 `ospf6d`/`ripngd`/`isisd` auth commands (WBS D2.3, D2.4, D9.1, D9.2).

## Inputs to read first
- `docs/status/tasks/F-ospf.md`, `F-isis-rip.md`, `F-vrrp-config-sync.md` ("Not built"), `docs/status/tasks/RV-A-review.md` (Disposition, F-igp-followups bullet)
- `docs/status/wave-BC-numbers.md` § F-ospf / § F-isis-rip / § F-vrrp-config-sync (numbers, message names, schema file names, test-secret names)
- `packages/schema/src/domains/routing.ts` (anchors `// wave-BC: F-ospf` :567/:723, `// wave-BC: F-isis-rip` :619/:629/:637/:653/:724),
  `packages/schema/src/domains/ha.ts` (anchors `// wave-BC: F-vrrp-config-sync` :90/:185/:227), `packages/schema/src/semantic/{isis-rip,ha}.ts`,
  `semantic/index.ts` + `src/index.ts` (anchored import/spread lines)
- `packages/proto/ngfw/v1/dataplane.proto`: `service Dataplane` anchor :90, `EventKind` anchors :703-705, `RoutingConfig` :1342-1343,
  `OspfInterface` :1753, `IsisInterface`/`IsisConfig`/`RipInterface`/`RipConfig` :1786/:1803/:1811/:1827, `VrrpInstance`/`HaCluster` :2727/:2769,
  the end-of-file `// ----- F-ospf -----` / `F-isis-rip` / `F-vrrp-config-sync` sections; `docs/contracts/proto.md` "Feature RPCs"
- `docs/04-api-datamodel.md` (secret references `<kind>/<name>`, D-051), D-173 in `docs/decisions/LOG.md`

## Contract changes
This row **is** the contract: commit each package as its own `contract(schema): …` / `contract(proto): …` commit on your task branch,
write `docs/status/tasks/F-igp-followups-a-contract.md` (field table, numbers, why additive) and tell the manager in
`docs/status/tasks/F-igp-followups-a-questions.md`. Renaming/reshaping an existing field is not allowed (PENDING, decision-policy #1).

## Scope — build exactly this
1. **Proto** (numbers exactly as allocated, append-only, under the existing anchors):
   `RoutingConfig` 13 `ospf6` → `Ospf6Config{router_id, vrf, areas, interfaces, redistribute}`, `Ospf6Area`, `Ospf6Interface`, `Ospf6Redistribute{connected, static, bgp, isis, ripng}`;
   `OspfInterface` 9 `auth` → `OspfInterfaceAuth{type "md5"|"none", key_id, key_ref}`; `RoutingConfig` 14 `ripng` → `RipngConfig`, `RipngInterface`,
   `RipngRedistribute{connected, static, bgp, isis, ospf6}`; `IsisConfig` 6 `area_password_ref`, 7 `domain_password_ref`; `IsisInterface` 6 `ipv4`, 7 `ipv6`
   (explicit presence, default both on); `RipConfig` 6 `version` (2 only); `RipInterface` 2 `auth` → `RipInterfaceAuth{key_ref}` (one key, plain MD5 key);
   `EventKind` 17 `EVENT_KIND_VRRP_STATE_CHANGED`, 20 `EVENT_KIND_OSPF_NEIGHBOR_CHANGED` (v2 and v3), 21 `EVENT_KIND_ISIS_ADJACENCY_CHANGED`;
   `rpc VrrpState(VrrpStateRequest) returns (VrrpStateResponse)` + `VrrpState*` messages (per VR: name, interface, vr_id, family, engine,
   state Init|Backup|Master|Fault, priority in use, master address, last transition time, transitions count); `HaCluster` 10 `sync_exclude` (repeated JSON pointers).
   No `OspfState`/`IsisRipState` RPC: P12's `RoutingState.readers` already returns the registered FRR readers by key (`dataplane.proto:274-277`).
2. **Schema** (Zod, one schema → types/OpenAPI/JSON Schema): `packages/schema/src/domains/ext/ospf.ts` (`Ospf6Schema`, `OspfInterfaceAuthSchema`) and
   `ext/isis-rip.ts` (`RipngSchema`, IS-IS password refs, family switches, `RipSchema.version` literal 2, `RipInterfaceAuthSchema`), key lines only under the
   anchors in `routing.ts`; `HaClusterSchema.syncExclude` in `ha.ts` under its anchor. Every `*Ref` is a D-051 secret reference (`password/<name>`), never a value.
3. **Semantic rules** (ids `routing.ospf-…`, `routing.isis-rip-…`, `ha.vrrp-config-sync-…`; never re-add existing ids): ospf6 interface area exists;
   ospf6/ripng interfaces exist; OSPF auth `md5` needs `key_id` 1–255 and `key_ref`; `none` forbids both; IS-IS interface with `ipv4: false` and `ipv6: false` →
   error; secret refs point at an existing secret (reuse the existing `secrets.ref-exists` path if the API tier owns it — do not duplicate); `syncExclude`
   entries are valid JSON pointers and never `/ha/cluster` itself. Each rule → 400 problem+json with `pointer`.
4. **Generated outputs**: `pnpm gen` (Go/TS stubs, OpenAPI, JSON Schema, `packages/api-client`, `packages/yang/generated` if it changes) — never hand-edit.
5. **Docs**: `docs/contracts/proto.md` sections `### F-igp-followups-a: VrrpState` + the new fields; examples under `packages/schema/examples/` (one valid
   document using every new field, one invalid per rule).

## Acceptance (paste the evidence)
- [ ] `pnpm --filter @ngfw/schema exec vitest run <your new/changed test files>` green (paste); every rule has a failing-input test with its pointer
- [ ] `go test ./internal/contracttest/...` in apps/agent green (schema ↔ proto drift guard) — paste
- [ ] `git diff main --stat -- packages/proto packages/schema` shows only additions under the anchors (paste); generated files reproduce with no diff after `pnpm gen`
- [ ] An API validation of the invalid examples returns 400 problem+json with the expected `pointer` (unit level through the schema validator is enough; paste)
- [ ] `tools/ci-slot.sh --base main` green in your worktree (tail pasted)

## Out of scope (do not build)
FRR rendering of the new fields (-b, -c), VrrpState/events implementation (-d), API state routes and UI (-e), config sync (-f); `HaConfig` 4 /
`VrrpInstance` 15 keepalived stand-ins (D-086) and VRRPv2 auth; BFD fields (F-bfd-redistribution); `lcp.osi-proto`; any secret-channel work
(PENDING-secret-channel); renaming or moving any existing field.

## Open questions to surface, not to decide silently
Whether RIP auth stays one plain key (FRR key-chain) or needs a key-chain object; whether `Ospf6Config` also needs `default_information_originate`
(TNSR has it) — only with a proven need, numbers from your own `###` section in wave-BC-numbers.md.

## Files you own
`packages/schema/src/domains/ext/{ospf,isis-rip}.ts` (+ tests, new), `packages/schema/src/domains/routing.ts` and `ha.ts` (lines under the F-ospf /
F-isis-rip / F-vrrp-config-sync anchors only), `packages/schema/src/semantic/{ospf,isis-rip,ha}*.ts` (+ tests), `packages/schema/src/semantic/index.ts`
and `src/index.ts` (anchored lines), `packages/schema/examples/{ospf6,ripng,isis-auth,vrrp-state,cluster-sync}*.json` (new),
`packages/proto/ngfw/v1/dataplane.proto` (additive) + generated stubs, `packages/api-client/**` (generated), `docs/contracts/proto.md` (your sections),
`docs/status/tasks/F-igp-followups-a*`.

## Rules
- Files you own: above. Everything else read-only; a needed edit elsewhere → `docs/status/tasks/F-igp-followups-a-questions.md`.
- Shared VPP: not needed by this row; if you touch it anyway: slot prefix `w<N>`, tables N000–N999 (`eval "$(tools/lab env <N>)"`), never restart or
  kill VPP, `timeout 10` on every vppctl, packet trace banned (D-128). Daemons: none.
- D-210a: write tests for your change and get them passing in your package (paste output); run them through `tools/heavy.sh` (D-224), e.g. `../../tools/heavy.sh go test ./internal/<pkg>/...` from apps/agent, `tools/heavy.sh pnpm --filter <pkg> …` from the repo root; no full suite, no lint, no other packages' tests.
- Contracts: additive only — `contract(schema|proto): …` commits first, numbers only from `docs/status/wave-BC-numbers.md`; renaming/reshaping = PENDING.
- Finish: `tools/ci-slot.sh --base main` green (compile-only gate, D-220/D-222; never `tools/ci.sh` directly), commit on your branch,
  `docs/status/tasks/F-igp-followups-a.md` with pasted real output.
