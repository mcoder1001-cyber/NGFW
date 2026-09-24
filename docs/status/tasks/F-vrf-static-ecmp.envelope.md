# TASK ENVELOPE — F-vrf-static-ecmp
id: F-vrf-static-ecmp   branch: task/F-vrf-static-ecmp   worktree: /root/ngfw-wt/F-vrf-static-ecmp   base: main@<BASE>   started: <STARTED>
title: Wave A (day 7-9): VRF mgmt, static routes, ECMP, FIB browser (paged), ping/traceroute actions
prompt: prompts/features/F-vrf-static-ecmp.md   (template: prompts/FEATURE-TEMPLATE.md; checked against main + P08 in wave-A prep)   wbs: D2.1, D2.2, D2.5, D2.6
scope: source-VRF select (new `svs` descriptors), next hop in another VRF, the D-072 `viaFrr` flag + selector, ECMP/blackhole (exist since P05/P08 — verify and test only), agent-side paged FIB browser (`ListRoutes` RPC), ping action (default table only), traceroute = UNIMPLEMENTED + one V-item, the generic Action bridge in the API, UI, docs
merged deps you can rely on: P08
  - P08: desired/ (Sink, `interface/<name>` aliases), subsystems.Register/Domains + Wiring stores, projection.go already mapping `vrfs` and `routing.static` (weights, blackhole, distance, `frr.StaticOwnedByFRR` skip), InterfaceState state-RPC pattern, test/topology/interfaces
  - through P08: P05 core `vrf/<id>` + `ip.route/<table>/<prefix>` (weighted multipath, DROP path for blackhole, best-source claim rule, owner table), P06 API (actions controller = 501 stub, `/state/routes` paging a full Retrieve), P07a/b UI, RF-1 `frr.RegisterStaticSelector`
  - also on main: TD-3 (V19 sanitizer apps/agent/internal/vpp/ifsanitize + cmd/vrx-vpp-preflight), TD-2 (API auth/users follow-ups)
read first: prompts/features/F-vrf-static-ecmp.md · docs/status/wave-A-hotspots.md (§0 rules, §2 numbers, A1 A2 A4 A6 A7 C1–C7 P1–P5 W1–W3) · apps/agent/internal/descriptors/core/README.md · docs/status/vertical-slice.md · docs/vpp-code-track.md V15, V22 · docs/decisions/LOG.md D-063, D-065, D-069, D-071, D-072, D-073, D-076, D-080, D-087, D-094, D-095, D-101
slot: <SLOT> → VRX_SLOT=<SLOT> VRX_TEST_PREFIX=w<SLOT> VRX_HTTP_PORT=3000+100·<SLOT> VRX_WEB_PORT=5000+100·<SLOT> VRX_METRICS_PORT=9100+10·<SLOT>+1 VRX_AGENT_SOCKET=/run/vrx-test/w<SLOT>/agent.sock VRX_PG_DATABASE=vrx_w<SLOT> VRX_VALKEY_DB=<SLOT> VRX_VPP_TABLE_BASE=<SLOT>000 VRX_LAB_LOCK=/run/lock/vrx-lab.lock
  - source of truth: `eval "$(tools/lab env <SLOT>)"`
  - rig prefix w<SLOT> → 10.<SLOT>.{1,2}.0/24; every VRF, SVS table and FIB-browser table id in <SLOT>000–<SLOT>999; route prefixes inside 10.<SLOT>.0.0/16 (+ one slot-unique IPv6 /48)
  - test agents run with VRX_GLOBALS_OWNER=0 (D-071)
  - slots 1–11 only; 12 is CI
