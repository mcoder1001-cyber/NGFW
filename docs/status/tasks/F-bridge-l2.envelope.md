# TASK ENVELOPE — F-bridge-l2
id: F-bridge-l2   branch: task/F-bridge-l2   worktree: /root/ngfw-wt/F-bridge-l2   base: main@task/W-seed@8b7558e (SPECULATIVE, D-114/D-120: P08 fix round 2 still running — do NOT merge main until the manager tells you P08 has landed)   started: 2026-09-24T17:27
title: Wave A (day 7-9): bridge domains, L2XC/L3XC, split-horizon, MAC aging, time-range MAC filter
prompt: prompts/features/F-bridge-l2.md   (template: prompts/FEATURE-TEMPLATE.md; checked against main + P08 in wave-A prep)   wbs: D1.6
scope: the L2 model + its builder onto DF-1's l2/l3xc descriptors (incl. l2.vlan-tag-rewrite on L2 members), a new mactime descriptor, the BridgeDomainState/BridgeDomainMacs RPCs, the API module, the Bridging page, docs. No second l2/l3xc descriptor.
merged deps you can rely on: P08, DF-1
  - P08: desired/ Sink + Ptr + Assemble, subsystems registry + Wiring stores, projection.go, InterfaceState state-RPC pattern, test/topology/interfaces
  - DF-1: l2.bridge-domain/-member/xconnect/fib-entry/flags/vlan-tag-rewrite, l3xc.l3xc, interface/<name> alias, ClaimStore
  - also on main: TD-3 (V19 sanitizer apps/agent/internal/vpp/ifsanitize + cmd/vrx-vpp-preflight; every new interface starts in L3 mode) and TD-2 (API auth/users follow-ups)
read first: prompts/features/F-bridge-l2.md · docs/status/vertical-slice.md · docs/status/wave-A-hotspots.md (§0 rules, §2 numbers; your ids: A1, A2, A6, A7, C1–C7, P1, P4, P5, W1, W2, W3, D1) · docs/agent/descriptors/{l2,l3xc,interface}.md · docs/decisions/LOG.md D-045, D-053, D-063, D-065, D-069, D-071, D-076, D-080, D-095, D-101, D-104
slot: 7 → VRX_SLOT=7 VRX_TEST_PREFIX=w7 VRX_HTTP_PORT=3000+100·7 VRX_WEB_PORT=5000+100·7 VRX_METRICS_PORT=9100+10·7+1 VRX_AGENT_SOCKET=/run/vrx-test/w7/agent.sock VRX_PG_DATABASE=vrx_w7 VRX_VALKEY_DB=7 VRX_VPP_TABLE_BASE=7000 VRX_LAB_LOCK=/run/lock/vrx-lab.lock
  - source of truth: `eval "$(tools/lab env 7)"`
  - rig prefix w7 → 10.7.{1,2}.0/24
  - bridge-domain ids come from 7000–7999
  - mactime device names start with w7
  - test agents run with VRX_GLOBALS_OWNER=0 (D-071)
  - slots 1–11 only; 12 is CI
