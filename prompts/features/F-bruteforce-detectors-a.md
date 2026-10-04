# Task: F-bruteforce-detectors-a — host brute-force detectors → AutoBlockService.observe() (observation half)   (prepend 00-CONTEXT.md)

> Split of board row F-bruteforce-detectors (8 h, D-171): **-a (this, 9 h)** = contract + agent detectors + API subscriber +
> slot driver up to "the API blocks the source"; **-b (6 h)** = pushing the live auto-block set to the data plane
> (`_gb.*` / `b4_bad`), expiry/unblock re-permit, anti-self-lockout at the data plane, TD-H2. -b starts after -a merges.
> Reason: the API → agent enforcement push of the `auto_block` set does not exist on main (only `auto-block.service.ts` reads
> or writes that table; nothing projects it into Global Blocking), so "10 bad logins → blocked on the data plane" is two builds.

## Goal
SSH brute force, VPN (charon) auth failures and port scans against the box reach the one ingestion point
`AutoBlockService.observe(source, kind)` (`apps/api/src/features/auto-block/auto-block.service.ts:128`), exactly like the
`webLogin` detector does today. Source: `docs/status/tasks/F-bruteforce-block-host-questions.md` Q1/Q2, `-answer.md`, D-171,
RV-H2 R2 MINOR 1 (`docs/status/tasks/RV-H2-review-R2.md:20`).

## Inputs to read first
- `docs/status/tasks/F-bruteforce-block.md` ("Notes for the follow-up"), `F-bruteforce-block-host.md` (items 2/3), `docs/user/security/auto-block.md`.
- API: `apps/api/src/features/auto-block/{auto-block.service,engine}.ts` (rules per `source` kind, allow-list + loopback always
  win, sliding windows); raw agent events reach features through `Bus.onAgentEvent` (`apps/api/src/infra/bus.ts:85-90`; example
  subscriber `apps/api/src/features/dashboard-prom-alarms/alarms.service.ts:58,100`) — no edit to `telemetry/relay.service.ts` needed.
- Agent events: `EventKind` + `Event{attributes}` in `packages/proto/ngfw/v1/dataplane.proto` (:680-770), the feature anchors
  above `EVENT_KIND_NEIGHBOR_CHANGED`; publishing through `Wiring.Publish` (`apps/agent/internal/subsystems/seams.go:184-190`).
- Config the detectors follow: `security.autoBlock` (proto `DesiredState.security` 14 → `SecurityConfig.auto_block` 1, `AutoBlock`, `AutoBlockRule` :2795-2840;
  schema `packages/schema/src/domains/security.ts`). Today the agent has no `security` domain: a non-empty one is warned as
  `agent.unimplemented-domain` (`agent/projection.go:250`).
- Log/firewall plumbing to reuse: allowlisted argv runner `renderers.NewSystemRunner(renderers.NewAllowlist(...))` (example
  `subsystems/pppoe.go:80-84`); slot netns mode of the host firewall (`renderers/nftables/paths.go:17-42`, `NGFW_HOST_ACL_NETNS`);
  slot charon writes `charon.log` under its slot tree (`renderers/strongswan/paths.go:30,68`), the product logs to the journal.
- Numbers: `docs/status/wave-BC-numbers.md` (critic pass: `EventKind` 18–19 open); `docs/contracts/proto.md` (per-feature EventKind sections).

## Contract changes
`EVENT_KIND_AUTOBLOCK_OBSERVED = 18` — **proposed; the manager confirms it in your envelope**; add `### F-bruteforce-detectors`
to `docs/status/wave-BC-numbers.md`. Attributes (documented on the enum line and in `docs/contracts/proto.md`): `source` (the
offending IP, canonical), `detector` (`ssh` | `vpnAuth` | `portScan` — exactly the schema's `AutoBlockSourceKind` values),
`count` (observations coalesced into this event). Never usernames, passwords or key material. Commit first on
`contract/F-bruteforce-detectors-a` (`contract(proto): …`, regenerated Go/TS stubs), `docs/status/tasks/F-bruteforce-detectors-a-contract.md`,
tell the manager in your questions file, keep building.

## Scope — build exactly this
1. **Agent consumes `security.autoBlock`** (`desired/autoblock.go`, `subsystems/autoblock.go`): a `security` domain entry in
   `Domains` (anchored) whose one singleton object carries the enabled detector kinds of `security.autoBlock.rules` (and
   `enabled`); its descriptor (in `apps/agent/internal/detectors/`) starts/stops the detectors on Create/Update/Delete and
   `Retrieve` reports what runs, so `/state/drift` stays clean. Thresholds stay the API's (it counts); the agent only observes.
2. **Detectors** (`apps/agent/internal/detectors/{ssh,vpnauth,portscan}*`), each emitting `EVENT_KIND_AUTOBLOCK_OBSERVED`
   through `Wiring.Publish`, coalesced per (source, detector) per second, bounded memory, ctx-stopped, no goroutine leak:
   - `ssh`: sshd failure lines ("Failed password …", "Invalid user …", "authentication failure … rhost=") from the journal on the
     product (`journalctl --follow --output=json --unit=ssh.service`, argv only via the allowlisted runner) and from a
     **test-scoped log file** on a slot (input path from the agent env, under `/run/ngfw-test/w<N>/`);
   - `vpnAuth`: charon authentication failures (pre-shared key / EAP / signature verification failed) from the journal on the
     product and the slot charon's `charon.log`; pin the message patterns with unit fixtures;
   - `portScan`: an nftables chain in the agent's own table (slot: inside the host-firewall netns of the slot, never the root
     netns) that records `saddr . dport` of new inbound connections in a dynamic set with timeout; the detector counts distinct
     ports per source and emits when it grows. Names, table and chain prefixed with the owner; removed on Delete.
