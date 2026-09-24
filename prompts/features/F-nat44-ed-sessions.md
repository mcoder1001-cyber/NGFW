# Task: F-nat44-ed-sessions — NAT44 endpoint-dependent + session browser   (prepend 00-CONTEXT.md)

> Regenerated 2026-09-24 (D-104) against the merged DF-3 descriptors, the P02b NAT schema/proto and the P08 vertical slice.
> **The NAT44-ED descriptors, the NAT schema, its semantic rules and the `NatConfig` proto already exist.** This task builds the
> glue that is missing: the `nat` domain projection in the agent, the session state/kill path, the API module, the NAT screen, the
> tests and the docs.

## Goal
Outbound NAT (source NAT / PAT), 1:1 static mappings and port forwards on VPP `nat44-ed`, end to end in FAST MODE, plus a live, paged
session browser with session kill. Reference: TNSR "NAT: outbound, 1:1, port forwards"; VPP plugin `nat44_ed` (WBS D4.1, D4.7).

## Dependencies (must be merged before you start)
- **DF-3** (merged): `apps/agent/internal/descriptors/nat44ed/` + `natcommon/`. Docs `docs/agent/descriptors/nat44-ed.md` and
  **`nat-common.md` (read first**: D-071 globals owner, claims, emptiness checks, ED/EI exclusivity, slot lock). Descriptors:
  `nat44-ed.enable`, `.timeouts`, `.forwarding` (VPP globals), `.interface-feature`, `.output-feature`, `.interface-address`,
  `.address-pool`, `.static-mapping`, `.identity-mapping`, `.lb-static-mapping`, `.vrf-table`. Retrieve-only helpers: `Plugin.Users`,
  `Plugin.UserSessions(user, offset, limit)` (`nat44_user_session_v3_dump`, per user), `Plugin.DeleteSession` (`nat44_del_session`).
  Entry points: `nat44ed.Register(r, client, owner, opts...)` / `nat44ed.New(...)` with `natcommon.WithGlobalsOwner`, `WithClaims`.
- **P02b** (merged): `packages/schema/src/domains/nat.ts` (top level of `nat` = NAT44: `enabled, mode ed|ei, inside, outside, outputFeature,
  insideVrf, outsideVrf, forwarding, sessionLimit ≥ 1024, pools[] (range | interface), staticMappings[], identityMappings[],
  loadBalancedMappings[], timeouts`) and `semantic/nat.ts` (`nat.interfaces-exist`, `nat.inside-outside-disjoint`, `nat.pools-valid`
  = names unique + ranges non-overlapping, `nat.static-mappings` = external port requires protocol, …, `nat.mode-ed-features`,
  `nat.vrfs-exist`). D-062: use `isNat44Enabled()`, never read `nat.enabled` raw. Proto `NatConfig` in `packages/proto/vrx/v1/dataplane.proto`.
- **P08** (vertical slice): the patterns you extend. Registry + domain map in `apps/agent/internal/subsystems/subsystems.go` (`Domains`,
  `Wiring.KeyedClaims("nat")` = the persisted NAT claim store, `Env.GlobalsOwner` = D-071 flag, `VRX_GLOBALS_OWNER=0` on slots). The builder
  package is `apps/agent/internal/desired/` (pointer-carrying `Sink`, `interface/<name>` alias references). The projection hook is
  `apps/agent/internal/agent/projection.go`. The live-state pattern is a read-only agent RPC merged in the API (`InterfaceState`). For the
  topology test pattern, see `test/topology/interfaces/`. Read `docs/status/vertical-slice.md`. If P08 is not merged yet, read it with
  `git show task/P08:<path>` and do not start coding.

## Inputs to read first
- `apps/agent/binapi/nat44_ed/`: the only source of message names (every one you need is already used by DF-3)
- `docs/lab/shared-host-rules.md`: slot prefix, NAT pools and rig addresses only in `10.<N>.0.0/16`, tables `N000–N999`
- D-060, D-064 (crash rule), D-071, D-082 (globals lock in tests), `docs/status/tasks/DF-3.md` (host evidence, known VPP quirks)
- VPP docs: https://s3-docs.fd.io/vpp/26.06/ → NAT44-ED

