# TASK ENVELOPE — F-ha-state-sync
id: F-ha-state-sync   branch: task/F-ha-state-sync   worktree: /root/ngfw-wt/F-ha-state-sync   base: main@<BASE>   started: <STARTED>
title: Wave C (T2, partial ok): HA state sync — NAT44-EI session HA, ED/ACL/IPsec gaps made explicit, failover test automation
prompt: prompts/features/F-ha-state-sync.md   (template: prompts/FEATURE-TEMPLATE.md; refreshed on task/prep-rest against main + the F-vrrp-config-sync / NAT envelopes)   wbs: D5.6, D9.3, D9.4
scope: a new `descriptors/hasync` for VPP's NAT44-EI HA listener/failover (the only state sync VPP 26.06 exposes by API), resync/flush actions, DryRun warnings for the unsupported kinds (ED, ACL, IPsec — V2 + a new V-item), a HaSyncState RPC, the "State sync" panel on F-vrrp-config-sync's HA page, the single-host failover automation + a committed-but-deferred two-node script, the support-matrix doc. Honest partial delivery (D-058). No C code, no VPP kills.
merged deps you can rely on: P08, F-vrrp-config-sync, F-nat44-ed-sessions, F-nat44-ei-64-66-nptv6
  - P08: desired/, subsystems.Register/Domains + Wiring stores, Env.GlobalsOwner, projection.go
  - F-vrrp-config-sync: `Domains["ha"]`, the HA page (/system/ha) with its panel registry, VrrpState + the VRRP EventKind, the failover fixture design (VPP VR vs keepalived VR on the rig) and its manager-window rules
  - F-nat44-ed-sessions / F-nat44-ei-64-66-nptv6: the `nat` builder (mode ed|ei), the EI descriptors (gap-owner: F-nat44-ei) and nattest helpers
  - also on main: W-seed (anchors, event sink), TD-3 (V19 sanitizer + cmd/vrx-vpp-preflight), TD-2
read first: prompts/features/F-ha-state-sync.md · docs/status/wave-A-hotspots.md (§0 rules, A1 A2 A4 A7 C1–C7 P1 P4 P5 W3) · docs/status/wave-BC-numbers.md (section F-ha-state-sync) · docs/agent/descriptors/nat-common.md (first) + nat44-ei.md · docs/status/tasks/F-vrrp-config-sync.md + F-nat44-ei-64-66-nptv6.md · docs/vpp-code-track.md V2, V22 · test/topology/vrx-b.yml · docs/decisions/LOG.md D-012, D-058, D-063, D-064, D-071, D-076, D-080, D-082, D-087, D-090, D-095, D-101
slot: <SLOT> → VRX_SLOT=<SLOT> VRX_TEST_PREFIX=w<SLOT> VRX_HTTP_PORT=3000+100·<SLOT> VRX_WEB_PORT=5000+100·<SLOT> VRX_METRICS_PORT=9100+10·<SLOT>+1 VRX_AGENT_SOCKET=/run/vrx-test/w<SLOT>/agent.sock VRX_PG_DATABASE=vrx_w<SLOT> VRX_VALKEY_DB=<SLOT> VRX_VPP_TABLE_BASE=<SLOT>000 VRX_LAB_LOCK=/run/lock/vrx-lab.lock
  - source of truth: `eval "$(tools/lab env <SLOT>)"`
  - rig prefix w<SLOT> → 10.<SLOT>.{1,2}.0/24; HA listener/failover peer addresses inside 10.<SLOT>.0.0/16, UDP ports 20000+100·<SLOT>+30 (listener) / +31 (failover peer); VRFs in <SLOT>000–<SLOT>999
  - test agents run with VRX_GLOBALS_OWNER=0 (D-071)
  - slots 1–11 only; 12 is CI