3. **API subscriber** (`apps/api/src/features/auto-block/**`): subscribe with `bus.onAgentEvent`, accept only kind 18 with a
   valid `detector` attribute, call `observe(source, detector)` once per event (respect `count` without amplifying beyond the
   rule's threshold), drop malformed events with one rate-limited warning. Unit tests (engine + service): trip after threshold,
   allow-listed and loopback sources never blocked, unknown detector ignored.
4. **Slot driver** (`test/topology/autoblock/`, Go module like `test/topology/host-acl-nftables`, `NGFW_INTEGRATION=1`): a
   test-scoped `sshd -D -e -f <slot config>` inside a slot netns (port 22 there; log to the slot log file the agent reads; never
   the host's sshd or its config), 10 wrong-password logins from a peer netns (e.g. `SSH_ASKPASS_REQUIRE=force` with a script that
   prints a wrong password — no new dependency) → `GET /api/v1/state/auto-block` on the slot API lists the source with reason `ssh`;
   a port scan of ≥ threshold ports → reason `portScan`; a source in `security.autoBlock.allowlist` doing the same is never listed.
   `vpnAuth`: unit fixtures always; the live charon step only if your envelope names `daemon-owner: strongswan` (else list it as owed).
5. **Docs**: `docs/user/security/auto-block.md` — the host detectors, what each watches, how to test, CLI equivalent.

## Acceptance (paste the evidence; `.txt` under `docs/status/tasks/F-bruteforce-detectors-a-evidence/`)
- [ ] Agent log + `StreamEvents` excerpt: one `AUTOBLOCK_OBSERVED` per coalesced burst with `source`/`detector`/`count`, no username
- [ ] 10 failed SSH logins → `GET /api/v1/state/auto-block` shows the source (`reason: ssh`), API log shows the block
- [ ] Port scan → `reason: portScan`; `nft list table …` (slot netns) shows your chain/set, gone after the rule is disabled + commit
- [ ] Allow-listed source (RV-H2 R2): same SSH burst → not listed (state output before/after)
- [ ] Rollback of `security.autoBlock` stops the detectors (agent log) and removes the nft chain; agent-restart simulation brings them back
- [ ] Validation: a rule with an unknown `source` → 400 problem+json with the `pointer` (existing schema; paste it)
- [ ] Own unit tests output (Go detectors + API service); `tools/ci-slot.sh --base main` tail

## Out of scope (do not build)
The data-plane push of the auto-block set (`_gb.*` ACLs, nftables `b4_bad`), unblock/expiry re-permit and TD-H2 (→ -b) ·
changing thresholds/escalation logic in `engine.ts` · new detector kinds (IPS, honeypots, HTTP floods) · reading the host's real
journal or nft tables from a slot · touching the host sshd, `/etc/ssh`, the system strongSwan or system units · UI (Firewall ›
Auto-block exists) · `apps/web/**`, `packages/ui-kit/**` · VPP objects (none needed).

## Open questions to surface, not to decide silently
- EventKind 18 confirmation (until confirmed, keep it on your contract branch only).
- The SSH unit name on the product image (`ssh.service` vs `sshd.service`) — make it a constant, cite P10's packaging if it says.

## Files you own
`packages/proto/ngfw/v1/dataplane.proto` (the one EventKind line + comment, contract commit) + regenerated Go/TS stubs ·
`docs/status/wave-BC-numbers.md` (own `###` section) · `docs/contracts/proto.md` (own section) · `apps/agent/internal/detectors/**` ·
`apps/agent/internal/desired/autoblock*.go` · `apps/agent/internal/subsystems/autoblock*.go` · `apps/agent/internal/agent/rpc_events_autoblock*.go`
(only if you need it) · shared hunks under `// wave-BC: F-bruteforce-detectors-a`, listed under `## Shared hunks`: one `Domains`
entry + one `register()` line in `apps/agent/internal/subsystems/subsystems.go`, one call line in `apps/agent/internal/agent/projection.go`,
one line in `apps/agent/internal/subsystems/reachability_test.go` · `apps/api/src/features/auto-block/**` · `test/topology/autoblock/**` ·
`docs/user/security/auto-block.md` · `docs/status/tasks/F-bruteforce-detectors-a*`.

## Rules
- Files you own: the list above. Everything else is read-only; a needed edit elsewhere → `docs/status/tasks/F-bruteforce-detectors-a-questions.md`.
- Shared host: everything you create carries your slot prefix `w<N>` (`eval "$(tools/lab env <N>)"`): netns, log files under
  `/run/ngfw-test/w<N>/`, nft tables inside your slot netns; no VPP objects are needed (never restart or kill VPP; `timeout 10` on
  any vppctl; packet trace banned, D-128). Test sshd/charon instances only in your netns, stopped by PID when done; daemon-owner
  only as your envelope says.
- D-210a: write tests for your change and get them passing in your package; paste the output; no full suite, no lint, no other
  packages' tests. Run them through `tools/heavy.sh` (D-224), e.g. from `apps/agent`: `../../tools/heavy.sh go test ./internal/detectors/...`;
  API: `tools/heavy.sh pnpm --filter @ngfw/api exec vitest run src/features/auto-block`.
- Contracts: additive only — `contract(schema|proto): …` commits first, numbers only from `docs/status/wave-BC-numbers.md`;
  renaming/reshaping = PENDING (decision-policy #1). Never usernames/passwords in events, logs, fixtures or status files.
- Finish: `tools/ci-slot.sh --base main` green (compile-only gate, D-220/D-222; never `tools/ci.sh` directly), commit on your
  branch, `docs/status/tasks/F-bruteforce-detectors-a.md` with pasted real output, stop every process you started.
