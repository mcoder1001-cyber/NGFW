# Wave A: shared hotspots and merge protocol

Manager-assistant prep task (`task/prep-waveA`, 2026-09-24), read-only analysis: no code was changed. Sources: `main`@e7c6803;
`git diff main...task/P08` (61 files, @3c318d0; merges before wave A); `task/prompts-s4b`@70c5918 (its "shared hot spots" note);
the TD-2, TD-3 and TD-6 branch diffs; the 12 prio-3 S4 prompts.

**Wave A** = F-vlan-qinq, F-bonding, F-bridge-l2, F-vrf-static-ecmp, F-neighbors-ra, F-rpf-adl-pbr, F-object-model, F-nat44-ed-sessions.
Follow-ons on the same hotspots: F-loopback-bvi-gso-lldp-span, F-acl, F-host-acl-nftables, F-nat44-ei-64-66-nptv6.

## 0. Rules (these go into every wave-A envelope)
1. **No feature owns a hotspot.** The envelope lists each under `shared hotspots (append-only, conflicts resolved by the manager at merge)`.
   Logic goes in files the feature owns; a hotspot gets only registration lines.
2. **Append only, and only under your anchor.** Before spawning, the manager seeds one line `// wave-A: <task-id>` (`#` in shell) per
   touching task, in board order, in each hotspot marked **anchor**. A worker inserts directly **below its own anchor**, never edits,
   reorders or reformats other lines, never removes an anchor, and lists each hunk under "Shared hunks" in `docs/status/tasks/<id>.md`.
   Verified in a scratch repo (git 2.53): inserts under two adjacent anchors merge cleanly; two plain appends at one spot conflict.
3. **Generated files are never hand-merged** (§1 C7). On a conflict: take main's side, finish the source merge, run
   `pnpm gen && make -C apps/cli gen docs`, and add the output to the merge commit (the pre-merge-commit gate re-checks it).
4. **Numbers are allocated, not "next free".** Proto field/oneof/enum numbers and V-item numbers come from §2 or the manager;
   two parallel contract branches that both take "the next free number" collide.
5. **Names are unique by prefix.** Proto messages/RPCs start with the feature's noun (`NatSessions*`, `Neighbor*`, `Bond*`); no shared
   `Page*`/`List*` message in parallel. Nest controllers are `<PascalSlug>Controller` (operationId = `<Class minus Controller>_<method>`,
   `apps/api/src/app.ts`; the CLI binds by operationId). Validators are `<domain>.<slug>-…` (the registry throws on duplicates).
   Locale namespace = task slug.
6. Workers never edit `tools/ci.sh`, `plan/tasks.yaml`, `docs/decisions/LOG.md`, `docs/status/PROGRESS.md`, `apps/agent/binapi/**`,
   `packages/ui-kit/**` or another feature's owned files; a need goes in the questions file.

## 1. Hotspot map
Risk: **H** same lines, conflict on every merge · **M** adjacent hunks or a numbering collision · **L** distinct hunks or new files.
Protocol: **anchor** = rule 2 · **regen** = rule 3 · **own file** = logic in a new owned file, at most one registration line.

