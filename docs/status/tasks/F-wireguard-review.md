# F-wireguard review — WireGuard interfaces, peers, keys (wave B, D6.5)

Reviewer pass over `task/F-wireguard` @ `0e19dcaf` (merge base with main `10059d57`; main merged in twice:
`41e42258` TD-8, `d07c064d` TD-7/TD-11b/TD-4), against `/root/ngfw-wt/F-wireguard.envelope.md`,
`prompts/features/F-wireguard.md`, `docs/status/tasks/F-wireguard{,-questions,-contract}.md`, 00-CONTEXT rules 1/2/5/6/10,
`docs/decisions/PENDING-secret-channel.md`, LOG D-040/D-051/D-063/D-091/D-096/D-119/D-125/D-128/D-129/D-132/D-133/D-134
and the TD-11b ownership protocol (`descriptors/dfkit/persist`). 69 files, +8069/−441 (about 60 % generated or tests).

## Verdict: APPROVE WITH CHANGES

The security core holds: no key material crosses the API→agent boundary, nothing secret is persisted by the agent,
the key-pair action never returns or logs the private key, and a peer is never created without its PSK. The "secret
store" is an in-memory `vpn.Resolver` stand-in that nothing fills in a product build, so it does **not** pre-empt
PENDING-secret-channel. Nothing is at BLOCK level. Before merge, three small items are required. **F1**: a validation
rule against the routing loop that `routeAllowedIps` can build. **F2**: a guard test that keeps the file fixture
test-only. **F3**: correct Q1's "nothing else changes" claim, and the manager adds that requirement to the PENDING.
Everything else is a follow-up. After the fixes, a focused re-verify of F1–F3 is enough; no full re-review.

## 1. Secrets (top priority)

**What the "secret store" is.** `subsystems.WireguardSecrets` (`apps/agent/internal/subsystems/wireguard_secrets.go`) is
a per-process, in-memory map from D-051 reference to 32-byte material, with the D-051→DF-5 mapping
(`key/<n>` → `x25519:<pub>`, `psk/<n>` → `hmac:<hex>` via the D-096 keyer). It implements `vpn.Resolver`. It is
compiled into the product agent, and in a product build it is **always empty**:
- no RPC, proto field, socket or file fills it (grep of the non-test tree: `Put`/`PutBase64` are called only from the
  fixture loader);
- it is not persisted (no state-dir file, no sealed cache);
- the only non-test fill path is `wireguard_fixture.go`. That file is behind `//go:build vrxtestsecrets` (line 1) and
  hooked through the nil-by-default `wireguardFixture` var (`subsystems/wireguard.go:49-51, 75-79`). It reads a
  0600, same-uid, `O_NOFOLLOW` JSON file named by `VRX_TEST_WG_SECRETS`, and it logs a WARN when it loads. The product
  `Makefile` builds without tags. Only `test/topology/wireguard/stack.sh:57` builds `bin/vrx-agent-wgtest` with the tag.

This is the envelope's "slot-local `vpn.MapResolver` fixture", extended to a test-build agent binary. It is test-only
and sits behind the Resolver seam, so **not BLOCK**. Two caveats:
- The build tag is the only thing that keeps the file channel out of a product build (F2).
- Q1 describes the store as the channel's future receiver ("its receiver calls `WireguardSecrets.Put`; nothing else
  changes"). That pre-judges option 1's shape and is also inaccurate (F3). Treat the store as a stand-in. The
  PENDING answer (and its ARCH-07 cleanup of the resolver interfaces, now one more) defines the receiver.

**Key-pair action** (`apps/api/src/features/wireguard/wireguard.controller.ts:196-220`):
- `@MinRole('admin')`, the same as `POST /secrets` (D-091). The new route is in `ADMIN_ONLY` (`route-guard.test.ts:37`).
- Uses `generateKeyPairSync('x25519')` and stores the private key through `SecretsService.put('key', name, …,
  {replace:false})`: AES-GCM with the name as AAD, a version row, and 409 on an existing name.
