# Task: F-bridge-l2 — bridge domains, L2XC/L3XC, split-horizon, MAC aging, time-range MAC filter   (prepend 00-CONTEXT.md)

> Checked 2026-09-24 against main + P08 (wave-A prep). The l2 and l3xc descriptors already exist (DF-1). P08 already provides the
> builder, the registry, the projection hook, the live-state RPC pattern and the topology-test pattern. This task builds the L2 model,
> its projection, the `mactime` descriptor, the L2 state RPCs, the API module, the screen, the tests and the docs.
> Tag rewrite on L2 members is **yours**: F-vlan-qinq's regenerated prompt fences it to this task.
> Shared files follow `docs/status/wave-A-hotspots.md`: anchors, allocated numbers, and logic only in your own files.

## Goal
Implement L2 switching end to end in FAST MODE:
- bridge domains with members (incl. one BVI), split-horizon groups, flags and MAC aging;
- static L2 FIB entries;
- L2 cross-connects and L3 cross-connects;
- VLAN tag rewrite on bridged / cross-connected sub-interfaces;
- the `mactime` time-range MAC filter.

Reference: TNSR "Bridge domains / L2 cross-connect"; VPP `l2`, `l3xc`, `mactime` (WBS D1.6 in `plan/wbs.csv`).

## Inputs to read first
- `apps/agent/internal/descriptors/l2/` + `docs/agent/descriptors/l2.md` (DF-1, merged). Reuse:
  - `l2.bridge-domain` (`bd_tag` ownership, mac-age inside the object);
  - `l2.bridge-domain-member` (port_type normal/bvi/uu-fwd, `shg`; one BVI per BD);
  - `l2.xconnect` (one object per direction);
  - `l2.fib-entry`;
  - `l2.flags` (a dependent of the membership);
  - `l2.vlan-tag-rewrite` (push/pop/translate, a dependent of the L2 membership).
- `apps/agent/internal/descriptors/l3xc/` + `docs/agent/descriptors/l3xc.md`: `l3xc.l3xc/<rx if>/<ip4|ip6>` (`l3xc_update`/`l3xc_del`).
- `apps/agent/binapi/mactime/`: `mactime_enable_disable`, `mactime_add_del_range`, `mactime_dump`. This is the new descriptor you write.
  - `mactime_dump` returns the device table (name, MAC, flags, ranges), so ranges can be read back.
  - The per-interface enable has **no** readback, so it is write-only (D-063/D-076, D-080 boot-identity record keyed on sw_if_index
    **and** logical name).
  - The device table is VPP-wide, so device names carry the owner prefix.
- **P08 (vertical slice, merged)**: the patterns you extend. Read `docs/status/vertical-slice.md` and `docs/status/wave-A-hotspots.md` first.
  - Builder package `apps/agent/internal/desired/`: `Sink`, `interface/<name>` alias references, `Ptr`, `Assemble`.
  - Your builder iterates `ds.Interfaces` (and your L2 records) from your own `desired/l2*.go`. Call it once from `project()` and once from
    `assemble()` in `apps/agent/internal/agent/projection.go`; the `assemble()` call comes after `desired.Assemble` and adds your leaves.
    **Do not edit `desired/interfaces.go`.**
  - Registry `apps/agent/internal/subsystems/subsystems.go`: `Domains` names one per line, and `l2.Register`/`l3xc.Register`/your
    `mactime.Register` go at the end of `Register()`. Any new `Wiring` method goes in your own `subsystems/bridge_l2.go`.
  - Live state is a read-only agent RPC merged in the API (the `InterfaceState` pattern). The methods go in your own
    `internal/agent/rpc_bridge_l2.go`; `server.go` needs no edit.
  - Topology test pattern `test/topology/interfaces/`: V19 guard, D-101 veth quiesce, `NRestarts` checked before and after.
- `docs/agent/descriptors/interface.md`: `interface/<name>` alias (D-065/D-069), ClaimStore for physical NICs (D-075).
- `packages/schema/src/domains/interfaces.ts`: **no L2 model exists** (only L3 fields). `packages/schema/src/domains/tunnels.ts` already
  references a bridge domain **by numeric id** (`bridgeDomain`, L2 GRE/TEB/VXLAN). Whatever placement is chosen, a BD must stay
  addressable by that id.
- `docs/lab/shared-host-rules.md`: take bridge-domain ids from your slot's table range (`N000–N999`). VPP 26.06 docs → "L2 bridge domains".