| id | path | why features touch it | wave-A touchers | risk | protocol |
|---|---|---|---|---|---|
| A1 | `apps/agent/internal/subsystems/subsystems.go` (P08) | `Domains` map (domain → descriptor names, which becomes `Health.subsystems`); `Register()` (one `<pkg>.Register(r,c,owner,opts…)` per family); domain consts | bonding, bridge-l2, vrf-static-ecmp (svs), neighbors-ra, rpf-adl-pbr, nat44-ed (`Domains["nat"]`), object-model (`Domains["objects"]` with its agent-local `objects.*` descriptor family from `internal/objects`, so Retrieve returns the applied objects document and `/state/drift` stays clean) | H | anchor: one anchor in each of the `Interfaces`/`VRFs`/`Routing` slices (one name per line), one for new domain entries, one at the end of `Register()`. Store options only through `Wiring` (`KeyedClaims("nat")`, `ClassifyStore()`, `BootStore()`). New `*Wiring` methods go in an owned `subsystems/<slug>.go`. Background work (object-model FQDN resolver) starts from that file, never from `agent.go` |
| A2 | `apps/agent/internal/agent/projection.go` (P08) | `project()` and `assemble()` call each domain's builder | all 7 config features | H | own file + anchor: the builder and the assembler live in `internal/desired/<slug>.go`. projection.go gets one call in `project()` and one in `assemble()` (after `desired.Assemble`, so the feature adds its leaves to the assembled map). The board's `internal/agent/project_<slug>*.go` is equivalent: the manager aligns `files_owned` (prompts-s4b note 1) |
| A3 | `apps/agent/internal/desired/interfaces.go` (P08) | `KindOf` (interface kinds and creators), per-interface leaves, `Assemble` | bonding (new `KindBond` + creator), bridge-l2 / neighbors-ra / rpf-adl-pbr (per-interface leaves), vlan-qinq (proven defect only) | H | own file: per-interface leaves are projected by the feature's own builder (iterates `ds.Interfaces`, depends on `interface/<name>`, pointers via `desired.Ptr`). interfaces.go is edited only for F-bonding's kind (one const, one `KindOf` case, one creator case) and for defects proven by a test |
| A4 | `apps/agent/internal/agent/server.go` (P08) | `Action()` today returns Unimplemented as a whole; new RPC methods | vrf-static-ecmp (ping), neighbors-ra (arp flush), nat44-ed (kill) | H | The first of these to merge turns `Action` into a type switch with a default Unimplemented, and the manager seeds the three case anchors. New RPC methods (`func (g *server) NatSessions`) go in owned `internal/agent/rpc_<slug>.go`: `server` embeds `UnimplementedDataplaneServer`, so it compiles without them |
| A5 | `internal/agent/{agent,service,ifstate}.go`, `cmd/vrx-agent`, `cmd/vrx-agentctl` | P08 core, reconnect hook, `typeOf` (already passes `bond` etc. through) | none expected | L | read-only; a need goes in the questions file |
| A6 | `apps/agent/internal/descriptors/core/coretest/*` | agent-level fake VPP model (P08 and TD-3 both edit `fakevpp.go`) | bonding, bridge-l2, vrf-static-ecmp, neighbors-ra | M | own file: a new `coretest/<slug>.go`. Existing files are read-only |
| A7 | `docs/vpp-code-track.md` | new V-items (vrf-static-ecmp traceroute, any workaround) | any | M | append `### V-new (<task-id>)`; the manager numbers it at merge |
| C1 | `packages/schema/src/domains/{interfaces,vrfs,routing,services,objects,nat}.ts` | additive contract fields (prompts) | interfaces.ts: bonding `bond`, bridge-l2 `l2`, neighbors-ra `ipv6Ra/proxyArp/proxyNd`, rpf `urpf/adl`; vrfs.ts: vrf-static-ecmp `sourceSelect`, neighbors-ra `proxyArpRanges`; routing.ts: vrf-static-ecmp `static[].viaFrr`, neighbors-ra `neighbors`, rpf `pbr`; services.ts: rpf `autoSdl`; objects/nat: only if needed | H (interfaces.ts: 4 features in one strictObject) | own file + anchor: the sub-schema goes in a new `packages/schema/src/domains/ext/<slug>.ts`; the domain object gets one key line under its anchor. `x-vrx-ui` group = slug. Everything is on `contract/<id>` first (ci.sh contract guard). No new root key (a reshape is PENDING; F-bridge-l2 asks) |
| C2 | `packages/schema/src/semantic/index.ts` (+ `semantic/<domain>.ts`) | new cross-field rules | all config features | M | own file `semantic/<slug>.ts` exporting `<slug>Validators`; one spread line under the anchor in `SEMANTIC_VALIDATORS`. Domain rule files are not edited |
| C3 | `packages/schema/src/index.ts` | `export * from './domains/ext/<slug>.js'` | features with a C1 file | M | anchor |
| C4 | `packages/schema/examples/`, `packages/proto/test/fixtures/` | test corpora (both are directory-scanned) | all | L | new `<slug>-*.json` only. Never edit `group-a-full.json` or `all-domains.json` |
| C5 | `packages/proto/vrx/v1/dataplane.proto` | mirror fields (the drift guard `contracttest/drift_test.go` fails a schema leaf that has no proto field), new RPCs, `ActionRequest` oneof, `EventKind` | all but vlan-qinq | H (numbers + service block) | numbers from §2. RPCs go under the anchor at the end of `service Dataplane`. New messages go in a `// ----- <task-id> -----` section at the end of the file. `buf breaking` stays green |
| C6 | `docs/contracts/proto.md` | a section for each new RPC and oneof member | all with C5 | M | append `### <task-id>: <Rpc>` under a "Feature RPCs" heading (the manager adds it once); never renumber |
| C7 | generated: `apps/agent/gen/**`, `packages/proto/gen/ts/**`, `packages/api-client/src/generated/schema.d.ts`; ignored: `packages/schema/dist`, `packages/api-client/openapi.json`; **not in GEN_PATHS**: `apps/cli/internal/api/operations_gen.go`, `docs/user/cli/reference.md` (`make -C apps/cli gen docs`) | every proto, schema or route change | every feature with a contract or a route | H (guaranteed) | regen. `go -C apps/cli test ./internal/api/` fails on a stale table (P08 review F2), but ci.sh does not run apps/cli (P13 Q1). The manager adds that step after TD-3/TD-6 |
| P1 | `apps/api/src/app.module.ts` | `controllers`/`providers` arrays (there is no `imports`; `apps/api/src/features/` does not exist yet) | all but vlan-qinq | H | own file + anchor: `apps/api/src/features/<slug>/index.ts` exports `{controllers, providers}`; app.module gets one import line and one spread in each array, under anchors |
| P2 | `apps/api/src/state/state.controller.ts` (P08, TD-2) | `GET /state/routes` (extend), `GET /state/neighbors` (501 stub) | vrf-static-ecmp, neighbors-ra | M (handlers are 1 line apart) | Move the handler into the feature controller and delete the old block in one hunk (Fastify rejects duplicate routes). Keep the blank separator line. interfaces/system/drift are read-only |
| P3 | `apps/api/src/actions/actions.controller.ts` | generic `POST /actions/:action` (501) | vrf-static-ecmp only (owns the dispatch) | L | Everyone else registers static routes in their own controller (`/actions/arp-flush`, `/actions/nat/sessions/kill`); the static route beats `:action` |
| P4 | `apps/api/src/agent/agent.client.ts` | one method per new RPC (`unary` is private) | all with a new RPC | M | anchor, one method each |
| P5 | `apps/api/src/testing/fake-agent.ts` | `impl(): DataplaneServer` is exhaustively typed: **every new RPC breaks `pnpm typecheck` until the fake has a handler** | all with a new RPC | H | The contract branch adds a stub `UNIMPLEMENTED` handler under the anchor. Real fake behaviour goes in owned `features/<slug>/fake.ts`, wired by the same line |
| P6 | `apps/api/src/infra/bus.ts` `TOPICS`, `telemetry/relay.service.ts` | live WebSocket topics | neighbors-ra (`neighbor.events`), object-model (`objects.events`) | M | anchor, one topic line + one EventKind case each |
| W1 | `apps/web/src/router.tsx` | lazy route per screen | all but vlan-qinq | H | anchor above `system/users`, one route line each |
| W2 | `apps/web/src/nav/nav.ts` + `nav.test.ts` | `BUILT_DOMAINS` (one-line Set); fixed non-domain items; the test's exact `available` list | domain screens: nat (nat44-ed), objects (object-model), vrfs + routing (vrf-static-ecmp). Non-domain items: bridging, neighbours, PBR | H | The manager reformats both to one entry per line with anchors. Non-domain items are `NavItem`s whose `labelKey` is in the feature's own namespace |
| W3 | `apps/web/src/i18n.ts` | 2 imports, `NAMESPACES`, `en`/`fa` literals, all single-line | every feature with UI strings | H | The manager reformats to one entry per line with anchors (or replaces it with `import.meta.glob`, after which there are zero edits) |
| W4 | `apps/web/src/locales/{en,fa}/<slug>.json` | feature strings | each | L | owned. `nav.json`, `common.json` and `interfaces.json` are not edited (`nav:domains.<key>` exists for all 13 keys). en and fa have identical key sets |
| W5 | `apps/web/src/domains/interfaces/{InterfacesPage,InterfaceDrawer}.tsx` (P08) | Bonds tab, sub-interface table, Security group, IPv6 RA tab | vlan-qinq (extracts the table: one hunk), bonding, rpf-adl-pbr, neighbors-ra (later loopback-bvi) | H | The manager adds a tab/section registry (like F-nat44-ed's `natTabs`); each feature adds one line. Without it, merge vlan-qinq first |
| D1 | `docs/user/interfaces/basics.md` (P08) | QinQ line, see-also links | vlan-qinq, bonding, bridge-l2 | L | one line each at the end (vlan-qinq: the one line its prompt names) |
| D2 | `docs/user/` index | none exists (`interfaces/`, `system/`, `cli/`) | none | L | features write only their own page; the manager writes `docs/user/README.md` after the wave |
| D3 | `tools/ci.sh` | `GEN_PATHS`/`CONTRACT_PATHS`/`CONTROL_PLANE_PATHS`. `test/**` Go modules are found by `find`, so a new `test/topology/<slug>` needs no registration (`full` runs each one serially on slot 12, which adds wall time) | none (manager only). TD-3 (V19 preflight) and TD-6 (deploy/vpp step) both edit it | M (pre-wave) | Merge TD-3 and TD-6 one at a time. Workers run `TMPDIR=/tmp/g-w<N> tools/ci.sh --base main` (short socket paths); no host-wide CI lock — golangci-lint serializes itself since fc0fe68 (D-106) |
| D4 | `pnpm-lock.yaml`, `apps/agent/go.{mod,sum}` | new dependencies (none expected) | none | L | if one is needed: questions file; the manager runs `pnpm install` / `go mod tidy` on main |

## 2. Number allocation (copy into each envelope; reusing a number or taking "next free" is a merge blocker)
Base: P08's `dataplane.proto`.

| message | current max | allocation |
|---|---|---|
| `Interface` | 12 | 13 `bond` (F-bonding) · 14 `l2` (F-bridge-l2) · 15 `ipv6_ra`, 16 `proxy_arp`, 17 `proxy_nd` (F-neighbors-ra) · 18 `urpf`, 19 `adl` (F-rpf-adl-pbr) · 20–29 waves B+ |
| `Subinterface` | 11 | 12 `l2` · 13–15 neighbors-ra · 16–17 rpf-adl-pbr (only if the leaf is mirrored on sub-interfaces) |
| `Vrf` | 2 | 3 `source_select` (F-vrf-static-ecmp) · 4 `proxy_arp_ranges` (F-neighbors-ra) |
| `StaticRoute` | 6 | 7 `via_frr` (F-vrf-static-ecmp) |
| `RoutingConfig` (2,3 reserved) | 9 | 10 `neighbors` (F-neighbors-ra) · 11 `pbr` (F-rpf-adl-pbr) |
| `ServicesConfig` | 7 | 8 `auto_sdl` (F-rpf-adl-pbr) |
| `ObjectsConfig` / `NatConfig` | 7 / 24 | 8–9 F-object-model / 25–26 F-nat44-ed-sessions, only if needed |
| `ActionRequest.action` oneof | 3 | 4 `arp_flush` (F-neighbors-ra) · 5 `nat_session_kill` (F-nat44-ed) · 6 spare for F-vrf-static-ecmp (ping and traceroute exist) |
| `EventKind` | 9 | 10 `EVENT_KIND_NEIGHBOR_CHANGED` (F-neighbors-ra) · 11 `EVENT_KIND_FQDN_CHANGED` (F-object-model) |

**Proposed for the follow-ons and wave B (P08 maxima checked; the manager confirms or changes them before spawn):**
`Interface` 20 `gso`, 21 `mirror` (F-loopback-bvi-gso-lldp-span), 22 LCP pair leaf (P12, if on the interface) · `NextHop` 4 `vrf`
(F-vrf-static-ecmp; NextHop max is 3) · `StaticRoute` 8 `tag` (P12) · `ServicesConfig` 9 `nsim` (F-loopback) · `AclConfig` (max 6)
7 F-acl settings, 8 F-host-acl-nftables host settings, each only with a config gap · `WireguardInterface` (max 11) 12 `route_allowed_ips`
· `SyslogTarget` (max 5) 6–9 facilities/format/queue_size/tls (F-unbound-chrony-syslog, D-086) · `ActionRequest.action` 7 `dns_lookup`
(F-unbound-chrony-syslog), 8 IPsec initiate/terminate (P11, optional) · `EventKind` 12 IPsec SA (P11), 13 WireGuard peer
(F-wireguard), 14 routing change + 15 BGP neighbour (P12) · `DesiredState` (max 13) 14 only if F-bridge-l2's root-key option (a) is
chosen (PENDING, decision-policy #1).
F-bridge-l2 bridge-domain/xconnect records: container and number are assigned with the answer to its root-key question.
Suggested RPC names (keep the prefix): `NatSessions` (+`NatSummary`), `ListNeighbors`, `ListRoutes`, `BondState`,
`BridgeDomainState` / `BridgeDomainMacs`, `PbrState`, `FqdnObjectState`.

## 3. Envelope lines per task (the hotspot ids above)
- F-vlan-qinq: W5, W3, D1; defect-only: A3, C2
- F-bonding: A1, A2, A3 (kind), A6, C1–C7, P1, P4, P5, W1, W2, W3, D1 (Bonds is its own route, not a W5 tab — prep-waveA critic)
- F-bridge-l2: A1, A2, A6, C1–C7, P1, P4, P5, W1, W2, W3, D1
- F-vrf-static-ecmp: A1, A2, A4, A6, A7, C1–C7, P1–P5, W1, W2, W3
- F-neighbors-ra: A1, A2, A4, A6, C1–C7, P1, P2, P4, P5, P6, W1, W2, W3, W5
- F-rpf-adl-pbr: A1, A2, C1–C7, P1, P4, P5, W1, W2, W3, W5
- F-object-model: A1, A2, C1–C7 (if needed), P1, P4, P5, P6, W1, W2, W3
- F-nat44-ed-sessions: A1, A2, A4, C2, C4–C7, P1, P4, P5, W1, W2, W3
- follow-ons: F-loopback-bvi-gso-lldp-span A1, A2, A6, A7, C1–C7, P1, P4, P5, W1–W3, D1 · F-acl A1, A2, A5 (Resync hook, manager), C4–C7,
  P1, P4, P5, (P6), W1–W3 · F-host-acl-nftables A1, A2, C5–C7 (+C1–C3 on a gap), P1, P4, P5, W1–W3, ALLOWLIST · F-nat44-ei-64-66-nptv6 A1,
  A2, A4, A7, C4–C7, P1, P4, P5, W3 + ED's `desired/nat.go`, `rpc_nat44_ed*.go`, natTabs
- A5 event sink: F-neighbors-ra (EventKind 10) and F-object-model (EventKind 11) need to publish into the agent's unexported bus
  (`agent/events.go`); P11, P12 and F-wireguard need the same. The manager seeds one `subsystems.Env` publish hook (and F-acl's Resync
  hook) in the anchor commit; no worker edits agent.go

## 4. Manager sequence
1. **Pre-wave merges, one at a time**, each followed by `pnpm gen`: TD-2 and P08 both edit `state.controller.ts` and `schema.d.ts`;
   P08 and TD-3 both edit `coretest/fakevpp.go`; TD-3 and TD-6 both edit `tools/ci.sh`.
2. **One anchor commit on main after P08**, no behaviour change: seed the anchors for A1, A2, A4 (if P08's `Action` stays), C1, C2, C3,
   C5, P1, P4, P5, P6, W1, W2, W3; reformat the single-line lists (W2, W3, the `Domains` slices) to one entry per line; optionally add
   the W5 registry. Commit the schema/proto/generated part as `contract(wave-A): anchors`, the rest as `chore(wave-A): hotspot anchors`.
   Then spawn.
3. **Each merge:** `git merge --no-ff task/<id>` → hotspots: take the union, keep the order · numbers: check against §2 · generated:
   regenerate (rule 3) → `tools/ci.sh` on main under the lock. Remove anchors only after the whole wave has merged. Workers do not merge
   other features' branches and do not pre-resolve them.
4. **Without step 2**, A1–A4, C1, C5, P1, P4, P5, W1–W3 and W5 conflict on nearly every merge. The conflicts are trivial unions,
   but budget about 10 minutes of manager time per merge and merge vlan-qinq, then vrf-static-ecmp, first (the W5 and A4 first-touchers).
