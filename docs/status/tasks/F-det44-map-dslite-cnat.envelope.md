# TASK ENVELOPE — F-det44-map-dslite-cnat
id: F-det44-map-dslite-cnat   branch: task/F-det44-map-dslite-cnat   worktree: /root/ngfw-wt/F-det44-map-dslite-cnat   base: main@<BASE>   started: <STARTED>
title: Wave B (day 10-12): DET44 CGNAT, MAP-E/T, DS-Lite, LW4o6, 464XLAT, CNAT policies (+ PNAT 1:1)
prompt: prompts/features/F-det44-map-dslite-cnat.md   (template: prompts/FEATURE-TEMPLATE.md; refreshed on task/prep-rest against main + P08 + the ED/EI envelopes)   wbs: D4.4, D4.5, D4.6
scope: project nat.det44 / nat.map / nat.dslite / nat.cnat onto the existing DF-3 descriptors, a new `dslite` descriptor, a new `nat.pnat` contract leaf onto DF-3's pnat, DET44 + CNAT session state, DET44 lookup/close and CNAT purge, NAT page tabs (CGNAT, MAP, CNAT, PNAT), tests, docs. No new NAT claim store, no reshaping of ED's builder or RPCs.
merged deps you can rely on: P08, DF-3, F-nat44-ed-sessions
  - P08: desired/ (Sink, interface/<name> aliases, Ptr), subsystems.Register/Domains, Wiring.KeyedClaims("nat") + BootStore(), Env.GlobalsOwner, projection.go
  - DF-3: descriptors/{det44,mapnat,cnat,pnat} (det44 Sessions/CloseSessionIn/CloseSessionOut; cnat guarded SNAT entry, V10; pnat index via `pnat_bindings_get`, V11) + natcommon (AppliedRecord, WithGlobalsOwner, WithClaims, WithLockDir) + natcommon/nattest (EnsurePlugin, SlotLock, Addr6/Prefix6/Table)
  - F-nat44-ed-sessions: desired/nat.go (the `nat` builder whose dispatch still reports det44/dslite/map/cnat as agent.unsupported-field), NatSessions RPC, the natTabs registry, the NAT API module
  - also on main: W-seed (anchors/seams), TD-3 (V19 sanitizer + cmd/vrx-vpp-preflight), TD-2. F-nat44-ei-64-66-nptv6 may be running in parallel (launch plan §3) — it appends to the same builder dispatch and natTabs
read first: prompts/features/F-det44-map-dslite-cnat.md · docs/status/wave-A-hotspots.md (§0 rules, A1 A2 A4 A7 C1–C7 P1 P4 P5 W3) · docs/status/wave-BC-numbers.md (section F-det44-map-dslite-cnat) · docs/agent/descriptors/nat-common.md (first), det44.md, map.md, cnat.md, pnat.md · docs/status/tasks/F-nat44-ed-sessions.md + F-nat44-ed-sessions-contract.md · docs/status/tasks/DF-3.md + DF-3-questions.md (Q0, Q5, Q7, Q8) · docs/vpp-code-track.md V9, V10, V11 · docs/decisions/LOG.md D-043, D-062, D-063, D-064, D-068, D-071, D-076, D-077, D-080, D-082, D-095, D-101, D-113
slot: <SLOT> → VRX_SLOT=<SLOT> VRX_TEST_PREFIX=w<SLOT> VRX_HTTP_PORT=3000+100·<SLOT> VRX_WEB_PORT=5000+100·<SLOT> VRX_METRICS_PORT=9100+10·<SLOT>+1 VRX_AGENT_SOCKET=/run/vrx-test/w<SLOT>/agent.sock VRX_PG_DATABASE=vrx_w<SLOT> VRX_VALKEY_DB=<SLOT> VRX_VPP_TABLE_BASE=<SLOT>000 VRX_LAB_LOCK=/run/lock/vrx-lab.lock
  - source of truth: `eval "$(tools/lab env <SLOT>)"`
  - rig prefix w<SLOT> → 10.<SLOT>.{1,2}.0/24 (IPv4 only); every DET44 inside/outside prefix, CNAT VIP/backend, PNAT address and MAP/DS-Lite IPv4 prefix inside 10.<SLOT>.0.0/16; IPv6 (MAP BR prefix, DS-Lite AFTR/B4, softwire) only inside fd00:<SLOT hex>::/32 (natcommon.Scope); VRFs/tables in <SLOT>000–<SLOT>999
  - test agents run with VRX_GLOBALS_OWNER=0 (D-071)
  - slots 1–11 only; 12 is CI
