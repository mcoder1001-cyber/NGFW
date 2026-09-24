# TASK ENVELOPE — F-vlan-qinq
id: F-vlan-qinq   branch: task/F-vlan-qinq   worktree: /root/ngfw-wt/F-vlan-qinq   base: main@<BASE>   started: <STARTED>
title: Wave A (day 7-9): 802.1q sub-interfaces + QinQ stacking
prompt: prompts/features/F-vlan-qinq.md   (regenerated for D-104 on task/prompts-s4b, on main before wave A)   wbs: D1.4
scope: close the QinQ / 802.1ad gap on top of DF-1 + P08 and prove it (tests, UI tag-stack column, docs). No new descriptor, projection, state route or screen.
merged deps you can rely on: P08, DF-1
  - P08: desired/interfaces.go sub-interface projection + assembler; InterfaceState RPC with parent/vlan_id/inner_vlan_id; /state/interfaces rows; InterfaceDrawer sub-interface table; test/topology/interfaces pattern
  - DF-1: interface.subinterface, interface/<name> alias, attribute descriptors
  - also on main: TD-3 (V19 sanitizer apps/agent/internal/vpp/ifsanitize + cmd/vrx-vpp-preflight; sub-interface Create sanitizes) and TD-2 (API auth/users follow-ups)
read first: prompts/features/F-vlan-qinq.md · docs/status/vertical-slice.md · docs/status/tasks/P08.md · docs/status/wave-A-hotspots.md (§0 rules; your ids: W5, W3, D1; defect-only A3, C2) · docs/agent/descriptors/interface.md
slot: <SLOT> → VRX_SLOT=<SLOT> VRX_TEST_PREFIX=w<SLOT> VRX_HTTP_PORT=3000+100·<SLOT> VRX_WEB_PORT=5000+100·<SLOT> VRX_METRICS_PORT=9100+10·<SLOT>+1 VRX_AGENT_SOCKET=/run/vrx-test/w<SLOT>/agent.sock VRX_PG_DATABASE=vrx_w<SLOT> VRX_VALKEY_DB=<SLOT> VRX_VPP_TABLE_BASE=<SLOT>000 VRX_LAB_LOCK=/run/lock/vrx-lab.lock
  - source of truth: `eval "$(tools/lab env <SLOT>)"`
  - rig prefix w<SLOT> → 10.<SLOT>.{1,2}.0/24
  - test agents run with VRX_GLOBALS_OWNER=0 (D-071)
  - slots 1–11 only; 12 is CI
daemon-owner: none
obligations:
  - D-065/D-069: logical names; a sub-interface is `<parent>.<id>`
  - D-104: use, do not rebuild, DF-1/P08 objects
  - persisted stores come only from subsystems.Wiring (D-080; no in-memory stores in the product agent)
  - D-095c: the restart simulation deletes addresses before sub-interfaces
  - exact-match stays hard-coded; record the open question (F-bridge-l2 decides L2 use)
  - tag rewrite belongs to F-bridge-l2
  - D-105 (P08 F1): `/state/interfaces` `items[].config` keeps the Retrieve view; the running config is a new additive field. Read the tag type (dot1ad) from `config`, and recheck after P08's fix round lands
