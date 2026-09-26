# TASK ENVELOPE — F-bonding
id: F-bonding   branch: task/F-bonding   worktree: /root/ngfw-wt/F-bonding   base: main@task/W-seed@df67a8e (SPECULATIVE, D-114; contains P08 fix round 2 + TD-5 — safe rings and quiesce)   started: 2026-09-24T19:10
title: Wave A (day 7-9): LACP/XOR/RR/active-backup bonds
prompt: prompts/features/F-bonding.md   (template: prompts/FEATURE-TEMPLATE.md; checked against main + P08 in wave-A prep)   wbs: D1.5
scope: LACP/XOR/RR/active-backup bonds. The work: bond model + builder onto DF-1's bond.bond/bond.member, a new bond.member-weight descriptor, the BondState RPC, the API module, the Bonds page, docs. No second bond descriptor.
merged deps you can rely on: P08, DF-1
  - P08: desired/ Sink + Assemble + KindOf, subsystems registry + Wiring stores, projection.go, InterfaceState state-RPC pattern, test/topology/interfaces
  - DF-1: bond.bond, bond.member, interface/<name> alias, ClaimStore
  - also on main: TD-3 (V19 sanitizer apps/agent/internal/vpp/ifsanitize + cmd/vrx-vpp-preflight; bond.bond Create sanitizes) and TD-2 (API auth/users follow-ups)
read first: prompts/features/F-bonding.md · docs/status/vertical-slice.md · docs/status/wave-A-hotspots.md (§0 rules, §2 numbers; your ids: A1, A2, A3 kind-only, A6, C1–C7, P1, P4, P5, W1, W2, W3, D1) · docs/agent/descriptors/{bond,interface}.md · docs/decisions/LOG.md D-063, D-065, D-069, D-071, D-075, D-076, D-080, D-095, D-101, D-104
slot: 6 → VRX_SLOT=6 VRX_TEST_PREFIX=w6 VRX_HTTP_PORT=3000+100·6 VRX_WEB_PORT=5000+100·6 VRX_METRICS_PORT=9100+10·6+1 VRX_AGENT_SOCKET=/run/vrx-test/w6/agent.sock VRX_PG_DATABASE=vrx_w6 VRX_VALKEY_DB=6 VRX_VPP_TABLE_BASE=6000 VRX_LAB_LOCK=/run/lock/vrx-lab.lock
  - source of truth: `eval "$(tools/lab env 6)"`
  - rig prefix w6 → 10.6.{1,2}.0/24
  - bond ids come from 6000–6999
  - test agents run with VRX_GLOBALS_OWNER=0 (D-071)
  - slots 1–11 only; 12 is CI
daemon-owner: none
obligations:
  - D-065/D-069: the bond's logical name = its config key (VPP name BondEthernet<id>). Every reference goes through interface/<name>, whose creator is bond.bond/<name>.
  - D-075: untagged members are ours only through the persisted ClaimStore from subsystems.Wiring (D-080 boot identity); no in-memory stores
  - D-063/D-076: the weight descriptor has a real Retrieve (sw_member_interface_dump.weight), so do not make it write-only
  - D-077: base new code on descriptors/dfkit
  - D-095c: the restart simulation deletes members before the bond
  - D-104: use, do not rebuild, DF-1's bond descriptors
numbers (wave-A-hotspots §2; reusing one or taking "next free" blocks the merge):
  - yours: Interface.bond = 13, the nested Bond/BondMember messages, and rpc BondState (+ BondState* messages, prefix `Bond`)
  - not yours: Interface 14–19 and Subinterface 12–17 (F-bridge-l2, F-neighbors-ra, F-rpf-adl-pbr)