## Contract changes
Additive only. Commit them first as separate `contract(schema): l2` / `contract(proto): …` commits (as P08 did), with
`docs/status/tasks/F-bridge-l2-contract.md`. Tell the manager in `F-bridge-l2-questions.md`. Numbers come from your envelope, never
"next free".

1. **Config model**, in your own `packages/schema/src/domains/ext/bridge-l2.ts` (one key line in the domain object, one export line in
   `src/index.ts`). Content:
   - `bridgeDomains{<name>:{id, flood, uuFlood, forward, learn, arpTerm, macAgeMin, members{<if>:{shg?, bvi?, uuFwd?, tagRewrite?}},
     staticMacs[]}}`;
   - `xconnects{<rxIf>: {tx}}`;
   - `l3xc{<rxIf>:{ipv4Paths[], ipv6Paths[]}}`;
   - `macFilters{<name>:{mac, ranges[{days,start,end}], action}}`.

   Records are keyed by name (D-045/D-053).

   **Placement is a manager decision**, and the answer is in your envelope. `interfaces` is a record keyed by interface name, so BD-level
   settings cannot be sibling keys there. `docs/04-api-datamodel.md` has no L2 root key, so a new root key deviates from it. If the
   envelope carries no answer:
   - write the question;
   - use the least-reshaping variant: per-member settings on `interfaces.<if>.l2` (`Interface` field 14) /
     `interfaces.<if>.subinterfaces.<id>.l2` (`Subinterface` field 12), and BD-level settings in one keyed record inside an existing domain;
   - keep going.
2. **State.** Read-only unary RPCs:
   - `BridgeDomainState` (per BD: members, BVI, flags, learned-MAC count);
   - `BridgeDomainMacs(bd_id, offset, limit ≤ 1000)`: agent-side paging over `l2_fib_table_dump`, so a gRPC message is always bounded.

   Put the RPCs under the service anchor and the messages in a `// ----- F-bridge-l2 -----` section. In the same commit, add stub
   `UNIMPLEMENTED` handlers in `apps/api/src/testing/fake-agent.ts` and a section in `docs/contracts/proto.md`.

## Scope — build exactly this
1. **Schema** rules, in your own `packages/schema/src/semantic/bridge-l2.ts` (`bridgeL2Validators`, rule names `<domain>.bridge-l2-…`,
   one spread line in `semantic/index.ts`; examples in `packages/schema/examples/bridge-l2-*.json`):
   - a member interface is in at most one BD or xconnect;
   - a member has no L3 addresses/VRF while bridged (except the BVI);
   - one BVI per BD, and the BVI must be a loopback;
   - shg 0–255; mac-age 0–255 min;
   - xconnect rx ≠ tx;
   - tag rewrite only on an L2 member / xconnect rx;
   - mactime ranges well-formed.