## Contract changes
Config needs none: `NatConfig` exists. The **state/kill path is new**, so make one additive change on `contract/F-nat44-ed-sessions` first
(`contract(proto): nat sessions`, `docs/status/tasks/F-nat44-ed-sessions-contract.md`, manager told in `F-nat44-ed-sessions-questions.md`),
then continue on your task branch without waiting:
- a read-only unary RPC `NatSessions(NatSessionsRequest{page_token|offset, limit ≤ 1000, filter: inside/outside addr, port, protocol, vrf})
  → NatSessionsResponse{sessions[], next, total_users, total_sessions}` (never all sessions in one message);
- a `NatSummary` in the same response or its own RPC (per-pool utilisation, session counts);
- a `NatSessionKillAction` in the `ActionRequest` oneof (take the next free field number and note it in the contract file; F-vrf-static-ecmp
  and F-neighbors-ra add oneof members too).

Optional semantic rule (same contract branch, `contract(schema): nat adjacent pools`): DF-3 retrieves two **adjacent** range pools of one
class/VRF as ONE range, so reject adjacent pools (or merge them in the builder and say which you chose). Renaming or reshaping fields is PENDING.

## Scope — build exactly this
1. **Schema**: tests only for existing rules on the ED paths you project (disjoint inside/outside, overlapping pools → pointer, external
   port without protocol, `staticMappingOnly`/`connectionTracking` are `UNSUPPORTED` in VPP 26.06, so they become a DryRun
   `agent.unsupported-field` warning), plus the adjacent-pool rule above.
2. **Agent: projection (new `nat` domain)**:
   - Write `desired/nat.go`, which maps `nat` with `mode: ed` onto the DF-3 descriptors: enable (insideVrf/outsideVrf/sessionLimit),
     timeouts, forwarding, inside/outside → `interface-feature`, `outputFeature` → `output-feature`, pools (range → `address-pool`,
     interface → `interface-address`), `staticMappings` (1:1 / port-forward / twice-NAT flags), `identityMappings`, `loadBalancedMappings`.
   - Every interface reference goes through the `interface/<name>` alias; specs are built with `natcommon.Encode`.
   - Write the assembler back to `NatConfig` (canonical form) and a round-trip unit test with the DF-3 fake.
   - Register the family in `subsystems.Register` with `natcommon.WithGlobalsOwner(env.GlobalsOwner)` +
     `WithClaims(<Wiring.KeyedClaims("nat")>)`, add `Domains["nat"]`, and hook the builder into `projection.go`.
   - `mode: ei` and the sibling translators (`nat64`, `nat66`, `nptv6`, `det44`, `dslite`, `map`, `cnat`, `ipfix`) are **not** projected
     here: they stay `agent.unsupported-field` until F-nat44-ei-64-66-nptv6 / F-det44-map-dslite-cnat append to this domain.
3. **Agent: sessions**:
   - Implement the `NatSessions` RPC over `Plugin.Users` + `Plugin.UserSessions` with agent-side paging and filtering (users page first;
     never materialise the full table).
   - Handle `NatSessionKillAction` in `server.go`'s Action dispatch → `Plugin.DeleteSession`.
   - The code lives in `apps/agent/internal/actions/nat44-ed-sessions/` (pure functions + fake-client unit tests).
4. **API**:
   - Config goes through the generic pointer routes (nothing new).
   - Add a new Nest module `apps/api/src/features/nat44-ed-sessions/` with `GET /api/v1/state/nat/sessions?page&pageSize&filter` (paged
     via the RPC), `GET /api/v1/state/nat/summary`, and `POST /api/v1/actions/nat/sessions/kill` (body = session 5-tuple + vrf).
   - Every kill writes an audit entry. Write operations stay out of `/state/**` (rule 8).
   - Leave the generic `actions.controller.ts` alone (F-vrf-static-ecmp owns its dispatch).
   - Update OpenAPI and regenerate `packages/api-client`.
5. **UI**:
   - Build a NAT page at `apps/web/src/domains/firewall/nat44-ed-sessions/` with tabs: Outbound (mode, inside/outside, forwarding,
     timeouts), Static & Port forwards (static + identity + LB mappings), Pools (with a utilisation bar), and Sessions (`ServerDataGrid`,
     server-side paging, filter, kill with confirm).
   - Use the generated schema with SchemaForm throughout.
   - Export a small **tab registry** (`natTabs`) that F-nat44-ei-64-66-nptv6 and F-det44-map-dslite-cnat append to.
   - en + fa strings.
   - Take a screenshot against the real endpoint.
6. **Docs**: `docs/user/firewall/nat44.md` (three classic scenarios: outbound PAT, 1:1, port forward; session browser; restart note: sessions
   are lost; REST + CLI equivalent).

