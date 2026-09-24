# TASK ENVELOPE — F-loopback-bvi-gso-lldp-span
id: F-loopback-bvi-gso-lldp-span   branch: task/F-loopback-bvi-gso-lldp-span   worktree: /root/ngfw-wt/F-loopback-bvi-gso-lldp-span   base: main@<BASE>   started: <STARTED>
title: Wave A (day 7-9): loopback/BVI, GSO/offload flags, LLDP, SPAN/ERSPAN, nsim
prompt: prompts/features/F-loopback-bvi-gso-lldp-span.md   (template: prompts/FEATURE-TEMPLATE.md; checked against main + P08 in wave-A prep)   wbs: D1.11, D1.8, D1.7, D1.10, D1.9
scope: loopback/BVI (verify + document; creation exists since P08), GSO, SPAN/ERSPAN mirroring, LLDP wiring + the LldpNeighbors state RPC, nsim (lab tool)
merged deps you can rely on: P08, DF-1, DF-7, F-bridge-l2 (a wave-A follow-on: spawn only after F-bridge-l2 merges)
  - P08: desired/ Sink + Ptr + Assemble, subsystems registry + Wiring stores + Env.GlobalsOwner, projection.go, InterfaceState state-RPC pattern, loop<N> create end to end, test/topology/interfaces
  - DF-1: interface/<name> alias, interface.mtu (jumbo)
  - DF-7: lldp.global/lldp.interface, lldp.Neighbours, span.mirror, df7test.AlignedLoopback
  - F-bridge-l2: L2 model + BVI member support; consume it, do not edit it
  - also on main: TD-3 (V19 sanitizer apps/agent/internal/vpp/ifsanitize + cmd/vrx-vpp-preflight; loopback Create sanitizes) and TD-2 (API auth/users follow-ups)
read first: prompts/features/F-loopback-bvi-gso-lldp-span.md · docs/status/vertical-slice.md · docs/status/wave-A-hotspots.md (§0 rules, §2 numbers; your ids: A1, A2, A6, A7, C1–C7, P1, P4, P5, W1, W2, W3, D1) · docs/agent/descriptors/{lldp,span,interface}.md · docs/status/tasks/F-bridge-l2.md · docs/vpp-code-track.md V19–V21 · docs/decisions/LOG.md D-063, D-064, D-071, D-076, D-080, D-082, D-090, D-095, D-101, D-104
slot: <SLOT> → VRX_SLOT=<SLOT> VRX_TEST_PREFIX=w<SLOT> VRX_HTTP_PORT=3000+100·<SLOT> VRX_WEB_PORT=5000+100·<SLOT> VRX_METRICS_PORT=9100+10·<SLOT>+1 VRX_AGENT_SOCKET=/run/vrx-test/w<SLOT>/agent.sock VRX_PG_DATABASE=vrx_w<SLOT> VRX_VALKEY_DB=<SLOT> VRX_VPP_TABLE_BASE=<SLOT>000 VRX_LAB_LOCK=/run/lock/vrx-lab.lock
  - source of truth: `eval "$(tools/lab env <SLOT>)"`
  - rig prefix w<SLOT> → 10.<SLOT>.{1,2}.0/24
  - test agents run with VRX_GLOBALS_OWNER=0 (D-071)
  - slots 1–11 only; 12 is CI
