# P03 — review of `task/P03` (gRPC contract `vrx.v1.Dataplane`)

Reviewer: review agent (did not write the branch). Base `main@2de6c2f`, branch head `d1ba8ab`, worktree `/root/ngfw-wt/P03`.
Everything below was re-run by the reviewer on host `ngfw` on 2026-09-23; commands and results are in §"Re-verification".

Scope of the review checklist (REVIEW-PROMPT.md): items 2 (VPP integration tests), 3 (restart safety), 4 (VPP API provenance),
5 (shared-host objects), 7 (transaction rollback on VPP), 8 (UI honesty), 10 (i18n) are **n/a** — pure contract task, no VPP
objects, daemons, routes or UI were created. Items 1, 6, 9, 11 and the design review are below.

## Verdict summary

The deliverable is solid: every RPC of docs/04 is present with the specified semantics plus `Health`; `ApplyRequest` carries
`{txn_id, desired_state, subsystems, confirm_timeout_sec, confirm_txn_id, owner}`; per-object results carry RFC 6901 pointers;
`StatsBatch`/`Event`/`ActionRequest`/`ActionOutput` are supersets of the sketch; every field is commented; `buf lint`,
`buf breaking`, determinism and the gate are green and match the pasted output; `docs/contracts/proto.md` is a genuinely
useful semantics document. The design decisions the manager adopted in D-034 (strings for enums, maps for `interfaces`/`vrfs`,
`RetrieveResponse`/`DryRunRequest`, no `reserved` ranges, the extra lint exception) are the right calls and I would not reopen them.

What must change before merge are things that would force a **breaking change right after `contracts-v1`** or that break a
consumer workflow the contract document promises: a 64-bit counter time bomb in the TS stubs (F1), four field types/presence
choices that P02a's already-committed model contradicts (F2), and a JSON-default-omission hazard in the documented
running-vs-actual diff path (F3). Each is a pre-tag fix of minutes to an hour.

## Findings (ranked)

### F1 — HIGH — `uint64` counters decode to `number` and **throw** above 2^53; the doc's "decades" claim is off by ~1000×
- `packages/proto/buf.gen.yaml:15-19` (ts-proto opts, no `forceLong`), generated `packages/proto/gen/ts/vrx/v1/dataplane.ts:24809-24817`
  (`longToNumber` throws `"Value is larger than Number.MAX_SAFE_INTEGER"`) and `:4474` (`message.rxBytes = longToNumber(reader.uint64())`);
  `docs/contracts/proto.md:222-223` ("exact below 2^53 — fine for byte counters for decades at 100 Gbit/s").
- Failure: counters are **absolute since VPP start** (D-P03-10, `dataplane.proto:316-336`). 2^53 bytes = 9.0 PB; at 100 Gbit/s that is
  **8.3 days**, at 10 Gbit/s 83 days. When any interface's `rx_bytes`/`tx_bytes` crosses it, `StatsBatch.decode` throws inside the
  grpc-js stream in `vrx-api`'s telemetry relay (P06 §8) and every subsequent batch fails until counters are cleared or VPP restarts.
  The same applies to `IpsecRekey.esp_bytes`/`esp_packets`, `WorkerCpu.calls/vectors`, `seq`.
- Fix (TS-only, no wire change, but it changes the TS type of every 64-bit field, so it must land **before P06 codes against `number`**):
  `forceLong=string` (or `=bigint`) in `buf.gen.yaml`, regenerate, update the test, and correct §9 of `proto.md` with the real arithmetic.
  Alternatively keep `number` only for fields that are provably small (`seq`) via a per-field option — not worth it; take `string`/`bigint`.

### F2 — HIGH — four P02a fields are already known to be typed wrongly, and each is a `buf breaking` violation to fix after the tag
P02a's model is committed at `task/P02a@df554dc` (not merged). The proto guessed the P02a shapes (P03-questions #6) instead of reading that
branch; comparing against it:

| proto | P02a HEAD (`packages/schema/src/domains/…`) | kind |
|---|---|---|
| `dataplane.proto:624` `optional string corelist` ("2-5,8") | `dataplane.ts` `corelist: z.array(cpuCore).optional()` — **array of CPU ids** | type: string → repeated uint32 |
| `dataplane.proto:595` `string banner` | `system.ts` `banner: BannerSchema{login?, motd?}` — **object** | type: string → message |
| `dataplane.proto:646` `string rx_mode` (implicit presence) | `interfaces.ts` `rxMode: RxMode.optional()` | presence: the file's own rule (`.optional()` → proto3 `optional`, header lines 7-12) is violated |
| `dataplane.proto:720` `NextHop.string address` (implicit) | `routing.ts` `NextHopSchema.address: … .optional()` ("may be omitted for an interface route") | presence, same rule |