- Returns `{ref, publicKey, version}` only. The audit `after` holds `{ref, publicKey, version}`, and the service's
  system event holds `ref → version`.
- The e2e test decrypts the stored row, checks that it is the private key of the returned public key, and asserts it
  is absent from the response and from `GET /secrets`. Operator and read-only get 403. `../x` gets 400 problem+json.
- Clean.

**Where key material could leak. Checked, none found:**
| surface | finding |
|---|---|
| agent logs / errors | every added `fmt.Errorf`/`slog` carries D-051 handles or `vpn.Redact`ed DF-5 refs. The store formats as `WireguardSecrets(n secrets)` for `%v/%+v/%#v`/slog (unit-tested: raw, hex, decimal) |
| Retrieve | the interface Retrieve uses `show_private_key=false` + `vpn.Zero`. The PSK is fingerprinted by DF-5. A unit test asserts no `x25519:`/`hmac:` in the Retrieve JSON (one fallback exception, F6) |
| DryRun | the `agent.secret-unavailable` warning names the D-051 ref only |
| WireguardState RPC / `GET /state/vpn/wireguard` | `DumpState` (`descriptors/wireguard/state.go:56-72`) uses `show_private_key=false` + `Zero` and the **v1** peers dump (no PSK). Only public keys go out |
| WS `wireguard.events` | the attributes are `public_key`, `peer_index`, `established`, `dead` (`subsystems/wireguard.go:219-239`) |
| audit rows | see above |
| `vppctl` evidence in docs | every pasted `show wireguard interface/peer` line carries `private-key: <redacted>`, `pre-shared key: <redacted>` or `<hex-redacted>`. `stack.sh:30-37` redacts everything after `private-key` on VPP's one-line format (public key and mac-key included) |
| fixtures / tests | vectors are SHA-256 of `VRX_TEST_PSK_F-wireguard_*` labels (derivable, not secrets, 00-CONTEXT fixture rule). The stack fixture file is written at run time under `/run/vrx-test/w6/wg` (0700 dir, 0600 file) and removed on exit. Every key-shaped string the branch adds is a **public** key: `HIgo9…ykw=`, `xTIBA5…8Dg=` (both already in `packages/schema/examples/vpn-wireguard.json` on main), the byte-ramp test value `AAECAw…Hh8=`, and the host peers' public keys in the status file |
| UI | the browser-generated client private key lives only in React state (`WireguardPage.tsx:104`), goes into a Blob download whose URL is revoked, and is never sent. The key-pair screenshot shows the reference and public key only (checked the PNG) |

**PSK rule (Q1).** A peer is never created without its PSK. The builder emits `unavailable:<psk ref>`, and
`Peer.Create` refuses it before any VPP call (`peer.go:106-108`, unit test `TestWireguardWithoutSecretMaterial`:
ROLLED_BACK, 0 peers, no material in the error). The same holds for the interface private key. WARNING at DryRun plus
a loud failure at the object is the right choice **for now**, for a reason the questions file does not give: a
projection ERROR aborts the whole reconcile (`agent/service.go:382`, "validation failed"). An ERROR would therefore let
one WireGuard interface without material block the resync of *every* domain after an agent restart. The WARNING keeps
that damage inside the WireGuard objects. The cost is that a user commit touches VPP before it fails and rolls back
(AD-4), follow-up F4. See F3 for the resync consequence.

**gitleaks.** `gitleaks git --log-opts=10059d57..HEAD` finds one hit: `generic-api-key`,
`apps/api/test/e2e/wireguard.e2e.test.ts:22` @ `efcf783a` (`const peerKey = 'HIgo9…ykw='`). That value is the public
schema example key already on main (`packages/schema/examples/vpn-wireguard.json:47`, `vpn.test.ts:80`), a false
positive of the same class as D-067. `gitleaks dir` over the working tree reports no leaks. `git diff 10059d57 HEAD |
gitleaks stdin` (the squash-equivalent diff) reports no leaks. The D-112 squash drops it. `refs/archive/F-wireguard`
keeps it, but CI's gitleaks scan is scoped to `merge-base..tip`.

