# TASK ENVELOPE — F-mpls-srmpls
id: F-mpls-srmpls   branch: task/F-mpls-srmpls   worktree: /root/ngfw-wt/F-mpls-srmpls   base: main@<BASE>   started: <STARTED>
title: Wave C (day 13-15): static MPLS + SR-MPLS (LDP split to F-mpls-ldp, D-085/D-109)
prompt: prompts/features/F-mpls-srmpls.md   (template: prompts/FEATURE-TEMPLATE.md; refreshed on task/prep-rest against main, task/P08 and task/W-seed)   wbs: D2.8
scope: the new `routing.mpls` model (contract: RoutingConfig 15 + an `MplsConfig` section whose field 10 stays reserved for F-mpls-ldp's `ldp`), projection onto DF-7's `mpls-*` and DF-6's `sr-mpls.*` descriptors, an `MplsState` RPC (MPLS FIB paged, tunnels), API, UI (MPLS page whose tab list F-mpls-ldp extends by one entry), docs. No LDP, no FRR
merged deps you can rely on: P08, DF-7, DF-6, W-seed (+ the wave-B/C anchor pass, docs/status/wave-BC-numbers.md "Pack rules")
  - DF-7: apps/agent/internal/descriptors/mpls/ + docs/agent/descriptors/mpls.md — `mpls-table/<id>`, `mpls-interface/<if>`, `mpls-route/<t>/<label>/<eos|neos>`, `mpls-ip-bind` (write-only), `mpls-tunnel/<name>`; table 0 VPP-global; shared-table-0 ownership by D-080 boot record (review H2); `df7.WithIDRange`
  - DF-6: apps/agent/internal/descriptors/sr_mpls/ + docs/agent/descriptors/sr_mpls.md — `sr-mpls.policy/<bsid>` (depends on `mpls-table/0`), `sr-mpls.steering/<table>/<prefix>`, `sr-mpls.endpoint-color` — all partial/write-only with exact presence probes (V14: no dump)
  - P08: desired/ + subsystems/ + projection patterns, `Wiring.BootStore()` / `IfaceClaims()` (df6.WithClaims) / `df7.SetBootStore`, the `interface/<name>` alias (D-065/D-069); `subsystems.SlotIDRange()` (W-seed)
  - also on main: TD-2, TD-3 (V19 sanitizer + preflight), TD-5 if merged
read first: prompts/features/F-mpls-srmpls.md · docs/status/wave-BC-numbers.md (Pack rules, section F-mpls-srmpls) · docs/status/wave-A-hotspots.md (§0 rules; ids A1 A2 A6 C1–C7 P1 P4 P5 W1–W3 A7) · docs/agent/descriptors/{mpls,sr_mpls,df6}.md · docs/status/tasks/{DF-7,DF-6}.md · docs/vpp-code-track.md V14, V15, V22 · docs/decisions/LOG.md D-063, D-064, D-071, D-074, D-076, D-080, D-082, D-085, D-087, D-094, D-095, D-101, D-109
slot: <SLOT> → VRX_SLOT=<SLOT> VRX_TEST_PREFIX=w<SLOT> VRX_HTTP_PORT=3000+100·<SLOT> VRX_WEB_PORT=5000+100·<SLOT> VRX_METRICS_PORT=9100+10·<SLOT>+1 VRX_AGENT_SOCKET=/run/vrx-test/w<SLOT>/agent.sock VRX_PG_DATABASE=vrx_w<SLOT> VRX_VALKEY_DB=<SLOT> VRX_VPP_TABLE_BASE=<SLOT>000 VRX_LAB_LOCK=/run/lock/vrx-lab.lock
  - source of truth: `eval "$(tools/lab env <SLOT>)"`
  - rig prefix w<SLOT> → 10.<SLOT>.{1,2}.0/24; MPLS tables, VRFs and any id in <SLOT>000–<SLOT>999 (`df7.WithIDRange`); test labels and BSIDs from a slot band (e.g. <SLOT>×10000 … +9999, all ≥ 16)
  - test agents run with VRX_GLOBALS_OWNER=0 (D-071)
  - slots 1–11 only; 12 is CI
daemon-owner: none
obligations:
  - D-104: use, do not rebuild — DF-7/DF-6 descriptors are gap-only (proven defect + named test); no new MPLS descriptor family
  - D-071: MPLS table 0 is VPP-global — only the globals owner declares `mpls-table/0`; slot agents require it (clear error). On the shared host there is no table 0: `mpls-interface` enable, `mpls-ip-bind` and every SR-MPLS policy (depends on `mpls-table/0`) run only with `VRX_DF7_GLOBALS=1` under `flock -x /run/lock/vrx-globals.lock` (D-082) in a manager window — otherwise the host check covers slot tables + label routes + tunnels and the rest is fake-client evidence, said so in F-mpls-srmpls.md
  - D-063/D-076/D-080: `mpls-ip-bind` and all `sr-mpls.*` are write-only — idempotent Create or an applied-once record keyed by the boot identity in a store from `Wiring` (never in-memory); never echo desired state from Retrieve
  - D-074: SR-MPLS segment lists kept sorted; every delete checks existence first (V8/V14 crash family)
  - V15 / V22a: label routes and steering before their tables on every delete path (reconciler order, rollback, restart simulation, test cleanup); never `ip_table_flush` / a table flush; prove "no stray entry" in `show mpls fib table <t>`
  - F-mpls-ldp (next task) extends you additively: `routing.mpls` stays a plain object; seed in YOUR files the anchors it needs — `// wave-BC: F-mpls-ldp` inside `MplsSchema` (ext/mpls-srmpls.ts), a `// 10 reserved: ldp (F-mpls-ldp)` comment + anchor inside `MplsConfig` (your proto section), and a tab anchor in `apps/web/src/domains/routing/mpls-srmpls/tabs.ts` (the tab list is a plain array)
  - answer in F-mpls-srmpls.md who declares `mpls-table/0` in production (proposed: the globals owner, whenever `routing.mpls` is non-empty) — F-mpls-ldp carries it over
files you own exclusively:
  - apps/agent/internal/descriptors/{mpls,sr_mpls}/** and docs/agent/descriptors/{mpls,sr_mpls}.md (gap-only)
  - apps/agent/internal/descriptors/core/coretest/mpls_srmpls*.go (A6: new file only)
  - apps/agent/internal/desired/mpls_srmpls*.go (builder + assembler), apps/agent/internal/subsystems/mpls_srmpls*.go (mpls.Register / sr_mpls.Register with Wiring stores + `df7.WithIDRange`), apps/agent/internal/agent/rpc_mpls_srmpls*.go (`MplsState`)
  - packages/schema/src/domains/ext/mpls-srmpls*.ts, packages/schema/src/semantic/mpls-srmpls*.ts (rule ids `routing.mpls-srmpls-…`), packages/schema/examples/mpls-srmpls-*.json, packages/proto/test/fixtures/mpls-srmpls-*.json
  - apps/api/src/features/mpls-srmpls/** (index.ts exports {controllers, providers}; `MplsSrmplsController`; `GET /api/v1/state/routing/mpls/{fib,tunnels}`; real fake behaviour in fake.ts), apps/api/test/e2e/mpls-srmpls*
  - apps/web/src/domains/routing/mpls-srmpls/** (incl. tabs.ts), apps/web/src/locales/{en,fa}/mpls-srmpls.json
  - docs/user/routing/mpls-srmpls.md, test/topology/mpls-srmpls/**, docs/status/tasks/F-mpls-srmpls*
shared hotspots (append-only, conflicts resolved by the manager at merge):
  - protocol: docs/status/wave-A-hotspots.md §0. Insert only directly below your own anchor `// wave-BC: F-mpls-srmpls` (manager's wave-B/C anchor pass); if missing, say so in the questions file and insert at the end of that anchor block. dataplane.proto: keep the one-blank-line framing. Never reorder or reformat other lines. List every hunk under "Shared hunks" in docs/status/tasks/F-mpls-srmpls.md
  - A1 apps/agent/internal/subsystems/subsystems.go: the mpls-* and sr-mpls.* descriptor names (package constants) into `Domains[Routing]`; one call into your subsystems/mpls_srmpls.go at the end of `Register()`
  - A2 apps/agent/internal/agent/projection.go: one builder call in project(), one assembler call in assemble()
  - C1 packages/schema/src/domains/routing.ts: `RoutingSchema.mpls` key line (sub-schema in ext/mpls-srmpls.ts) · C2 semantic/index.ts · C3 schema/src/index.ts · C4 new fixture files only
  - C5 packages/proto/vrx/v1/dataplane.proto: `RoutingConfig` 15, the `MplsState` rpc under the service anchor; `MplsConfig` and every new message in `// ----- F-mpls-srmpls -----`
  - C6 docs/contracts/proto.md (`### F-mpls-srmpls: MplsState`) · C7 generated, never hand-edited (apps/agent/gen/**, packages/proto/gen/ts/**, packages/api-client/src/generated/**, apps/cli/internal/api/operations_gen.go, docs/user/cli/reference.md)
  - P1 apps/api/src/app.module.ts · P4 apps/api/src/agent/agent.client.ts · P5 apps/api/src/testing/fake-agent.ts (UNIMPLEMENTED stub on the contract commit; real fake in features/mpls-srmpls/fake.ts)
  - W1 apps/web/src/router.tsx · W2 apps/web/src/nav/nav.ts + nav.test.ts (an MPLS NavItem in the routing group, labelKey in `mpls-srmpls:`) · W3 apps/web/src/i18n.ts
  - A7 docs/vpp-code-track.md: `### V-new (F-mpls-srmpls)` only for a new gap; the manager numbers it
  - allocated numbers (docs/status/wave-BC-numbers.md, a merge blocker if reused): **RoutingConfig 15 `mpls`**; inside your new `MplsConfig` fields 1–9 are yours and **10 is F-mpls-ldp's** — nothing else (no EventKind, no ActionRequest)
contract: commit `contract(schema): routing mpls` and `contract(proto): routing mpls, MplsState` as separate commits on YOUR branch first + docs/status/tasks/F-mpls-srmpls-contract.md; tell the manager in the questions file and keep building. Never a `contract/` branch
files you must not touch:
  - everything else
  - never: /root/ngfw (main), other worktrees, /etc (incl. /etc/vpp), /root/vpp; no kernel modules, no `net.mpls.*` sysctls
  - apps/agent/binapi + tools/binapi-gen.sh, tools/lab, tools/ci.sh, plan/tasks.yaml, docs/decisions/LOG.md, docs/status/PROGRESS.md, packages/ui-kit/** (a need → questions file)
  - apps/agent/internal/descriptors/{df6,df7,dfkit}/** (shared helpers), sr/** (F-srv6), every other descriptor package
  - F-mpls-ldp's files: apps/agent/internal/renderers/frr/ldp/**, apps/agent/internal/frrsync/**, docs/agent/renderers/frr-ldp.md; all of apps/agent/internal/renderers/**
  - agent core (A5): apps/agent/internal/agent/{agent,service,state,ifstate}.go, apps/agent/cmd/**; apps/agent/internal/desired/interfaces.go (A3); packages/schema/src/domains/routing.ts beyond the anchored key line; apps/api/src/state/**
host facts:
  - VPP mpls is core; `srmpls_plugin.so` loaded (plugin on disk, default-enabled); binapi `mpls` (`mpls_table_add_del`, `mpls_route_add_del`, `mpls_route_dump`, `mpls_ip_bind_unbind`, `mpls_tunnel_add_del`, `mpls_tunnel_dump`, `sw_interface_set_mpls_enable`, `mpls_interface_dump`) and `sr_mpls` (`sr_mpls_policy_add/mod/del`, `sr_mpls_steering_add_del`, `sr_mpls_policy_assign_endpoint_color`) — confirm every name in apps/agent/binapi/
  - no MPLS table 0 on the shared host (DF-7 host run: `mpls-interface` Create → `NO_SUCH_FIB`); kernel MPLS is not loaded (/proc/sys/net/mpls absent) — irrelevant here (no FRR), never load it
  - source (VPP 26.06 lcp_mpls_sync.c): enabling MPLS on an interface that has an LCP pair also enables it on the host tap and writes `net.mpls.conf.<tap>.input` in the pair's netns — do not enable MPLS on LCP-paired interfaces in tests (no kernel MPLS here); note it for F-mpls-ldp
  - no data NICs bound (D-026): af_packet rig only (D-010); packet-level MPLS test not required
coordination: (1) F-mpls-ldp depends on you — the anchors above, your `/state/routing/mpls/{fib,tunnels}` routes and the table-0 answer · (2) F-srv6 (parallel) owns `descriptors/sr` (SRv6); you own `descriptors/sr_mpls` — both DF-6, shared helpers in `df6` read-only for both · (3) F-igmp-mfib: MPLS multicast out of scope for both
V19/V24 SAFETY (D-095, D-101): before ANY packet through the rig (optional labelled ping only), run TD-3's preflight on the rig interfaces' sw_if_index and never send packets through an unchecked interface; every af_packet delete with its veth down; one host test package at a time (D-087); `systemctl show vpp -p NRestarts` before/after every host run pasted — stop host runs and write it down if it rises
evidence: Playwright is not installed — use the headless Chrome approach from P07a/P07b/P08 (kept outside the product code) for screenshots (en + fa/RTL: MPLS interfaces, label routes, tunnels, SR policies/steering tabs); paste `vppctl show mpls fib table <t>`, `show mpls tunnel`, `show sr mpls policies` (globals window only)
time box: 15 h — when exceeded: stop, commit WIP, write docs/status/tasks/F-mpls-srmpls.md with what is left
WIP: commit at least every 45 min; keep docs/status/tasks/F-mpls-srmpls-wip.md current
CI: `TMPDIR=/tmp/g-w<SLOT> tools/ci.sh --base main` — short TMPDIR (unix socket paths ≤ 108 chars); no host-wide CI lock: golangci-lint serializes itself since main fc0fe68 (D-106 rejected serialising whole gates). Ports 3000/8080/9101 and /run/vrx/agent.sock belong to the running product stack (tools/app) — never touch them
finish: `tools/ci.sh --base main` green in the worktree · docs/status/tasks/F-mpls-srmpls.md with pasted real output · everything committed · final message = 10-line summary (branch, last commit, CI result, evidence, open questions, decisions taken with options)
cleanup: stop every process you started (API/agent/vite) by PID · lab lock released · vrx_w<SLOT> dropped · no w<SLOT> MPLS tables/routes/tunnels/SR objects left (Retrieve + `show mpls fib table <t>` pasted; routes removed before tables) · rig down · dist/ and apps/agent/bin removed
questions: docs/status/tasks/F-mpls-srmpls-questions.md — write and keep going; never wait for a human
never: merge · restart/kill VPP · Docker · pkill · secrets in files · edit files you do not own
