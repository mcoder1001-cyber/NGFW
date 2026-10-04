# Task: F-pppoe-client-wiring — drive the PPPoE client runtime from config   (prepend 00-CONTEXT.md)

## Goal
The PPPoE client runtime is built and unit-tested but nothing in production feeds it (F-pppoe-client-host Q3, D-186). Wire it
in FAST MODE: `interfaces.<name>.pppoe` → resolved sessions → `PppoeRuntime.Apply`; the ip-up/ip-down hook → `Mirror(up/down)`;
pppd exit tracking → `failCount`/`lastError`; `PppoeSessionState` on `InterfaceState.pppoe`. The live dial stays owed until the
owner installs `ppp pppoe` on ngfw-a (D-168 Q2(b)) — you prove the wiring with a pppd stand-in. Reference behaviour: TNSR
"PPPoE client" (WBS: none on the board row; source = `docs/status/tasks/F-pppoe-client-host-questions.md` Q3 items 1–3).

## Inputs to read first
- `docs/status/tasks/F-pppoe-client-host.md` ("Owed (Q3)", "Still NOT possible on this host") and `-questions.md` Q3, `-answer.md`; D-186, D-168, D-169.
- Runtime (built, reuse — do not rewrite): `apps/agent/internal/subsystems/pppoe.go` (`PppoeRuntime.Apply/Mirror/State/Reconnect`,
  refusing runner on non-globals-owner agents, `pppoeRoutePolicy`), `apps/agent/internal/descriptors/pppoe/client.go`
  (`ClientMirror`, `Mirror`, multipath single-path default route, `RouteTablePolicy`), `apps/agent/internal/agent/rpc_pppoe.go`.
- Renderer (read-only here): `apps/agent/internal/renderers/pppoe/{renderer,supervisor,state,paths}.go`, `templates/hook.tmpl`
  (writes `<StateDir>/<hostif>.state`), `templates/unit.tmpl` (`ngfw-pppoe-<hostif>.service`, `Restart=on-failure`).
- Contract (exists, no change): proto `Pppoe` (Interface field 23), `PppoeSessionState` (InterfaceState field 21);
  `packages/schema/src/domains/ext/pppoe.ts`, `packages/schema/src/semantic/pppoe.ts`.
- Parent tap name: `apps/agent/internal/lcpmap/lcpmap.go` `HostName(vppName, lcp)`; LCP projection `desired/lcp.go`.
- Secrets: `apps/agent/internal/descriptors/vpn/secret.go` (`Resolver` :68, `Redact`), `docs/decisions/PENDING-secret-channel.md`
  (open: the product has no resolver), the WireGuard pattern `desired/wireguard.go:111-127` (warn at projection, the apply fails
  at the object) and strongSwan's `psk/<name>` resolution `renderers/strongswan/model.go:290-310`.
- `docs/agent/scheduler-validators.md` (TD-13 StageDaemon + Validator), `apps/agent/internal/renderers/rfkit` (`Redactor`).
- `docs/lab/shared-host-rules.md` §2, §3, §5, §12; `docs/agent/descriptors/pppoe.md`; `docs/user/network/pppoe.md`.

## Contract changes
None expected (everything above exists). A gap → FEATURE-TEMPLATE contract rule (`contract/F-pppoe-client-wiring`, questions file, keep going).

## Scope — build exactly this
1. **Projection** (`desired/pppoe.go` + one call under a new `// wave-BC: F-pppoe-client-wiring` anchor in `agent/projection.go`):
   every enabled `interfaces.<name>.pppoe` → one session record keyed by the config interface: `HostIf` = the linux-cp host name of
   `pppoe.parent` (or the interface itself) via `lcpmap.HostName`; no LCP pair on the parent → projection error with pointer
   `/interfaces/<name>/pppoe/parent`. Username, service name, MTU, MSS clamp, default route, DNS, IPv6 mode, holdoff/maxFail from
   the config; the password stays a **reference** in desired state (never plaintext in `desired.pb`, rule 10). A missing resolver
   or unresolvable `passwordRef` → warning `pppoe.secret-unavailable` at `/interfaces/<name>/pppoe/passwordRef` (DryRun passes) and
   the apply fails at that object — a session is never dialled without its password.
2. **Client descriptor** (`descriptors/pppoe/client_config*.go`, registered from `subsystems/pppoe.go`): one singleton object
   (all sessions of the owner) that resolves passwords through a `vpn.Resolver` (nil in the product until PENDING-secret-channel;
   tests use a fixture with the literal `NGFW_TEST_PSK_F-pppoe-client-wiring`), calls `PppoeRuntime.Apply`, and `Retrieve`s what
   the runtime applied (no password). It implements `scheduler.Validator` and declares `StageDaemon` (TD-13): `Validate` =
   render + the renderer's own check into a private temp dir, read-only, ctx-bounded, every resolved plaintext masked
   (`rfkit.Redactor`). On a slot agent (not globals owner) it renders into the slot tree and does not supervise (the runner
   refuses, D-079/D-173): commit succeeds with warning `pppoe.not-supervised` instead of failing. Tests: a failing checker leaves
   the fake VPP untouched; `Validate` makes exactly one checker call on a staged path; the slot runtime never calls systemctl.