daemon-owner: none (the VRRP part of the failover run uses the VPP engine; keepalived stays F-vrrp-config-sync's path — if you need it, ask the manager for daemon-owner keepalived)
obligations:
  - D-071/D-082: the NAT44-EI HA listener and failover are VPP-wide globals → `nat44-ei-ha.listener/global` and `nat44-ei-ha.failover/global` are setters only with `WithGlobalsOwner(true)`; slot agents require (Retrieve via `nat44_ei_ha_get_listener` / `_get_failover`) and never set, reset or flush
  - a host test that sets them is opt-in (`VRX_FHA_GLOBALS=1`), holds `flock -x /run/lock/vrx-globals.lock`, restores exactly the previous values read through the getters (never VPP defaults; shared-host-rules §7) and runs in a manager window
  - ED/EI exclusivity: never disable nat44-ed to make room for EI; if another owner holds nat44-ed on the shared VPP, the EI HA host steps skip with that reason and you ask for a window in the questions file
  - V2: `stateSync.nat` with `nat.mode: ed` and `stateSync.acl` → DryRun WARNING "not supported by VPP (V2)", never an error; `stateSync.ipsec` → warning "re-key on failover" + a `### V-new (F-ha-state-sync)` item for IPsec SA sequence/replay sync (no API in 26.06)
  - resync = `nat44_ei_ha_resync` + wait for `nat44_ei_ha_resync_completed_event` with a timeout; flush = `nat44_ei_ha_flush` (globals owner only)
  - D-012: nobody kills or restarts VPP while handover is pending — the two-node script's `--kill-vpp` mode is for the manager after handover and is never run by you; the two-node run is committed and marked **deferred** (no vrx-b)
  - security note for the docs: VPP's NAT HA protocol is unauthenticated UDP → dedicated sync interface/VRF; `ha.cluster.secretRef` cannot protect it (say so)
files you own exclusively:
  - apps/agent/internal/descriptors/hasync/** and docs/agent/descriptors/hasync.md (new; base on descriptors/dfkit, D-077)
  - apps/agent/internal/desired/hasync*.go, apps/agent/internal/subsystems/hasync*.go, apps/agent/internal/agent/rpc_ha_sync*.go (`HaSyncState`), apps/agent/internal/actions/ha-state-sync/**
  - apps/agent/internal/descriptors/core/coretest/hasync*.go (A6: new file, only if needed)
  - packages/schema/src/domains/ext/ha-state-sync*.ts, packages/schema/src/semantic/ha-state-sync*.ts (rule ids `ha.ha-state-sync-…`)
  - packages/schema/examples/ha-state-sync-*.json, packages/proto/test/fixtures/ha-state-sync-*.json
  - apps/api/src/features/ha-state-sync/** (index.ts exports {controllers, providers}; `HaStateSyncController`; GET /state/ha/sync, POST /actions/ha/sync/resync — distinct from F-vrrp-config-sync's POST /actions/ha/sync (config sync); real fake behaviour in fake.ts), apps/api/test/e2e/ha-state-sync*
  - apps/web/src/domains/system/ha-state-sync/** (the State-sync panel), apps/web/src/locales/{en,fa}/ha-state-sync.json
  - docs/user/system/ha-state-sync.md, test/topology/ha-state-sync/** (single-host run + the deferred two-node script), docs/status/tasks/F-ha-state-sync*
shared hotspots (append-only, conflicts resolved by the manager at merge):
  - protocol: docs/status/wave-A-hotspots.md §0. Insert only directly below your own anchor `// wave-BC: F-ha-state-sync` if the manager seeded one, else at the end of the block. Never reorder or reformat other lines. List every hunk under "Shared hunks" in docs/status/tasks/F-ha-state-sync.md
  - A1 apps/agent/internal/subsystems/subsystems.go: append hasync names to `Domains["ha"]` (F-vrrp-config-sync added the key) + one Register line calling your subsystems/hasync.go
  - A2 apps/agent/internal/agent/projection.go: one call in project(), one in assemble()
  - A4 apps/agent/internal/agent/server.go: one case `ha_sync` in the Action type switch; RPC methods live in your rpc_ha_sync*.go
  - A7 docs/vpp-code-track.md: `### V-new (F-ha-state-sync)` (IPsec SA sync); never edit the V2 row — the manager does
  - F-vrrp-config-sync's HA-page panel registry in apps/web/src/domains/system/vrrp-config-sync/ (dep-chained): one line
  - C1 packages/schema/src/domains/ha.ts (P02c's): key lines for `cluster.stateSync.natListener` / `natFailover` (sub-schemas in your ext file) · C2 packages/schema/src/semantic/index.ts: one spread line · C3 packages/schema/src/index.ts: one export line
  - C4 packages/schema/examples/ + packages/proto/test/fixtures/: new files only; ha-*.json, vrrp-config-sync-*.json and all-domains.json are read-only
  - C5 packages/proto/vrx/v1/dataplane.proto: rpc `HaSyncState` under the service anchor; new messages (HaNatListener, HaNatFailover, HaSyncAction, HaSyncState*) in a `// ----- F-ha-state-sync -----` section at the end
  - allocated numbers (docs/status/wave-BC-numbers.md, a merge blocker if reused): **HaCluster.StateSync 4 `nat_listener`, 5 `nat_failover`**, **ActionRequest 13 `ha_sync`** (op resync|flush); no EventKind (the resync-completed event is consumed inside the action); nothing else
  - C6 docs/contracts/proto.md: `### F-ha-state-sync: HaSyncState, ha_sync action` · C7 generated, regenerated and never hand-edited: apps/agent/gen/**, packages/proto/gen/ts/**, packages/api-client/src/generated/**, apps/cli/internal/api/operations_gen.go, docs/user/cli/reference.md (`packages/proto/gen.sh`, `pnpm gen`, `make -C apps/cli gen docs`)
  - P1 apps/api/src/app.module.ts · P4 apps/api/src/agent/agent.client.ts (`haSyncState`; the action reuses F-vrf-static-ecmp's generic Action stream method) · P5 apps/api/src/testing/fake-agent.ts (UNIMPLEMENTED stub under the anchor on the contract commit) · W3 apps/web/src/i18n.ts
contract: commit `contract(schema): ha stateSync nat listener/failover` and `contract(proto): ha state sync` as separate commits on YOUR branch first (the ci.sh contract guard checks the subject), plus docs/status/tasks/F-ha-state-sync-contract.md; tell the manager in the questions file and keep building. No own branches (the prompt's old `contract/F-ha-state-sync` wording is void)
files you must not touch:
  - everything else
  - never: /root/ngfw (main), other worktrees, /etc (incl. /etc/vpp), /root/vpp
  - apps/agent/binapi (P04/manager-owned), tools/lab, tools/ci.sh, plan/tasks.yaml, docs/decisions/LOG.md, docs/status/PROGRESS.md, packages/ui-kit/**
  - apps/agent/internal/descriptors/{nat44ei,nat44ed,natcommon,vrrp}/** (F-nat44-ei / F-nat44-ed / shared / F-vrrp-config-sync — read-only → questions file), apps/agent/internal/renderers/keepalived/**, apps/agent/internal/desired/{nat,nat_*,nat44ei,vrrp}*.go
  - F-vrrp-config-sync's files beyond the panel-registry line (features/vrrp-config-sync/**, domains/system/vrrp-config-sync/**), apps/api/src/{commit,secrets}/**
  - agent core (A5): apps/agent/internal/agent/{agent,service,state,ifstate}.go, apps/agent/cmd/**; apps/agent/internal/vpp/ifsanitize/** (TD-3)
host rules:
  - VPP SAFETY (D-064): `systemctl show vpp -p NRestarts` before and after every host run (pasted); stop host runs and write it down if it rises
  - the EI plugin is required: enable only through nattest.EnsurePlugin under the D-082 globals lock (never disable nat44-ed); nattest.SlotLock(t, "nat44") in every NAT host test
  - listener/failover set + resync packets towards the peer address in ns-w<SLOT>-wan (`tcpdump` there) = opt-in `VRX_FHA_GLOBALS=1` in a manager window (see obligations); outside a window: config round-trip with the fake + require-only host check
  - the VRRP failover timings reuse F-vrrp-config-sync's fixture; its VPP-engine VRRP steps are V22b opt-in and run ALONE in a manager window (D-087, D-090 (3))
  - V19 SAFETY (D-095): before ANY packet through the rig, `go -C apps/agent run ./cmd/vrx-vpp-preflight` must exit 0 (TD-3)
  - D-101: bring the veth down before any af_packet delete; af_packet rings per D-113
  - never kill or restart VPP (D-012); the two-node script is never executed on vrx-a
  - with VRX_INTEGRATION=1, one Go package at a time; hold `flock -s` on the lab lock only during a run (D-094)
coordination: F-vrrp-config-sync (merged) owns VRRP, config sync, cluster membership and the HA page · F-nat44-ei-64-66-nptv6 (merged) owns EI config · P11 / F-ikev2-native own IPsec (you only warn and document "re-key on failover") · the product owner decides whether HA users are steered to NAT44-EI (write the trade-off in the questions file)
evidence: Playwright is not installed. Take the screenshots (en + fa/RTL: State-sync panel with supported/unsupported badges) with the headless Chrome approach from P07a/P07b/P08 (`test/topology/interfaces/shots_test.go`, kept outside the product code), and say so. Paste `vppctl show nat44 ei ha` (or the `_get_listener`/`_get_failover` replies) and record which steps ran in a manager window
time box: 18 h — when exceeded: stop, commit WIP, write docs/status/tasks/F-ha-state-sync.md with what is left
WIP: commit at least every 45 min; keep docs/status/tasks/F-ha-state-sync-wip.md current
CI: `TMPDIR=/tmp/g-w<SLOT> tools/ci.sh --base main` — short TMPDIR (unix socket paths ≤ 108 chars); no host-wide CI lock: golangci-lint serializes itself since main fc0fe68 (D-106 rejected serialising whole gates). Ports 3000/8080/9101 and /run/vrx/agent.sock belong to the running product stack (tools/app) — never touch them
finish: `tools/ci.sh --base main` green in the worktree · docs/status/tasks/F-ha-state-sync.md with pasted real output · everything committed · final message = 10-line summary (branch, last commit, CI result, evidence, open questions, decisions taken with options)
cleanup: stop every process you started (API/agent/vite/tcpdump), by PID · lab lock released · vrx_w<SLOT> dropped · HA globals restored to the values read before the run (pasted) · plugins left as the fixtures found them · no w<SLOT> NAT/VRRP objects left (dumps pasted) · rig down · dist/ and apps/agent/bin removed
questions: docs/status/tasks/F-ha-state-sync-questions.md — write and keep going; never wait for a human
never: merge · restart/kill VPP · Docker · pkill · secrets in files · edit files you do not own
