# `vrx.v1.Dataplane` — the agent↔API gRPC contract

Source: `packages/proto/vrx/v1/dataplane.proto`. Generated stubs: Go `apps/agent/gen/vrx/v1` (module path
`ngfw/agent/gen/vrx/v1`, package `vrxv1`), TypeScript `packages/proto/gen/ts` (`@ngfw/proto`, ts-proto v2 with
`@grpc/grpc-js` service stubs — D-005). Regenerate with `pnpm gen`; CI fails on dirty output. The same module also
holds the agent-internal reconciler object model `vrx.model.*.v1` (§10), generated for Go only
(`buf.gen.model.yaml`); `buf.gen.yaml` is restricted to `vrx/v1`.

**Changing this contract requires a `contract(proto): …` commit** (`tools/ci.sh --base main` contract guard) and
`buf breaking --against "../../.git#branch=main,subdir=packages/proto"` (run from `packages/proto`; in a worktree `.git`
is a file, hence the repo-root path) must stay green: field numbers are never reused, fields and messages are never
renamed or retyped after `contracts-v1` — only added, and removed fields become `reserved` (decision-policy #1). Lint
is buf `STANDARD` minus `SERVICE_SUFFIX` (D-008) and `RPC_RESPONSE_STANDARD_NAME` (stream element types and the DryRun
report keep their docs/04 names). Review fixes before the tag: `docs/status/tasks/P03-contract.md` (D-039…D-042).

Transport: gRPC over the unix socket `/run/vrx/agent.sock` (0660, group `VRX_SOCKET_GROUP`); tests use their slot's
`VRX_AGENT_SOCKET`. Only `vrx-api` talks to the agent (00-CONTEXT rule 1). No TLS, no auth on the socket — the file
mode is the boundary.

## 1. DesiredState — the configuration document as protobuf

`DesiredState` is a **1:1 projection of `RootConfig`** (`packages/schema`):

| Zod | proto | example |
|---|---|---|
| top-level key | field 1–13 of `DesiredState`, in `ROOT_KEYS` order | `routing` → `RoutingConfig routing = 5` |
| object domain | `<Key>Config` message | `NatConfig`, `ManagementConfig` |
| record (`z.record(name, X)`) | `map<string, X>` keyed exactly like the JSON | `interfaces`, `vrfs`, `objects.addresses`, `vpn.ipsec.tunnels` |
| array | `repeated` | `routing.static`, `nat.pools` |
| field name `fooBar` | `foo_bar` (protobuf JSON name is `fooBar` again) | `rxMode` ↔ `rx_mode` |
| `z.enum([...])`, literals | `string` — allowed values in the field comment | `mode: "ed" \| "ei"` |
| every scalar leaf (`.optional()` or not, with or without `.default()`) | proto3 `optional` — explicit presence, **D-039** | `optional uint32 mtu`, `optional bool enabled`, `optional uint32 id` |
| `z.discriminatedUnion(k, …)` | one message: discriminator + every variant's fields, non-active ones unset | `AddressObject{type, address?, prefix?, start?, end?, fqdn?}` |
| numbers | `uint32` (ids, ports, counts), `uint64` (byte/packet lifetimes — a JSON **string** in protobuf JSON and in TS), `int32` only where negative is legal | `HostAttachment.priority`, `IpsecRekey.esp_bytes` |
| schema leaf flagged `secret: true` | **no field** (D-040) — only `*_ref` references cross the boundary | `passwordHash` → nothing; `auth.secretRef` |

Consequence (tested in `apps/agent/internal/contracttest/desiredstate_test.go` and
`packages/proto/test/desired-state.test.ts` over every `packages/schema/examples/*.json` and `packages/proto/test/fixtures/*.json`):
**the protobuf JSON mapping of a valid configuration document *is* a `DesiredState`.** The API converts with
`DesiredState.fromJSON(stripSecrets(RootConfig.parse(doc)))`, the agent with strict `protojson.Unmarshal`
(`DiscardUnknown=false` — an unknown key is an error); no hand-written mapper per domain. `Retrieve` comes back the
same way.

The two record-shaped domains (`interfaces`, `vrfs`) are maps directly on `DesiredState` (a wrapper message would break
the projection). All other domains are `<Key>Config` messages, so per-domain renderers get one typed message.

### Presence (D-039)

Every scalar leaf under `DesiredState` is proto3 `optional`. Rationale (review F3): with implicit presence,
`toJSON`/`protojson.Marshal` drop proto3 defaults, so `vrfs.default.id = 0` came back as `{}` (false drift on every
Retrieve) and a tunnel disabled in VPP (`enabled: false` omitted, then re-filled to the Zod default `true`) compared
equal to an enabled one (hidden drift). With explicit presence only what was set is on the wire and comes back out —
`id: 0` and `enabled: false` included; fields the document does not set stay unset in Go (`nil` pointer) and TS
(`undefined`). Consequences for consumers:

- Go: scalars are pointers; use the `Get*()` accessors (zero value when unset) and `proto.String/Bool/Uint32` in
  literals; test presence with `x.Field != nil`.
- TS: fields are `T | undefined`; `fromPartial` and `fromJSON` leave unset fields `undefined`, `toJSON` omits them.
- `repeated` and `map` fields have no presence in proto3: absent and empty are the same thing (they are in Zod too:
  `.default([])` / `.default({})`).
- Zod fills defaults before Apply, so a *parsed* document always carries every defaulted leaf; the agent must still
  treat an unset scalar as "not specified" (never assume the Zod default).

### Running-vs-actual diff

`Retrieve` returns the same messages, so drift is `diff(toJSON(fromJSON(running)), toJSON(actual))` with the schema
package's `diff()`: **always pass the running document through the proto first**. That normalises what the projection
changes on purpose — 64-bit leaves become JSON strings (`espBytes: 1073741824` → `"1073741824"`), secret-flagged leaves
disappear (D-040), unknown keys are rejected — so the two sides are comparable. Never apply `RootConfig.parse()` to a
Retrieve result (it would re-fill defaults the agent deliberately left unset). The tests pin
`toJSON(fromJSON(doc)) deep-equals doc` for every corpus document (none carries a 64-bit number).

### Sync state and the drift guard (P03b)

Every domain mirrors the **merged** schema leaf for leaf: `system`, `dataplane`, `interfaces`, `vrfs`, `routing` and
`management` were synced on `task/P02a`, `nat`/`objects`/`acl` on `task/P02b`, `vpn`/`tunnels`/`services`/`ha` on
`task/P02c` (D-061); P03b cross-checked the result (no leaf missing, no type or presence mismatch, every field
commented). Numbering is append-only: a new field takes the next free number regardless of its position in the Zod
declaration; removed fields are `reserved` by number **and** name (`SystemConfig` 4 `ntp`, `RoutingConfig` 2/3
`prefix_lists`/`route_maps`, `ManagementUser` 4 `password_hash`, `CnatConfig.Snat.PolicyInterface` 2 `side`) — except
where the name was re-used on a new number (`HaConfig` 1, `vrrp` moved to field 2 as a map, D-053).

Two tests keep it that way; both run in `tools/ci.sh` and fail the gate on drift:

| guard | what it compares | catches |
|---|---|---|
| `apps/agent/internal/contracttest/drift_test.go` `TestSchemaProtoDrift` (Go) | the JSON Schema `pnpm gen` writes from `RootConfig` (`packages/schema/dist/json-schema/root.json`, `io: 'input'`) walked alongside the `DesiredState` descriptor, **both directions** | schema leaf without a proto field (same JSON name); proto field without a schema leaf; record ↔ `map<string,…>`, array ↔ `repeated`, object ↔ message mismatches; scalar type and width (string, bool, `uint32` for non-negative ranges ≤ 2³²−1, `uint64` above, `int32`/`int64` when the minimum is negative, `double` for non-integers); a scalar without explicit presence (D-039); a `secret: true` leaf that has a proto field (D-040); unions of different kinds |
| `packages/proto/test/parsed-documents.test.ts` (TS) | `DesiredState.toJSON(fromJSON(redactSecrets(RootConfig.parse(doc))))` against the parsed document, for `{}` (every default, all 13 domains prefaulted) and every valid example and fixture | keys the TS stubs drop or invent once Zod has filled defaults; value changes; secret leaves reaching `fromJSON` |

Known, accepted differences are listed in `acceptedDrift` in `drift_test.go`, each with its reason, and a stale entry
fails the test. Today there are four, all proto→schema supersets: the one shared `Redistribute` message has a key for
every protocol, while `routing.<p>.redistribute` rejects the protocol's own key (`bgp.redistribute.bgp`, …). A
schema→proto gap is never accepted. `TestSchemaProtoDriftDetectsBreakage` feeds the guard a deliberately broken schema
(extra leaf, removed leaf, retyped leaf, widened integer, secret leaf with a field, array → object) and requires every
finding, so a guard that passes everything cannot go unnoticed. Outside CI, a missing generated schema skips the Go
guard with the command to run (`pnpm --filter @ngfw/schema gen`); under `CI` it fails. `VRX_DRIFT_SCHEMA=<file>`
points the guard at another schema (used to demonstrate a failure, `docs/status/tasks/P03b.md`).

**Adding a schema leaf** therefore means, in the same branch: the Zod change, the proto field (next free number,
`optional` for scalars, a comment with the allowed values / Zod default), `pnpm gen`, and a `contract(proto):` commit.

Secrets never travel in `DesiredState` (00-CONTEXT rule 10, **D-040**): schema leaves flagged `secret: true` in their
`withUi()` meta (password hashes, private keys, PSKs, PINs) have **no proto field** — the API strips them generically
before `fromJSON`, and the strict tests reject a document that still carries one (`passwordHash` → "unknown field").
Only `*_ref` references cross the boundary; the agent resolves them through its own channel to the secret store and
never logs or persists their values. The agent therefore never holds authentication material, and `desired.pb` on
disk (P05) contains none.

## 2. Apply

`Apply(ApplyRequest) → ApplyResponse` — one transaction:

```
validate (every selected KV has a descriptor; mandatory dependencies present)
  → plan (diff desired vs Retrieve(): Create / Update / Recreate / Delete, topological order)
  → apply (creates/updates in order, deletes in reverse order; daemons after VPP, risky backends last — AD-4)
  → verify (re-Retrieve, compare)
  → persist desired state (/var/lib/vrx/agent/desired.pb) → confirmed, or start the confirm timer
```

Request forms: **apply** (`txn_id` + `desired_state` [+ `subsystems`, `confirm_timeout_sec`]), **confirm**
(`confirm_txn_id` only), **confirm-and-apply** (both: the pending transaction is confirmed first, then the new one is
applied). Any other combination is `INVALID_ARGUMENT`.

### Idempotency

1. **Declarative idempotency.** The plan is `diff(desired, actual)`; applying the same desired state twice yields an
   empty plan, status `APPLIED`, no `results`, `summary.unchanged = n`. The agent never performs an operation whose
   effect is already present (P05 acceptance: "applying the same desired state twice produces an empty plan").
2. **Retry idempotency.** `txn_id` is the idempotency key. The agent keeps the responses of the last ≥ 16 transactions
   (and the last one persistently). A repeated `txn_id` with an identical `desired_state` + `subsystems` returns the
   stored `ApplyResponse` without touching the data plane; a repeated `txn_id` with different content fails with
   `ABORTED`. The API therefore retries a lost response safely with the same id.
3. Transactions are serialised: one at a time per agent. A second `Apply` while one is running blocks until it finishes
   (bounded by the caller's deadline), it is never interleaved.

### Subsystems and authority (D-041)

Which domains a transaction manages is decided per top-level key (`ROOT_KEYS`):

| `desired_state.<key>` | `<key>` in `subsystems` | effect |
|---|---|---|
| **unset** (message absent; for `interfaces`/`vrfs`: empty map) | no | **skipped** — "not managed by this transaction"; nothing of that domain is touched |
| unset / empty | yes | **authoritative and empty** — every owned object of that domain is deleted |
| **present**, even as an empty message `{}` | no, `subsystems` empty | authoritative — whatever is absent from it is deleted (owned objects only, §6) |
| present | no, `subsystems` non-empty and does not name it | skipped — `subsystems` narrows |
| present | yes | authoritative |

So `subsystems` **narrows** the set of domains considered (empty = every domain present in `desired_state`), and
naming a key there is the only way to make an unset/empty map domain authoritative. A partial document — an API bug, a
hand-crafted `grpcurl` Apply, a caller that only knows about `system` — can therefore never wipe interfaces, VRFs or
routes it does not mention (review F7). The API sends the whole parsed document (all 13 domains present, Zod
prefaults every key) and selects with `subsystems`, so from its side "empty `subsystems` = all" still holds. Unknown
keys → `INVALID_ARGUMENT`; keys not implemented by this agent build → `UNIMPLEMENTED` (`HealthResponse.subsystems`
lists the implemented ones). Dotted sub-keys (`routing.bgp`) are not accepted in v1 (reserved for an additive
extension). `DryRun` applies the same table when planning.

### Outcomes

| `status` | data plane | gRPC status |
|---|---|---|
| `APPLIED` | converged and verified (unconfirmed if a timer runs) | OK |
| `FAILED` | untouched — validation/planning failed; `validation` explains | OK |
| `ROLLED_BACK` | back at the previous state; `results` lists what failed and what was reverted | OK |
| `DEGRADED` | intermediate — an operation and its rollback both failed; `Health.degraded = true`, Event `DEGRADED` (AD-4) | OK |
| `CONFIRMED` | unchanged — pure confirm | OK |
| — | malformed request, owner mismatch, unknown subsystem | `INVALID_ARGUMENT` |
| — | confirm of a txn that is not pending; new apply while another txn is pending confirmation (§4) | `FAILED_PRECONDITION` |
| — | `txn_id` reused with different content | `ABORTED` |
| — | VPP disconnected | `UNAVAILABLE` |

`results` has one `ObjectResult` per object touched or failed: scheduler `key` (`<descriptor>/<id>`), `op`, `code`,
`message` (never secrets), RFC 6901 `pointer` into the document (`/` in interface names escaped as `~1`) and
`subsystem`. The API maps a `FAILED`/`ROLLED_BACK` response to RFC 9457 `problem+json` using `pointer`. Unchanged
objects are not listed. `summary` carries counts; the same counts go out as Event `RECONCILE_DONE`.

## 3. DryRun

`DryRun(DryRunRequest) → ValidationReport`: validation + planning, **nothing is applied, nothing is persisted, no
events are emitted**. Same `desired_state`/`subsystems`/`owner` rules as Apply; no confirm fields (a dry run is not a
transaction). `errors` holds every finding (`ISSUE_SEVERITY_ERROR` first, then by `pointer`; `rule` is a stable id such as
`interfaces.vrf-exists`), `ok` is true when none is an ERROR, `plan` lists the operations in execution order with
`code` unset — converged objects are not listed, `summary.unchanged` counts them — and `summary` counts the rest. DryRun never fails with an application error; gRPC errors as for Apply. The commit engine runs DryRun as its
tier-3 validation before it touches the datastore.

## 4. Confirm timeout (self-revert)

`confirm_timeout_sec > 0` on an apply makes the transaction **pending**:

1. The agent applies as usual and answers `APPLIED` with `confirm_deadline` (= `applied_at` + timeout, agent clock).
   `Health` shows `pending_confirm_txn_id` / `confirm_deadline`.
2. Before the deadline the caller sends `Apply{confirm_txn_id}` → `CONFIRMED`; the timer is cancelled and the state
   becomes the confirmed baseline. `Apply{txn_id: B, desired_state, confirm_txn_id: A}` confirms A and applies B in one
   call.
3. If the deadline passes unconfirmed, **the agent itself** re-applies the last *confirmed* desired state as a normal
   transaction (rollback of the pending one), emits `CONFIRM_REVERTED` (`txn_id` = the reverted transaction), then
   `RECONCILE_START`/`RECONCILE_DONE` for the revert. It does not need the API for this (the API may be the thing that
   became unreachable — that is the point of the feature). A later `Apply{confirm_txn_id}` for that id fails with
   `FAILED_PRECONDITION`.
4. While a transaction is pending, a new apply **without** `confirm_txn_id` fails with `FAILED_PRECONDITION`: the
   caller must confirm or let it revert; there is never more than one pending transaction.
5. Persistence: the agent stores both the pending desired state and the confirmed baseline with the deadline. After an
   agent restart it resumes the timer; if the deadline already passed it reverts immediately after its startup resync.
   `kill -9` of the agent therefore never leaves an unconfirmed state confirmed by accident.
6. A revert that fails leaves the agent `DEGRADED` exactly like a failed rollback (AD-4).

The API layer implements `POST /config/commit?confirm=<sec>` as `Apply{confirm_timeout_sec}` and
`POST /config/commit/confirm` as `Apply{confirm_txn_id}`; the revision is persisted in PostgreSQL only after a
`CONFIRMED` (or after an `APPLIED` without timer).

## 5. Retrieve — what it must include

`Retrieve(RetrieveRequest) → RetrieveResponse{desired_state, subsystems, owner, retrieved_at}` dumps the **actual** state
and is what makes drift detection and restart safety possible (AD-3). Rules the agent must satisfy:

- **Everything owned, nothing else.** For each requested subsystem, the union of every descriptor's and renderer's
  `Retrieve()`: all objects tagged with this agent's owner (§6), including owned objects that are *not* in the current
  desired state (leftovers/drift — that is how they get deleted on the next Apply). Objects of other owners, VPP's own
  objects (`local0`, default tables) and unmanaged daemon state are never returned.
- **Same messages, canonical form.** Decoded into the `DesiredState` messages, canonicalised so that `proto.Equal` is a
  correct diff: addresses through `net/netip` (lower-case, no leading zeros), `repeated` fields sorted (addresses,
  next hops by address, rules by sequence), MACs lower-case, map keys = the object's document key (interface name,
  VRF name). **Presence is part of the value** (D-039): a scalar is set in the result exactly when the object carries
  it on VPP/in the daemon (`enabled: false` is returned as `false`, not omitted; a leaf the backend cannot report — e.g.
  `description` on objects without a tag — is left unset, never invented).
- **Configuration only.** No read-only status (link state, counters, sw_if_index, SA lifetimes) — those come from
  `StreamStats`/`StreamEvents`/state RPCs. Runtime handles live in descriptor `Meta`, never in the value.
- **No secrets.** `*_ref` fields are returned as stored (they are references). There are no write-only fields any
  more (D-040), so a Retrieve result never differs from the running document because of stripped material.
- **Partial coverage is explicit.** `subsystems` in the response lists what was actually dumped. With an empty request
  list, subsystems this build does not implement are omitted; naming one explicitly fails with `UNIMPLEMENTED`.
- **Consistency.** One Retrieve is a snapshot taken while no transaction is applying (it waits for a running Apply to
  finish); it may be called concurrently with itself and with the streams.
- **Never mutates.** Retrieve performs dumps only.

The API exposes it as running-vs-actual diff — `diff(toJSON(fromJSON(running)), toJSON(actual))`, see §1 "Running-vs-actual
diff" — and uses it in the integration proof "after commit, `Retrieve()` equals desired".

## 6. Ownership scoping

An agent process serves **exactly one owner** (`VRX_OWNER`, product default `vrx`; tests use their slot's
`VRX_TEST_PREFIX`, e.g. `w7`). Every object it creates is stamped — interface tag `<owner>:<id>` via
`sw_interface_tag_add_del`, owner-prefixed names or the owner table in the state dir for objects without tags (D-030,
`internal/vpp.OwnerTag`). Plan, rollback, resync and Retrieve act **only on owned objects**; two agents with different
owners on one VPP never touch each other's objects.

`owner` on `ApplyRequest`, `DryRunRequest` and `RetrieveRequest` is the caller's statement of which owner it expects to
be talking to: empty = the agent's owner; a different non-empty value fails with `INVALID_ARGUMENT` before anything is
planned. `HealthResponse.owner` reports the agent's owner. This makes a request that reaches the wrong agent socket on
the shared host fail loudly instead of being applied under a foreign tag. (Multi-owner agents were considered and
rejected: one desired state file, one confirm timer and one resync per process keep P05 simple.)

## 7. Streams — ordering guarantees

Common: each stream is independent (its own `seq` starting at 1, strictly increasing, no gaps unless stated); the agent
never blocks the data plane on a slow consumer; a stream ends only when the client cancels, the agent shuts down
(`UNAVAILABLE`) or an internal error occurs (`INTERNAL`). Reconnecting starts a fresh stream (`seq` restarts).

**StreamStats** (`StreamStatsRequest` → `stream StatsBatch`): one batch per `interval_ms` (default 1000; 200–60000),
read from the VPP stats segment without API round-trips (AD-5). Every batch is a **consistent snapshot** of all
requested interfaces taken at `ts`; counters are **absolute** (monotonic since VPP start or the last clear) so a
dropped batch loses nothing — the consumer derives rates from consecutive batches. `seq`/`ts` strictly increase; a gap
in `seq` means batches were dropped for a slow consumer (the agent buffers at most a few batches, then drops the
oldest). Interfaces that no longer exist are omitted; new ones appear in the next batch; entries are sorted by name.
Designed for ≤ 1000 interfaces at 1 Hz (≈ 100 KB/s); `worker_cpu` is only present when requested.

**StreamEvents** (`StreamEventsRequest` → `stream Event`): events are delivered in the order the agent observed them,
`seq` strictly increasing, **no replay** — the stream starts with events after the subscription (Health gives the
current snapshot: degraded, pending confirm, last reconcile). Ordering promises across kinds: `RECONCILE_START`
precedes every `RECONCILE_DONE` with the same `txn_id`; `CONFIRM_REVERTED` precedes the `RECONCILE_START` of the revert;
`VPP_DISCONNECTED`/`VPP_CONNECTED` alternate and a `VPP_CONNECTED` is followed by a resync `RECONCILE_START/DONE`
(empty `txn_id`); a `RECONCILE_START/DONE` pair whose `attributes.source` is set is a dynamic desired source's own
sync (S1, TD-8: empty `txn_id`, emitted once the sync finished, never for a sync that changed nothing), and an `ERROR`
event with `attributes.source` names a source that a transaction left out or whose loop stopped (`attributes.reason`,
and `attributes.key` when one object caused it); `LINK_UP`/`LINK_DOWN` reflect `want_interface_events` and are filtered by `interfaces`. The event
buffer is bounded; on overflow the agent drops the oldest events and emits one `ERROR` event `"dropped N events"` so
the gap is visible. Filters (`kinds`, `interfaces`) are applied before buffering.

**Action** (`ActionRequest` → `stream ActionOutput`): output chunks in production order; exactly one terminal `done`
(also after a failure: `exit_code ≠ 0`, `summary` explains). `pcap_chunk` bytes concatenate to a valid pcap file (the
first chunk starts with the global header). Cancelling the call stops the action within one interval. Actions run via
the VPP API or fixed-argv allow-listed binaries (`internal/renderers` runner) — no user input reaches a shell
(00-CONTEXT rule 9); the agent validates every argument (address literals, bounded counts, token-list filter).
P05 returns `UNIMPLEMENTED` until P08/F-* implement the actions.

## 8. Health

`Health` is cheap (no VPP round-trip beyond the cached connection state) and polled by the API every few seconds. Beyond
the original fields (`agent_version`, `vpp_connected`, `vpp_version`) it reports `owner`, implemented `subsystems`,
`last_txn_id`, `pending_confirm_txn_id`/`confirm_deadline`, `degraded`, `last_reconcile_at`, `reconcile_in_progress`.
The API's `/api/v1/state/system` and the UI's status bar derive "data plane OK / degraded / unconfirmed commit" from it.

### 8a. InterfaceState (P08, additive)

`InterfaceState(InterfaceStateRequest{names, owner}) → InterfaceStateResponse{interfaces[], owner, retrieved_at}` is the
**state RPC** §5 points to for interfaces: the live table the API's `GET /api/v1/state/interfaces` merges with the
configuration. One entry per interface this agent can name (its own tagged/created ones and untagged ones such as
DPDK NICs — never another owner's, never `local0`), sorted by logical name: `name` (D-069 logical name = config key),
`vpp_name`, `sw_if_index`, `type`, `admin_up`, `link_up`, `mtu` (L3), `link_mtu`, `mac`, `ipv4[]`/`ipv6[]`, `vrf`
(+ `table_id`), sub-interface `parent`/`vlan_id`/`inner_vlan_id`, `managed`, `link_speed_kbps`, `rx_mode`,
`description` (from the stored desired state, D-073b). Dumps only; `UNAVAILABLE` without VPP. Counters stay in
`StreamStats` (keyed by `vpp_name`).

## 9. Compatibility rules for consumers

- Treat unknown enum values as `UNSPECIFIED` (new kinds/codes may be added).
- Never rely on message field order in JSON; rely on names.
- **64-bit integers are `string` in TypeScript** (ts-proto `forceLong=string`, D-039/F1) and `uint64` in Go: every
  `StatsBatch` counter (`rx_bytes`, `tx_bytes`, `seq`, …), `WorkerCpu.calls/vectors`, `IpsecRekey.esp_bytes/esp_packets`.
  The default `number` mapping *throws* above 2^53 = 9.0 PB; counters are absolute since VPP start, so at 100 Gbit/s
  that is ~8.3 days, at 10 Gbit/s ~83 days — not decades. Consumers convert with `BigInt(s)` for arithmetic and derive
  rates from consecutive batches; `string` is also the canonical protobuf JSON form, so `toJSON` output is identical
  between Go and TS.
- Explicit presence everywhere under `DesiredState` (§1): check `!== undefined` / `!= nil`, never `!== 0` / `!== ""`.
- `google.protobuf.Timestamp` is a `Date` in TypeScript and `*timestamppb.Timestamp` in Go; all times are agent clock,
  UTC.

## 10. Reconciler object model (`vrx.model.*.v1`, agent-internal, D-055)

`packages/proto/vrx/model/**` holds the **Value types of the scheduler's descriptors** — one message per VPP object
kind, in VPP terms (table ids, address ranges, protocol numbers, logical interface names), i.e. what the desired-state
builder (P08) produces from `DesiredState` and what `Retrieve()` decodes VPP dumps into. They are not part of the
API↔agent wire contract: `dataplane.proto` does not import them (pinned by `TestModelStaysAgentInternal`) and no
TypeScript is generated for them. They live here so that the contract owner versions them, `buf lint`/`buf breaking`
cover them and the factories stop carrying `*structpb.Struct` stand-ins.

| package (Go import) | mirrors | messages |
|---|---|---|
| `vrx.model.acl.v1` (`ngfw/agent/gen/vrx/model/acl/v1`, `aclv1`) | DF-4 typed specs, `apps/agent/internal/descriptors/acl/spec.go` (structpb field names) | `Acl`, `AclRule`, `MacipAcl`, `MacipRule`, `InterfaceBinding`, `EtypeWhitelist`, `MacipBinding`, `StatsEnable` |
| `vrx.model.nat.v1` (`…/model/nat/v1`, `natv1`) | DF-3 typed specs (`task/DF-3@08d0af4`, json tags) of nat44-ed, nat44-ei, nat64, nat66, det44, map, cnat, pnat | shared `Endpoint`, `Timeouts`, `InterfaceFeature`, `OutputFeature`, `Forwarding`, `IdentityMapping`; per plugin `Nat44Ed*`, `Nat44Ei*`, `Nat64*`, `Nat66*`, `Det44*`, `Map*`, `Cnat*`, `Pnat*` |
| `vrx.model.iface.v1` (`…/model/iface/v1`, `ifacev1`) | DF-1's descriptor-local `iface_model.proto` (identical names, numbers, types, enum values) | `AdminState`, `Mtu`, `MacAddress`, `Promisc`, `RxMode` (+`RxModeKind`), `RxPlacement`, `Subinterface`, `InterfaceAlias` |

Rules that differ from `DesiredState` on purpose:

- **Implicit presence.** The scheduler diffs values with `proto.Equal`; the typed specs always carry every field and
  the zero value is each field's canonical "not set / any" (documented per field). Explicit presence (D-039) exists
  for the JSON running-vs-actual diff, which these messages never take part in.
- **Field names = the spec's structpb/json names** (snake_case), so the factory's adaptation is mechanical: replace
  `Encode`/`Decode` or `Proto()`/`FromProto()` with the typed message. Strings with documented values stay strings
  (`side: "inside" | "outside"`, `action`, `protocol`), as in the specs and in `DesiredState`.
- **Canonical form is the descriptor's**, unchanged (netip addresses, sorted repeated fields, `CanonProto`).

Adaptation (not done here — descriptor code belongs to the factories): each factory switches its Value type in a
small follow-up commit on its next task (D-055); DF-1 then deletes its local `iface_model.proto`. Until then both
copies exist; the P03b evidence shows them field-for-field identical. New descriptor families add their messages here
under `vrx/model/<family>/v1` with a `contract(proto):` commit.

## 11. Feature RPCs

Each wave-A/B feature documents its new RPCs, `ActionRequest` members and `EventKind` values here, as
`### <task-id>: <Rpc>` directly below its own anchor (docs/status/wave-A-hotspots.md C6). Sections are appended,
never renumbered; field and enum numbers come from wave-A-hotspots.md §2.

<!-- wave-A: F-bonding -->
<!-- wave-A: F-bridge-l2 -->
<!-- wave-A: F-loopback-bvi-gso-lldp-span -->
<!-- wave-A: F-vrf-static-ecmp -->
<!-- wave-A: F-neighbors-ra -->
<!-- wave-A: F-rpf-adl-pbr -->
<!-- wave-A: F-object-model -->
<!-- wave-A: F-acl -->
<!-- wave-A: F-host-acl-nftables -->
<!-- wave-A: F-nat44-ed-sessions -->
<!-- wave-A: F-nat44-ei-64-66-nptv6 -->
<!-- wave-A: P11 -->
<!-- wave-A: F-wireguard -->
<!-- wave-A: P12 -->
<!-- wave-A: F-kea-dhcp-relay -->
<!-- wave-A: F-unbound-chrony-syslog -->

### F-lisp: LispState

`rpc LispState(LispStateRequest) returns (LispStateResponse)` — additive (F-lisp, WBS D6.8). Read-only snapshot of the live
LISP / LISP-GPE state from the VPP dumps: the two global switches (`show_lisp_status`), the PITR locator set
(`show_lisp_pitr`), local locator sets with their locators (`lisp_locator_set_dump` / `lisp_locator_dump`), the EID table
— local EIDs and the map-cache, static and learned (`lisp_eid_table_dump`), adjacencies per VNI (`lisp_eid_table_vni_dump`
+ `lisp_adjacencies_get`), EID-table maps (`lisp_eid_table_map_dump`, L2 and L3), map-resolvers / map-servers and the VNIs
that carry LISP-GPE forwarding entries (`gpe_fwd_entry_vnis_get`; the entries' locator pairs cannot be read, V13). LISP
objects carry no owner tag, so the snapshot is VPP-wide, like `show lisp …`; `owner` echoes the request. Owner mismatch →
`PERMISSION_DENIED`, VPP not connected → `UNAVAILABLE`, LISP plugin missing → the response has `enabled: false` and empty
lists. Config: `DesiredState.tunnels.lisp` (`TunnelsConfig` field **10**, `LispConfig`; docs/status/wave-BC-numbers.md).
