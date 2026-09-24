# TASK ENVELOPE — F-nat44-ed-sessions
id: F-nat44-ed-sessions   branch: task/F-nat44-ed-sessions   worktree: /root/ngfw-wt/F-nat44-ed-sessions   base: main@<BASE>   started: <STARTED>
title: Wave A (day 7-9): NAT44-ED outbound/1:1/port-forward + session browser/kill
prompt: prompts/features/F-nat44-ed-sessions.md   (regenerated for D-104 on task/prompts-s4b, on main before wave A)   wbs: D4.1, D4.7
scope: glue only on top of DF-3 + P08: the `nat` domain builder (mode ed), a paged NatSessions RPC + NatSessionKillAction, the API module, a NAT page with a natTabs registry for the EI/CGNAT siblings, tests, docs. No new NAT descriptor package, no hand-written NAT types, no second claim store.
merged deps you can rely on: P08, DF-3
  - P08: desired/ (Sink, interface/<name> aliases), subsystems.Register/Domains, Wiring.KeyedClaims("nat"), Env.GlobalsOwner, projection.go, InterfaceState state-RPC pattern, test/topology/interfaces
  - DF-3: descriptors/nat44ed (nat44ed.Register/New, Users/UserSessions/DeleteSession) + natcommon (WithGlobalsOwner, WithClaims, Encode) + natcommon/nattest (EnsurePlugin, SlotLock)
  - also on main: TD-3 (V19 sanitizer apps/agent/internal/vpp/ifsanitize + cmd/vrx-vpp-preflight), TD-2 (API auth/users follow-ups)
read first: prompts/features/F-nat44-ed-sessions.md · docs/status/wave-A-hotspots.md (§0 rules, §2 numbers, A1 A2 A4 C2 C4–C7 P1 P3 P4 P5 W1–W3) · docs/agent/descriptors/nat-common.md (first) + nat44-ed.md · docs/status/tasks/DF-3.md · docs/status/vertical-slice.md · docs/decisions/LOG.md D-060, D-062, D-064, D-071, D-080, D-082, D-095, D-101, D-104
slot: <SLOT> → VRX_SLOT=<SLOT> VRX_TEST_PREFIX=w<SLOT> VRX_HTTP_PORT=3000+100·<SLOT> VRX_WEB_PORT=5000+100·<SLOT> VRX_METRICS_PORT=9100+10·<SLOT>+1 VRX_AGENT_SOCKET=/run/vrx-test/w<SLOT>/agent.sock VRX_PG_DATABASE=vrx_w<SLOT> VRX_VALKEY_DB=<SLOT> VRX_VPP_TABLE_BASE=<SLOT>000 VRX_LAB_LOCK=/run/lock/vrx-lab.lock
  - source of truth: `eval "$(tools/lab env <SLOT>)"`
  - rig prefix w<SLOT> → 10.<SLOT>.{1,2}.0/24; NAT pools and every address you use only inside 10.<SLOT>.0.0/16
  - test agents run with VRX_GLOBALS_OWNER=0 (D-071)
  - slots 1–11 only; 12 is CI
daemon-owner: none
obligations:
  - D-071: slot agents only *require* nat44-ed enable/timeouts/forwarding, never set them. The test fixture enables the plugin with nattest.EnsurePlugin under the D-082 globals lock and disables it only if it enabled it and the plugin is empty
  - D-062: use isNat44Enabled(), never read nat.enabled raw
  - persisted NAT claims only: natcommon.WithClaims(<Wiring.KeyedClaims("nat")>); no in-memory default in the product agent (D-080)
  - D-063/D-076: DF-3's write-only / applied-once rules stay unchanged
  - mode ei, nat64, nat66, nptv6, det44, dslite, map, cnat and ipfix stay agent.unsupported-field; the siblings append to desired/nat.go, so keep its dispatch appendable
  - D-104: use, do not rebuild — DF-3 descriptors, P02b schema/semantic rules, NatConfig, P08 wiring
