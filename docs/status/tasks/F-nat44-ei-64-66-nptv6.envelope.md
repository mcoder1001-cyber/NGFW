# TASK ENVELOPE — F-nat44-ei-64-66-nptv6
id: F-nat44-ei-64-66-nptv6   branch: task/F-nat44-ei-64-66-nptv6   worktree: /root/ngfw-wt/F-nat44-ei-64-66-nptv6   base: main@<BASE>   started: <STARTED>
title: Wave A (day 7-9): NAT44-EI, NAT64, NAT66, NPTv6 (npt66 loaded since D-060)
prompt: prompts/features/F-nat44-ei-64-66-nptv6.md   (template: prompts/FEATURE-TEMPLATE.md; updated on task/prep-waveA for DF-3/P08 and the regenerated ED prompt)   wbs: D4.2, D4.3
scope: project nat.mode ei, nat.nat64 and nat.nat66 onto the existing DF-3 descriptors; a new write-only npt66 descriptor for nat.nptv6; EI + NAT64 session state/kill as variants of the ED session path; NAT page tabs; tests; docs
merged deps you can rely on: P08, DF-3, F-nat44-ed-sessions
  - P08: desired/ (Sink, interface/<name> aliases), subsystems.Register/Domains, Wiring.KeyedClaims("nat"), Env.GlobalsOwner, projection.go
  - DF-3: descriptors/{nat44ei,nat64,nat66} (nat44ei Users/UserSessions/DeleteSession, nat64 Sessions, ErrOtherVariant) + natcommon (AppliedRecord, WithGlobalsOwner, WithClaims) + natcommon/nattest (EnsurePlugin, SlotLock, Addr6/Prefix6/Table)
  - F-nat44-ed-sessions: desired/nat.go (the `nat` builder), NatSessions RPC + NatSessionKillAction (oneof 5), natTabs registry, NAT API module
  - also on main: TD-3 (V19 sanitizer + cmd/vrx-vpp-preflight), TD-2
read first: prompts/features/F-nat44-ei-64-66-nptv6.md · docs/status/wave-A-hotspots.md (§0 rules, A1 A2 A4 A7 C4–C7 P1 P4 P5 W3) · docs/agent/descriptors/nat-common.md (first), nat44-ei.md, nat64.md, nat66.md · docs/status/tasks/F-nat44-ed-sessions.md + F-nat44-ed-sessions-contract.md · docs/status/tasks/DF-3.md · docs/decisions/LOG.md D-060, D-062, D-063, D-064, D-071, D-076, D-080, D-082, D-095, D-101
slot: <SLOT> → VRX_SLOT=<SLOT> VRX_TEST_PREFIX=w<SLOT> VRX_HTTP_PORT=3000+100·<SLOT> VRX_WEB_PORT=5000+100·<SLOT> VRX_METRICS_PORT=9100+10·<SLOT>+1 VRX_AGENT_SOCKET=/run/vrx-test/w<SLOT>/agent.sock VRX_PG_DATABASE=vrx_w<SLOT> VRX_VALKEY_DB=<SLOT> VRX_VPP_TABLE_BASE=<SLOT>000 VRX_LAB_LOCK=/run/lock/vrx-lab.lock
  - source of truth: `eval "$(tools/lab env <SLOT>)"`
  - rig prefix w<SLOT> → 10.<SLOT>.{1,2}.0/24 (IPv4 only); IPv6 only inside fd00:<SLOT hex>::/32 (natcommon.Scope)
  - test agents run with VRX_GLOBALS_OWNER=0 (D-071)
  - slots 1–11 only; 12 is CI
daemon-owner: none
obligations:
  - D-071: slot agents only *require* the nat44-ei/nat64/nat66 enables, timeouts and forwarding, never set them. Test fixtures use nattest.EnsurePlugin under the D-082 globals lock, and disable a plugin only if they enabled it and it is empty
  - npt66 is write-only (D-063): prove idempotency with a fake that models duplicate add. If VPP's add is not idempotent, use the D-076 applied-once record: natcommon.AppliedRecord(key, <bootid.Current identity>) in the persisted KeyedClaims("nat") store (the descriptors/cnat newSnatExcludePfx pattern; D-080)
  - ED/EI exclusivity: never disable nat44-ed to make room for EI. If another owner holds nat44-ed on the shared VPP, the EI host checks skip (the nat44ei_integration_test.go pattern); record it and ask the manager for a window in the questions file
  - D-104: use, do not rebuild — DF-3 descriptors and F-nat44-ed-sessions' builder/RPC/tabs; do not reshape them
