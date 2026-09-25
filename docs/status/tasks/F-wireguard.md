# F-wireguard — WireGuard interfaces, peers and keys (wave B, WBS D6.5)

Branch `task/F-wireguard` (worktree `/root/ngfw-wt/F-wireguard`), slot 6, owner prefix for host objects `w6wg` / `w6wh`
(F-bonding shares slot 6). Base: main with TD-8, TD-7, TD-11b and TD-4 merged in (`41e42258`, later merge of main).
Contract: `docs/status/tasks/F-wireguard-contract.md`. Questions: `docs/status/tasks/F-wireguard-questions.md`.

## What was built

| layer | what |
|---|---|
| contract | `vpn.wireguard.interfaces.<n>.routeAllowedIps` (default false) + proto `WireguardInterface.route_allowed_ips = 12`; `EVENT_KIND_WIREGUARD_PEER_CHANGED = 13`; `rpc WireguardState` + 4 messages (F-wireguard section); semantic rules `vpn.wireguard-public-key-unique`, `vpn.wireguard-allowed-ips`, `vpn.wireguard-endpoint-family` (`packages/schema/src/semantic/wireguard.ts`) |
| agent: descriptors (gap-only) | `wireguard.meta` (agent-local names/descriptions/D-051 refs, file store), `DumpState`, TD-11b declarations, the `unavailable:` secret marker refused by Create, `PartialCreate` on the peer event-registration failure (manager addendum, D-133), PSK doc fix (hmac) |
| agent: builder/assembler | `internal/desired/wireguard.go`: interfaces, peers, meta, admin state / MTU / VRF / addresses on `interface/wg<N>`, allowed-IP routes (NBMA: next hop inside the prefix); Retrieve moves them back under `vpn.wireguard` |
| agent: wiring | `internal/subsystems/wireguard*.go`: `Domains["vpn"]`, `WireguardOptions` (D-096 keyer, D-071 flag, secret store), secret store + test-build fixture loader, the peer-event watcher → TD-8 `Env.Publish`, the handshake observer |
| agent: RPC | `internal/agent/rpc_wireguard.go` `WireguardState` (dumps without keys + stats counters, serialised — D-132) |
| API | `features/wireguard`: `GET /api/v1/state/vpn/wireguard` (joined with the running names), `POST /api/v1/actions/vpn/wireguard/keypair` (admin; private key → secret `key/<name>`, returns `{ref, publicKey, version}`), WS topic `wireguard.events`, fake agent behaviour; OpenAPI, api-client, CLI operations table, SDK regenerated |
| UI | VPN page → **WireGuard** tab (first tab of the W-seed shell): interfaces with peer sub-tables, live status chips from `wireguard.events` over a 30 s snapshot + Refresh, schema-driven interface/peer forms, server key pair (admin), browser-side client keys + client `.conf` export (private key only within the session); en + fa |
| docs | `docs/user/vpn/wireguard.md` (site-to-site, road warrior, routing through NBMA, CLI/REST), `docs/agent/descriptors/wireguard.md` (product wiring), `docs/vpp-code-track.md` V-new (per-peer telemetry gap) |
| tests | schema rules (6), agent unit (builder, secret store, meta, state, partial Create, Service round trip over coretest with a WireGuard model, events), host checks (2), API unit (3) + e2e (5), web model + render (4), full-stack script `test/topology/wireguard/stack.sh` |

## Acceptance (evidence)