daemon-owner: none
L2 model placement (the manager fills this in before spawn, logged as a D-id): D-109 (c): per-member leaves Interface.l2 (14) / Subinterface.l2 (12) plus a NAMED CONTAINER for bridge-domain records inside an existing domain object (variant b; never a new root key — that would be a reshape, always-PENDING #1)
  - wave-A-hotspots C1 says "no new root key (a reshape is PENDING; F-bridge-l2 asks)"
  - options: (a) a new root key `l2`, i.e. DesiredState field 14 — it deviates from docs/04, so decision-policy #1 → PENDING; (b) per-member `interfaces.<if>.l2` (Interface 14) / `subinterfaces.<id>.l2` (Subinterface 12), with the BD/xconnect/l3xc/macFilter records in a container the manager names
  - if D-109 (c): per-member leaves Interface.l2 (14) / Subinterface.l2 (12) plus a NAMED CONTAINER for bridge-domain records inside an existing domain object (variant b; never a new root key — that would be a reshape, always-PENDING #1) is still here: take variant (b) with the least-reshaping container, write the question, and keep going
obligations:
  - D-065/D-069: logical names via interface/<name>
  - BDs stay addressable by the numeric id that tunnels.*.bridgeDomain already uses
  - D-063/D-076/D-080: the mactime per-interface enable is write-only. Keep an applied-once record keyed by boot identity + sw_if_index + logical name, in a store from subsystems.Wiring (no in-memory stores).
  - D-071: the mactime device table is shared; touch only w7/owner-prefixed devices
  - D-077: base new descriptors on descriptors/dfkit
  - D-095c: the restart simulation and rollback delete dependents (flags, tag rewrite, members, mactime enable) before the BD or interfaces
  - tag rewrite on L2 members is yours (F-vlan-qinq fences it to you)
  - D-104: use, do not rebuild, DF-1's l2/l3xc descriptors
numbers (wave-A-hotspots §2; reusing one or taking "next free" blocks the merge):
  - yours: Interface.l2 = 14 and Subinterface.l2 = 12 (variant b); the BD-record container/number comes with D-109 (c): per-member leaves Interface.l2 (14) / Subinterface.l2 (12) plus a NAMED CONTAINER for bridge-domain records inside an existing domain object (variant b; never a new root key — that would be a reshape, always-PENDING #1)
  - yours: rpc BridgeDomainState + rpc BridgeDomainMacs (+ BridgeDomain* messages)
  - not yours: Interface 13 or 15–19, Subinterface 13–17, ServicesConfig 8
files you own exclusively:
  - apps/agent/internal/descriptors/{l2,l3xc,mactime}/**, docs/agent/descriptors/{l2,l3xc,mactime}.md
  - apps/agent/internal/desired/l2*.go, apps/agent/internal/agent/rpc_bridge_l2*.go, apps/agent/internal/subsystems/bridge_l2*.go
  - apps/agent/internal/descriptors/core/coretest/bridge_l2*.go
  - apps/api/src/features/bridge-l2/** (index.ts exports {controllers, providers}; BridgeL2Controller; fake.ts), apps/api/test/e2e/bridge-l2*.ts
  - apps/web/src/domains/interfaces/bridge-l2/**, apps/web/src/locales/{en,fa}/bridge-l2.json
  - packages/schema/src/domains/ext/bridge-l2*.ts, packages/schema/src/semantic/bridge-l2*.ts
  - packages/schema/examples/bridge-l2-*.json, packages/proto/test/fixtures/bridge-l2-*.json
  - docs/user/interfaces/bridge-l2.md, test/topology/bridge-l2/**
  - docs/status/tasks/F-bridge-l2*
shared hotspots (append-only, conflicts resolved by the manager at merge). Insert only under your `wave-A: F-bridge-l2` anchor, and name every hunk in F-bridge-l2.md.
  - A1 apps/agent/internal/subsystems/subsystems.go: Domains names + l2/l3xc/mactime Register
  - A2 apps/agent/internal/agent/projection.go: one call in project(), one in assemble() after desired.Assemble; never apps/agent/internal/desired/interfaces.go
  - A7 docs/vpp-code-track.md: append `### V-new (F-bridge-l2)`
  - C1 packages/schema/src/domains/<file named by the placement answer>.ts: key lines
  - C3 packages/schema/src/index.ts: one export
  - C2 packages/schema/src/semantic/index.ts: one spread
  - C5 packages/proto/vrx/v1/dataplane.proto: your numbers + the RPCs under the service anchor + a `// ----- F-bridge-l2 -----` message section
  - C6 docs/contracts/proto.md
  - P1 apps/api/src/app.module.ts
  - P4 apps/api/src/agent/agent.client.ts
  - P5 apps/api/src/testing/fake-agent.ts: an UNIMPLEMENTED stub in the contract commit
  - W1 apps/web/src/router.tsx; W2 apps/web/src/nav/{nav.ts,nav.test.ts} (a Bridging NavItem with the label key in your namespace); W3 apps/web/src/i18n.ts
  - D1 docs/user/interfaces/basics.md: one see-also line at the end; not the "Not in this release" line
  - only if the generated drawer form breaks: one named line in apps/web/src/domains/interfaces/model.ts
  - C7 generated files are never hand-merged: `pnpm gen && make -C apps/cli gen docs`
contract: separate `contract(schema): …` / `contract(proto): …` commits first on YOUR branch (P08 pattern; no own branches) + docs/status/tasks/F-bridge-l2-contract.md. Tell the manager in the questions file and keep building.
files you must not touch:
  - everything else
  - never: /root/ngfw (main), other worktrees, /etc/vpp, /root/vpp
  - apps/agent/binapi (P04/manager-owned)
  - apps/agent/internal/vpp/ifsanitize/** (TD-3, manager-owned)
  - tools/ci.sh, tools/lab, plan/tasks.yaml, docs/decisions/LOG.md, packages/ui-kit/**
  - helper packages apps/agent/internal/descriptors/{interface,dfkit,core,df6}/** except your own coretest file
  - apps/agent/internal/descriptors/af_packet/** (TD-5)
  - apps/agent/internal/desired/interfaces.go
  - apps/api/src/state/** (P08)
  - apps/web/src/domains/interfaces/{InterfacesPage,InterfaceDrawer}.tsx
  - apps/web/src/locales/*/{nav,common,interfaces}.json
  - packages/schema/src/domains/tunnels.ts and packages/schema/src/semantic/tunnels*.ts (F-tunnels)
  - sibling wave-A dirs: apps/agent/internal/descriptors/{bond,lldp,span,gso,nsim}/**, apps/web/src/domains/interfaces/{subinterfaces,bonding,loopback-bvi-gso-lldp-span}/**
host rules:
  - bridge only w7 interfaces: loopbacks, the rig's host-w7…, their sub-interfaces, fixture taps
  - V19 SAFETY: send no packets through the rig or a BD until TD-3's pre-flight (`go -C apps/agent run ./cmd/vrx-vpp-preflight` exits 0) or a dump shows no classify/ACL/SPD binding on your interfaces' sw_if_index
  - D-101: bring the veth down before any af_packet delete (TD-5 may not be merged)
  - run `systemctl show vpp -p NRestarts` before and after every host run; stop and write the questions file if it rises
  - with VRX_INTEGRATION=1, run one Go package at a time
  - hold `flock -s` on the lab lock only during a run (D-094)
evidence: Playwright is not installed. Take the UI screenshot with the headless Chrome approach of P07a/P07b/P08 (`test/topology/interfaces/shots_test.go`), kept outside the product code, and say so.
time box: 15 h — when exceeded: stop, commit WIP, write docs/status/tasks/F-bridge-l2.md with what is left
WIP: commit at least every 45 min; keep docs/status/tasks/F-bridge-l2-wip.md current
CI: `TMPDIR=/tmp/g-w7 tools/ci.sh --base main` — short TMPDIR (unix socket paths ≤ 108 chars); no host-wide CI lock: golangci-lint serializes itself since main fc0fe68 (D-106 rejected serialising whole gates). Ports 3000/8080/9101 and /run/vrx/agent.sock belong to the running product stack (tools/app) — never touch them
finish: `tools/ci.sh --base main` green in the worktree · docs/status/tasks/F-bridge-l2.md with pasted real output · everything committed · final message = 10-line summary (branch, last commit, CI result, evidence, open questions, decisions taken with options)
cleanup:
  - stop every process you started (API/agent/vite), by PID
  - lab lock released
  - vrx_w7 dropped
  - your rig removed
  - no w7 BDs, cross-connects, l3xc or mactime devices left (dump pasted)
  - dist/ and apps/agent/bin removed
questions: docs/status/tasks/F-bridge-l2-questions.md — write and keep going; never wait for a human
never: merge · restart/kill VPP · Docker · pkill · secrets in files · edit files you do not own