daemon-owner: none
obligations:
  - D-071 globals: lldp.global (via RegisterGlobals) and nsim.config are applied only when Env.GlobalsOwner is true. Slot agents skip them with a warning.
  - D-082: any host test that changes a global holds `flock -x /run/lock/vrx-globals.lock` behind its opt-in env var
  - nsim has no getter, so a test cannot restore the previous value: the nsim host test is opt-in (VRX_NSIM_HOST=1) and runs only in a manager VPP window. Default-gate nsim evidence is the fake client.
  - D-063/D-076/D-080: gso.interface, lldp.interface and nsim.* are write-only. Keep applied-once records keyed by boot identity + sw_if_index + logical name, in a store from subsystems.Wiring (no in-memory stores).
  - V20: LLDP host tests use df7test.AlignedLoopback or skip with the reason
  - V19/V21: disable GSO/SPAN/nsim before an interface is deleted. Inherited-state findings go to docs/vpp-code-track.md + the questions file (ifsanitize is manager-owned).
  - D-095c: the restart simulation and rollback delete mirror/GSO/LLDP before interfaces
  - services domain: if Domains["services"] is not on main yet, create it and emit agent.unsupported-field for every other non-empty services.* leaf. F-rpf-adl-pbr (services.autoSdl) may have done it first: then append and take lldp/nsim off its list.
  - D-064: a test that crashes VPP goes behind an opt-in env var at once
  - D-077: base new descriptors on descriptors/dfkit
  - D-104: use, do not rebuild, DF-7's lldp/span and P08's loopback path
  - D-105 (TD-3 M2): add a semantic rule on your contract commit that refuses config loopbacks loop16000–loop16383 (reserved for the agent's quarantine holders); rule id `interfaces.loopback-bvi-gso-lldp-span-…`, pointer to the offending key
numbers: NOT yet in wave-A-hotspots §2 (you are a follow-on). Proposed from the "20–29 waves B+" block; the manager confirms and records them in §2 before spawn. Reusing a number or taking "next free" blocks the merge.
  - Interface.gso = 20 and Interface.mirror = 21 (+ a nested Mirror message)
  - ServicesConfig.nsim = 9 (ServicesConfig 8 is F-rpf-adl-pbr's auto_sdl) (+ NsimService)
  - rpc LldpNeighbors (+ LldpNeighbor* messages)
files you own exclusively:
  - apps/agent/internal/descriptors/{lldp,span,gso,nsim}/**, docs/agent/descriptors/{lldp,span,gso,nsim}.md
  - apps/agent/internal/desired/{gso,mirror,lldp,nsim}*.go, apps/agent/internal/agent/rpc_loopback_bvi_gso_lldp_span*.go, apps/agent/internal/subsystems/loopback_bvi_gso_lldp_span*.go
  - apps/agent/internal/descriptors/core/coretest/loopback_bvi_gso_lldp_span*.go
  - apps/api/src/features/loopback-bvi-gso-lldp-span/** (index.ts exports {controllers, providers}; LoopbackBviGsoLldpSpanController; fake.ts), apps/api/test/e2e/loopback-bvi-gso-lldp-span*.ts
  - apps/web/src/domains/interfaces/loopback-bvi-gso-lldp-span/**, apps/web/src/locales/{en,fa}/loopback-bvi-gso-lldp-span.json
  - packages/schema/src/domains/ext/loopback-bvi-gso-lldp-span*.ts, packages/schema/src/semantic/loopback-bvi-gso-lldp-span*.ts
  - packages/schema/examples/loopback-bvi-gso-lldp-span-*.json, packages/proto/test/fixtures/loopback-bvi-gso-lldp-span-*.json
  - docs/user/interfaces/loopback-bvi-gso-lldp-span.md, test/topology/loopback-bvi-gso-lldp-span/**
  - docs/status/tasks/F-loopback-bvi-gso-lldp-span*
shared hotspots (append-only, conflicts resolved by the manager at merge). Insert only under your `wave-A: F-loopback-bvi-gso-lldp-span` anchor, and name every hunk in the task's status file.
  - A1 apps/agent/internal/subsystems/subsystems.go: Domains["interfaces"] names, Domains["services"], lldp/span/gso/nsim Register
  - A2 apps/agent/internal/agent/projection.go: one call in project(), one in assemble() after desired.Assemble; never apps/agent/internal/desired/interfaces.go
  - A7 docs/vpp-code-track.md: append `### V-new (F-loopback-bvi-gso-lldp-span)`
  - C1 packages/schema/src/domains/{interfaces,services}.ts: one key line per field
  - C3 packages/schema/src/index.ts: one export
  - C2 packages/schema/src/semantic/index.ts: one spread
  - C5 packages/proto/vrx/v1/dataplane.proto: your numbers + the RPC under the service anchor + a `// ----- F-loopback-bvi-gso-lldp-span -----` message section
  - C6 docs/contracts/proto.md
  - P1 apps/api/src/app.module.ts
  - P4 apps/api/src/agent/agent.client.ts
  - P5 apps/api/src/testing/fake-agent.ts: an UNIMPLEMENTED stub in the contract commit
  - W1 apps/web/src/router.tsx (LLDP, Mirroring, tools/nsim); W2 apps/web/src/nav/{nav.ts,nav.test.ts} (label keys in your namespace); W3 apps/web/src/i18n.ts
  - D1 docs/user/interfaces/basics.md: one see-also line at the end; not the "Not in this release" line
  - only if the generated drawer form breaks: one named line in apps/web/src/domains/interfaces/model.ts
  - C7 generated files are never hand-merged: `pnpm gen && make -C apps/cli gen docs`
contract: separate `contract(schema): …` / `contract(proto): …` commits first on YOUR branch (P08 pattern; no own branches) + docs/status/tasks/F-loopback-bvi-gso-lldp-span-contract.md. Tell the manager in the questions file and keep building.
files you must not touch:
  - everything else
  - never: /root/ngfw (main), other worktrees, /etc/vpp, /root/vpp
  - apps/agent/binapi (P04/manager-owned)
  - apps/agent/internal/vpp/ifsanitize/** (TD-3, manager-owned)
  - tools/ci.sh, tools/lab, plan/tasks.yaml, docs/decisions/LOG.md, packages/ui-kit/**
  - helper packages apps/agent/internal/descriptors/{interface,dfkit,core,df7,df6,gre}/** except your own coretest file (a fixture may call DF-6's gre descriptor; do not edit it)
  - F-bridge-l2's files: apps/agent/internal/descriptors/{l2,l3xc,mactime}/**, apps/agent/internal/desired/l2*.go, packages/schema/src/{domains/ext,semantic}/bridge-l2*.ts
  - apps/agent/internal/descriptors/af_packet/** (TD-5)
  - apps/agent/internal/desired/interfaces.go
  - apps/api/src/state/** (P08)
  - apps/web/src/domains/interfaces/{InterfacesPage,InterfaceDrawer}.tsx
  - apps/web/src/locales/*/{nav,common,interfaces}.json
  - packages/schema/src/domains/tunnels.ts (F-tunnels)
  - sibling dirs: apps/agent/internal/descriptors/bond/**, apps/web/src/domains/interfaces/{subinterfaces,bonding,bridge-l2}/**
host rules:
  - mirror/LLDP/GSO only on w<SLOT> loopbacks, taps, rig interfaces, and a fixture ERSPAN GRE tunnel named w<SLOT>…
  - V19 SAFETY: send no packets through the rig until TD-3's pre-flight (`go -C apps/agent run ./cmd/vrx-vpp-preflight` exits 0) or a dump shows no classify/ACL/SPD binding on your interfaces' sw_if_index
  - D-101: bring the veth down before any af_packet delete
  - run `systemctl show vpp -p NRestarts` before and after every host run; stop and write the questions file if it rises
  - with VRX_INTEGRATION=1, run one Go package at a time
  - hold `flock -s` on the lab lock only during a run (D-094)
evidence: Playwright is not installed. Take the UI screenshots with the headless Chrome approach of P07a/P07b/P08 (`test/topology/interfaces/shots_test.go`), kept outside the product code, and say so. nsim in the default gate is fake-client only; say so.
time box: 15 h — when exceeded: stop, commit WIP, write docs/status/tasks/F-loopback-bvi-gso-lldp-span.md with what is left
WIP: commit at least every 45 min; keep docs/status/tasks/F-loopback-bvi-gso-lldp-span-wip.md current
CI: `TMPDIR=/tmp/g-w<SLOT> tools/ci.sh --base main` — short TMPDIR (unix socket paths ≤ 108 chars); no host-wide CI lock: golangci-lint serializes itself since main fc0fe68 (D-106 rejected serialising whole gates). Ports 3000/8080/9101 and /run/vrx/agent.sock belong to the running product stack (tools/app) — never touch them
finish: `tools/ci.sh --base main` green in the worktree · docs/status/tasks/F-loopback-bvi-gso-lldp-span.md with pasted real output · everything committed · final message = 10-line summary (branch, last commit, CI result, evidence, open questions, decisions taken with options)
cleanup:
  - stop every process you started (API/agent/vite), by PID
  - lab lock released
  - vrx_w<SLOT> dropped
  - your rig removed
  - no w<SLOT> loopbacks, mirrors or GRE fixtures left (dump pasted)
  - GSO/LLDP disabled on everything you enabled
  - dist/ and apps/agent/bin removed
questions: docs/status/tasks/F-loopback-bvi-gso-lldp-span-questions.md — write and keep going; never wait for a human
never: merge · restart/kill VPP · Docker · pkill · secrets in files · edit files you do not own
