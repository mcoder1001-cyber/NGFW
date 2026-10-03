# F-vrrp-config-sync — VRRPv3 (VPP plugin + keepalived path), config sync, cluster UI (status)

Branch `claude/modest-keller-upaw4m` (cloud session modest-keller). Scope built here: **the agent side of D9.1**
(both VRRP engines projected from `ha.vrrp`) and **the routing of WEB-4b's pre-built HA page** (D9.5, config only).
**No contract change**: the existing `ha.vrrp` / `ha.cluster` schema and proto keys are projected; everything that
needs new contract fields (VRRP state RPC + event kind, `cluster.syncExclude`, D-086 keepalived stand-ins) or new
API infrastructure (config sync) is **not built** — listed below.

## What was built

| layer | files | what |
|---|---|---|
| agent | `internal/desired/vrrp.go` | builder `Vrrp`: `engine: vpp` → `vrrp.vr` (priority, interval = ms/10, preempt default true, accept, unicast flag, canonical sorted addresses), `vrrp.vr-peers` (unicast), `vrrp.vr-track-interface` per `track[]`, `vrrp.vr-state` while `enabled` (disabled = VR configured, stopped), `vrrp.meta` (name, description, vrf). `engine: keepalived` → the singleton `keepalived.config/ngfw` whose value holds the keepalived instances **plus the linux-cp pairs of their interfaces** (+ hostname for the router id). Rules: `ha.vrrp-duplicate-vrid` (error, pointer `/ha/vrrp/<name>/vrId`), `ha.vrrp` (value errors), warnings `ha.vrrp-keepalived-no-lcp` and `ha.vrrp-keepalived-vrf` (keepalived instance skipped with the reason, never dropped silently). Assembler `AssembleVrrp`: `ha.vrrp` from vrrp.* + meta + the keepalived stage value (fallback name `vr-<if>-<vrid>-<af>`). `vrrp.meta` descriptor (agent-local, `CheckPersistent`) |
| agent | `internal/subsystems/vrrp.go` (+ 1 call under the `wave-BC: F-vrrp-config-sync` register anchor, anchor note in `Domains`) | new domain `ha` (`Domains["ha"]` via init, F-ha-state-sync appends later): DF-7 `vrrp.Register` with `dfkit.DefaultInterfaceKey`, tolerant Retrieve on a VPP without the vrrp plugin (as F-lisp/F-tunnels), `vrrp.meta` over `<state dir>/vrrp-meta-<owner>.json` |
| agent | `internal/subsystems/keepalived.go` | the keepalived renderer stage `keepalived.config` (F-snmp pattern, D-109 (d)): Render → `keepalived -t` → Apply (SIGHUP, convergence) → record; Delete = empty rendering; Retrieve = record while the live file equals its rendering. InterfaceMapper = P12's `lcpmap.Mapper`, set from the stage value before every render. Product paths, or `NGFW_TEST_PREFIX` → `TestPaths(prefix, $NGFW_KEEPALIVED_BIN_DIR, $NGFW_KEEPALIVED_NETNS)` + pidfile controller. `RecordsNoOwnership` |
| agent | `internal/agent/projection.go` (2 lines under the anchors) | `desired.Vrrp` in project(), `desired.AssembleVrrp` in assemble(); `ha` is now an implemented domain (no more `agent.unimplemented-domain` for it) |
| agent (DF-7 gap, TD-11b) | `internal/descriptors/vrrp/ownership.go` | `vrrp.vr` `CheckPersistent` (claims on untagged interfaces), peers/track/state `RecordsNoOwnership`. Proven by `subsystems.Register`'s persistence guard: every test that registers the product wiring (e.g. `TestRegisterGuardsEveryDescriptor`, `TestVPNOptionsArePersisted`) refused to start without it |
| agent (test model) | `internal/descriptors/core/coretest/vrrp.go` | the vrrp plugin (update/add_del/dump, peers refused while running, tracking, start/stop → Backup), ported from DF-7's unit fake |
| agent tests | `internal/agent/rpc_vrrp_test.go`, `internal/desired/vrrp_test.go` | apply → Retrieve == desired; idempotent; agent restart (names kept, 0 changes); VRs lost (VPP restart) re-created and started; disable = stopped; rollback removes every VR; DryRun duplicate VRID error + keepalived-without-LCP warning; keepalived stage value carries the LCP pair and round-trips through the assembler |
| board tests | `subsystems/reachability_test.go` | `vrrp` (descriptors) and `keepalived` (renderer) → wired, `maxPending` 21 → 19 |
| web | `apps/web/src/router.tsx`, `nav/nav.ts`, `nav/nav.test.ts`, `i18n.ts` (one line per anchor) | `/system/ha` routed to WEB-4b's `HaPage`; `ha` in `BUILT_DOMAINS` (nav test lists it after `dataplane`, schema order in the system group) |
| web | `domains/system/ha/locale.ts`, `locales/{en,fa}/ha.json`, `HaPage.tsx`, `HaPage.test.tsx`, `App.test.tsx` | locale moved to `locales/{en,fa}/ha.json` (registered in i18n.ts like every namespace; `locale.ts` re-exports for the parity test); HaPage test now asserts the nav entry is available at `/system/ha`; App.test's "not yet available" example switched to `/firewall/security` (still unbuilt), same assertions |
| docs | `docs/agent/descriptors/vrrp.md`, `docs/agent/renderers/keepalived.md` | "Wired by F-vrrp-config-sync" sections |