- Verified that all four are breaking under the branch's own `buf.yaml` (FILE category): on a scratch copy, `string rx_mode` → `optional string rx_mode`
  reports `FIELD_SAME_CARDINALITY` (exit 100); `string banner` → `SystemBanner banner` reports `FIELD_SAME_TYPE` + cardinality (exit 100).
- Failure: P03b (D-035) cannot fix these without disabling `buf breaking` against `main` — the first contract change after this branch merges
  would have to bypass the very guard this task installs. If P03b is skipped or slips, the tag freezes a `banner` that cannot hold P02a's
  document and a `corelist` that cannot hold an array; `fromJSON`/`protojson.Unmarshal` of a parsed P02a document then fails outright
  (`protojson`: "invalid value for string type: {…}").
- Fix (pre-merge, ~30 min): mirror `task/P02a@df554dc` for `system`, `dataplane`, `interfaces`, `routing.static`, `management.users` exactly
  (types and presence), since the branch exists and P02a is in review. Additive gaps found at the same time (fill now or leave to P03b, either is
  non-breaking): `Interface.promiscuous`, `Subinterface.dot1ad`, `DataplaneConfig.tx_queues`, `StaticRoute.distance`/`description`,
  `ManagementUser.ssh_keys`/`full_name`/`disabled`, and the `SystemNtp`/`SystemDns`/`ManagementAaa`/`ManagementTls`/`SyslogTarget` shells.
- Good news recorded for P03b: the *container* shapes of the shells match the now-committed P02c/P02a models — `tunnels.{gre,vxlan,ipip}` are
  records, `ha.vrrp[]` is an array (`ha.cluster` is new, additive), `services.{dhcp,dns,snmp,lldp,ipfix}` exist as objects (new siblings are
  additive), `routing.{prefixLists,routeMaps}` are records and `bgp/ospf/isis/rip/bfd` objects; `nat`/`objects`/`acl`/`vpn` mirrors have no
  renames vs `task/P02b@757edaa` / `task/P02c@a306c2b` (only `NatStaticMapping.External.pool` was added). So after F2 the P03b delta is additive only.

### F3 — HIGH — the documented running-vs-actual diff on `DesiredState.toJSON()` loses proto3 defaults (hidden and false drift)
- `docs/contracts/proto.md:37` ("`diff(running, actual)` … works on `DesiredState.toJSON()`") and `:164`; generated `Vrf.toJSON`/`Interface.toJSON`
  (`dataplane.ts:176-190`: `if (message.enabled !== false)`, `if (message.id !== 0)`).
- Probe run by the reviewer (temporary vitest file, removed): `DesiredState.toJSON(DesiredState.fromJSON({vrfs:{default:{id:0}}, interfaces:{x:{enabled:false,…}},
  acl:{lists:{l:{rules:[{…, enabled:false}]}}}}))` →
  `{"interfaces":{"x":{"vrf":"default","rxMode":"polling"}},"vrfs":{"default":{}},"acl":{"lists":{"l":{"rules":[{"sequence":1,"action":"permit","ipVersion":"any"}]}}}}`
  — `id: 0`, `enabled: false` (interface and ACL rule) are gone. Go `protojson.Marshal` behaves the same unless `EmitDefaultValues` is set.
- Failure: (a) **false drift** on every Retrieve: running `vrfs.default.id = 0` vs actual `{}` → `diff()` reports a removed `id`; the P06 proof
  "after commit `Retrieve()` equals desired" fails on the default VRF alone. (b) **hidden drift** wherever the Zod default is `true` and the
  actual value is `false` — P02c's `enabledFlag = z.boolean().default(true)` (every IPsec/WireGuard/remote-access object), `AclRule.enabled`,
  `system.ntp.enabled`, `NtpServer.iburst`, `VrrpInstance.preempt`, `DhcpServer.authoritative`, `HaCluster.configSync`: the omitted `false`
  is re-filled to `true` by `RootConfig.parse()`, so a disabled-in-VPP tunnel compares equal to an enabled one.