files you own exclusively:
  - apps/web/src/domains/interfaces/subinterfaces/**
  - apps/web/src/locales/{en,fa}/vlan-qinq.json
  - apps/api/test/e2e/vlan-qinq*.ts (your own e2e file instead of editing P08's interfaces.e2e.test.ts)
  - apps/agent/internal/desired/interfaces_qinq_test.go
  - packages/schema/src/semantic/interfaces-qinq.test.ts
  - packages/schema/examples/vlan-qinq-*.json
  - docs/user/interfaces/vlan-qinq.md
  - test/topology/vlan-qinq/**
  - docs/status/tasks/F-vlan-qinq*
shared hotspots (append-only, conflicts resolved by the manager at merge). Insert only under your `wave-A: F-vlan-qinq` anchor, and name every hunk in F-vlan-qinq.md.
  - W5 apps/web/src/domains/interfaces/InterfaceDrawer.tsx: one hunk that swaps in SubinterfaceTable. You are the first W5 toucher, so merge early.
  - W3 apps/web/src/i18n.ts: the vlan-qinq namespace
  - D1 docs/user/interfaces/basics.md: only the one line your prompt names (the "QinQ is not supported…" line → a link). Leave the "Not in this release" line alone; the manager updates it after the wave.
  - defect-only, with the proving test named:
    - A3 apps/agent/internal/desired/interfaces.go
    - apps/agent/internal/descriptors/interface/subinterface.go
    - C2 packages/schema/src/semantic/interfaces.ts (as a contract commit)
  - generated files are never hand-merged: after any contract commit, run `pnpm gen && make -C apps/cli gen docs`
contract: none expected. For a real gap: separate additive `contract(schema|proto): …` commits first + docs/status/tasks/F-vlan-qinq-contract.md, and tell the manager. No proto numbers are allocated to you; ask the manager (wave-A-hotspots §2), never take "next free".
files you must not touch:
  - everything else
  - never: /root/ngfw (main), other worktrees, /etc/vpp, /root/vpp
  - apps/agent/binapi (P04/manager-owned)
  - apps/agent/internal/vpp/ifsanitize/** (TD-3, manager-owned)
  - tools/ci.sh, plan/tasks.yaml, docs/decisions/LOG.md, packages/ui-kit/**
  - apps/api/src/state/** and apps/api/test/e2e/interfaces.e2e.test.ts (P08's)
  - apps/web/src/locales/*/{interfaces,nav,common}.json (existing `sub.*` keys stay)
  - sibling wave-A dirs: apps/web/src/domains/interfaces/{bonding,bridge-l2,loopback-bvi-gso-lldp-span}/**, apps/agent/internal/descriptors/{bond,l2,l3xc,mactime,lldp,span,gso,nsim}/**
host rules:
  - V19 SAFETY: send no packets through the rig (including the optional scapy test) until TD-3's pre-flight (`go -C apps/agent run ./cmd/vrx-vpp-preflight` exits 0) or a dump shows no classify/ACL/SPD binding on your interfaces' sw_if_index
  - D-101: bring the veth down before any af_packet delete (rig hand-over, simulated loss, cleanup)
  - run `systemctl show vpp -p NRestarts` before and after every host run; stop and write the questions file if it rises
  - with VRX_INTEGRATION=1, run one Go package at a time
  - hold `flock -s` on the lab lock only during a run (D-094)
evidence: Playwright is not installed. Take the screenshots (en + fa/RTL) with the headless Chrome approach of P07a/P07b/P08 (`test/topology/interfaces/shots_test.go`), kept outside the product code, and say so.
time box: 15 h — when exceeded: stop, commit WIP, write docs/status/tasks/F-vlan-qinq.md with what is left
WIP: commit at least every 45 min; keep docs/status/tasks/F-vlan-qinq-wip.md current
CI: `TMPDIR=/tmp/g-w<SLOT> tools/ci.sh --base main` — short TMPDIR (unix socket paths ≤ 108 chars); no host-wide CI lock: golangci-lint serializes itself since main fc0fe68 (D-106 rejected serialising whole gates). Ports 3000/8080/9101 and /run/vrx/agent.sock belong to the running product stack (tools/app) — never touch them
finish: `tools/ci.sh --base main` green in the worktree · docs/status/tasks/F-vlan-qinq.md with pasted real output · everything committed · final message = 10-line summary (branch, last commit, CI result, evidence, open questions, decisions taken with options)
cleanup:
  - stop every process you started (API/agent/vite), by PID
  - lab lock released
  - vrx_w<SLOT> dropped
  - your rig removed (`tools/lab rig down`)
  - no w<SLOT> sub-interfaces left in VPP (dump pasted)
  - dist/ and apps/agent/bin removed
questions: docs/status/tasks/F-vlan-qinq-questions.md — write and keep going; never wait for a human
never: merge · restart/kill VPP · Docker · pkill · secrets in files · edit files you do not own