2. **Agent**:
   - Project the model onto the DF-1 descriptors above, plus a new `mactime` descriptor package (`mactime.range/<name>`, enable per
     interface).
   - Retrieve covers every readable object. Fake-client unit tests (model VPP's duplicate-add behaviour for the write-only enable); an
     agent-level fake extension goes in `descriptors/core/coretest/bridge_l2.go`.
   - Disable mactime on an interface before it is deleted (V19 family: per-interface feature state can outlive the interface).
   - ONE host integration check (`VRX_INTEGRATION=1`, lab lock shared). Members: slot loopbacks (BVI), the rig's `host-<prefix>…`
     af_packet interfaces and their sub-interfaces, or slot-prefixed taps made by the fixture. Check:
     - Retrieve == desired;
     - `vppctl show bridge-domain <id> detail` / `show l2patch` / `show l3xc` contain it;
     - rollback leaves nothing;
     - the agent-restart simulation recreates it.
3. **API**:
   - config via the pointer routes;
   - `GET /api/v1/state/l2/bridge-domains` (members, BVI, learned-MAC count);
   - `GET /api/v1/state/l2/bridge-domains/{id}/macs?page&pageSize` (server-side paged).

   Both are served from the RPCs by `BridgeL2Controller` in your own `apps/api/src/features/bridge-l2/`. Its `index.ts` exports
   `{controllers, providers}`, and `app.module.ts` gets one import + spreads under the anchors. The fake behaviour goes in
   `features/bridge-l2/fake.ts`. Add an e2e test with the fake agent.
4. **UI**:
   - A "Bridging" page in the Interfaces nav group, on its own route, added through the router/nav anchors; the label key is in your
     `bridge-l2` namespace. Do not restructure P08's `InterfacesPage.tsx`.
   - BD list + SchemaForm, member table (incl. tag rewrite), MAC table (ServerDataGrid), cross-connect list; en + fa.
   - If members live on `interfaces.<if>.l2`, P08's generated drawer form shows them under your `x-vrx-ui` group. Only if that breaks the
     drawer, exclude the field with one named line in `apps/web/src/domains/interfaces/model.ts`.
5. **Docs**: `docs/user/interfaces/bridge-l2.md` (BD with BVI, bridged VLAN sub-interface with pop-1, xconnect, time-range filter; CLI
   equivalent). Add one see-also line at the end of `docs/user/interfaces/basics.md`; do not edit its "Not in this release" line.

**Files you own:**
- `apps/agent/internal/descriptors/{l2,l3xc,mactime}/**`, `docs/agent/descriptors/{l2,l3xc,mactime}.md`
- `apps/agent/internal/desired/l2*.go`, `apps/agent/internal/agent/rpc_bridge_l2*.go`, `apps/agent/internal/subsystems/bridge_l2*.go`
- `apps/agent/internal/descriptors/core/coretest/bridge_l2*.go`
- `apps/api/src/features/bridge-l2/**`, `apps/api/test/e2e/bridge-l2*.ts`
- `apps/web/src/domains/interfaces/bridge-l2/**`, `apps/web/src/locales/*/bridge-l2.json`
- `packages/schema/src/domains/ext/bridge-l2*.ts`, `packages/schema/src/semantic/bridge-l2*.ts`
- `packages/schema/examples/bridge-l2-*.json`, `packages/proto/test/fixtures/bridge-l2-*.json`
- `docs/user/interfaces/bridge-l2.md`, `test/topology/bridge-l2/**`, `docs/status/tasks/F-bridge-l2*.md`

**Shared hotspots: insert only under your `wave-A: F-bridge-l2` anchor, and name each hunk in `F-bridge-l2.md`:**
- agent: `apps/agent/internal/subsystems/subsystems.go`, `apps/agent/internal/agent/projection.go`
- schema: the domain file the placement answer names, `packages/schema/src/index.ts`, `packages/schema/src/semantic/index.ts`
- proto: `packages/proto/vrx/v1/dataplane.proto`, `docs/contracts/proto.md`
- API: `apps/api/src/app.module.ts`, `apps/api/src/agent/agent.client.ts`, `apps/api/src/testing/fake-agent.ts`
- web: `router.tsx`, `nav/nav.ts`, `nav/nav.test.ts`, `i18n.ts`
- docs: `docs/user/interfaces/basics.md` (end), `docs/vpp-code-track.md` (append `### V-new (F-bridge-l2)`)

Generated files are never hand-merged: `pnpm gen && make -C apps/cli gen docs`.

## Acceptance (paste the evidence)
- [ ] `vppctl show bridge-domain <id> detail` shows members, shg, BVI and mac-age as committed; Retrieve == desired (pasted)
- [ ] Agent-restart simulation → BD, members, xconnect, l3xc back within 30 s (log excerpt)
- [ ] Rollback returns members to L3 and deletes the BD (Retrieve)
- [ ] Interface in two bridge domains → 400 problem+json with `pointer` to the second membership
- [ ] UI screenshot against the real endpoint in `docs/status/tasks/F-bridge-l2.md`
- [ ] `tools/ci.sh --base main` green in your worktree

## Out of scope (do not build)
- Loopback creation and BVI UX on the loopback screen. F-loopback-bvi-gso-lldp-span consumes your BVI member support; loopback
  creation already exists since P08.
- SPAN/ERSPAN (F-loopback-bvi-gso-lldp-span).
- VLAN sub-interface creation and the QinQ UI (F-vlan-qinq). Tag rewrite on L2 members is yours.
- Bonding (F-bonding).
- ARP termination tables beyond the BD flag (F-neighbors-ra).
- VXLAN/GRE-L2 tunnels as BD members, beyond showing that any `interface/<name>` works (F-tunnels).
- EVPN.
- New l2/l3xc descriptors for objects DF-1 already has.

## Open questions to surface, not to decide silently
- Where the L2 model lives (in-domain record vs. new root key), unless your envelope answers it.
- Whether mactime (a "device allow-list by time") is worth shipping in the UI or stays config-only.
- Non-exact-match sub-interfaces for L2 use (F-vlan-qinq's open question: P08 creates every sub-interface exact-match). Default: keep
  exact-match.
