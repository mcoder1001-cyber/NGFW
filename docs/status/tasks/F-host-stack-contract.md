# F-host-stack — contract changes (additive)

Commits on task/F-host-stack (no own branches):

1. `contract(schema): services.hostStack` — `packages/schema/src/domains/ext/host-stack.ts` (new), one key line
   `hostStack` in `domains/services.ts` (C1, unanchored: no `wave-BC: F-host-stack` anchor in the key block — appended
   after the last anchor) and one export in `src/index.ts` (C3, unanchored likewise); regenerated
   `packages/api-client/src/generated/schema.d.ts`.
   Shape: `services.hostStack?{enabled, namespaces{<id>:{secretRef?(key/<name>), interface?, vrf}},
   sessionRules[{tag, scope: global|local, transport: tcp|udp, local, localPort?, remote, remotePort?,
   action: allow|deny|redirect, redirectAppIndex?, appNamespace?}], tcpSourceAddresses?{first,last,vrf},
   httpStatic?{enabled, wwwRootPath, uri, cacheSizeMb}}`.
   Additions beyond the prompt's sketch (needed by `session_rule_add_del`): `localPort`/`remotePort` (unset = any)
   and `redirectAppIndex` (the `action_index` of a redirect rule).
2. `contract(proto): HostStackService + HostStackState` — `ServicesConfig.host_stack = 10` (wave-BC-numbers),
   messages `HostStackService`, `HostStackNamespace`, `HostStackSessionRule`, `HostStackTcpSource`,
   `HostStackHttpStatic` (config) and `HostStackStateRequest/Response`, `HostStackRuleState` (RPC), all in the
   `// ----- F-host-stack -----` section; RPC `HostStackState` under its anchor. Regenerated Go/TS stubs; fake-agent
   UNIMPLEMENTED stub (P5); `docs/contracts/proto.md` § F-host-stack (C6, unanchored — no anchor in that file);
   drift fixture `packages/proto/test/fixtures/host-stack-basic.json`.

Drift guard: `go test ./internal/contracttest -run Drift` → 897 leaves, 0 findings; `packages/proto` vitest 70/70.
