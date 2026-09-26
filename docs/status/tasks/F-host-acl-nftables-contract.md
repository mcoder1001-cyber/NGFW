# F-host-acl-nftables — contract change: `HostAclState` + `acl.hostSettings` (additive)

Branch `task/F-host-acl-nftables` (P08 pattern: the contract commit sits on the task branch, no own branch). Commit subject
`contract(proto): host acl state`.

## What
- `packages/proto/vrx/v1/dataplane.proto`
  - `service Dataplane`: `rpc HostAclState(HostAclStateRequest) returns (HostAclStateResponse);` under the
    `// wave-A: F-host-acl-nftables` anchor (framed by blank lines, C5).
  - `AclConfig`: `HostAclSettings host_settings = 8;` under the `// wave-A: F-host-acl-nftables` anchor — the **config gap**
    (see below). Number 8 is the one wave-A-hotspots §2 *proposes* for "F-host-acl-nftables host settings"; the envelope says
    a DesiredState number needs the manager first, so it is asked in the questions file (Q1) and used meanwhile.
  - `// ----- F-host-acl-nftables -----` section: `HostAclSettings {default_input = 1, allow_icmp = 2, anti_lockout = 3}`,
    `HostAclAntiLockout {enabled = 1, sources = 2, interfaces = 3, ports = 4}`, `HostAclStateRequest {owner = 1}`,
    `HostAclStateResponse {owner, retrieved_at, table, mode, present, in_sync, sets, chains}`,
    `HostAclSetState {name, type, object, elements}`, `HostAclChainState {name, hook, priority, policy, list, rules}`,
    `HostAclRuleState {kind, list, sequence, pointer, text, verdict, comment, packets, bytes}`. Every name starts with
    `HostAcl` (§0.5).
- `packages/schema` (C1–C3, own files + one line each):
  - new `src/domains/ext/host-acl-nftables.ts`: `HostAclSettingsSchema` / `HostAclAntiLockoutSchema`
    (`defaultInput: accept|drop = accept`, `allowIcmp = true`, `antiLockout {enabled = true, sources: ipPrefix[] = [],
    interfaces: linuxInterfaceName[] = [], ports: 1–65535[] (1–8) = [22, 443]}`);
  - `src/domains/acl.ts`: one import + one key line `hostSettings: HostAclSettingsSchema.optional()` at the end of `AclSchema`
    (acl.ts has no seeded anchor; envelope: "else at the end of the block"). **Optional**, so every existing document parses
    to the same output (no default injected);
  - new `src/semantic/host-acl-nftables.ts`: rule `acl.host-settings` (duplicate anti-lockout sources — canonical prefix
    compare —, interfaces, ports), one import + one spread line under the C2 anchors of `semantic/index.ts`;
  - `src/index.ts`: `export * from './domains/ext/host-acl-nftables.js'` under the C3 anchor.
- Fixtures (C4, new files only): `packages/proto/test/fixtures/host-acl-nftables-basic.json` (joins the DesiredState and
  parsed-document corpora); unit test `packages/schema/src/semantic/host-acl-nftables.test.ts`.
- Regenerated (C7, never hand-edited): `apps/agent/gen/vrx/v1/dataplane{,_grpc}.pb.go`, `packages/proto/gen/ts/vrx/v1/dataplane.ts`,
  `packages/api-client/src/generated/schema.d.ts` (the new `acl.hostSettings` component). `make -C apps/cli gen docs`: no change
  (no route yet).
- `apps/api/src/testing/fake-agent.ts` (P5): the UNIMPLEMENTED stub handler under the anchor; the task's real fake behaviour
  replaces that one line later (`features/host-acl-nftables/fake.ts`).
- `docs/contracts/proto.md` §11 (C6): `### F-host-acl-nftables: HostAclState`.

## Why
- **State RPC** (prompt "Contract changes"): the API can read the rendered host firewall and its per-rule counters only through
  a read-only RPC (P08's live-state pattern); `Retrieve` stays configuration-only (proto.md §5).
- **Config gap**: the anti-lockout rule needs "the configured source" of management traffic, and the renderer needs the input
  default policy; no existing leaf carries either (`management` has users/AAA/TLS/syslog, no listen addresses). The prompt names
  exactly this gap (`acl.hostSettings{defaultInput, managementInterfaces[], allowIcmp}`); `managementInterfaces` became
  `antiLockout.interfaces` next to the sources and ports it applies to.

## Compatibility
Additive only: a new RPC, seven new messages and one new message field (8) on `AclConfig`; the schema key is optional.
`buf lint` clean; the Go drift guard (`internal/contracttest`, schema ⊆ proto both ways) passes; the proto corpus tests pass
with the new fixture. An older agent answers UNIMPLEMENTED (the API maps it to 501) and reports `acl` as not implemented.
