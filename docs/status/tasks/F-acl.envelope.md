# TASK ENVELOPE — F-acl
id: F-acl   branch: task/F-acl   worktree: /root/ngfw-wt/F-acl   base: main@<BASE>   started: <STARTED>
title: Wave A (day 7-9): MACIP/L3/L4 ACLs, attachments, hit counters, 100k-rule editor (ADL/Auto-SDL: info link only)
prompt: prompts/features/F-acl.md   (template: prompts/FEATURE-TEMPLATE.md; updated on task/prep-waveA for P08's layout)   wbs: D5.2, D5.4, D5.5
scope: MACIP/L3/L4 ACLs, attachments (interface/zone), per-rule hit counters, 100k-rule editor. ADL/Auto-SDL is only a link to F-rpf-adl-pbr's screens: no descriptors or screens here.
merged deps you can rely on: P08, DF-4, F-object-model
  - P08: desired/ (Sink, interface/<name> aliases), subsystems.Register/Domains/Wiring.KeyedClaims("acl"), Env.GlobalsOwner, projection.go, InterfaceState state-RPC pattern, test/topology/interfaces
  - DF-4: descriptors/acl — acl.acl, acl.macip-acl, acl.interface-binding, acl.macip-interface-binding, acl.etype-whitelist, acl.stats-enable, StatsReader, LookupIndex
  - F-object-model: internal/objects (Expand/ExpandService/Active, FQDN resolver + change event), object picker UI
  - also on main: TD-3 (V19 sanitizer apps/agent/internal/vpp/ifsanitize + cmd/vrx-vpp-preflight), TD-2 (API auth/users follow-ups)
read first: prompts/features/F-acl.md · docs/status/wave-A-hotspots.md (§0 rules, A1 A2 A4 A5 C5–C7 P1 P4 P5 P6 W1–W3) · docs/agent/descriptors/acl.md · docs/status/vertical-slice.md · docs/status/tasks/F-object-model.md · docs/vpp-code-track.md V7, V19 · docs/decisions/LOG.md D-065, D-066, D-069, D-071, D-080, D-082, D-095, D-101
slot: <SLOT> → VRX_SLOT=<SLOT> VRX_TEST_PREFIX=w<SLOT> VRX_HTTP_PORT=3000+100·<SLOT> VRX_WEB_PORT=5000+100·<SLOT> VRX_METRICS_PORT=9100+10·<SLOT>+1 VRX_AGENT_SOCKET=/run/vrx-test/w<SLOT>/agent.sock VRX_PG_DATABASE=vrx_w<SLOT> VRX_VALKEY_DB=<SLOT> VRX_VPP_TABLE_BASE=<SLOT>000 VRX_LAB_LOCK=/run/lock/vrx-lab.lock
  - source of truth: `eval "$(tools/lab env <SLOT>)"`
  - rig prefix w<SLOT> → 10.<SLOT>.{1,2}.0/24
  - test agents run with VRX_GLOBALS_OWNER=0 (D-071)
  - slots 1–11 only; 12 is CI
daemon-owner: none
obligations:
  - D-071: acl.stats-enable is projected only when Env.GlobalsOwner; slot agents report "counters unavailable". A test that switches the counters on holds `flock -x /run/lock/vrx-globals.lock` (D-082) and never switches them off (V7)
  - D-066: foreign ACLs on an interface are preserved in order; keys acl.acl/<name> and tags `<owner>:<name>` stay stable (F-rpf-adl-pbr resolves ACLs by name)
  - persisted stores only from subsystems.Wiring (KeyedClaims("acl") for acl.WithEtypeClaims; no in-memory stores in the product agent, D-080)
  - restart-safety trap: the agent persists only implemented domains, so `objects` must be one (prompt "Inputs"). If F-object-model did not register it, add the one line in A1 and say so
  - the re-projection trigger (60 s schedules, FQDN change) needs a hook in the read-only agent core (A5): implement the ticker in your subsystems/acl*.go and ask the manager for the one-line Resync hook in the questions file
files you own exclusively:
  - apps/agent/internal/descriptors/acl/** (gap-only: edit only for a proven defect, name the test) and docs/agent/descriptors/acl.md
  - apps/agent/internal/desired/acl*.go
  - apps/agent/internal/actions/acl/**
  - apps/agent/internal/agent/rpc_acl*.go
  - apps/agent/internal/subsystems/acl*.go
  - apps/agent/internal/descriptors/core/coretest/acl*.go
  - apps/api/src/features/acl/** (index.ts exports {controllers, providers}; `AclController`; real fake behaviour in fake.ts)
  - apps/api/test/e2e/acl*
  - apps/web/src/domains/firewall/acl/**
  - apps/web/src/locales/*/acl.json
  - docs/user/firewall/acl.md
  - test/topology/acl/**
  - docs/status/tasks/F-acl*
shared hotspots (append-only, conflicts resolved by the manager at merge):
  - protocol: docs/status/wave-A-hotspots.md §0. Insert only directly below your own anchor `// wave-A: F-acl` if the manager seeded one, else at the end of the block. Never reorder or reformat other lines. List every hunk under "Shared hunks" in docs/status/tasks/F-acl.md
  - A1 apps/agent/internal/subsystems/subsystems.go: acl.Register(r, c, owner, acl.WithEtypeClaims(<KeyedClaims("acl")>)) and Domains["acl"]. The acl key is shared with F-host-acl-nftables, and F-rpf-adl-pbr uses the same KeyedClaims("acl") store
  - A2 apps/agent/internal/agent/projection.go: one call in project(), one in assemble()
  - C5 packages/proto/vrx/v1/dataplane.proto: AclState RPC under the service anchor; messages in a `// ----- F-acl -----` section at the end. No DesiredState field numbers are allocated to you: a config gap (e.g. AclConfig 7 `settings`) needs a number from the manager first (questions file)
  - C6 docs/contracts/proto.md: `### F-acl: AclState`
  - C7 generated, regenerated and never hand-edited: apps/agent/gen/**, packages/proto/gen/ts/**, packages/api-client/src/generated/**, apps/cli/internal/api/operations_gen.go, docs/user/cli/reference.md (`packages/proto/gen.sh`, `pnpm gen`, `make -C apps/cli gen docs`)
  - C4 packages/schema/examples/: new files only, named with the existing conventions (e.g. `acl-editor-*.json`); acl-basic.json and the other existing files are read-only
  - P1 apps/api/src/app.module.ts · P4 apps/api/src/agent/agent.client.ts · P5 apps/api/src/testing/fake-agent.ts (UNIMPLEMENTED stub under the anchor) · P6 apps/api/src/infra/bus.ts TOPICS (only if you push counters over WS)
  - W1 apps/web/src/router.tsx · W2 apps/web/src/nav/nav.ts (+ nav.test.ts) · W3 apps/web/src/i18n.ts
contract: one additive `contract(proto): acl state` commit on YOUR branch first (read-only, paged AclState; never a whole 100k list in one message) + docs/status/tasks/F-acl-contract.md. Tell the manager in the questions file and keep building. No own branches (P08 pattern). packages/schema: none expected; semantic/acl.ts and domains/acl.ts are P02b's and are not edited
files you must not touch:
  - everything else
  - never: /root/ngfw (main), other worktrees, /etc/vpp, /root/vpp
  - apps/agent/binapi (P04/manager-owned), tools/lab, tools/ci.sh, plan/tasks.yaml, docs/decisions/LOG.md, docs/status/PROGRESS.md, packages/ui-kit/** (a need → questions file)
  - agent core (A5): apps/agent/internal/agent/{agent,service,ifstate}.go, apps/agent/cmd/**; apps/agent/internal/vpp/ifsanitize/** (TD-3)
  - apps/agent/internal/objects/** and apps/web/src/domains/firewall/object-model/** (F-object-model: import only)
  - apps/agent/internal/descriptors/{adl,auto_sdl,abf,classify,urpf,ip_session_redirect}/** (F-rpf-adl-pbr, parallel), apps/agent/internal/renderers/nftables/** + apps/agent/internal/desired/hostacl*.go (F-host-acl-nftables, parallel), apps/api/src/actions/actions.controller.ts (F-vrf-static-ecmp), apps/api/src/state/** (P08)
host rules:
  - V19 SAFETY (D-095, crash on first packet): before ANY ping through the rig, `go -C apps/agent run ./cmd/vrx-vpp-preflight` must exit 0 (TD-3). Stop if it does not
  - run `systemctl show vpp -p NRestarts` before and after every host run; stop and write the questions file if it rises
  - delete order everywhere (tests, restart simulation, rollback): unbind ACLs from an interface before deleting the ACL or the interface (D-095c)
  - D-101: bring the veth down before any af_packet delete (TD-5 may not be merged)
  - 100k rules: one acl_add_replace with 100k rules is a very large API message. Run 10k first, then 100k, with NRestarts checked around each step (D-064). On a crash: stop, keep the case opt-in behind an env var, record it in the questions file
  - with VRX_INTEGRATION=1, run one Go package at a time; hold `flock -s` on the lab lock only during a run (D-094)
coordination: F-host-acl-nftables runs in parallel on the same `acl` root key; each reports the other's leaves (acl.host*, hostAttachments ↔ lists/macip/attachments) as agent.unsupported-field until it lands · F-rpf-adl-pbr runs in parallel: shared KeyedClaims("acl") store, ABF resolves ACLs by name
evidence: Playwright is not installed. Take the screenshots (en + fa/RTL, the rule editor scrolled at 100k) with the headless Chrome approach from P07a/P07b/P08 (`test/topology/interfaces/shots_test.go`, kept outside the product code), and say so.
time box: 15 h — when exceeded: stop, commit WIP, write docs/status/tasks/F-acl.md with what is left
WIP: commit at least every 45 min; keep docs/status/tasks/F-acl-wip.md current
CI: `TMPDIR=/tmp/g-w<SLOT> tools/ci.sh --base main` — short TMPDIR (unix socket paths ≤ 108 chars); no host-wide CI lock: golangci-lint serializes itself since main fc0fe68 (D-106 rejected serialising whole gates). Ports 3000/8080/9101 and /run/vrx/agent.sock belong to the running product stack (tools/app) — never touch them
finish: `tools/ci.sh --base main` green in the worktree · docs/status/tasks/F-acl.md with pasted real output · everything committed · final message = 10-line summary (branch, last commit, CI result, evidence, open questions, decisions taken with options)
cleanup: stop every process you started (API/agent/vite), by PID · lab lock released · vrx_w<SLOT> dropped · no w<SLOT> ACLs/bindings left in VPP (dump pasted) · rig down · dist/ and apps/agent/bin removed
questions: docs/status/tasks/F-acl-questions.md — write and keep going; never wait for a human
never: merge · restart/kill VPP · Docker · pkill · secrets in files · edit files you do not own