- Fix: decide the presence policy **now** (changing it later is `FIELD_SAME_CARDINALITY`-breaking, see F2). Recommended: make every scalar field
  of the `DesiredState` subtree `optional` (explicit presence) — mechanical, ~300 declarations; `toJSON` then emits exactly what was set,
  `fromJSON(parsedDoc)` round-trips losslessly, and Go descriptors read `Get*()` unchanged. Minimum acceptable alternative: remove the
  `toJSON`-diff promise from `proto.md` §1/§5 and specify that the API compares `DesiredState.fromJSON(running)` against `actual` at the
  message level (and state that `RootConfig.parse()` must never be applied to a Retrieve result). Add the probe above as a test either way.

### F4 — MEDIUM — `ManagementUser.password_hash` puts a secret in the wire contract and contradicts P05's persistence design
- `dataplane.proto:829-838`; `docs/contracts/proto.md:50-51` ("the API strips it *unless the agent needs it*, the agent never logs or persists it,
  and `Retrieve` leaves it unset").
- Failure: P05 §5 persists the **whole** last desired state in `/var/lib/vrx/agent/desired.pb`; with the field present in the message, either
  P05 special-cases one field forever or the hash lands on disk in the agent's state dir (00-CONTEXT rule 10). "Unless the agent needs it" is
  a loophole with no defined need (the agent never authenticates users). And because `Retrieve` "leaves it unset", every user with a hash shows
  as drift in the F3 diff path.
- Fix: drop the field from the proto. P02a's model already marks it `secret: true` in `withUi()` meta (`management.ts` `passwordHash`), so the API
  can strip *all* secret-flagged leaves generically before `fromJSON`, and the strict-projection tests operate on the stripped document. State the
  rule in `proto.md` §1: "leaves flagged `secret` in the schema have no proto field". If the field is kept, replace "unless the agent needs it"
  with "always".

### F5 — MEDIUM — the status report overstates what the compile-check tests prove
- `docs/status/tasks/P03.md:46,146,165` ("strict protojson / `DesiredState.fromJSON`", "proven by strict … `fromJSON` tests", "protobuf JSON of a
  *parsed* `RootConfig` document is a `DesiredState`").
- Facts: (a) ts-proto `fromJSON` is **not strict** — the reviewer's probe fed `unknownRoot: 1` and `system.bogusField` and they were silently
  dropped (`dataplane.ts:144-158` reads known keys only, and also accepts `rx_mode` as an alias). Only the Go test (`desiredstate_test.go:76`,
  `protojson.Unmarshal` without `DiscardUnknown`) is strict. (b) The corpus is `{}` and `two-interfaces.json` — about 15 of 603 fields, all P02a-shaped;
  nothing in `nat`/`objects`/`acl`/`vpn` is exercised by a document, only by hand-written literals that cannot detect a mirror error.
  (c) Neither test calls `RootConfig.parse()`; the "parsed document" property (prefaulted 13 keys, Zod defaults filled) is untested.
- Fix: in the TS test, assert strictness structurally (recursively compare the key set of `doc` with `toJSON(fromJSON(doc))`, or use
  `@bufbuild/protobuf`'s strict `fromJson` — the runtime is already a dependency) and run the examples through `RootConfig.parse()` from
  `@ngfw/schema` first; correct the wording in P03.md. The schema⊆proto drift guard proposed in P03-questions #7 belongs in **P03b**, not P09:
  it is the only thing that will catch a P02x field landing without a proto follow-up before the tag.

### F6 — MEDIUM — P03b is not "a mechanical 1–2 h diff"; plan it as a real task
- `docs/status/tasks/P03.md:153-154`, `P03-questions.md` #4.
- Measured against the branches as they are today: P02c HEAD adds ~160 leaf keys the proto lacks (`services` 118, `ha` 24, `tunnels` 16), P02a
  adds ~70 in `routing`, ~18 in `management`, ~9 in `system`, plus F2's retypes. Not a defect of this branch, but the board row and the tag gate
  (D-035) should carry the real size (half a day plus the drift guard), and P03b must start from the branch heads, not from `ec0ccda`/`b815d15`.

### F7 — MEDIUM — destructive default: an unset domain in `ApplyRequest` deletes every owned object of that domain
- `dataplane.proto:63-69`, `docs/contracts/proto.md:83-86` ("an unset or empty domain means 'no objects': every owned object of that domain is
  deleted … the API therefore always sends the whole parsed document").
- Failure: with `subsystems` empty (= all), a partial document — a P06 bug, a hand-crafted `grpcurl` Apply, or a future caller that only knows
  about `system` — wipes interfaces, VRFs and routes in one transaction, with a valid `APPLIED`. The doc argues maps have no presence; true for
  `interfaces`/`vrfs`, but the other 11 domains are messages and do have presence, and the contract throws that information away.
- Fix (semantics only, no wire change): an **unset** `<Key>Config` message that is not explicitly named in `subsystems` is skipped, not emptied;
  an explicitly named subsystem is authoritative as today; `interfaces`/`vrfs` (maps) are authoritative only when explicitly named or when
  `subsystems` is empty *and* at least one of the two is non-empty — or simply require `subsystems` to be non-empty. Pick one and write it down.

### F8 — LOW — hand-written Go test inside the contract-guarded generated tree
- `apps/agent/gen/vrx/v1/desiredstate_test.go`; `packages/proto/gen.sh:8` now deletes only `*.pb.go` to keep it alive; `tools/ci.sh:24` treats
  every change under `apps/agent/gen` as a contract change.
- Failure: any edit to the test needs a `contract(` commit; a future `rm -rf apps/agent/gen` (main's previous `gen.sh`) or `golangci-lint`
  excludes for generated dirs silently drop the only strict test. The envelope granted `apps/agent/gen/**` as owned files, it did not require
  tests to live there.
- Fix: move to `apps/agent/internal/contract/desiredstate_test.go` (or wherever the P05 owner prefers) once P05a's package layout is merged; restore
  `gen.sh` to a full wipe. Can be done by P05/P03b; note it in P03-questions.

### F9 — LOW (pre-existing on `main`, but `packages/proto` is this task's file) — `@ngfw/proto` is not importable at runtime
- `packages/proto/package.json:6-11` (`main`/`types`/`exports` → `./gen/ts/vrx/v1/dataplane.js` / `.d.ts`, which do not exist; only `dataplane.ts`
  is generated; `build` is an `echo`). `apps/api/package.json:22` already depends on it.
- Verified: `tsc --moduleResolution nodenext` from `apps/api` resolves it (TS maps `.js` → `.ts`), but `node --input-type=module -e 'import("@ngfw/proto")'`
  from `apps/api` fails with `ERR_MODULE_NOT_FOUND …/dataplane.js`. NestJS at runtime (P06) will hit this unless it bundles or runs a TS loader.
- Fix: either add a real `build` (tsc emit to `dist/`, point `exports` there, add `dist/**` to turbo outputs) or point `exports` at the `.ts`
  and document that consumers run under a TS loader. Decide before P06's first import.

### F10 — LOW — documentation and convention nits (fix in the same pass)
- `docs/contracts/proto.md:8`: `buf breaking --against .git#branch=main,subdir=packages/proto` does not work from `packages/proto` (no `.git` there,
  and in a worktree `.git` is a file); the working form the worker actually ran is `../../.git#branch=main,subdir=packages/proto`.
- `dataplane.proto:401` `Event.interface` is a plain `string` while the task sketch says `interface?` and the file's convention is proto3 `optional`
  for optional scalars; harmless (empty = none) but inconsistent, and (F2) cannot be changed later.
- `dataplane.proto:548-550` numbering convention says "Zod fields in declaration order from 1"; once a message exists that cannot be honoured
  (P02a reordered several). Say "append-only; new fields take the next free number regardless of Zod order".
- `dataplane.proto:336` `rx_misses` comment ("packets received without a matching IP protocol") does not describe `/if/rx-no-buf` + `/if/rx-miss`.
- `dataplane.proto:138,152,220` `Operation`, `ResultCode`, `Severity` are very generic names in package `vrx.v1`; later files in the package
  (state RPCs, events) will want them. Renaming is free today (`ApplyOperation`, `ObjectResultCode`, `IssueSeverity`), impossible after the tag.
- `HealthResponse` fields 1–3 are byte-identical to `main` — good; note that `Health` is the only RPC P05a's stub already implements.

### F11 — LOW / process — no `docs/status/tasks/P03-contract.md`
- REVIEW-PROMPT §1 and 00-CONTEXT ("labelled `contract`" = `contract(<pkg>)` commit **plus** `<id>-contract.md`). The branch has the two
  `contract(proto):` commits and the CI contract guard passes; `P03.md` covers the content. Either the manager waives it for the contract task
  itself or the worker adds a five-line file pointing at `P03.md` and `docs/contracts/proto.md`.

## Scope (checklist §9)

- Files: everything changed is inside `packages/proto/**`, `apps/agent/gen/**`, `docs/contracts/proto.md`, `docs/status/tasks/P03*`,
  `pnpm-lock.yaml` (vitest for `@ngfw/proto`). No file-level scope creep. Commit subjects: `contract(proto)` ×2, `docs(P03)`, `chore(P03)` (envelope).
- Content: the task prompt lists "NAT/ACL/VPN messages (reserved numbers only)" as **out of scope**; the branch fully models `nat`, `objects`,
  `acl`, `vpn` (~90 messages) from the P02b/P02c WIP. The manager adopted this as D-P03-5 (D-034), so it is recorded here as an accepted
  deviation, not a finding. Its cost is F5/F6 (a large surface only literal-tested, and a bigger P03b); its benefit is that DF-*/P11 have a
  target to code against. The `reserved`-ranges decision (D-P03-6) is correct — `reserved` means "never use", and `RESERVED_FIELD_NO_DELETE` would
  have made filling them in a breaking change.

## Security (checklist §6)

- No `exec.Command`/`child_process` (contract only). No secrets in fixtures (`psk/site-b`, `wg/wg0` are references). `*_ref` convention is
  consistently applied in `vpn`. The one exception is F4 (`password_hash`).

## Re-verification (checklist §11) — all run by the reviewer in `/root/ngfw-wt/P03`

```
$ tools/ci.sh --base main            → … == agent ==  CI GATE PASSED   exit=0      (log: /root/ngfw-wt/logs/P03-review-ci.log)
$ cd packages/proto && buf --version → 1.73.0
$ buf lint                            → exit=0
$ buf breaking --against ../../.git#branch=main,subdir=packages/proto   → exit=0
  sanity (scratch copy, HealthResponse.agent_version string→int32):   → "changed type from string to int32", exit=100  (the tool really compares)
  scratch copy, Interface.rx_mode string→optional string:            → FIELD_SAME_CARDINALITY, exit=100  (basis of F2/F3 urgency)
$ pnpm gen; sha256sum …; pnpm gen; sha256sum … | diff   → "checksums identical";  git status --porcelain → (empty)
$ cd apps/agent && go test -count=1 ./gen/... -run DesiredState -v
  --- PASS: TestDesiredStateMirrorsRootKeys / TestDesiredStateFromSchemaExamples/{minimal,two-interfaces}.json   ok  ngfw/agent/gen/vrx/v1
$ pnpm --filter @ngfw/proto test      → ✓ test/desired-state.test.ts (7 tests)   Tests 7 passed (7)
$ toJSON probe (temporary test file, removed; tree clean afterwards)  → see F3
$ node import("@ngfw/proto") from apps/api → ERR_MODULE_NOT_FOUND (F9); tsc resolves (TS .js→.ts mapping)
```
Pasted output in `docs/status/tasks/P03.md` matches these runs.

## Required before merge (the "changes")

1. F1 — `forceLong=string` (or `bigint`) in `buf.gen.yaml`, regenerate, fix `proto.md` §9.
2. F2 — mirror `task/P02a@df554dc` types/presence for `system.banner`, `dataplane.corelist`, `Interface.rx_mode`, `NextHop.address`
   (additive P02a fields optional now, mandatory in P03b).
3. F3 — choose and implement the presence policy (recommended: `optional` on every `DesiredState` scalar), or remove the `toJSON`-diff promise from
   `proto.md`; add the probe as a test.
4. F4 — remove `password_hash` (or make stripping unconditional) and write the "secret-flagged leaves have no proto field" rule.
5. F7 — one paragraph in `proto.md` §2 defining unset-domain semantics (skip, or `subsystems` required).
6. F5/F10/F11 — wording fixes in `P03.md` and `proto.md`; `Event.interface` → `optional`; enum names; numbering sentence; `-contract.md` or waiver.

F6, F8, F9 go to P03b / P05 / P06 via `P03-questions.md` (add three lines).

**APPROVE WITH CHANGES**