**Files you own:**
- `apps/agent/internal/desired/nat*.go` (+ tests)
- `apps/agent/internal/actions/nat44-ed-sessions/**`
- `apps/api/src/features/nat44-ed-sessions/**`
- `apps/api/test/e2e/nat44-ed-*`
- `apps/web/src/domains/firewall/nat44-ed-sessions/**`
- `apps/web/src/locales/{en,fa}/nat44-ed-sessions.json`
- `docs/user/firewall/nat44.md`
- `test/topology/nat44-ed-sessions/**`
- `docs/status/tasks/F-nat44-ed-sessions*.md`

**Gap-only** (edit only for a proven defect or a missing helper, and say so in the PR): `apps/agent/internal/descriptors/nat44ed/**`,
`docs/agent/descriptors/nat44-ed.md`. **Read-only:** `descriptors/natcommon/**` (shared with the sibling NAT tasks; any change goes to the
questions file).

**Shared, minimal hunks only (state each in the PR):**
- `apps/agent/internal/subsystems/subsystems.go` (register + `Domains["nat"]`)
- `apps/agent/internal/agent/projection.go` (builder hook)
- `apps/agent/internal/agent/server.go` (RPC + one Action case)
- `apps/api/src/app.module.ts`
- web router/nav, `apps/web/src/i18n.ts`
- `packages/api-client` (regenerated)

## Acceptance (paste the evidence)
- [ ] Packet test on the af_packet rig (path recorded): from `ns-<prefix>-lan` to an `ns-<prefix>-wan` address, `tcpdump` in the wan netns shows
      the pool address, not the lan address. `vppctl show nat44 sessions` shows the session, and the UI session browser / `GET /state/nat/sessions`
      shows the same 5-tuple
- [ ] Port forward: a connection from wan to `external:8080` reaches the lan host on `:80` (`trace` shows `nat44-ed-out2in`)
- [ ] 1:1 mapping works in both directions
- [ ] Kill a session through `POST /api/v1/actions/nat/sessions/kill` → it is gone from `vppctl show nat44 sessions`; the audit entry is shown
- [ ] Agent-restart simulation (stop your agent, delete your pool/mappings/features via binapi, start it) → NAT config back within 30 s
      (log excerpt). Sessions are expected to be lost (documented)
- [ ] Rollback removes the interface features, pools and mappings (Retrieve, not assumption); the plugin stays enabled per D-071
- [ ] Overlapping (and adjacent, if you chose "reject") pools → 400 problem+json with `pointer`
- [ ] Sessions page with ≥ 2 000 sessions: `pageSize=100` never returns more than 100, and the gRPC message is bounded (pasted)
- [ ] `tools/ci.sh --base main` green in your worktree

**Globals on the shared host:** your slot agent runs with `VRX_GLOBALS_OWNER=0`, so enable/timeouts are *required*, not set. The test fixture
enables the plugin with `nattest.EnsurePlugin` exactly as `nat44ed_integration_test.go` does (D-082 globals lock), and disables it only if it
enabled it and it is empty. Serialise on `nattest.SlotLock(t, "nat44")` against the EI task.

## Out of scope (do not build)
- **Use, do not rebuild:**
  - DF-3 `nat44ed` descriptors and helpers (`Users`/`UserSessions`/`DeleteSession`), `natcommon` (globals, claims, Encode);
  - P02b's NAT schema/semantic rules and `NatConfig` proto;
  - P08's `subsystems` registry, stores, `desired` package, projection and API state pattern.

  No new NAT descriptor package, no hand-written NAT types, no second claim store.
- NAT44-EI, NAT64, NAT66, NPTv6 (F-nat44-ei-64-66-nptv6).
- DET44/CGNAT, DS-Lite, MAP, CNAT/PNAT (F-det44-map-dslite-cnat).
- NAT IPFIX logging (`nat.ipfix`, DF-8 / F-ipfix-sflow).
- HA session sync (F-ha-state-sync).
- `nat44-ed.vrf-table` (no schema field).
- ALGs beyond VPP defaults; hairpinning tuning; the generic `actions.controller.ts`; ACL/object-model integration of NAT rules.

## Open questions to surface, not to decide silently
- Adjacent pools: reject in the schema or merge in the builder (see Contract changes). Default: reject.
- Outbound traffic that matches no pool: VPP default (drop, unless `forwarding`). Document it and don't invent a policy.
