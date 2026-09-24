# F-bridge-l2 — questions for the manager

Written while working; none blocks the task (each has the default I took). Newest last.

## Q1 — container name and number for the bridge-domain records (D-109 c left both open)
D-109 (c) says "a named container inside an existing domain object" but names neither; the envelope's fallback is "take
variant (b) with the least-reshaping container, write the question, keep going".
- **Taken: `routing.l2` (`RoutingConfig.l2 = 20`, message `BridgeL2Config`).** `routing` is the forwarding domain that
  already takes mpls / multicast / srv6 in the wave-B/C pack; l3xc is an L3 forwarding feature.
- Rejected: `dataplane` (start-up tuning only; its renderer reads the domain as start-up config), `services`
  (daemon-style services), `tunnels` (F-tunnels owns `tunnels.ts`), `interfaces` (a record keyed by interface name — no
  sibling keys possible).
- Number: D-109 (c) allocated none; **20** is "next free" in `wave-BC-numbers.md` (13–17 routing pack, 18–19 spare, 12
  P12). The allocation rule forbids "next free" without the manager — please confirm 20 or give another; renumbering is a
  one-line proto change plus `pnpm gen` before merge.
- `routing.ts` / `RoutingConfig` / the agent's `Routing` Domains slice had no F-bridge-l2 anchor; I added my own anchor
  line above the first existing one in `routing.ts` and `RoutingConfig` (named hunks in `F-bridge-l2.md`). All l2 / l3xc /
  mactime descriptors are in the `interfaces` domain (the only Domains anchor seeded for this task); the API always sends
  every implemented domain, so both halves of the model are always in one transaction.

## Q2 — acceptance "interface in two bridge domains → 400 with pointer to the second membership"
With D-109 (c) the membership is the single-valued leaf `interfaces.<if>.l2.bridgeDomain`, so two bridge domains cannot
be expressed at all (the document shape prevents it). The rule `interfaces.bridge-l2-single-membership` covers what can
still collide — a bridge member that is also an L2 cross-connect rx (or an L3 cross-connect rx) — and reports the
**second** membership (`/routing/l2/xconnects/<if>`). The acceptance evidence uses that case. Default: accepted as the
D-109 (c) equivalent; say if you want something else.

## Q3 — domain files have no import anchor
`interfaces.ts` and `routing.ts` need `import { … } from './ext/bridge-l2.js'` for the key lines; neither file has an
import anchor, so every wave-A C1 feature adds an import after the last import line (same spot → a trivial union
conflict at merge). Mine carry a trailing `// wave-A: F-bridge-l2` comment. Same for the `semantic/index.ts` import
(that one has an anchor — fine).

## Q4 — `packages/schema/examples/bridge-l2-*.json` is rejected by `examples.test.ts`
`examples.test.ts` fails on any example whose prefix is not in its `SIBLING` regex (nat|objects|acl|vpn|tunnels|services|ha)
and the file is not mine. I put the valid corpus document in `packages/proto/test/fixtures/bridge-l2-full.json` (owned,
scanned by both drift guards) and the invalid cases inline in `semantic/bridge-l2.test.ts`. If you add `bridge-l2` to
`SIBLING`, the fixture can be copied to `examples/` unchanged.

## Q5 — mactime in the UI (prompt's open question)
Default taken: **ship it in the UI** as a small table on the Bridging page (device, MAC, action, weekly ranges) plus the
per-interface `macFilter` switch in the member form — the backend is complete, and hiding it would leave a config-only
feature without a list. Caveat shown in the UI help and the docs: times are VPP's mactime clock, whose time zone is the
VPP start-up setting `mactime { timezone_offset }` (default −5, US daylight rule) — a VPP-global the product does not
render yet (D-071; a future F-startup-gen field).

## Q6 — non-exact-match sub-interfaces for L2 (F-vlan-qinq's open question)
Default kept: every sub-interface stays exact-match (P08). VPP bridges exact-match sub-interfaces fine; `pop-1` on a
bridged VLAN sub-interface is host-verified in the integration test.

## Q7 — bridge-domain record names live in the VPP bd_tag (DF-1 descriptor extension)
Retrieve must name a bridge domain again, and nothing else in VPP stores the name. The DF-1 `l2.bridge-domain` model got
an optional `name`; the tag becomes `<owner>:<id>/<name>` (was `<owner>:<id>`; the old form still parses, name = "").
A rename is a recreate (VPP has no tag update). This is an extension of a descriptor I own for this task, not a second
descriptor (D-104).

## Q8 — VPP crash 18:41:08 (manager incident note): not F-bridge-l2
Answer to the manager's incident message: no F-bridge-l2 code or test touched VPP before 19:14. Until then this slot ran
only fake-VPP unit tests (`go test ./...` without `VRX_INTEGRATION`, all host tests skip), the API e2e with the in-process
fake agent and web unit tests. The first host run was `TestMactimeOnHost` at 19:14 (NRestarts 1 before and after, i.e.
already 1 from the 18:41 crash). F-bridge-l2 has no classify code; its only interface cleanup is `ifacetest.Loopback`'s
`ifsanitize.BeforeDelete` on its own loopback (recorded bindings only). D-126 noted: no sweeps; the topology test sends
no packets at all (veth peers stay down) and configures no 802.1ad push on the rig.

## Q9 — coretest/fakevpp.go: one line outside the owned files
`coretest.New()` must install the F-bridge-l2 handlers, otherwise every agent unit test fails once the l2 / l3xc /
mactime descriptors are registered (their Retrieve runs on every plan). `fakevpp.go` has no extension hook, so I added one
line after P08's `v.installIfExt()`: `v.installBridgeL2() // F-bridge-l2 (coretest/bridge_l2.go)`. The handlers live in my
own `coretest/bridge_l2.go` (A6). Please keep it at merge (or seed a hook list for the other A6 features).

## Q10 — `interfaces/model.ts` (P08): `l2` is an opaque JSON field in the interface drawer
The drawer broke exactly as the prompt anticipated: SchemaForm materialises the absent optional `l2` object and its
nested `tagRewrite` (required `op`), so every drawer save was blocked (3 P08 web tests failed). Excluding the field is
not safe (the drawer's merge patch would then send `l2: null` and silently remove a membership on any MTU edit), so the
one named hunk makes it opaque instead: `interfaceItemSchema()` returns `drawerSafeL2(...)` (bridge-l2/model.ts) —
`l2` becomes a JSON-widget field (absent stays absent, set round-trips unchanged; the server validates it). Two lines:
the import and the return, both marked `wave-A: F-bridge-l2`.
