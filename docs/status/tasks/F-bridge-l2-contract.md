# F-bridge-l2 — contract changes (additive)

Branch `task/F-bridge-l2` (P08 pattern: contract commits first on the task branch, no own contract branch). Placement
follows D-109 (c): per-port leaves + a named container inside an existing domain, **no new root key**. D-109 (c) named
neither the container nor its number; the choice below is mine and is question Q1 in `F-bridge-l2-questions.md`.

## Schema (`contract(schema): l2`)

| where | what |
|---|---|
| `packages/schema/src/domains/ext/bridge-l2.ts` (new, owned) | `BridgeL2PortSchema` (per-port leaf), `BridgeL2Schema` (container: `bridgeDomains` by name, `xconnects` / `l3xc` by rx interface, `macFilters` by name), sub-schemas (`BridgeDomainSchema`, `BridgeStaticMacSchema`, `L2XconnectSchema`, `L3xcSchema`, `L3xcPathSchema`, `MacFilterSchema`, `MacFilterRangeSchema`, `VlanTagRewriteSchema`), primitives `bridgeDomainId` (1–16777215), `bridgeDomainName` (≤ 32 chars, it is stored in the 63-byte VPP bd_tag), `macFilterName`, `macFilterTime`, `MacFilterDay` (the `objects` WEEKDAYS spelling) |
| `domains/interfaces.ts` (C1 anchors) | `InterfaceSchema.l2` and `SubinterfaceSchema.l2` key lines + one import line (the file has no import anchor — named hunk) |
| `domains/routing.ts` (own anchor added above F-neighbors-ra's) | `RoutingSchema.l2` key line + one import line |
| `src/index.ts` (C3) | `export * from './domains/ext/bridge-l2.js'` |
| `semantic/bridge-l2.ts` (+ test, owned), `semantic/index.ts` (C2) | `bridgeL2Validators` (12 rules, names `interfaces.bridge-l2-*` / `routing.bridge-l2-*`) |

Shape (defaults as Zod fills them):

```jsonc
"interfaces": { "<if>": { "l2": { "bridgeDomain": "<bd name>"?, "shg": 0, "bvi": false, "uuFwd": false,
                                  "tagRewrite": { "op": "pop-1", "tag1"?, "tag2"?, "dot1ad": false }?, "macFilter": false },
                          "subinterfaces": { "<id>": { "l2": { …same… } } } } },
"routing": { "l2": {
  "bridgeDomains": { "<name>": { "id": 7001, "flood": true, "uuFlood": true, "forward": true, "learn": true,
                                 "arpTerm": false, "macAgeMin": 0, "staticMacs": [{ "mac", "interface" }] } },
  "xconnects":  { "<rx if>": { "tx": "<if>" } },
  "l3xc":       { "<rx if>": { "ipv4Paths": [{ "nextHop"?, "interface"?, "vrf": "default", "weight": 1, "preference": 0 }], "ipv6Paths": [] } },
  "macFilters": { "<name>": { "mac", "action": "allow"|"drop", "ranges": [{ "days": ["mon"], "start": "HH:MM", "end": "HH:MM" }] } } } }
```

## Proto (`contract(proto): F-bridge-l2 …`)

| where | what | number |
|---|---|---|
| `Interface` (anchor) | `BridgeL2Port l2` | **14** (wave-A §2) |
| `Subinterface` (anchor) | `BridgeL2Port l2` | **12** (wave-A §2) |
| `RoutingConfig` (own anchor) | `BridgeL2Config l2` | **20** — not allocated by D-109 (c); 20 is "next free" in wave-BC-numbers.md (13–17 routing pack, 18–19 spare, 12 P12). Manager to confirm or renumber (Q1) |
| `service Dataplane` (anchor) | `rpc BridgeDomainState`, `rpc BridgeDomainMacs` | — |
| `// ----- F-bridge-l2 -----` | `BridgeL2Port`, `BridgeL2TagRewrite`, `BridgeL2Config`, `BridgeL2Domain`, `BridgeL2StaticMac`, `BridgeL2Xconnect`, `BridgeL2L3xc`, `BridgeL2L3xcPath`, `BridgeL2MacFilter`, `BridgeL2MacFilterRange`; `BridgeDomainStateRequest/Response`, `BridgeDomainStatus`, `BridgeDomainMember`, `BridgeDomainMacsRequest/Response`, `BridgeDomainMac` | from 1 |

- `docs/contracts/proto.md` §11: `### F-bridge-l2: BridgeDomainState, BridgeDomainMacs` under the anchor.
- `apps/api/src/testing/fake-agent.ts` (P5 anchor): both RPCs answer `UNIMPLEMENTED` (like an agent without them). The
  e2e test installs the working fake from `apps/api/src/features/bridge-l2/fake.ts` on its own instance.
- Generated: `apps/agent/gen/**`, `packages/proto/gen/ts/**`, `packages/api-client/src/generated/schema.d.ts` (`pnpm gen`).
- Drift guards: `go test ./internal/contracttest` — "919 scalar leaves and 203 messages compared, 0 finding(s)" after the
  change; `packages/proto` vitest 70/70 with the new fixture `packages/proto/test/fixtures/bridge-l2-full.json`.