## 2. Architecture

- **`wireguard.meta`** (`descriptors/wireguard/meta.go`) is an agent-local table of name, description, D-051 ref,
  underlay-VRF name and flag. It is journaled, rolled back and never written by DryRun. The store is a 0600 JSON file
  under the state dir, written via temp file, fsync, rename and directory fsync. It is declared with `CheckPersistent`
  (TD-11b). It is restart-safe: VPP loss leaves the table, and resync recreates the objects. A lost file self-heals,
  because the desired meta KVs are recreated. **It is not a D-063 echo.** The table is the agent-local object itself,
  like a rendered file. The assembler uses it only for facts VPP cannot hold, and only for objects VPP actually reports
  (`desired/wireguard.go:286-314`). `routeAllowedIps` is recomputed from the routes that really exist (`:333-371`), and
  the underlay VRF comes from the peers' real `table_id` when peers exist. It is better than a stored-document seam:
  that would echo desired config into Retrieve, and names would show even after a failed apply. Follow-ups F7 and F8.
- **Events.** The watcher publishes through TD-8's `Wiring.Publish` → `bus.publishFeature`. The bus clones, never
  blocks and drops UNSPECIFIED (`agent/events.go:79-86`). Re-subscription on every connect waits for the old watcher,
  so `want(0)` happens before `want(1)`. Correct.
- **The unanchored line in `Wiring.Connected`** (`subsystems/subsystems.go:314`, Q4) is acceptable. `Connected` is the
  only per-connection hook: VPP drops the event registration together with the client, and `DynamicSource.Run` starts
  once. F-neighbors-ra adds the same kind of line (`neighborsRaConnected(ctx)`), so it is a trivial union at merge. See
  the Q4 recommendation.
- **`vpn.Require` wrapper** (`subsystems/wireguard.go:97-113`, Q8) is acceptable as a stopgap. I verified that the
  declaration is true: `vpn.Require` only reads the global, has a no-op Delete and records in no store
  (`descriptors/vpn/globals.go:47-96`). The embedding forwards every method (including `DeleteOnAbsence`). A test pins
  the declaration.
- **Architecture rules.** Node never talks to VPP: the API does X25519 key generation only. All VPP names come from
  binapi; `vppctl` appears only in the test script and host tests. Config types come from the one schema: the web forms
  derive from `domainSchemas`, and the API state output schemas are API-local, like the other state routes. The agent
  stays declarative. Rule 10 holds (§1). The A5 files (`agent.go`, `service.go`, `state.go`, `ifstate.go`) are
  untouched. `service_test.go` carries only the D-129 F5 registry-derived lines.

## 3. VPP

- **NBMA confirmed from source.** `wireguard_if.c:170` sets `VNET_HW_INTERFACE_CLASS_FLAG_NBMA`, and
  `fib_path.c:703-706` makes an attached path on an NBMA interface a **drop**. Without a route whose next hop lies
  inside a peer's allowed IPs, the tunnel carries nothing, not even replies to its own peer. The host evidence agrees:
  the ping failed without the flag and passed with it.
- **Default `routeAllowedIps=false`.** It is the right *schema* default (TNSR parity), because `true` would
  auto-install `0.0.0.0/0` for full-tunnel peers, which builds the loop in F1. For users it is a trap, though, so see
  the Q2 recommendation. The peer adjacency stacks on the endpoint's /32 in the *underlay* table
  (`wireguard_peer.c:121-132`, `adj_midchain_delegate_stack`). A `0.0.0.0/0 via 0.0.0.1 wg<N>` in the same table
  therefore resolves the tunnel's own UDP through itself.
- **V-new: no per-peer counters.** Correct. The plugin registers no stats counters at all (no `vlib_*counter_main_t` in
  `src/plugins/wireguard`), and `wireguard_peers_details` has flags and endpoint only. The fallback (interface
  counters, plus handshake times observed from events) is honest and documented. The merger assigns a V-number.