files you own exclusively:
  - apps/agent/internal/desired/nat.go and apps/agent/internal/desired/nat_*.go (incl. tests). This is narrower than the prompt's `desired/nat*.go`, so that F-nat44-ei-64-66-nptv6's desired/{nat44ei,nat64,nat66,nptv6}*.go stay separate; the envelope wins
  - apps/agent/internal/actions/nat44-ed-sessions/**
  - apps/agent/internal/agent/rpc_nat44_ed*.go (the NatSessions method; `server` embeds UnimplementedDataplaneServer)
  - apps/agent/internal/subsystems/nat44_ed*.go (new Wiring methods, if any)
  - apps/agent/internal/descriptors/core/coretest/nat44ed*.go
  - apps/api/src/features/nat44-ed-sessions/** (index.ts exports {controllers, providers}; `Nat44EdSessionsController`; static route POST /actions/nat/sessions/kill; real fake behaviour in fake.ts)
  - apps/api/test/e2e/nat44-ed-*
  - apps/web/src/domains/firewall/nat44-ed-sessions/** (incl. the natTabs registry)
  - apps/web/src/locales/{en,fa}/nat44-ed-sessions.json
  - docs/user/firewall/nat44.md
  - test/topology/nat44-ed-sessions/**
  - packages/schema/src/semantic/nat44-ed-sessions*.ts (the optional adjacent-pool rule, id `nat.nat44-ed-sessions-…`; contract commit)
  - docs/status/tasks/F-nat44-ed-sessions*
  - gap-only (edit only for a proven defect, name the test): apps/agent/internal/descriptors/nat44ed/**, docs/agent/descriptors/nat44-ed.md
shared hotspots (append-only, conflicts resolved by the manager at merge):
  - protocol: docs/status/wave-A-hotspots.md §0. Insert only directly below your own anchor `// wave-A: F-nat44-ed-sessions`. Never reorder or reformat other lines. List every hunk under "Shared hunks" in docs/status/tasks/F-nat44-ed-sessions.md
  - A1 apps/agent/internal/subsystems/subsystems.go: nat44ed.Register(r, c, owner, natcommon.WithGlobalsOwner(env.GlobalsOwner), natcommon.WithClaims(<KeyedClaims("nat")>)) and Domains["nat"]
  - A2 apps/agent/internal/agent/projection.go: one call in project(), one in assemble()
  - A4 apps/agent/internal/agent/server.go: the NatSessionKillAction case in Action, which F-vrf-static-ecmp and F-neighbors-ra also extend. If you merge first, turn Action into a type switch with a default Unimplemented
  - C2 packages/schema/src/semantic/index.ts: one spread line (only with the adjacent-pool rule) · C3 packages/schema/src/index.ts if you export anything
  - C4 packages/schema/examples/ + packages/proto/test/fixtures/: new files only (e.g. nat44-ed-*.json); existing nat-*.json are read-only
  - C5 packages/proto/vrx/v1/dataplane.proto: NatSessions (+ NatSummary) under the service anchor; messages in a `// ----- F-nat44-ed-sessions -----` section at the end
  - allocated numbers (wave-A-hotspots §2, a merge blocker if reused): ActionRequest.action oneof **5 `nat_session_kill`**; NatConfig 25–26 only if needed; nothing else
  - C6 docs/contracts/proto.md: `### F-nat44-ed-sessions: NatSessions`
  - C7 generated, regenerated and never hand-edited: apps/agent/gen/**, packages/proto/gen/ts/**, packages/api-client/src/generated/**, apps/cli/internal/api/operations_gen.go, docs/user/cli/reference.md (`packages/proto/gen.sh`, `pnpm gen`, `make -C apps/cli gen docs`)
  - P1 apps/api/src/app.module.ts · P4 apps/api/src/agent/agent.client.ts · P5 apps/api/src/testing/fake-agent.ts (UNIMPLEMENTED stub under the anchor)
  - W1 apps/web/src/router.tsx · W2 apps/web/src/nav/nav.ts (+ nav.test.ts; the `nat` domain item) · W3 apps/web/src/i18n.ts
contract: the prompt says `contract/F-nat44-ed-sessions`. Instead, commit `contract(proto): nat sessions` (and optionally `contract(schema): nat adjacent pools`) as separate commits on YOUR branch first, plus docs/status/tasks/F-nat44-ed-sessions-contract.md. Tell the manager in the questions file and keep building. No own branches (P08 pattern)
files you must not touch:
  - everything else
  - never: /root/ngfw (main), other worktrees, /etc/vpp, /root/vpp
  - apps/agent/binapi (P04/manager-owned), tools/lab, tools/ci.sh, plan/tasks.yaml, docs/decisions/LOG.md, docs/status/PROGRESS.md, packages/ui-kit/** (a need → questions file)
  - agent core (A5): apps/agent/internal/agent/{agent,service,ifstate}.go, apps/agent/cmd/**; apps/agent/internal/vpp/ifsanitize/** (TD-3)
  - apps/agent/internal/descriptors/natcommon/** (read-only, shared with the sibling NAT tasks → questions file) and every other NAT descriptor package
  - packages/schema/src/{domains,semantic}/nat.ts (P02b's; C1/C2 own-file rule), apps/api/src/actions/actions.controller.ts (F-vrf-static-ecmp), apps/api/src/state/** (P08)
host rules:
  - V19 SAFETY (D-095, crash on first packet): before ANY packet through the rig, `go -C apps/agent run ./cmd/vrx-vpp-preflight` must exit 0 (TD-3). Stop if it does not
  - run `systemctl show vpp -p NRestarts` before and after every host run; stop and write the questions file if it rises
  - delete order (tests, restart simulation, rollback): mappings, pools and interface features before the interfaces they sit on (D-095c)
  - D-101: bring the veth down before any af_packet delete (TD-5 may not be merged)
  - ≥ 2 000 sessions: generate the flows only from ns-w<SLOT>-lan to your own 10.<SLOT>.0.0/16 addresses
  - nattest.SlotLock(t, "nat44") in every NAT host test; with VRX_INTEGRATION=1, one Go package at a time; hold `flock -s` on the lab lock only during a run (D-094)
coordination: F-nat44-ei-64-66-nptv6 starts only after you merge (board dep; ED and EI are mutually exclusive on one VPP). It will add a variant to NatSessions/NatSessionKillAction and append tabs to natTabs, so keep both small and appendable · F-det44-map-dslite-cnat (wave B) appends to the same builder and tabs · F-vrf-static-ecmp and F-neighbors-ra share the A4 Action switch
evidence: Playwright is not installed. Take the screenshots (en + fa/RTL, sessions tab with ≥ 2 000 sessions) with the headless Chrome approach from P07a/P07b/P08 (`test/topology/interfaces/shots_test.go`, kept outside the product code), and say so.
time box: 15 h — when exceeded: stop, commit WIP, write docs/status/tasks/F-nat44-ed-sessions.md with what is left
WIP: commit at least every 45 min; keep docs/status/tasks/F-nat44-ed-sessions-wip.md current
CI: `TMPDIR=/tmp/g-w<SLOT> tools/ci.sh --base main` — short TMPDIR (unix socket paths ≤ 108 chars); no host-wide CI lock: golangci-lint serializes itself since main fc0fe68 (D-106 rejected serialising whole gates). Ports 3000/8080/9101 and /run/vrx/agent.sock belong to the running product stack (tools/app) — never touch them
finish: `tools/ci.sh --base main` green in the worktree · docs/status/tasks/F-nat44-ed-sessions.md with pasted real output · everything committed · final message = 10-line summary (branch, last commit, CI result, evidence, open questions, decisions taken with options)
cleanup: stop every process you started (API/agent/vite), by PID · lab lock released · vrx_w<SLOT> dropped · no w<SLOT> NAT objects left in VPP (dump pasted) · nat44-ed left as the fixture found it · rig down · dist/ and apps/agent/bin removed
questions: docs/status/tasks/F-nat44-ed-sessions-questions.md — write and keep going; never wait for a human
never: merge · restart/kill VPP · Docker · pkill · secrets in files · edit files you do not own