files you own exclusively:
  - apps/agent/internal/descriptors/bond/**, docs/agent/descriptors/bond.md
  - apps/agent/internal/desired/bond*.go, apps/agent/internal/agent/rpc_bonding*.go, apps/agent/internal/subsystems/bonding*.go
  - apps/agent/internal/descriptors/core/coretest/bonding*.go
  - apps/api/src/features/bonding/** (index.ts exports {controllers, providers}; BondingController; fake.ts), apps/api/test/e2e/bonding*.ts
  - apps/web/src/domains/interfaces/bonding/**, apps/web/src/locales/{en,fa}/bonding.json
  - packages/schema/src/domains/ext/bonding*.ts, packages/schema/src/semantic/bonding*.ts
  - packages/schema/examples/bonding-*.json, packages/proto/test/fixtures/bonding-*.json
  - docs/user/interfaces/bonding.md, test/topology/bonding/**
  - docs/status/tasks/F-bonding*
shared hotspots (append-only, conflicts resolved by the manager at merge). Insert only under your `wave-A: F-bonding` anchor, and name every hunk in F-bonding.md.
  - A3 apps/agent/internal/desired/interfaces.go: the bond kind only (one const, one KindOf case, one creator case)
  - A1 apps/agent/internal/subsystems/subsystems.go: Domains["interfaces"] names + bond.Register
  - A2 apps/agent/internal/agent/projection.go: one call in project(), one in assemble() after desired.Assemble
  - C1 packages/schema/src/domains/interfaces.ts: one key line
  - C3 packages/schema/src/index.ts: one export
  - C2 packages/schema/src/semantic/index.ts: one spread
  - C5 packages/proto/vrx/v1/dataplane.proto: field 13 + the RPC under the service anchor + a `// ----- F-bonding -----` message section
  - C6 docs/contracts/proto.md: `### F-bonding: BondState`
  - P1 apps/api/src/app.module.ts: one import + spreads
  - P4 apps/api/src/agent/agent.client.ts: one method
  - P5 apps/api/src/testing/fake-agent.ts: an UNIMPLEMENTED stub in the contract commit
  - W1 apps/web/src/router.tsx; W2 apps/web/src/nav/{nav.ts,nav.test.ts} (the label key is in your namespace, not nav.json); W3 apps/web/src/i18n.ts
  - D1 docs/user/interfaces/basics.md: one see-also line at the end; not the "Not in this release" line
  - only if the generated drawer form breaks: one named line in apps/web/src/domains/interfaces/model.ts
  - C7 generated files are never hand-merged: `pnpm gen && make -C apps/cli gen docs` (apps/agent/gen, packages/proto/gen, packages/api-client, apps/cli/internal/api/operations_gen.go, docs/user/cli/reference.md)
  - deviation from wave-A-hotspots §3: Bonds is its own route (W1/W2), not a tab in P08's InterfacesPage (W5)
contract: separate `contract(schema): …` / `contract(proto): …` commits first on YOUR branch (P08 pattern; no own branches) + docs/status/tasks/F-bonding-contract.md. Tell the manager in the questions file and keep building.
files you must not touch:
  - everything else
  - never: /root/ngfw (main), other worktrees, /etc/vpp, /root/vpp
  - apps/agent/binapi (P04/manager-owned)
  - apps/agent/internal/vpp/ifsanitize/** (TD-3, manager-owned)
  - tools/ci.sh, tools/lab, plan/tasks.yaml, docs/decisions/LOG.md, packages/ui-kit/**
  - helper packages apps/agent/internal/descriptors/{interface,dfkit,core}/** except your own coretest file (findings → questions file)
  - apps/agent/internal/descriptors/af_packet/** (TD-5)
  - apps/api/src/state/** (P08)
  - apps/web/src/domains/interfaces/{InterfacesPage,InterfaceDrawer}.tsx (P08 / F-vlan-qinq)
  - apps/web/src/locales/*/{nav,common,interfaces}.json
  - sibling wave-A dirs: apps/agent/internal/descriptors/{l2,l3xc,mactime,lldp,span,gso,nsim}/**, apps/web/src/domains/interfaces/{subinterfaces,bridge-l2,loopback-bvi-gso-lldp-span}/**
host rules:
  - members are only slot-prefixed af_packet host-interfaces (veths w6…) or fixture taps named w6…; never ens* NICs
  - V19 SAFETY: send no packets through the rig until TD-3's pre-flight (`go -C apps/agent run ./cmd/vrx-vpp-preflight` exits 0) or a dump shows no classify/ACL/SPD binding on your interfaces
  - D-101: bring the veth down before any af_packet delete (TD-5 may not be merged)
  - run `systemctl show vpp -p NRestarts` before and after every host run; stop and write the questions file if it rises
  - with VRX_INTEGRATION=1, run one Go package at a time
  - hold `flock -s` on the lab lock only during a run (D-094)
evidence: Playwright is not installed. Take the UI screenshot with the headless Chrome approach of P07a/P07b/P08 (`test/topology/interfaces/shots_test.go`), kept outside the product code, and say so. LACP will likely not negotiate without a partner: the evidence is config + `show lacp`.
time box: 15 h — when exceeded: stop, commit WIP, write docs/status/tasks/F-bonding.md with what is left
WIP: commit at least every 45 min; keep docs/status/tasks/F-bonding-wip.md current
CI: `TMPDIR=/tmp/g-w6 tools/ci.sh --base main` — short TMPDIR (unix socket paths ≤ 108 chars); no host-wide CI lock: golangci-lint serializes itself since main fc0fe68 (D-106 rejected serialising whole gates). Ports 3000/8080/9101 and /run/vrx/agent.sock belong to the running product stack (tools/app) — never touch them
finish: `tools/ci.sh --base main` green in the worktree · docs/status/tasks/F-bonding.md with pasted real output · everything committed · final message = 10-line summary (branch, last commit, CI result, evidence, open questions, decisions taken with options)
cleanup:
  - stop every process you started (API/agent/vite), by PID
  - lab lock released
  - vrx_w6 dropped
  - your rig and extra veths/taps removed
  - no w6 bonds left in VPP (dump pasted)
  - dist/ and apps/agent/bin removed
questions: docs/status/tasks/F-bonding-questions.md — write and keep going; never wait for a human
never: merge · restart/kill VPP · Docker · pkill · secrets in files · edit files you do not own