- **`peer.go` PartialCreate** (`peer.go:136-138`, D-133). After `wireguard_peer_add_v2` succeeded, a
  `want_wireguard_peer_events` failure now returns Meta + `scheduler.PartialCreate(err)`. The reconciler therefore
  journals the peer and the rollback deletes it. `TestPeerCreatePartialWhenEventRegistrationFails`
  (`meta_state_test.go:160-199`) asserts `IsPartialCreate` and that no peer or interface is left, so it cannot pass on
  the old bare-error code. I could not execute it against the old code, because the sandbox refused a scratch copy of
  the module. The assertion settles it.

## 4. D-132

- **Per call.** `WireguardState` does one `sw_interface_dump`, one `wireguard_interface_dump` (VPP-wide, then filtered
  by owner tag) and one v1 `wireguard_peers_dump` (VPP-wide, filtered), plus one stats-segment read.
- **Serialisation.** Calls are serialised by `wireguardStateMu` (`agent/rpc_wireguard.go:24, 39-40`).
- **Rate.** The UI polls every 30 s (`queries.ts:17`) and on Refresh. Live status comes from events. The events path
  does one single-peer lookup per event (DF-5). The rule is met in substance.
- **Gaps (F9).** The query inherits the global `staleTime: 5_000` (`App.tsx:10`), so window focus and remount refetch
  sooner than every 30 s. Refresh has no cooldown. With no single-flight in the API or agent, N browsers produce N full
  walks per 30 s.

## 5. Contract

The change is additive:
- `WireguardInterface.route_allowed_ips = 12` (`optional bool`), `EVENT_KIND_WIREGUARD_PEER_CHANGED = 13` and
  `rpc WireguardState`, plus four messages in the `// ----- F-wireguard -----` section.
- The schema adds one key line, `routeAllowedIps` (default false), and three rules in the owned
  `semantic/wireguard.ts`.

These are the ledger's numbers (`wave-A-hotspots.md:82-85`: WireguardInterface 12, EventKind 13). Collision scan over
every `task/*` branch's `dataplane.proto`:
- EventKind 10: F-neighbors-ra; 13: F-wireguard only; 14/15: P12 (ledger); 12: reserved for P11, which has no branch.
- No other branch touches `WireguardInterface` or defines the `WireguardState*` names.
- F-kea-dhcp-relay and F-unbound-chrony-syslog add no EventKind.

The drift guard (`contracttest`) passes. There is one doc mismatch, F10.

## 6. Tests and evidence

Run by me (no host runs, no VPP access):
```
cd apps/agent && TMPDIR=/tmp/g-rv6 go test -race -count=1 ./internal/descriptors/wireguard/... ./internal/descriptors/vpn/... ./internal/subsystems/... ./internal/desired/...
ok  ngfw/agent/internal/descriptors/wireguard 1.266s · ok .../descriptors/vpn 1.095s · ok .../subsystems 6.679s · ok .../desired 1.165s
TMPDIR=/tmp/g-rv6 go test -race -count=1 ./internal/agent/...          → ok  ngfw/agent/internal/agent 12.742s
go test -count=1 ./internal/contracttest/... ./internal/descriptors/core/... → ok, ok
apps/api:  vitest run src/features/wireguard src/auth/route-guard.test.ts  → 2 files, 9 tests passed
apps/web:  vitest run src/domains/vpn src/nav                              → 2 files, 9 tests passed
packages/schema: vitest run src/semantic/wireguard.test.ts                → 6 tests passed
gitleaks: tree clean; squash-equivalent diff clean; history 1 hit (efcf783a, public key, §1)
```

Host evidence, judged from the pasted output (it is consistent and specific):
- **Retrieve == desired.** `TestWireguardOnHost` lists 13 objects with pointers, then prints "Retrieve == desired",
  and an idempotent re-apply gives `unchanged: 13`. The full-stack run shows `drift: {"changes":[]}` for `/vpn`.
