# F-mpls-srmpls — contract change (additive)

Branch `task/F-mpls-srmpls`, two commits: `contract(schema): routing mpls` and `contract(proto): routing mpls, MplsState`.
Numbers from `docs/status/wave-BC-numbers.md` § F-mpls-srmpls; nothing renamed or reshaped.

## Schema (`packages/schema`)
- New `src/domains/ext/mpls-srmpls.ts`: `MplsSchema` (a plain `z.strictObject`; F-mpls-ldp's key goes below its
  `// wave-BC: F-mpls-ldp` anchor inside it) and its parts.
- `src/domains/routing.ts`: one key line `mpls: routingMpls` under `// wave-BC: F-mpls-srmpls` (plus one import line at the
  top, see "Shared hunks" in F-mpls-srmpls.md). `routing.mpls` is optional: absent = no MPLS.
- `src/semantic/mpls-srmpls.ts` (`mplsSrmplsValidators`, ids `routing.mpls-srmpls-…`), one spread + one import line under
  the anchors of `src/semantic/index.ts`; `src/index.ts` exports the ext module under its anchor.

```
routing.mpls {
  interfaces: string[]                                  MPLS enabled (interfaces keys)
  tables: { "<id 1–4294967295>": {} }                   additional MPLS FIBs (table 0 = default, implicit)
  labelRoutes: [{ table=0, label, eos=true, payload?: ip4|ip6|ethernet,
                  paths: [{ nextHop?, interface?, outLabels=[] (≤16), weight=1, vrf? }] (1–32) }]
  ipBindings: [{ label, vrf="default", prefix }]
  tunnels: { "<name>": { paths: [path] (1–32), l2Only=false } }
  sr: { policies: { "<bsid>": { segmentLists: [{ labels (1–16), weight=1 }] (1–32), spray=false } },
        steering: [{ vrf="default", prefix, bsid, vpnLabel? }] }
  // wave-BC: F-mpls-ldp   (F-mpls-ldp adds `ldp` here)
}
```
Labels are 16–1048575 everywhere (0–15 reserved). A path's `vrf` = pop and look the IP payload up in that VRF (end of stack
only, no next hop/interface/out labels) — the "basic L3VPN" termination of WBS D2.8.

Rules: table exists (0 or declared) · (table, label, eos) unique · MPLS interfaces exist and are listed once · path
interface = an interface or another MPLS tunnel · VRFs exist · tunnel name ≠ interface name · a BSID is not a table-0 label
route · a bound label is not a table-0 route / BSID / other binding, one label per (VRF, prefix) · steering names an
existing policy, (VRF, prefix) once · segment lists of a policy distinct.

## Proto (`packages/proto/vrx/v1/dataplane.proto`)
- `RoutingConfig` **15 `mpls`** → `MplsConfig` (under `// wave-BC: F-mpls-srmpls`, blank-line framed).
- `MplsConfig` fields 1–6 (`interfaces`, `tables`, `label_routes`, `ip_bindings`, `tunnels`, `sr`), 7–9 unused (this
  task's), **10 = F-mpls-ldp's `ldp`**: comment `// 10 reserved: ldp (F-mpls-ldp)` + `// wave-BC: F-mpls-ldp` anchor (no
  `reserved 10;`, so F-mpls-ldp's addition stays a pure insert).
- New messages in `// ----- F-mpls-srmpls -----`: `MplsConfig`, `MplsTable`, `MplsLabelRoute`, `MplsPath`,
  `MplsIpBinding`, `MplsTunnel`, `MplsSr`, `MplsSrPolicy`, `MplsSrSegmentList`, `MplsSrSteering`, `MplsStateRequest`,
  `MplsStateResponse`, `MplsStateTable`, `MplsStateFibEntry`, `MplsStatePath`, `MplsStateTunnel`.
- `rpc MplsState(MplsStateRequest) returns (MplsStateResponse)` under the service anchor: `view` "fib" (one page of one
  MPLS table, paged in the agent) or "tunnels". No EventKind, no ActionRequest member.
- `apps/api/src/testing/fake-agent.ts`: the contract commit's UNIMPLEMENTED stub under the anchor (replaced by the real
  fake line `...mplsSrmplsFake(this)` in the feature commit).
- Generated: `apps/agent/gen/**`, `packages/proto/gen/ts/**`, `packages/api-client/src/generated/schema.d.ts` (`pnpm gen`).
- Fixture: `packages/proto/test/fixtures/mpls-srmpls-full.json` (the document corpus of the round-trip and drift tests).

No example under `packages/schema/examples/`: `examples.test.ts` rejects file names outside its sibling list
(`nat|objects|acl|vpn|tunnels|services|ha`), and that test is not this task's file — see questions Q2.