3. **Hook watcher + exit tracking** (`subsystems/pppoe_watch*.go`): watch `<StateDir>/*.state` (poll or fsnotify — fsnotify is
   already an indirect dependency; say which and why); on `phase=up` call `Mirror(up=true)` with local/peer/DNS from the file and
   MTU/MSS/default route from the session; on `phase=down` or a removed session `Mirror(up=false)`. Exit tracking on the globals
   owner only: read the unit's result through the allowlisted runner (argv only, e.g. `systemctl show <unit> -p ExecMainStatus
   -p NRestarts`), map pppd's documented exit codes to `lastError`, count consecutive failures in `failCount`, reset on `up`.
   Stop the watcher with the agent (ctx), no goroutine leak (test).
4. **State** (`agent/ifstate.go`, one anchored hunk): attach `PppoeRuntime.State(iface, failCount, lastErr)` to
   `InterfaceState.pppoe` for every configured PPPoE client interface; non-PPPoE interfaces unchanged.
5. **Reachability**: the new descriptor kind is listed where the guards expect it (`subsystems/reachability_test.go`, one line).
6. **Docs**: `docs/user/network/pppoe.md` (what works now, the secret-channel and package caveats, CLI equivalent) and
   `docs/agent/descriptors/pppoe.md` (the client descriptor + watcher).

## Acceptance (paste the evidence; `.txt` under `docs/status/tasks/F-pppoe-client-wiring-evidence/`)
- [ ] Slot run (your slot API + agent): commit `interfaces.<yours>.pppoe` on a prefixed interface whose linux-cp pair lives in your
      slot netns (never table 0 objects of other slots) → rendered peer,
      hook and unit files under `/run/ngfw-test/w<N>/pppoe/` (password only in the 0600 secrets file — grep proof, no plaintext in logs
      or `desired.pb`), warning `pppoe.not-supervised`, no systemctl call (agent log)
- [ ] pppd stand-in: the driver runs the **rendered** ip-up hook with `PPP_IPPARAM`/`IPLOCAL`/`IPREMOTE` set → within 5 s
      `vppctl show interface addr <yours>` has the ISP /32 and `show ip fib table <T> 0.0.0.0/0` the session path (MSS clamp via
      `mss_clamp_get`); ip-down hook → both withdrawn; `GET /api/v1/state/interfaces` shows `pppoe.phase` up → down
- [ ] Missing secret: no resolver → commit fails at the client object with the `passwordRef` pointer; nothing rendered
- [ ] Rollback removes the rendered files and the mirrored address/route (Retrieve + vppctl output)
- [ ] Agent-restart simulation: stop your agent, delete your mirrored address via binapi, start it → the watcher re-mirrors from the
      state file within 30 s (log excerpt)
- [ ] Owed after `ppp pppoe` install (list it, do not install anything): live session up, NAT traffic, server restart → reconnect
      within holdoff+10 s, wrong password → `lastError`, drawer screenshot (T4)
- [ ] NRestarts before/after every host step; own unit tests output; `tools/ci-slot.sh --base main` tail

## Out of scope (do not build)
The API→agent secret channel or any sealed cache (PENDING-secret-channel) · installing `ppp`/`pppoe`/accel-ppp or any package ·
a PPPoE server/AC · touching the host's systemd or `/etc/ppp` from a slot · renderer/template changes (`renderers/pppoe/**` is
read-only — a defect there → questions file) · multi-WAN interplay (F-multiwan-wiring) · DHCPv6-PD over PPPoE · web/API changes
(the drawer and `PppoeReconnect` route exist) · `apps/web/**`, `packages/ui-kit/**`.

## Open questions to surface, not to decide silently
- How the product resolver will be injected once PENDING-secret-channel is answered (name the seam you left).
- pppd exit-code → message table: which codes you map and the source you used.

## Files you own
`apps/agent/internal/desired/pppoe*.go` · `apps/agent/internal/subsystems/pppoe*.go` · `apps/agent/internal/descriptors/pppoe/**` ·
`apps/agent/internal/agent/ifstate.go` (pppoe hunk) · `apps/agent/internal/agent/projection.go` (one anchored call line) ·
`apps/agent/internal/subsystems/reachability_test.go` (one line) — the three shared hunks listed under `## Shared hunks` ·
`test/topology/pppoe/**` · `docs/user/network/pppoe.md` · `docs/agent/descriptors/pppoe.md` · `docs/status/tasks/F-pppoe-client-wiring*`.

## Rules
- Files you own: the list above. Everything else is read-only; a needed edit elsewhere → `docs/status/tasks/F-pppoe-client-wiring-questions.md`.
- Shared VPP: slot prefix `w<N>` on every object, tables N000–N999 (`eval "$(tools/lab env <N>)"`); never restart or kill VPP;
  `timeout 10` on every vppctl; packet trace banned (D-128: no `trace add` / `show trace` / `clear trace`); `mss_clamp` off before an
  interface is deleted (TD-H5). Daemons: none (no pppd runs; the stand-in is the rendered hook script).
- D-210a: write tests for your change and get them passing in your package; paste the output; no full suite, no lint, no other
  packages' tests. Run them through `tools/heavy.sh` (D-224), e.g. from `apps/agent`:
  `../../tools/heavy.sh go test ./internal/desired/... ./internal/subsystems/... ./internal/descriptors/pppoe/...`.
- Contracts: additive only — `contract(schema|proto): …` commits first, numbers only from `docs/status/wave-BC-numbers.md`;
  renaming/reshaping = PENDING (decision-policy #1). Secrets: never in code, logs, fixtures or status files (fixture literal only).
- Finish: `tools/ci-slot.sh --base main` green (compile-only gate, D-220/D-222; never `tools/ci.sh` directly), commit on your
  branch, `docs/status/tasks/F-pppoe-client-wiring.md` with pasted real output (D-175 evidence rules), stop every process you started.