- **Restart.** In-process: binapi deletes, then resync converges in 0.145 s. Real process: SIGTERM, CLI deletes of 2
  peers and the interface, then start; everything is back in 0.85 s and the kernel peer re-establishes. Both are well
  inside 30 s.
- **Rollback.** In-process: `deleted: 13`, then an empty Retrieve and nothing of `w6wg` in VPP. API e2e with the fake
  agent. The real-agent rollback step did not run, because the shared VPP's V19 placeholder cap refused every interface
  create (another slot's classify churn, Q13). It is a re-run item once TD-25 lands; it does not block.
- **Handshake.** A real kernel WireGuard peer in the netns, reached over a **tap**, not af_packet (V24). The run shows
  the `EVENT_KIND_WIREGUARD_PEER_CHANGED established` event, a ping through the tunnel with 0 % loss, and the UI chip
  turning Established from the relayed event. VPP `NRestarts` stayed unchanged (1→1, 2→2). The Go handshake test
  relies on the tap descriptor's TD-3 sanitizer; `stack.sh` additionally ran `vrx-vpp-preflight`.
- **Duplicate public key.** 400 with pointer `/vpn/wireguard/interfaces/b/peers/dup/publicKey` (e2e and stack).
- **Limitation.** Every end-to-end run used the test-build fixture. The product path is proven only as "fails loudly,
  leaves nothing" (unit test), as designed until the PENDING is answered. IPv6 and `0/0` auto-routes were not exercised
  on the host (F12).

## 7. Merge fit

`git merge-tree --write-tree main task/F-wireguard` → **clean** (`68700a03…`). Pairwise with the queued branches
(`merge-tree task/<b> task/F-wireguard`):
- **TD-23**: conflicts only in `coretest/fakevpp.go`. At the rebase, drop F-wireguard's `extensions` slice and loop
  (`fakevpp.go:97-105`) and change `coretest/wireguard.go:31` to
  `func init() { RegisterExtension("wireguard", installWireguard) }`. Its messages (`wireguard_*`, `wg_set_async_mode`,
  `want_wireguard_peer_events`) collide with no other extension, so TD-23's `On` guard will not panic. F-wireguard adds
  no `action` handler, so fake-agent.ts needs no TD-23 registration. Its `wireguardState:` line stays under the anchor.
- **P12, F-kea-dhcp-relay, F-vrf-static-ecmp**: `fake-agent.ts` gets two unions. The import lines all go at the end of
  the import block (`fake-agent.ts:41`, no anchor). The handler lines sit under adjacent anchors (`:662`). Generated
  files: regenerate.
- **F-neighbors-ra**: the `Connected` line needs a union (`subsystems.go:314`). The rest of that list comes from its
  older base.
- **F-object-model, F-nat44-ed-sessions, F-rpf-adl-pbr**: the `service_test.go` implemented-domains lines are identical
  (D-129 F5: take main's).
- **F-unbound-chrony-syslog, F-vrf-static-ecmp, F-nat44-*, F-rpf-adl-pbr**: `docs/vpp-code-track.md` needs a V-section
  union.
- `route-guard.test.ts:37`: an end-of-block line with no anchor, so a union.
- **TD-13, TD-8b, TD-25, TD-11c, TD-10a, TD-24, UI-domain-editor**: clean.
- **Squash (D-112)**: the branch touches `packages/schema` and `packages/proto`, so the subject must start with
  `contract(`. Use `contract(schema,proto): F-wireguard …`, as the CI squash simulation already did.

## Findings

| id | sev | where | finding | action |
|---|---|---|---|---|
| F1 | **M (required)** | `desired/wireguard.go:211-223`, `packages/schema/src/semantic/wireguard.ts`, `docs/user/vpn/wireguard.md:66` | With `routeAllowedIps`, every allowed IP becomes `ip.route/<overlay table>/<prefix>`, including `0.0.0.0/0` / `::/0`. When `vrf == underlayVrf` and an allowed IP covers a peer's endpoint (the classic full-tunnel peer), the peer's midchain stacks on the endpoint /32 in the underlay table, which now resolves via `wg<N>` itself. The result is a FIB loop, a dead tunnel, and that VRF's default route hijacked. Nothing rejects it, and the user doc shows `0.0.0.0/0 via 0.0.0.1 wg0` without a warning | Add a semantic rule in the owned `semantic/wireguard.ts` (e.g. `vpn.wireguard-route-loop`): with `routeAllowedIps` and `vrf` == `underlayVrf`, an allowed IP that contains a peer's IP endpoint is an error at that allowedIps pointer. Add a warning paragraph to the user doc. A static route with the same prefix already fails as `agent.duplicate-object`; say so in the doc |
| F2 | **M (required)** | `subsystems/wireguard.go:49-51, 75-79`, `wireguard_fixture.go:1, 26` | The file secret channel is test-only by build tag alone | Add an untagged unit test asserting `wireguardFixture == nil` in a default build. It fails if the `init` ever moves out of the tagged file. The manager adds `vrxtestsecrets` to `tools/ci.sh` forbidden patterns outside `wireguard_fixture.go` / `test/topology/wireguard/` / docs (ci.sh is not this row's) |
| F3 | **M (required, doc)** | `desired/wireguard.go:90-102`; `descriptors/wireguard/interface.go:124-126`, `peer.go:145-147`; questions Q1 | "When the channel lands … nothing else changes" is wrong. The marker `unavailable:<ref>` differs from the retrieved `x25519:<pub>`/`hmac:`, both descriptors answer `ErrRecreate`, and a resync without material therefore **deletes working tunnels** (Delete succeeds, Create fails). This is harmless today (the product cannot create WireGuard), but it is a hard requirement for the channel | Correct Q1 and the status file. The manager adds to PENDING-secret-channel: "material must be loaded before the first resync (option 1's sealed cache is mandatory, not optional); otherwise the descriptors must keep existing objects while material is unavailable (needs a scheduler hook)" |
| F4 | L | commit path (API/agent) | With the WARNING design, a user commit applies unrelated objects, then fails at the WireGuard object and rolls back (AD-4 wants validate-before-touch). The agent must keep WARNING for resync (§1) | Follow-up (TD-10a): the API commit engine treats `agent.secret-unavailable` in the DryRun report as blocking for user commits |
| F5 | L | `subsystems/wireguard_secrets.go:73-77` | Two D-051 refs with identical material share one DF-5 ref. Replacing one zeroes and deletes the shared `byRef` entry, so the other ref still maps (`Ref`) but can no longer be resolved. It fails loudly and leaks nothing | Refcount `byRef`, or re-point it to the surviving ref's copy |
| F6 | L | `desired/wireguard.go:353` | Without a meta row, Retrieve reports `presharedKeyRef: "hmac:<8 hex>…"`. That is a keyed-fingerprint prefix (not material), but a DF-5 ref leaves the agent and it is not a valid D-051 ref | Use a fixed placeholder (e.g. `psk/<unknown>`) |
| F7 | L | `desired/wireguard.go:307-313, 349-351`, `meta.go:30-37` | The meta join trusts the stored ref. A key changed out of band, or a stale row, still shows `key/<n>` | Store the DF-5 ref in `MetaSpec`; attribute the D-051 ref only when VPP's key matches it, otherwise show drift |
| F8 | L | `meta.go:94-108, 119-133` | Every object does a full-file Load, rewrite and fsync: O(n²) bytes and n fsyncs per transaction (1000 road-warrior peers means 1000 fsyncs) | Batch per transaction (D-133 style) or fold into TD-16 |
| F9 | L | `web/.../wireguard/queries.ts:12-18`, `WireguardPage.tsx:143`, `agent/rpc_wireguard.go:39` | D-132 in practice: a 5 s staleTime means focus and remount refetches; Refresh has no cooldown; N browsers produce N walks | `staleTime: WG_STATE_POLL_MS`, `refetchOnWindowFocus: false`, a Refresh cooldown; optional single-flight in the agent |
| F10 | L | `docs/contracts/proto.md:382` vs `subsystems/wireguard.go:233` | The doc says the event `message` is `"established"/"dead"/"down"`; the code sends `"WireGuard peer <8>… on wgN: <state>"` | Fix the doc |
| F11 | L | `apps/api/src/features/wireguard/fake.ts` | The fake agent commits WireGuard configs that the real agent refuses (PENDING). The UI and e2e tests exercise a path users cannot reach yet | Note it in the status file (the user doc's release note covers users) |
| F12 | L | `desired/wireguard.go:229-239` | The auto-route next hops `0.0.0.1` (for 0/0) and `::1` (for ::/0), and IPv6 allowed IPs, were not exercised on the host | Add them to the post-TD-25 stack re-run |
| F13 | L | `desired/wireguard.go:63-66, 148-168` (Q6) | A gRPC Apply naming `interfaces` without `vpn` deletes the admin-state, MTU and addresses on `wg<N>` | Write "the API always sends all implemented domains" into `docs/contracts/proto.md` as an invariant, or project the interface leaves whenever `interfaces` is in the transaction |
| F14 | L | `subsystems/wireguard.go:42-47, 124-132` | `project()` reads the last registered family through a process global: fine for one product agent, order-dependent in tests (the F-rpf-adl-pbr pattern) | Follow-up when a projection env seam exists |

## Recommendations on Q1–Q14

- **Q1 (secret channel).** Keep it: WARNING at DryRun, plus Create refusing the `unavailable:` marker; a peer is never
  created without its PSK. An ERROR would abort resync of every domain (`service.go:382`). Required: F3. Follow-up: F4.
  The store stays a stand-in; the PENDING answer defines the receiver.
- **Q2 (routeAllowedIps default).** Keep the schema default false. It is TNSR parity and safe against auto-installing
  0/0. The field is unmerged, so changing it now would not be a reshape, but I do not recommend it. Add F1 now. Add the
  worker's option (b), switch on by default for *new* interfaces in the UI form, as a small follow-up or in the F1
  round. Also add an agent `Warnf` when an interface has peers, `routeAllowedIps=false` and no `routing.static` route
  names `wg<N>`: "this tunnel carries no traffic".
- **Q3.** Agree (a): no src_ip dependency. The host runs never hit an ordering failure.
- **Q4.** Accept the one line; union it with F-neighbors-ra at merge. Add a `Wiring.OnConnect(func(ctx))` registration
  seam to TD-8b (or a TD-23-style row) so both lines become registrations.
- **Q5.** Accept `wireguard.meta`. It is transactional, records what was applied and is joined only with objects VPP
  reports, so it is not a D-063 echo, and it is better than a stored-document seam. Follow-ups F7 and F8.
- **Q6.** Acceptable given the API invariant; see F13.
- **Q7.** Accept. P11, F-pki and F-ra-vpn each remove their own pointer from `desired/wireguard.go:71-79`. Put that in
  their envelopes as a merge note.
- **Q8.** Accept the wrapper; the declaration is verified truthful. Proper fix: add `func (*Require) RecordsNoOwnership()`
  in `descriptors/vpn/globals.go` (TD-22 or P11), then drop `wgRegistry`. P11 has the same gap.
- **Q9.** Agree; the merger assigns a V-number to `### V-new (F-wireguard)`.
- **Q10.** Agree. Hostname endpoints wait for F-object-model's FQDN machinery. A schema-level warning would surface the
  limit before commit.
- **Q11.** Done correctly (§3).
- **Q12.** Confirmed: a public key, the tree is clean, the squash drops it (§1). No history rewrite on the task branch
  is needed.
- **Q13.** An environment problem. Re-run `stack.sh`'s real-agent rollback step after TD-25 (non-blocking; add F12 to
  that run).
- **Q14.** A CLI follow-up row: `vrx show wireguard [<interface>]` and `vrx request wireguard keypair <name>` over the
  existing REST operations.