- [x] **After commit `Retrieve()` == desired and `vppctl show wireguard interface` / `show wireguard peer` list them (keys
  redacted).** Host check `TestWireguardOnHost` (in-process agent over the host VPP, owner `w6wg`):
  ```
  === RUN   TestWireguardOnHost
      rpc_wireguard_integration_test.go:228: VPP NRestarts=1 before
      rpc_wireguard_integration_test.go:266:   vrf/6051                                                     /vrfs/w6wg-red
      rpc_wireguard_integration_test.go:266:   wireguard.interface/wg6051                                   /vpn/wireguard/interfaces/site-a
      rpc_wireguard_integration_test.go:266:   interface-ip.table/wg6051                                    /vpn/wireguard/interfaces/site-a/vrf
      rpc_wireguard_integration_test.go:266:   interface-ip/wg6051/10.6.52.1/24                             /vpn/wireguard/interfaces/site-a/address/0
      rpc_wireguard_integration_test.go:266:   ip.route/6051/10.6.53.0/24                                   /vpn/wireguard/interfaces/site-a/peers/b1/allowedIps/0
      rpc_wireguard_integration_test.go:266:   ip.route/6051/10.6.54.0/24                                   /vpn/wireguard/interfaces/site-a/peers/b2/allowedIps/0
      rpc_wireguard_integration_test.go:266:   interface.admin-state/wg6051                                 /vpn/wireguard/interfaces/site-a/enabled
      rpc_wireguard_integration_test.go:266:   interface.mtu/wg6051                                         /vpn/wireguard/interfaces/site-a/mtu
      rpc_wireguard_integration_test.go:266:   wireguard.peer/wg6051/0xDtbBaoKPZmOKpEA5AIfn/LwrWojkEG2W6Jw20ehQU= /vpn/wireguard/interfaces/site-a/peers/b1
      rpc_wireguard_integration_test.go:266:   wireguard.peer/wg6051/o4WcKuLzxVIW6GryJhDVmNglzXHwcszBRJF9GyuZDmc= /vpn/wireguard/interfaces/site-a/peers/b2
      rpc_wireguard_integration_test.go:266:   wireguard.meta/wg6051                                        /vpn/wireguard/interfaces/site-a
      …
      rpc_wireguard_integration_test.go:271: Retrieve == desired (vrfs + vpn.wireguard; no wg leaves under interfaces/routing)
      rpc_wireguard_integration_test.go:278: idempotent apply: { "unchanged":  13 }
  ```
  `vppctl` during that run's pause (redaction filter: private-key / mac-key / pre-shared key lines and 64-hex runs):
  ```
  [0] wg6051 src:10.6.51.1 port:20610 private-key: <redacted>
  [0] endpoint:[10.6.51.1:20610->10.6.51.2:20611] wg6051 keep-alive:25 flags: 0, api-clients count: 1
    adj: 4
    pre-shared key: <redacted>
    public key:0xDtbBao… <hex-redacted>
    allowed-ips: 10.6.53.0/24
  [1] endpoint:[10.6.51.1:20610->0.0.0.0:0] wg6051 keep-alive:0 flags: 0, api-clients count: 1
    adj: 21
    public key:o4WcKuLz… <hex-redacted>
    allowed-ips: 10.6.54.0/24
  ```
  Full stack (real agent test build + API + web, `test/topology/wireguard/stack.sh`, run 2 at 04:48–04:50, owner `w6wg`):
  ```
  04:49:49 commit: {"status":"applied","revision":1,"errors":null}
  04:49:53 drift: {"changes":[]}
  --- vppctl show wireguard interface (redacted)
  [0] wg6070 src:10.6.72.1 port:20610 private-key: <redacted>
  --- vppctl show wireguard peer (redacted)
  [0] endpoint:[10.6.72.1:20610->10.6.70.2:20611] wg6070 keep-alive:5 flags: 2, api-clients count: 1
    adj: 5
    pre-shared key: <redacted>
    public key:5k3MaDHN… <hex-redacted>
    allowed-ips: 10.6.71.2/32
  [1] endpoint:[10.6.72.1:20610->0.0.0.0:0] wg6070 keep-alive:0 flags: 0, api-clients count: 1
    adj: 0
    public key:HIgo9xNz… <hex-redacted>
    allowed-ips: 10.6.71.10/32
  --- vppctl show interface wg6070
  wg6070                            6      up          1420/0/0/0     rx packets                     4
                                                                      rx bytes                     288
                                                                      tx packets                     3
                                                                      tx bytes                     468
  --- vppctl show interface address wg6070
  wg6070 (up):
    L3 10.6.71.1/24
  --- vppctl show ip fib 10.6.71.2/32
  10.6.71.2/32 fib:0 index:17 locks:2
    API refs:1 src-flags:added,contributing,active,
        path:[40] pl-index:19 ip4 weight=1 pref=0 attached-nexthop:  oper-flags:resolved,
          10.6.71.2 wg6070
        [@0]: ipv4 [features] via 10.6.71.2 wg6070: mtu:1420 next:12 flags:[features ] …
               stacked-on entry:7:
                 [@2]: ipv4 via 10.6.70.2 tap670: mtu:9000 next:6 flags:[] …
  ```