daemon-owner: none (FRR rendering of `viaFrr` routes is P12 / F-bfd-redistribution; you only add the flag and register the selector)
obligations:
  - D-104: use, do not rebuild — ECMP weights and blackhole already work end to end (P05 core + P08 projection). Extend `core/route*.go` + `core_model.proto` only for the next-hop table
  - D-072: one programmer per static route. Register `frr.RegisterStaticSelector` reading `viaFrr` exactly once — it panics on a second call and `subsystems.Register` runs many times in tests, so guard it with a `sync.Once` in your owned subsystems file. Never edit apps/agent/internal/renderers/**
  - D-063/D-076/D-080: `svs_dump` reports only interface enablements. `svs.table`/`svs.route` are read back from `ip_table_dump`/`ip_route_v2_dump` (svs FIB source) if VPP exposes them there (verify on the host), else they are write-only: idempotent Create or an applied-once record keyed by the boot identity, in a store taken from `Wiring` (never in-memory). Never echo desired state from Retrieve
  - V15: routes before their table on every delete path (reconciler order, rollback, restart simulation, test cleanup); prove "no stray /32" in `show ip fib table <id>`
  - V22a: never call `ip_table_flush`
  - ping: binapi `want_ping_finished_events{address, repeat, interval}` → one `ping_finished_event{request_count, reply_count}` — default table only, no source/size/per-reply output; a non-default VRF, `source` or `size` answers INVALID_ARGUMENT with a clear message; output = one summary `line` + `done` with stats. One V-item (`### V-new (F-vrf-static-ecmp)`, A7) for traceroute + VRF-aware ping
  - FIB browser: paging and filtering happen in the agent (`ListRoutes`); the API never holds the full table; `Retrieve` stays config-only (proto.md §5)
files you own exclusively:
  - apps/agent/internal/descriptors/core/{route,vrf}*.go and apps/agent/internal/descriptors/core/core_model.proto + core_model.pb.go (append-only: a next-hop table field on RoutePath; nothing renamed) — no other wave-A task edits them
  - apps/agent/internal/descriptors/svs/**, docs/agent/descriptors/svs.md
  - apps/agent/internal/descriptors/core/coretest/vrf_static_ecmp*.go (A6: new file; existing coretest files are read-only)
  - apps/agent/internal/desired/vrf_static_ecmp*.go (SVS builder + assembler), apps/agent/internal/subsystems/vrf_static_ecmp*.go (selector registration, Wiring helpers), apps/agent/internal/agent/rpc_vrf_static_ecmp*.go (`ListRoutes`)
  - apps/agent/internal/actions/vrf-static-ecmp/**
  - apps/api/src/actions/** (P3: the generic Action bridge is yours this wave; others serve static routes from their own controllers)
  - apps/api/src/features/vrf-static-ecmp/** (index.ts exports {controllers, providers}; `VrfStaticEcmpController`; real fake behaviour in fake.ts), apps/api/test/e2e/vrf-static-ecmp*
  - apps/web/src/domains/routing/vrf-static-ecmp/**, apps/web/src/locales/{en,fa}/vrf-static-ecmp.json
  - packages/schema/src/domains/ext/vrf-static-ecmp*.ts, packages/schema/src/semantic/vrf-static-ecmp*.ts (rule ids `vrfs.vrf-static-ecmp-…` / `routing.vrf-static-ecmp-…`), packages/schema/examples/vrf-static-ecmp-*.json, packages/proto/test/fixtures/vrf-static-ecmp-*.json
  - docs/user/routing/vrf-static-ecmp.md, test/topology/vrf-static-ecmp/**, docs/status/tasks/F-vrf-static-ecmp*
shared hotspots (append-only, conflicts resolved by the manager at merge):
  - protocol: docs/status/wave-A-hotspots.md §0. Insert only directly below your own anchor `// wave-A: F-vrf-static-ecmp`. Never reorder or reformat other lines. List every hunk under "Shared hunks" in docs/status/tasks/F-vrf-static-ecmp.md
  - A1 apps/agent/internal/subsystems/subsystems.go: svs registration, svs descriptor names in `Domains[VRFs]`, the call into your selector registration
  - A2 apps/agent/internal/agent/projection.go: one call in project() and one in assemble() for the SVS builder — plus the next-hop-VRF lines inside P08's `routing.static` project/assemble blocks (the only in-place edit of P08 code allowed this wave; nobody else edits those blocks)
  - A4 apps/agent/internal/agent/server.go: you are the first expected toucher — turn `Action` into a type switch with a default Unimplemented and a case anchor each for F-neighbors-ra and F-nat44-ed-sessions (unless the manager's anchor commit already did); your ping case
  - A7 docs/vpp-code-track.md: append `### V-new (F-vrf-static-ecmp)`; the manager numbers it
  - C1 packages/schema/src/domains/{vrfs,routing}.ts: one key line each for `VrfSchema.sourceSelect`, `NextHopSchema.vrf`, `StaticRouteSchema.viaFrr` (sub-schemas in your ext file) · C2 packages/schema/src/semantic/index.ts: one spread line · C3 packages/schema/src/index.ts if you export anything · C4 new fixture files only
  - C5 packages/proto/vrx/v1/dataplane.proto: `ListRoutes` under the service anchor; new messages in a `// ----- F-vrf-static-ecmp -----` section at the end
  - allocated numbers (wave-A-hotspots §2, a merge blocker if reused): **Vrf 3 `source_select`**, **StaticRoute 7 `via_frr`**, **NextHop 4 `vrf`** (not yet in §2 — NextHop is touched only by you; the manager adds it), ActionRequest 6 spare (not needed: ping = 1 exists); nothing else
  - C6 docs/contracts/proto.md: `### F-vrf-static-ecmp: ListRoutes` · C7 generated, regenerated and never hand-edited: apps/agent/gen/**, packages/proto/gen/ts/**, packages/api-client/src/generated/**, apps/cli/internal/api/operations_gen.go, docs/user/cli/reference.md (`packages/proto/gen.sh`, `pnpm gen`, `make -C apps/cli gen docs`)
  - P1 apps/api/src/app.module.ts · P2 apps/api/src/state/state.controller.ts: move the `GET /state/routes` handler into your controller and delete the old block in one hunk (Fastify rejects duplicate routes; keep the blank separator) · P4 apps/api/src/agent/agent.client.ts (`listRoutes` + a generic Action stream method the others reuse) · P5 apps/api/src/testing/fake-agent.ts (UNIMPLEMENTED stub under the anchor on the contract commit)
  - W1 apps/web/src/router.tsx · W2 apps/web/src/nav/nav.ts + nav.test.ts (`vrfs` and `routing` into BUILT_DOMAINS) · W3 apps/web/src/i18n.ts
contract: commit `contract(schema): vrfs source-select, next-hop vrf, viaFrr` and `contract(proto): ListRoutes` as separate commits on YOUR branch first (the ci.sh contract guard checks the subject), plus docs/status/tasks/F-vrf-static-ecmp-contract.md; tell the manager in the questions file and keep building. No own branches (P08 pattern)
files you must not touch:
  - everything else
  - never: /root/ngfw (main), other worktrees, /etc (incl. /etc/vpp), /root/vpp
  - apps/agent/binapi + tools/binapi-gen.sh (P04/manager-owned), tools/lab, tools/ci.sh, plan/tasks.yaml, docs/decisions/LOG.md, docs/status/PROGRESS.md, packages/ui-kit/** (a need → questions file)
  - agent core (A5): apps/agent/internal/agent/{agent,service,ifstate}.go, apps/agent/cmd/**; apps/agent/internal/desired/interfaces.go (A3); apps/agent/internal/vpp/ifsanitize/** (TD-3)
  - apps/agent/internal/renderers/** (RF-1/P12 — call the selector, never edit); every other descriptor package (DF-2 ip_neighbor/ip6_nd/arp are F-neighbors-ra's, urpf/adl/abf/classify F-rpf-adl-pbr's, acl F-acl's, nat* the NAT tasks', bond/l2/l3xc/mactime/lldp/span/gso/nsim F-bonding/F-bridge-l2/F-loopback's, af_packet TD-5's); core/{loopback,ifaddr,core}.go (P05/P08)
  - apps/api/src/{auth,users}/** (TD-4); apps/api/src/state/** beyond the P2 hunk
host rules:
  - plugins `ping`, `svs` and core `ip` are loaded on vrx-a (confirm once with `vppctl show plugins`); no data NICs, no linux-cp needed
  - V19 SAFETY (D-095, crash on first packet): before ANY packet through the rig, `go -C apps/agent run ./cmd/vrx-vpp-preflight` must exit 0 (TD-3). Stop if it does not
  - run `systemctl show vpp -p NRestarts` before and after every host run; stop and write the questions file if it rises (D-064)
  - D-101: bring the veth down before any af_packet delete (TD-5 may not be merged)
  - 100k-prefix FIB-browser test: one slot table, removed in t.Cleanup (routes first, then the table); paste its timing, no tuning (FAST MODE: no performance work)
  - with VRX_INTEGRATION=1, one Go package at a time; hold `flock -s` on the lab lock only during a run (D-094)
coordination: F-neighbors-ra and F-nat44-ed-sessions share the A4 Action switch and reuse your API Action stream method once you merge · F-neighbors-ra also appends to `Vrf` (4) and `RoutingConfig` (10); F-rpf-adl-pbr to `RoutingConfig` (11) · P12 (FRR) consumes the `viaFrr` flag through the selector you register
evidence: Playwright is not installed. Take the screenshots (en + fa/RTL: VRF list, ECMP route editor, FIB browser page, ping result) with the headless Chrome approach from P07a/P07b/P08 (`test/topology/interfaces/shots_test.go`, kept outside the product code), and say so.
time box: 15 h — when exceeded: stop, commit WIP, write docs/status/tasks/F-vrf-static-ecmp.md with what is left
WIP: commit at least every 45 min; keep docs/status/tasks/F-vrf-static-ecmp-wip.md current
CI: `TMPDIR=/tmp/g-w<SLOT> tools/ci.sh --base main` — short TMPDIR (unix socket paths ≤ 108 chars); no host-wide CI lock: golangci-lint serializes itself since main fc0fe68 (D-106 rejected serialising whole gates). Ports 3000/8080/9101 and /run/vrx/agent.sock belong to the running product stack (tools/app) — never touch them
finish: `tools/ci.sh --base main` green in the worktree · docs/status/tasks/F-vrf-static-ecmp.md with pasted real output · everything committed · final message = 10-line summary (branch, last commit, CI result, evidence, open questions, decisions taken with options)
cleanup: stop every process you started (API/agent/vite), by PID · lab lock released · vrx_w<SLOT> dropped · no w<SLOT> tables/routes/SVS objects left in VPP (Retrieve + `show ip fib table <id>` pasted) · rig down · dist/ and apps/agent/bin removed
questions: docs/status/tasks/F-vrf-static-ecmp-questions.md — write and keep going; never wait for a human
never: merge · restart/kill VPP · Docker · pkill · secrets in files · edit files you do not own