files you own exclusively:
  - apps/agent/internal/descriptors/npt66/** and docs/agent/descriptors/npt66.md
  - apps/agent/internal/desired/{nat44ei,nat64,nat66,nptv6}*.go
  - apps/agent/internal/actions/nat44-ei-64-66-nptv6/**
  - apps/agent/internal/agent/rpc_nat44_ei*.go
  - apps/agent/internal/descriptors/core/coretest/{nat44ei,nat64,nat66,npt66}*.go
  - apps/api/src/features/nat44-ei-64-66-nptv6/** (index.ts exports {controllers, providers}; `Nat44Ei6466Nptv6Controller`; static route POST /actions/nat/ei/sessions/kill; real fake behaviour in fake.ts)
  - apps/api/test/e2e/nat44-ei-*
  - apps/web/src/domains/firewall/nat44-ei-64-66-nptv6/**
  - apps/web/src/locales/*/nat44-ei-64-66-nptv6.json
  - docs/user/firewall/nat44-ei-64-66-nptv6.md
  - test/topology/nat44-ei-64-66-nptv6/**
  - docs/status/tasks/F-nat44-ei-64-66-nptv6*
  - gap-only (DF-3 built them; edit only for a proven defect, name the test): apps/agent/internal/descriptors/{nat44ei,nat64,nat66}/**, docs/agent/descriptors/{nat44-ei,nat64,nat66}.md
shared hotspots (append-only, conflicts resolved by the manager at merge):
  - protocol: docs/status/wave-A-hotspots.md §0. Insert only directly below your own anchor `// wave-A: F-nat44-ei-64-66-nptv6` if the manager seeded one, else at the end of the block. Never reorder or reformat other lines. List every hunk under "Shared hunks" in docs/status/tasks/F-nat44-ei-64-66-nptv6.md
  - apps/agent/internal/desired/nat.go (F-nat44-ed-sessions' builder): the dispatch to your builders; F-det44-map-dslite-cnat appends after you
  - A1 apps/agent/internal/subsystems/subsystems.go: nat44ei/nat64/nat66/npt66 Register with natcommon.WithGlobalsOwner(env.GlobalsOwner) + WithClaims(<KeyedClaims("nat")>); append to Domains["nat"]
  - A2 apps/agent/internal/agent/projection.go: the assemble hook
  - A4 variant dispatch: in F-nat44-ed-sessions' NatSessions handler (apps/agent/internal/agent/rpc_nat44_ed*.go) and in the nat_session_kill case of server.go Action
  - the natTabs registry file in apps/web/src/domains/firewall/nat44-ed-sessions/: append your tabs
  - A7 docs/vpp-code-track.md: `### V-new (F-nat44-ei-64-66-nptv6)` for npt66_binding_dump; the manager numbers it
  - C5 packages/proto/vrx/v1/dataplane.proto: the variant enum and fields are appended to F-nat44-ed-sessions' NatSessions*/NatSessionKillAction messages (after their current max, nothing renamed); new messages go in a `// ----- F-nat44-ei-64-66-nptv6 -----` section. No other numbers are allocated to you: ask the manager first
  - C4 packages/schema/examples/ + packages/proto/test/fixtures/: new files only (e.g. nat44-ei-*.json, nat64-*.json); existing nat-*.json are read-only
  - C6 docs/contracts/proto.md: `### F-nat44-ei-64-66-nptv6: NAT session variants`
  - C7 generated, regenerated and never hand-edited: apps/agent/gen/**, packages/proto/gen/ts/**, packages/api-client/src/generated/**, apps/cli/internal/api/operations_gen.go, docs/user/cli/reference.md (`packages/proto/gen.sh`, `pnpm gen`, `make -C apps/cli gen docs`)
  - P1 apps/api/src/app.module.ts · P4 apps/api/src/agent/agent.client.ts · P5 apps/api/src/testing/fake-agent.ts · W3 apps/web/src/i18n.ts
contract: one additive `contract(proto): nat session variants` commit on YOUR branch first (variant ED|EI|NAT64 on NatSessions + NatSessionKillAction; no parallel RPCs, nothing renamed) + docs/status/tasks/F-nat44-ei-64-66-nptv6-contract.md. Tell the manager in the questions file and keep building. No own branches (P08 pattern)
files you must not touch:
  - everything else
  - never: /root/ngfw (main), other worktrees, /etc/vpp, /root/vpp
  - apps/agent/binapi (P04/manager-owned), tools/lab (manager-owned; the rig stays IPv4), tools/ci.sh, plan/tasks.yaml, docs/decisions/LOG.md, docs/status/PROGRESS.md, packages/ui-kit/**
  - agent core (A5): apps/agent/internal/agent/{agent,service,ifstate}.go, apps/agent/cmd/**; apps/agent/internal/vpp/ifsanitize/** (TD-3)
  - apps/agent/internal/descriptors/natcommon/** (read-only → questions file), descriptors/{nat44ed,det44,mapnat,cnat,pnat}/**
  - the rest of F-nat44-ed-sessions' files beyond the hotspots above: desired/nat_*.go, actions/nat44-ed-sessions/**, features/nat44-ed-sessions/** (except the natTabs registry)
  - packages/schema/src/{domains,semantic}/nat.ts (P02b's)
host rules:
  - VPP SAFETY: npt66 has never been exercised on this VPP (enabled 2026-09-24, D-060). Run each new message on its own first, with `systemctl show vpp -p NRestarts` before and after. On a crash, disable the test behind an opt-in env var at once and record the message (D-064, A7)
  - V19 SAFETY (D-095): before ANY packet through the rig, `go -C apps/agent run ./cmd/vrx-vpp-preflight` must exit 0 (TD-3). Stop if NRestarts rises
  - delete order: npt66 bindings, NAT interface features and pools before the interfaces they sit on (D-095c)
  - D-101: bring the veth down before any af_packet delete (TD-5 may not be merged)
  - IPv6 on the rig: add it yourself inside fd00:<SLOT hex>::/32 on ns-w<SLOT>-lan/wan and your rig interfaces, and remove it in t.Cleanup
  - the NAT64 prefix is a slot /96 in a slot VRF (DF-3 nat64_integration_test.go), never 64:ff9b::/96 in the shared default table
  - nattest.SlotLock(t, "nat44") in every NAT44 host test; with VRX_INTEGRATION=1, one Go package at a time; hold `flock -s` on the lab lock only during a run (D-094)
coordination: starts after F-nat44-ed-sessions has merged (board dep) — build on its merged builder, RPC, kill action and tabs · F-det44-map-dslite-cnat (wave B) appends to the same builder/tabs after you · F-ha-state-sync (wave C) owns nat44_ei_ha
evidence: Playwright is not installed. Take the screenshots (en + fa/RTL, EI/NAT64/NAT66/NPTv6 tabs) with the headless Chrome approach from P07a/P07b/P08 (`test/topology/interfaces/shots_test.go`, kept outside the product code), and say so.
time box: 15 h — when exceeded: stop, commit WIP, write docs/status/tasks/F-nat44-ei-64-66-nptv6.md with what is left
WIP: commit at least every 45 min; keep docs/status/tasks/F-nat44-ei-64-66-nptv6-wip.md current
CI: `TMPDIR=/tmp/g-w<SLOT> tools/ci.sh --base main` — short TMPDIR (unix socket paths ≤ 108 chars); no host-wide CI lock: golangci-lint serializes itself since main fc0fe68 (D-106 rejected serialising whole gates). Ports 3000/8080/9101 and /run/vrx/agent.sock belong to the running product stack (tools/app) — never touch them
finish: `tools/ci.sh --base main` green in the worktree · docs/status/tasks/F-nat44-ei-64-66-nptv6.md with pasted real output · everything committed · final message = 10-line summary (branch, last commit, CI result, evidence, open questions, decisions taken with options)
cleanup: stop every process you started (API/agent/vite), by PID · lab lock released · vrx_w<SLOT> dropped · no w<SLOT> NAT/npt66 objects or IPv6 rig addresses left (dump pasted) · plugins left as the fixtures found them · rig down · dist/ and apps/agent/bin removed
questions: docs/status/tasks/F-nat44-ei-64-66-nptv6-questions.md — write and keep going; never wait for a human
never: merge · restart/kill VPP · Docker · pkill · secrets in files · edit files you do not own