daemon-owner: none
obligations:
  - D-104: use, do not rebuild — DF-3's det44/mapnat/cnat/pnat descriptors, natcommon, ED's builder/RPC/tabs. Only `descriptors/dslite` is new (base it on descriptors/dfkit, D-077, and natcommon's claim/globals options)
  - D-071 globals: det44 enable/timeouts, MAP parameters, DS-Lite AFTR and B4 addresses, the cnat SNAT policy/addresses/exclude prefixes and cnat session purge are VPP-wide. Slot agents only *require* them (clear error when absent), never set, reset or purge. DS-Lite pools use claim-store ownership like nat64 pools (natcommon.WithClaims(<Wiring.KeyedClaims("nat")>))
  - D-068 / V9: det44 is never disabled, not by rollback, not by the globals owner, not by a test. Rollback leaves it enabled and idle (document it)
  - V10: keep DF-3's cnat guards (SNAT entry ordering, no `n_paths=0`); every cnat mutation that dereferences the default SNAT entry holds `/run/lock/vrx-nat-cnat.lock` (nat-common.md "Host-wide lock")
  - V11: pnat calls stay guarded (no lookup/detach before the first attach); indexes via `pnat_bindings_get`
  - D-063/D-076/D-080: write-only objects (det44.enable, dslite AFTR/B4 if no getter answers, pnat attach side effects) re-apply once per VPP boot identity via natcommon.AppliedRecord in the persisted KeyedClaims("nat") store; prove it with a fake that models duplicate add
  - persisted NAT claims only (never the in-memory default); interface refs are `interface/<name>` (D-065/D-069)
  - sessions (DET44 per user, CNAT) are paged in the agent; the API never holds a full session table; Retrieve stays config-only
files you own exclusively:
  - apps/agent/internal/descriptors/dslite/** and docs/agent/descriptors/dslite.md (new)
  - gap-only (DF-3 built them; edit only for a proven defect, name the test): apps/agent/internal/descriptors/{det44,mapnat,cnat,pnat}/**, docs/agent/descriptors/{det44,map,cnat,pnat}.md
  - apps/agent/internal/desired/{det44,map,dslite,cnat,pnat}*.go (builders + assemblers; one dispatch hunk in ED's desired/nat.go, see hotspots)
  - apps/agent/internal/subsystems/det44_map_dslite_cnat*.go (Register calls' options, Wiring helpers)
  - apps/agent/internal/agent/rpc_{det44,cnat}*.go (Det44Sessions, Det44Lookup, CnatSessions)
  - apps/agent/internal/actions/det44-map-dslite-cnat/**
  - apps/agent/internal/descriptors/core/coretest/{det44,mapnat,dslite,cnat,pnat}*.go (A6: new files only)
  - packages/schema/src/domains/ext/det44-map-dslite-cnat*.ts (the `nat.pnat` sub-schema), packages/schema/src/semantic/det44-map-dslite-cnat*.ts (rule ids `nat.det44-map-dslite-cnat-…`)
  - packages/schema/examples/det44-map-dslite-cnat-*.json, packages/proto/test/fixtures/det44-map-dslite-cnat-*.json
  - apps/api/src/features/det44-map-dslite-cnat/** (index.ts exports {controllers, providers}; `Det44MapDsliteCnatController`; static routes GET /state/nat/det44/sessions, GET /state/nat/cnat/sessions, POST /actions/nat/det44/lookup, POST /actions/nat/det44/sessions/close, POST /actions/nat/cnat/sessions/purge; real fake behaviour in fake.ts), apps/api/test/e2e/det44-map-dslite-cnat*
  - apps/web/src/domains/firewall/det44-map-dslite-cnat/** (tabs CGNAT, MAP, CNAT, PNAT + the DET44 port-block calculator), apps/web/src/locales/{en,fa}/det44-map-dslite-cnat.json
  - docs/user/firewall/det44-map-dslite-cnat.md, test/topology/det44-map-dslite-cnat/**, docs/status/tasks/F-det44-map-dslite-cnat*
shared hotspots (append-only, conflicts resolved by the manager at merge):
  - protocol: docs/status/wave-A-hotspots.md §0. Insert only directly below your own anchor `// wave-BC: F-det44-map-dslite-cnat` if the manager seeded one, else at the end of the block (below F-nat44-ei-64-66-nptv6's lines if present). Never reorder or reformat other lines. List every hunk under "Shared hunks" in docs/status/tasks/F-det44-map-dslite-cnat.md
  - apps/agent/internal/desired/nat.go (F-nat44-ed-sessions' builder, dep-chained): replace the det44/dslite/map/cnat `agent.unsupported-field` cases of the CGNAT group under the `// wave-BC: F-det44-map-dslite-cnat` anchor that ED seeds (ED envelope obligation) by calls into your builders, and add the pnat call there — one hunk; never touch the EI group above it (F-nat44-ei-64-66-nptv6 may be editing it in parallel)
  - the natTabs registry file in apps/web/src/domains/firewall/nat44-ed-sessions/ (dep-chained): your four tabs under your ED-seeded anchor
  - A1 apps/agent/internal/subsystems/subsystems.go: det44/mapnat/cnat/pnat/dslite Register lines with natcommon.WithGlobalsOwner(env.GlobalsOwner) + WithClaims(<KeyedClaims("nat")>); append their names to Domains["nat"]
  - A2 apps/agent/internal/agent/projection.go: the assemble hook (the project side goes through ED's dispatch)
  - A4 apps/agent/internal/agent/server.go: two cases in the Action type switch (det44_session_close, cnat_session_purge); RPC methods live in your rpc_*.go
  - A7 docs/vpp-code-track.md: `### V-new (F-det44-map-dslite-cnat)` for any new gap (e.g. DS-Lite AFTR/B4 not deletable); the manager numbers it
  - C1 packages/schema/src/domains/nat.ts (P02b's): one key line `pnat` (sub-schema in your ext file) · C2 packages/schema/src/semantic/index.ts: one spread line · C3 packages/schema/src/index.ts: one export line
  - C4 packages/schema/examples/ + packages/proto/test/fixtures/: new files only; existing nat-*.json (incl. nat-cgnat.json), invalid-nat-*.json, all-domains.json and the ED/EI sets (nat44-ed-*, nat44-ei-*, nat64-*) are read-only
  - C5 packages/proto/vrx/v1/dataplane.proto: RPCs `Det44Sessions`, `Det44Lookup`, `CnatSessions` under the service anchor; `NatConfig.pnat` + every new message in a `// ----- F-det44-map-dslite-cnat -----` section at the end
  - allocated numbers (docs/status/wave-BC-numbers.md, a merge blocker if reused): **NatConfig 27 `pnat`** (25–26 stay F-nat44-ed-sessions'), **ActionRequest 9 `det44_session_close`**, **ActionRequest 10 `cnat_session_purge`**; Det44Config 8–9, DsliteConfig 5, MapConfig 4, MapDomain 12, CnatConfig 3, CnatTranslation 7 only for a proven gap; nothing else — do NOT append to ED's NatSessions*/NatSessionKillAction messages (EI does, in parallel)
  - C6 docs/contracts/proto.md: `### F-det44-map-dslite-cnat: Det44Sessions, Det44Lookup, CnatSessions`
  - C7 generated, regenerated and never hand-edited: apps/agent/gen/**, packages/proto/gen/ts/**, packages/api-client/src/generated/**, apps/cli/internal/api/operations_gen.go, docs/user/cli/reference.md (`packages/proto/gen.sh`, `pnpm gen`, `make -C apps/cli gen docs`)
  - P1 apps/api/src/app.module.ts · P4 apps/api/src/agent/agent.client.ts (det44Sessions, det44Lookup, cnatSessions; the two actions reuse F-vrf-static-ecmp's generic Action stream method) · P5 apps/api/src/testing/fake-agent.ts (UNIMPLEMENTED stubs under the anchor on the contract commit) · W3 apps/web/src/i18n.ts
contract: commit `contract(schema): nat.pnat` and `contract(proto): det44/cnat sessions, det44 lookup, pnat` as separate commits on YOUR branch first (the ci.sh contract guard checks the subject), plus docs/status/tasks/F-det44-map-dslite-cnat-contract.md; tell the manager in the questions file and keep building. No own branches (P08 pattern; the prompt's old `contract/…` branch wording is void)
files you must not touch:
  - everything else
  - never: /root/ngfw (main), other worktrees, /etc (incl. /etc/vpp), /root/vpp
  - apps/agent/binapi (P04/manager-owned), tools/lab (the rig stays IPv4), tools/ci.sh, plan/tasks.yaml, docs/decisions/LOG.md, docs/status/PROGRESS.md, packages/ui-kit/**
  - agent core (A5): apps/agent/internal/agent/{agent,service,state,ifstate}.go, apps/agent/cmd/**; apps/agent/internal/vpp/ifsanitize/** (TD-3); apps/agent/internal/desired/interfaces.go
  - apps/agent/internal/descriptors/natcommon/** (read-only, shared with the sibling NAT tasks → questions file), descriptors/{nat44ed,nat44ei,nat64,nat66,npt66}/**
  - the rest of F-nat44-ed-sessions' files beyond the two dep-chained hotspots (desired/nat_*.go, rpc_nat44_ed*.go, actions/nat44-ed-sessions/**, features/nat44-ed-sessions/**) and all of F-nat44-ei-64-66-nptv6's files (desired/{nat44ei,nat64,nat66,nptv6}*.go, rpc_nat44_ei*.go, features/nat44-ei-64-66-nptv6/**)
  - packages/schema/src/semantic/nat.ts (P02b's rules `nat.cnat-valid`, `nat.dslite-valid`, `nat.map-valid` — test them, add only in your own file); apps/api/src/actions/actions.controller.ts (F-vrf-static-ecmp); apps/api/src/state/** (P08)
host rules:
  - VPP SAFETY (D-064): `systemctl show vpp -p NRestarts` before and after every host run (pasted); on a rise, disable the test behind an opt-in env var at once and record the message (A7)
  - V9: every host step that enables det44 is opt-in (`VRX_FDET44_DET44_HOST=1`) and runs in a manager window: det44 cannot be disabled again until the next VPP restart, and enabling it is a global (D-071) — hold `flock -x /run/lock/vrx-globals.lock` around the enable, never disable. Without the window, det44 is covered by the fake + the det44.map/interface host steps only if det44 is already enabled (require, else skip with the reason)
  - globals (MAP parameters, DS-Lite AFTR/B4, cnat SNAT policy/addresses, cnat purge): host tests read them under `flock -s /run/lock/vrx-globals.lock`; any test that sets one is opt-in (`VRX_FDET44_GLOBALS=1`), holds the lock exclusively, restores exactly the previous value (never VPP defaults; shared-host-rules §7) and runs only in a manager window
  - nattest.SlotLock(t, "<plugin>") in every NAT host test; plugin enables only through nattest.EnsurePlugin under the D-082 globals lock
  - V19 SAFETY (D-095): before ANY packet through the rig, `go -C apps/agent run ./cmd/vrx-vpp-preflight` must exit 0 (TD-3). Stop if it does not
  - delete order (tests, restart simulation, rollback): pnat attachments before bindings; cnat translations/SNAT interfaces, det44 interfaces/maps, MAP interfaces/rules/domains and DS-Lite pools before the interfaces and tables they sit on (D-095c)
  - D-101: bring the veth down before any af_packet delete (TD-5 may not be merged); af_packet rings per D-113
  - IPv6 on the rig: add it yourself inside fd00:<SLOT hex>::/32 on ns-w<SLOT>-lan/wan and your rig interfaces, remove it in t.Cleanup
  - with VRX_INTEGRATION=1, one Go package at a time; hold `flock -s` on the lab lock only during a run (D-094)
coordination: F-nat44-ei-64-66-nptv6 may run in parallel — both of you append to ED's desired/nat.go dispatch and natTabs; you do not touch its NatSessions variant fields (your session RPCs are separate, prefix-named) · F-ha-state-sync (wave C) owns NAT HA · F-ipfix-sflow owns CGNAT logging export (you provide only the det44 lookup) · F-lb owns the lb plugin
evidence: Playwright is not installed. Take the screenshots (en + fa/RTL: CGNAT tab with the port-block calculator, MAP, CNAT, PNAT) with the headless Chrome approach from P07a/P07b/P08 (`test/topology/interfaces/shots_test.go`, kept outside the product code), and say so. Record per acceptance line which packet steps ran in a manager window and which were skipped with a reason
time box: 24 h — when exceeded: stop, commit WIP, write docs/status/tasks/F-det44-map-dslite-cnat.md with what is left (order of work: dslite + det44 + CNAT first, MAP next, PNAT last — D-099 says a leftover becomes a follow-up row)
WIP: commit at least every 45 min; keep docs/status/tasks/F-det44-map-dslite-cnat-wip.md current
CI: `TMPDIR=/tmp/g-w<SLOT> tools/ci.sh --base main` — short TMPDIR (unix socket paths ≤ 108 chars); no host-wide CI lock: golangci-lint serializes itself since main fc0fe68 (D-106 rejected serialising whole gates). Ports 3000/8080/9101 and /run/vrx/agent.sock belong to the running product stack (tools/app) — never touch them
finish: `tools/ci.sh --base main` green in the worktree · docs/status/tasks/F-det44-map-dslite-cnat.md with pasted real output · everything committed · final message = 10-line summary (branch, last commit, CI result, evidence, open questions, decisions taken with options)
cleanup: stop every process you started (API/agent/vite), by PID · lab lock released · vrx_w<SLOT> dropped · no w<SLOT> det44/MAP/DS-Lite/cnat/pnat objects or IPv6 rig addresses left (dumps pasted) · globals and plugins left exactly as the fixtures found them (det44 stays enabled if it was enabled) · rig down · dist/ and apps/agent/bin removed
questions: docs/status/tasks/F-det44-map-dslite-cnat-questions.md — write and keep going; never wait for a human
never: merge · restart/kill VPP · Docker · pkill · secrets in files · edit files you do not own