## Shared hunks
- `apps/agent/internal/subsystems/subsystems.go`: `registerVrrp` call under `// wave-BC: F-vrrp-config-sync` in register(); anchor note in `Domains`.
- `apps/agent/internal/agent/projection.go`: one line under each `// wave-BC: F-vrrp-config-sync`.
- `apps/agent/internal/subsystems/reachability_test.go`: two rows flipped, `maxPending` 21 → 19.
- `apps/web/src/{router.tsx,nav/nav.ts,nav/nav.test.ts,i18n.ts}`: one line per anchor (i18n: import pair, namespace list, en + fa resources).
- `apps/web/src/App.test.tsx`: the "not yet available" example route.

## Checks run here (cloud sandbox: no VPP, no keepalived, no PostgreSQL, no lab slot)
- agent: `go build ./...`, `go vet ./...` clean; `go test ./...` all packages ok (including `internal/renderers/rsyslog`
  in this run). `golangci-lint` (scratchpad binary) on desired, subsystems, agent, descriptors/vrrp, coretest: 0 issues.
- web: `tsc --noEmit`, eslint, logical-CSS check (392 files) ok; `vitest run` 87 files / 512 tests passed.
- API / schema / proto not touched.

## Not tested
- **No host run / no lab**: packet-level failover on the veth rig, `vppctl show vrrp vr`, the V22b manager-window VPP
  steps, keepalived on a real Linux side, the agent-restart timing (30 s) and the screenshots were not run. The fake
  VPP model covers apply / Retrieve / restart / VPP-restart re-creation / rollback; the keepalived stage is covered
  only up to the projection (the renderer's own RF-4 tests cover rendering; the stage's Apply needs the binary).

## Not built
- **VRRP state + events**: `VrrpState` RPC, `GET /api/v1/state/ha/vrrp`, `EVENT_KIND_VRRP_STATE_CHANGED` (17) and the
  `vrrp.events` bus topic need the `contract(proto)` commit (numbers allocated in wave-BC-numbers.md) → the UI shows
  config only, no live role chips. `vrrp.WatchEvents` / keepalived notify files are not yet published.
- **Config sync (D9.2)**: API-to-API push of the running revision minus `syncExclude` over HTTPS with the pinned peer
  certificate + cluster key, the peer endpoint (`kind: 'cluster-sync'`), `GET /api/v1/state/ha/cluster`,
  `POST /api/v1/actions/ha/sync`, the local-unsynced-edits refusal, the SY1 public-route entry — none of it is feasible
  without new infra: `cluster.syncExclude` (contract), a peer TLS client + certificate pinning store, and the open
  secret-sync decision. `ha.cluster` is stored and shown in the UI but not acted upon.
- D-086 keepalived stand-ins as contract fields (`ha.keepalived.*`, `ha.vrrp.<name>.keepalived.*`), VRRPv2 auth.
- Schema semantic rules of the prompt (priority 255 ⇒ owner address, tracked interfaces exist, `syncExclude`
  pointers) — the agent enforces duplicate (interface, family, VRID); the rest stays with the schema.
- Cluster view (members, revision per node, sync status, force-sync), the `clusterPanels.ts` registry for
  F-ha-state-sync, `docs/user/system/vrrp-config-sync.md`.

## Open questions
- DF-7's `vrrp.vr` Create claims the untagged interface *after* the VPP add (`tg.Claim()`), not claim-first (TD-11b
  review 3.3). Left as is (gap-only scope); a follow-up should switch it to `ClaimFirst`.
- Sync transport / secret sync: unchanged from the prompt's open questions (peer API over HTTPS with the cluster key;
  secrets not synced until the manager's PENDING is answered).
- Suggested split (prompts-s4): the not-built rows above as `F-vrrp-state` (contract + events + state endpoint +
  role chips) and `F-config-sync` (API sync + cluster view).
