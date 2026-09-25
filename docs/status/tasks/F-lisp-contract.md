# F-lisp — contract note (additive)

Two contract commits on `task/F-lisp`, first on the branch (never a `contract/` branch):

| commit | change |
|---|---|
| `contract(schema): tunnels lisp` | `TunnelsSchema.lisp` (optional; absent = no LISP managed) — one key line under the `// wave-BC: F-lisp` anchor plus the `import { LispSchema }` line at the top of `domains/tunnels.ts` (outside the anchor: an import cannot sit in the object literal); sub-schema `packages/schema/src/domains/ext/lisp.ts`; `export *` under the anchor in `schema/src/index.ts` |
| `contract(proto): tunnels lisp, LispState` | `TunnelsConfig.lisp = 10` (`LispConfig`, the allocated number, wave-BC-numbers.md); new messages in `// ----- F-lisp -----`: `LispConfig`, `LispLocatorSet`, `LispLocator`, `LispLocalEid`, `LispEidTable`, `LispRloc`, `LispRemoteMapping`, `LispAdjacency`, `LispLocatorPair`, `LispGpeEntry`, `LispStateRequest`, `LispStateResponse`, `LispStateLocatorSet`, `LispStateLocator`, `LispStateMapping`, `LispStateAdjacency`, `LispStateEidTable`; `rpc LispState` under the service anchor (blank-line framing kept); regenerated Go/TS stubs; `packages/proto/test/fixtures/lisp-full.json` (drift corpus); `docs/contracts/proto.md` `### F-lisp: LispState`; UNIMPLEMENTED `lispState` stub in `apps/api/src/testing/fake-agent.ts` (P5) |

Messages beyond the list in wave-BC-numbers.md (`LispRloc`, `LispLocatorPair`, `LispGpeEntry`, the `LispState*` parts) are all
in the F-lisp section; no other number is used. No EventKind, no ActionRequest member.

Drift guards: `packages/proto` vitest (parsed documents incl. `fixtures/lisp-full.json`) and
`apps/agent/internal/contracttest` (`TestSchemaProtoDrift`) pass on the contract commits.

Model (document side):

```
tunnels.lisp {
  enabled, gpe,                                   # VPP-global switches (globals owner sets, others require)
  locatorSets{<name>: {locators[{interface, priority, weight}]}},
  localEids[{vni, eid, locatorSet}],
  eidTables{<vni>: {vrf | bridgeDomain}},
  remoteMappings[{vni, eid, rlocs[{address, priority, weight}], action}],
  adjacencies[{vni, reid, leid}],
  gpeEntries[{vni, vrf, reid, leid, pairs[{local, remote, weight}], action}],
  mapResolvers[], mapServers[], pitr?
}
```