- [x] **Handshake evidence — ran both:** a kernel WireGuard peer (wireguard-tools v1.0.20250521) in the netns
  `ns-w6wh` / `ns-w6wg`, reached through a tap (no af_packet: V24; TD-3's `vrx-vpp-preflight` passed before any packet),
  and the event reaching the UI. `TestWireguardHandshakeOnHost` (`VRX_WG_HANDSHAKE=1`):
  ```
      rpc_wireguard_integration_test.go:359: VPP NRestarts=1 before
      rpc_wireguard_integration_test.go:445: event: kind=EVENT_KIND_WIREGUARD_PEER_CHANGED interface=wg6060 message="WireGuard peer Ke7vNra/… on wg6060: established" attributes=map[dead:false established:true peer_index:1 public_key:Ke7vNra/Py10C0qoV1bbZyRUABY…]
      rpc_wireguard_integration_test.go:455: ping through the tunnel:
          3 packets transmitted, 3 received, 0% packet loss, time 2002ms
      rpc_wireguard_integration_test.go:360: VPP NRestarts=1 after
  --- PASS: TestWireguardHandshakeOnHost (2.93s)
  ```
  Full stack: the relayed event turned the peer chip **Established** in the UI (screenshot below), the state route says
  `"status":"established"`, `wg show` on the kernel side `latest handshake: 3 seconds ago`, ping `3 received, 0% packet loss`.
  Note: two VPP wg interfaces cannot handshake over local addresses — `ip4-local` drops the initiation as a spoofed
  local-address packet (tried first; FIB counter `to:[1:176]`, error `ip4 spoofed local-address packet`).
- [x] **Agent-restart simulation → interface and peers back within 30 s.** In process (agent pieces rebuilt on the same
  state dir after deleting the interface and both peers via binapi): `restart after loss: resync {"created": 7, "updated": 2,
  "unchanged": 4}, converged in 145.465783ms`. Real agent process (SIGTERM, `wireguard peer remove` ×2 + `wireguard delete
  wg6070`, start):
  ```
  04:50:16 simulated loss: wg6070 present? 0
  04:50:17 restart: interface and 2 peers back after 0.85 s (agent pid 3780022)
  {"time":"2026-09-25T04:50:16.862226476+03:30","level":"WARN","msg":"TEST BUILD: WireGuard secret fixture loaded (never in a product build)","owner":"w6wg","component":"subsystems","file":"/run/vrx-test/w6/wg/secrets.json","secrets":2}
  {"time":"2026-09-25T04:50:16.900292533+03:30","level":"INFO","msg":"reconcile start","owner":"w6wg","txn_id":"","mode":"resync","domains":["interfaces","vrfs","routing","vpn"]}
  {"time":"2026-09-25T04:50:17.188068511+03:30","level":"INFO","msg":"reconcile done","owner":"w6wg","txn_id":"","mode":"resync","domains":["interfaces","vrfs","routing","vpn"],"status":"APPLY_STATUS_APPLIED","summary":"created:6  updated:2  unchanged:7","reapplied":0,"duration":287763551,"err":""}
  04:50:17 after restart: kernel peer {"status":"established","lastHandshake":"2026-09-25T01:20:17.467Z","endpoint":"10.6.70.2"}
  ```
- [x] **Rollback removes interfaces and peers (Retrieve empty).** Host check: `rollback: {"deleted": 13}` then `after
  rollback Retrieve: {} (vpn unset, no wg interface or peer of w6wg in VPP)` (binapi check). API e2e (PostgreSQL + fake
  agent): `POST /config/rollback/<rev before the WireGuard commit>` → the agent's document has no WireGuard interface and
  `/state/vpn/wireguard` returns `interfaces: []`. (The full-stack rerun that adds a real-agent rollback step, 04:52, could not
  create any interface: the shared VPP held 111–128 freed classify-table indices from other slots and TD-3's V19 sanitizer
  refused a clean sw_if_index — "placeholder cap reached before every freed classify table index was resurrected (64
  placeholders, cap 64 for 128 freed indices seen)". The run cleaned up; nothing of w6wg is left.)
- [x] **Duplicate public key on two interfaces → 400 problem+json with `pointer` to the second peer.** API e2e:
  ```
  ✓ F-wireguard e2e (PostgreSQL + fake agent) > duplicate public key on two interfaces → 400 problem+json pointing at the second peer
  ```
  (`/vpn/wireguard/interfaces/b/peers/dup/publicKey`, "public key is already used by peer 'p1' of WireGuard interface 'a' (VPP
  keys peers by public key across all interfaces)"). Full stack: `{"status":400,"type":"https://vrx.dev/problems/validation",
  "errors":[{"pointer":"/vpn/wireguard/interfaces/site/peers/kernel/publicKey","message":"public key is already used by peer
  'again' of WireGuard interface 'dup' …"}]}` (the stored document orders `dup` before `site`).
- [x] **No private key or PSK in logs, GET, fixtures, status files.** Keys are references everywhere (D-051); the agent's
  values are `x25519:<public key>` / `hmac:<hex>`; `WireguardState` carries public keys only (unit + host assertions that the
  JSON contains no material); `show_private_key` is never set and the v1 peer dump (no PSK) is used for state and events
  (unit assertions); `WireguardSecrets`, `Keyer` format as `…(n secrets)` (unit: `%v`, `%+v`, `%#v`, slog); the key-pair
  route returns `{ref, publicKey, version}` only (e2e asserts the body keys and that the private key is in neither the
  response nor `GET /secrets`); test vectors are SHA-256 of `VRX_TEST_PSK_F-wireguard_*` labels, the stack run's fixture
  file lives in `/run/vrx-test/w6/wg` (0600) and is removed by the run; every pasted `vppctl` output went through the filter.
- [x] **`tools/ci.sh --base main` green** — on the D-112 squash of the branch (see "CI" below).

### Screenshot (real endpoint: test-build agent + API + `vite preview` of the production web build, host VPP)
`docs/user/vpn/img/wireguard-list-en.png` (peer **Established** from the live event, last handshake, learnt endpoint;
the second peer "No handshake"), `wireguard-list-fa.png` (RTL, Persian digits), `wireguard-interface-dialog-en.png` (the
schema-driven form), `wireguard-keypair-en.png` (reference and public key only). Headless Chrome-for-Testing 153 +
playwright-core 1.63 from the npx cache, script in scratch (not committed), as P07a/P07b/P08.

## Obligations (envelope)
| obligation | how |
|---|---|
| D-051 references only | schema refs; agent maps `key/`→`x25519:`, `psk/`→`hmac:`; nothing else crosses the boundary (see above) |
| D-063 async mode write-only | DF-5 unchanged; the global is in no domain (no configuration leaf) |
| D-065/D-069 `interface/wg<N>` alias, logical names | wg interface provides `interface/wg<N>`; DF-1/core objects reference it; logical name = `wg<N>`, configuration names via `wireguard.meta` |
| D-071 globals owner | `WithGlobalsOwner(env.GlobalsOwner)`; slot agents `VRX_GLOBALS_OWNER=0` (stack script); non-owner requirement declared via `wgRegistry` |
| D-082 globals lock | no test reads or changes a VPP-wide setting (async mode untouched) |
| D-096 keyed PSK fingerprints | `hmac:` via `Wiring.VPNKeyer()`; unit test compares with the keyer |
| DF-5 Q3 default (a) | no src_ip dependency; ordering never failed (questions Q3) |
| DF-5 Q8 events → StreamEvents | watcher → TD-8 `Env.Publish`, EventKind 13, relayed on `wireguard.events` (host + full stack) |
| `generate_key` false | keys generated API-side (`generateKeyPairSync('x25519')`) or in the browser (client keys) |
| TD-11b addendum / D-133 | `scheduler.PartialCreate` in `peer.go`; test fails on the old code |
| TD-11b guard | every registered descriptor declares `RecordsNoOwnership`/`CheckPersistent`; agent tests pass with the guard |
| D-128 no trace | none used (FIB counters, interface counters, `wg show`, ping) |
| D-132 no fast polling | state 30 s + Refresh; one walk at a time in the agent |
| V19/V24 | no af_packet; taps; `vrx-vpp-preflight` before packets in the stack run; NRestarts logged (1→1 host checks; 2→2 stack runs; the 04:27 restart was another branch's dns crash) |

## Shared hunks (registration lines only)
- `apps/agent/internal/subsystems/subsystems.go`: `VPN = "vpn"` (const anchor), `VPN: wireguardDescriptors()` (new-domain
  anchor), `registerWireguard` (Register anchor), **`w.wireguardConnected(ctx)` at the end of `Connected` (no anchor; questions Q4)**
- `apps/agent/internal/agent/projection.go`: one `desired.Wireguard(...)` in `project()`, one `desired.AssembleWireguard(...)` in `assemble()`
- `apps/agent/internal/agent/service_test.go`: the two implemented-domains assertions made registry-derived (identical to F-object-model's lines, D-129 F5)
- `apps/agent/internal/descriptors/core/coretest/fakevpp.go`: the `extensions` seam (identical lines to F-nat44-ed-sessions', D-129); the model is in the owned-by-pattern new file `coretest/wireguard.go` (A6)
- `packages/schema/src/domains/vpn.ts`: one key line `routeAllowedIps`; `packages/schema/src/semantic/index.ts`: import + spread
- `packages/proto/vrx/v1/dataplane.proto`: rpc under the anchor, EventKind 13 under the anchor, field 12 in `WireguardInterface`, messages in the F-wireguard section; `docs/contracts/proto.md` §11 section
- `apps/api/src/app.module.ts` (import + 2 spreads), `agent/agent.client.ts` (type import + method), `testing/fake-agent.ts` (one handler line + **one import line at the end of the import block**), `infra/bus.ts` (topic), `telemetry/relay.service.ts` (case), `auth/route-guard.test.ts` (ADMIN_ONLY line at the end of the block: no F-wireguard anchor there)
- `apps/web/src/domains/vpn/tabs.ts` (tab entry + `import { lazy }`), `nav/nav.ts` + `nav.test.ts` (`'vpn'`), `i18n.ts` (imports, namespace, en, fa)
- `docs/vpp-code-track.md`: `### V-new (F-wireguard)`
- generated: `apps/agent/gen/**`, `packages/proto/gen/ts/**`, `packages/api-client/src/generated/schema.d.ts`, `apps/cli/internal/api/operations_gen.go`, `sdk/python/vrx/_generated/*`

## Decisions taken (options in the questions file)
- `routeAllowedIps` default false (TNSR parity), documented with the NBMA next-hop rule (Q2).
- Agent-local `wireguard.meta` table for names/descriptions/references (Q5), instead of a stored-document seam in `assemble()`.
- Missing secret material = WARNING at validation + loud failure at apply (marker), not a validation ERROR (P08's example
  test requires examples to project cleanly; a peer is never created without its PSK) (Q1).
- Peer-event watcher started from `Wiring.Connected` (Q4); TD-8's `Env.Publish` as the sink.
- Test-only secret fixture behind `-tags vrxtestsecrets` (never in a product build) for the full-stack run (Q1).

## Out of scope (not built)
IPsec (P11 etc.), PKI, dynamic routing over wg, VPP-generated keys, async crypto tuning, HA key sync, tunnel dashboards, QR
provisioning; the API→agent secret channel itself (PENDING-secret-channel); a dedicated `vrx show wireguard` CLI command
(apps/cli is not this row's; REST operations `Wireguard_state` / `Wireguard_keypair` exist); hostname endpoints.

## CI
`TMPDIR=/tmp/g-w6 VRX_CI_HEAD_REF=<squash> tools/ci.sh --base main`, where `<squash>` = `git commit-tree HEAD^{tree} -p
$(git merge-base main HEAD)` with a `contract(schema,proto): …` subject: a commit object only (no ref, no history
rewrite) with exactly the tree and the single commit the D-112 merge produces. Every step checks the working tree (= that
tree); the contract guard and gitleaks read the squash commit. Why not over the branch history: gitleaks flags the
intermediate commit `efcf783a` (a public example key in a variable named `peerKey`, questions Q12); the plain run
(`logs/ci/F-wireguard-20260925-045442-3832129`) stops there.
```
branch    task/F-wireguard @ dcdbd40c   (base: main)   (tip: df8c9235aa936fc76157f4d01fd8b3949daa8e60)
ok — contract commit(s) on the branch:
ok: gitleaks — scanned ~343486 bytes (343.49 KB) in 1.53s no leaks found
ok: no packet trace (trace add / show trace / clear trace / tracedump API) outside docs and the generated bindings
== summary (quick) ==
  contract guard: df8c9235aa936fc76157f4d01fd8b3949daa8e60 vs main   0m00s
  tools (golangci-lint, gitleaks)                    0m03s
  install (pnpm --frozen-lockfile --prefer-offline)   0m00s
  generate + generated-output gate                   2m19s
  forbidden patterns (+ gitleaks)                    0m05s
  packet-trace ban on the shared VPP (D-128)         0m01s
  lint · typecheck · unit tests · build (turbo)   1m58s
  apps/agent: make lint test build                   1m17s
  apps/cli: make lint test build                     0m18s
  test/ Go modules, unit mode (test/integration/smoke test/topology/interfaces)   0m08s
  deploy/vpp: shellcheck + apply-startup fake-host harness   4m49s
  mode quick · wall time 10m59s · logs /root/ngfw-wt/logs/ci/F-wireguard-20260925-050828-4093048

CI GATE PASSED
```
Also run on this tree: API e2e `test/e2e/wireguard.e2e.test.ts` 5/5 (slot DB), web vitest 15 files / 103 tests, agent
`go test -race` of the touched packages, host checks (above). Host runs stopped after 05:05 on the manager's order (TD-25:
interface creates fail closed on the shared VPP); the real-agent rollback step of `stack.sh` is to be rerun once TD-25 lands.

## Fix round 1 (review `docs/status/tasks/F-wireguard-review.md` @ 6b79e77a: APPROVE WITH CHANGES)

| id | fix | test (fails on the old code) |
|---|---|---|
| F1 | **Refuse** (not "require a more specific route"): new rule `vpn.wireguard-route-loop` in `packages/schema/src/semantic/wireguard.ts` — with `routeAllowedIps`, an allowed IP that contains a peer endpoint (IP literal) of any WireGuard interface whose `underlayVrf` is this interface's `vrf` is a 400 at `/vpn/wireguard/interfaces/<if>/peers/<peer>/allowedIps/<i>`. Twin in the Go builder (`desired/wireguard.go`): ERROR at the same pointer, the route is not projected. User guide: "Routing loop — refused" paragraph (full tunnel = own overlay VRF, narrower allowed IP, or switch off + a more specific route to the endpoint; a same-prefix static route is `agent.duplicate-object`) | `semantic/wireguard.test.ts` "vpn.wireguard-route-loop …" (old rule file: `1 failed`); `desired/wireguard_test.go` `TestWireguardRouteLoopRefused` (old builder: `no route-loop error: []`) |
| F2 | Guards: `subsystems/wireguard_fixture_guard_test.go` (untagged, static: only the tagged `wireguard_fixture.go` may set `wireguardFixture`) and `wireguard_fixture_nil_test.go` (every build without the tag: the hook is nil). `tools/ci.sh` forbidden patterns: `vrxtestsecrets` only in its tagged file, `*_test.go`, `test/`, docs (the one ci.sh line; product comments no longer spell the tag) | guards, not a fix: both fail when the tag line is dropped from `wireguard_fixture.go` ("sets wireguardFixture without the … constraint", "set in a build without the test-secrets tag"); the ci.sh grep catches an untracked `zz_probe.go` carrying the tag |
| F9 | `wireguardStateQuery`: `staleTime` = `refetchInterval` = 30 s, `refetchOnWindowFocus: false`; Refresh kept | `WireguardPage.test.tsx` "F9 (D-132) …" (old: `Cannot read properties of undefined (reading 'staleTime')`) |
| UI default | New interfaces open with `routeAllowedIps: true` (`newInterfaceDefaults()`) and an NBMA info line (en + fa); the schema default stays false | "new interfaces open with routeAllowedIps on …" (old: `newInterfaceDefaults is not a function`); the render test opens **Add interface** and finds the hint and the switch checked (old: fails) |
| F3 | Manager: PENDING-secret-channel. Mine: Q1 corrected (the store is a stand-in; without material a resync would delete working tunnels → material before the first resync) | doc |
| F10 | `docs/contracts/proto.md` §11: the event `message` text as the code sends it | doc |
| F11 | Note: the fake agent applies WireGuard configurations the real agent refuses until PENDING-secret-channel (the UI and the e2e exercise that path); users are told by the guide's release note | doc |

Not in this round (follow-ups per the review): F4 (API treats `agent.secret-unavailable` as blocking for user commits,
TD-10a), F5 (refcount shared material), F6 (placeholder instead of an `hmac:` prefix without a meta row), F7 (DF-5 ref in
meta), F8 (batched meta writes), F12 (0/0, ::/0 and IPv6 auto-routes on the post-TD-25 stack re-run), F13 (API invariant in
proto.md), F14 (projection env seam). No host runs this round (af_packet/interface creates blocked until TD-25).

### Fix round 1 — CI
Plain `TMPDIR=/tmp/g-w6 tools/ci.sh --base main` (`logs/ci/F-wireguard-20260925-095440-658311`): every step up to the
history scan passes (generated output clean, `ok: vrxtestsecrets only in test code`, `ok: no secret-shaped strings`); gitleaks
then reports the one known history hit — `efcf783a apps/api/test/e2e/wireguard.e2e.test.ts:22 generic-api-key`, the public
example key (review §1 / Q12: false positive, the D-112 squash drops it). On the squash of `f8a61e19` (`1d4d4159`, `git
commit-tree`, no ref) the full gate is green:
```
  contract guard: 1d4d415994fb4c1bfa8baeba4d40264b78b3b8bf vs main   0m00s
  generate + generated-output gate                   1m43s
  forbidden patterns (+ gitleaks)                    0m05s
  lint · typecheck · unit tests · build (turbo)   4m04s
  apps/agent: make lint test build                   0m59s
  apps/cli: make lint test build                     0m10s
  mode quick · wall time 7m21s · logs /root/ngfw-wt/logs/ci/F-wireguard-20260925-095645-675710
CI GATE PASSED
ok: vrxtestsecrets only in test code
ok: gitleaks — scanned ~390972 bytes (390.97 KB) in 1.19s no leaks found
```
