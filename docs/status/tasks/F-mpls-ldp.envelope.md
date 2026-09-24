# TASK ENVELOPE — F-mpls-ldp
id: F-mpls-ldp   branch: task/F-mpls-ldp   worktree: /root/ngfw-wt/F-mpls-ldp   base: main@<BASE>   started: <STARTED>
title: Wave C: LDP via FRR ldpd + agent-side FRR→VPP label sync (V5 fallback)
prompt: prompts/features/F-mpls-ldp.md   (template: prompts/FEATURE-TEMPLATE.md; refreshed on task/prep-rest against main, task/P08, task/W-seed and the F-mpls-srmpls/P12 envelopes)   wbs: D2.8
scope: exactly three things on top of the merged MPLS stack — the FRR `ldp` section, the agent-side sync that turns FRR's LDP label state into VPP `mpls-route.ldp` objects applied through the scheduler (seam S1), and the LDP state/tab/docs; `routing.mpls.ldp` (MplsConfig 10) is the only contract field
merged deps you can rely on: F-mpls-srmpls, RF-1, P12, P08, DF-7, W-seed (+ the wave-B/C anchor pass and seam S1, docs/status/wave-BC-numbers.md "Pack rules")
  - F-mpls-srmpls: `routing.mpls` (ext/mpls-srmpls.ts + `MplsConfig` with field 10 reserved and a `wave-BC: F-mpls-ldp` anchor), desired/mpls_srmpls*.go, the MPLS page and its tabs.ts (LDP anchor), `MplsState` + `/state/routing/mpls/{fib,tunnels}`, its answer on who declares `mpls-table/0`
  - DF-7: descriptors/mpls (`mpls-route` path set replaced on update, `mpls_route_dump`, shared-table-0 ownership by boot record, `dfkit.ErrNotOurs`)
  - RF-1: sections, `RegisterStateReader` (constant `show … json` only), `RegisterPoller`, `ShowJSON` (64 MiB → `ErrTruncated`), `rc.MapInterface`, `rc.Secret` + `neighbor … password` redaction, `frrtest` (`Options.Daemons`, `Options.NetNS`)
  - P12: FRR singleton descriptor, the linux-cp mapper `internal/lcpmap` (you need the reverse direction, Linux → VPP logical name: adapter in your package over its exported data, never a second LCP dump), LCP pairs, the FRR-state path to the API, the netns FRR peer pattern
  - P08: `subsystems.Register` / `Domains`, the transaction lock in `agent/service.go`, D-063/D-076 reconciler rules, D-080 boot identity
  - seam S1 (dynamic desired source): seeded by the manager if the recommended option was taken — otherwise see obligations
read first: prompts/features/F-mpls-ldp.md · docs/status/wave-BC-numbers.md (Pack rules incl. S1, section F-mpls-ldp) · docs/status/wave-A-hotspots.md (§0 rules; ids A1 C1–C7 P1 P4 P5 P6 W3 A7) · docs/status/tasks/F-mpls-srmpls.md (+ -contract.md) · docs/agent/descriptors/mpls.md · apps/agent/internal/renderers/frr/README.md · docs/status/tasks/P12.md · docs/vpp-code-track.md V5, V15, V22 · docs/decisions/LOG.md D-051, D-063, D-071, D-072, D-076, D-080, D-082, D-085, D-087, D-094, D-095, D-101, D-109 · docs/decisions/PENDING-secret-channel.md
slot: <SLOT> → VRX_SLOT=<SLOT> VRX_TEST_PREFIX=w<SLOT> VRX_HTTP_PORT=3000+100·<SLOT> VRX_WEB_PORT=5000+100·<SLOT> VRX_METRICS_PORT=9100+10·<SLOT>+1 VRX_AGENT_SOCKET=/run/vrx-test/w<SLOT>/agent.sock VRX_PG_DATABASE=vrx_w<SLOT> VRX_VALKEY_DB=<SLOT> VRX_VPP_TABLE_BASE=<SLOT>000 VRX_LAB_LOCK=/run/lock/vrx-lab.lock
  - source of truth: `eval "$(tools/lab env <SLOT>)"`
  - rig prefix w<SLOT> → 10.<SLOT>.{1,2}.0/24; the LDP test MPLS table in <SLOT>000–<SLOT>999 (`WithTable(<SLOT>xxx)`); LSR ids / loopbacks inside 10.<SLOT>.0.0/16
  - test agents run with VRX_GLOBALS_OWNER=0 (D-071)
  - slots 1–11 only; 12 is CI
daemon-owner: frr — slot test instances only through `frrtest` (pathspace w<SLOT>, `Daemons: ldpd`), the VRX side and the peer in `ns-w<SLOT>-wan`; `frr.service` stays disabled; nothing under /etc/frr (incl. /etc/frr/daemons); ldpd's child processes are stopped only by PIDs whose parent is your harness's ldpd AND whose cmdline names your socket dir; other FRR tasks may run in their own slots only after the manager has logged per-slot-instance FRR ownership (wave-BC-launch-queue.md M3; until then one FRR task at a time). A zebra in the ROOT netns (linux-nl FIB proof) is host-wide in any case: one slot at a time, manager window (P12 coordination 4)
obligations:
  - D-104: use, do not rebuild — DF-7 descriptors, F-mpls-srmpls' model/projection/page/state routes, RF-1, P12. No second MPLS page, no new descriptor family, no edits to the FRR framework files
  - seam S1: the sync hands its full desired set to the agent, which plans and applies it under the transaction lock with scope = the `mpls-route.ldp` instance only (never a second writer to VPP). Use the manager-seeded seam if it is on main. If it is not: keep the loop behind a small interface in `frrsync/ldp` with a fake apply in the unit tests, write the question, and do NOT edit apps/agent/internal/agent/{agent,service}.go (A5)
  - scope isolation: `mpls-route.ldp/<table>/<label>/<eos>` = an additive named constructor in descriptors/mpls (gap-only), registered in NO `Domains` entry — commits never plan or delete it; a label collision surfaces as `ErrNotOurs` → sync error + event
  - failure semantics exactly as the prompt (failed read ≠ empty; hold-down flush; withdraw → delete; LDP removed → flush); V15/D-087: LDP routes before their table or interface on every delete path
  - D-071/D-082: table 0 is VPP-global; product LDP routes go to table 0 (the globals owner declares it); the table-0 variant runs only with `VRX_DF7_GLOBALS=1` under `flock -x /run/lock/vrx-globals.lock`
  - D-051 + D-072 (second half): neighbour passwords are `password/<name>` refs via `rc.Secret`, redacted in DryRun/errors/Retrieve; PENDING-secret-channel: the end-to-end password step waits, tests use a fixture resolver with `VRX_TEST_PSK_F-mpls-ldp_<n>`
  - RF-1 review M3: `ldpNeighbors` is the only registered state reader; bindings are read and paged on demand, never in Retrieve
  - host: never load kernel modules or set `net.mpls.*` sysctls; never edit /etc/frr/daemons (write what the product needs in the questions file)
files you own exclusively:
  - apps/agent/internal/renderers/frr/ldp/** (section `ldp`, order 600), docs/agent/renderers/frr-ldp.md
  - apps/agent/internal/frrsync/ldp/** (the V5 loop; never a shared `frrsync/` root package), incl. redacted fixtures under its testdata/
  - apps/agent/internal/subsystems/mpls_ldp*.go (registration of the LDP-scoped route instance + the sync wiring), apps/agent/internal/desired/mpls_ldp*.go (only if a builder is needed), apps/agent/internal/agent/rpc_mpls_ldp*.go (`MplsLdpState`, only under the prompt's condition)
  - packages/schema/src/domains/ext/mpls-ldp*.ts, packages/schema/src/semantic/mpls-ldp*.ts (rule ids `routing.mpls-ldp-…`), packages/schema/examples/mpls-ldp-*.json, packages/proto/test/fixtures/mpls-ldp-*.json
  - apps/api/src/features/mpls-ldp/** (index.ts exports {controllers, providers}; `MplsLdpController`; `GET /api/v1/state/routing/mpls/ldp/{neighbors,bindings,sync}`; real fake behaviour in fake.ts), apps/api/test/e2e/mpls-ldp*
  - apps/web/src/domains/routing/mpls-ldp/**, apps/web/src/locales/{en,fa}/mpls-ldp.json
  - docs/user/routing/mpls-ldp.md, test/topology/mpls-ldp/**, docs/status/tasks/F-mpls-ldp*
shared hotspots (append-only, conflicts resolved by the manager at merge):
  - protocol: docs/status/wave-A-hotspots.md §0. Insert only directly below your own anchor `// wave-BC: F-mpls-ldp` (manager's wave-B/C anchor pass; in F-mpls-srmpls' files, its own seeded anchors); if missing, say so in the questions file and insert at the end of that anchor block. dataplane.proto: keep the one-blank-line framing. Never reorder or reformat other lines. List every hunk under "Shared hunks" in docs/status/tasks/F-mpls-ldp.md
  - dep-chained (F-mpls-srmpls' files, one hunk each under its anchor): packages/schema/src/domains/ext/mpls-srmpls.ts (`MplsSchema.ldp` key line), its proto section (`MplsConfig` field 10), apps/web/src/domains/routing/mpls-srmpls/tabs.ts (the LDP tab entry); gap-only: apps/agent/internal/descriptors/mpls/** + docs/agent/descriptors/mpls.md (the named `mpls-route.ldp` constructor / record scope — say so in the PR)
  - A1 apps/agent/internal/subsystems/subsystems.go: one call into your subsystems/mpls_ldp.go at the end of `Register()` (no `Domains` entry)
  - C2 packages/schema/src/semantic/index.ts · C3 packages/schema/src/index.ts · C4 new fixture files only
  - C5 packages/proto/vrx/v1/dataplane.proto: `EventKind` 23, the `MplsLdpState` rpc under the service anchor (only if needed); new messages in `// ----- F-mpls-ldp -----`
  - C6 docs/contracts/proto.md · C7 generated, never hand-edited (apps/agent/gen/**, packages/proto/gen/ts/**, packages/api-client/src/generated/**, apps/cli/internal/api/operations_gen.go, docs/user/cli/reference.md)
  - P1 apps/api/src/app.module.ts · P4 agent.client.ts + P5 fake-agent.ts (with the RPC) · P6 apps/api/src/infra/bus.ts (`mpls-ldp.events`) + telemetry/relay.service.ts (EventKind 23 case) · W3 apps/web/src/i18n.ts
  - A7 docs/vpp-code-track.md: V5 notes / `### V-new (F-mpls-ldp)`; the manager numbers it
  - allocated numbers (docs/status/wave-BC-numbers.md, a merge blocker if reused): **MplsConfig 10 `ldp`**, **EventKind 23** — nothing else (no "next free", no ActionRequest)
contract: commit `contract(schema): mpls ldp` and `contract(proto): mpls ldp (+ MplsLdpState), ldp neighbour event` as separate commits on YOUR branch first + docs/status/tasks/F-mpls-ldp-contract.md; tell the manager in the questions file and keep building. Never a `contract/` branch. Nothing F-mpls-srmpls added is renamed or reshaped (always-PENDING)
files you must not touch:
  - everything else
  - never: /root/ngfw (main), other worktrees, /etc (incl. /etc/vpp and /etc/frr), /root/vpp, the frr/vpp units, kernel modules and sysctls
  - apps/agent/binapi + tools/binapi-gen.sh, tools/lab, tools/ci.sh, plan/tasks.yaml, docs/decisions/LOG.md, docs/status/PROGRESS.md, packages/ui-kit/** (a need → questions file)
  - apps/agent/internal/renderers/frr/*.go, frr/frrtest/**, frr/templates/**, frr/{bgp,policy,ospf,isis,rip,bfd,redistribute,pim}/**; apps/agent/internal/frrsync/pim/** (F-igmp-mfib); descriptors/{df7,dfkit,sr_mpls}/**
  - F-mpls-srmpls' files beyond the three anchored hunks (desired/mpls_srmpls*, subsystems/mpls_srmpls*, rpc_mpls_srmpls*, features/mpls-srmpls/**, domains/routing/mpls-srmpls/** except tabs.ts); P12's files (lcp, lcpmap, desired/{bgp,lcp}*, subsystems/{frr,lcp}*, rpc_{bgp,routing}*)
  - agent core (A5): apps/agent/internal/agent/{agent,service,state,ifstate}.go, apps/agent/cmd/**; apps/agent/internal/desired/interfaces.go (A3)
host facts:
  - FRR 10.7.1 installed, `/usr/lib/frr/ldpd` present; ldpd = parent + children
  - kernel MPLS NOT loaded (/proc/sys/net/mpls absent; `mpls_router`/`mpls_iptunnel` only on disk) → zebra runs without MPLS and may not build its LFIB; ldpd's LIB (`show mpls ldp binding json`) does not depend on the kernel
  - source (VPP 26.06 lcp_router.c): linux-nl handles AF_MPLS netlink routes — with kernel MPLS on the product image it could sync zebra's LFIB itself; on this host it cannot, so the V5 agent sync is the path here. Record the product option under the prompt's open question 2
  - source (lcp_mpls_sync.c): `mpls-interface` on an LCP-paired interface also enables MPLS on the host tap and writes `net.mpls.conf.<tap>.input` — expect a failure/log line without kernel MPLS; record it, do not work around it
  - source (lcp_router.c / lcp_interface.c): linux-cp installs a (*,224.0.0.0/24) accept mfib entry on LCP pairs and punts unknown UDP/TCP — LDP hellos (224.0.0.2, UDP 646) and sessions (TCP 646) should reach the tap; confirm (prompt open question 5)
  - no MPLS table 0 on the shared host; no data NICs bound (D-026): af_packet rig only (D-010)
coordination: (1) F-igmp-mfib (parallel possible) needs the same seam S1 — reuse, never a second loop framework · (2) F-mpls-srmpls' table-0 answer is binding for you · (3) ingress label imposition is a follow-up row, not built (D-072: IP prefixes belong to linux-nl)
V19/V24 SAFETY (D-095, D-101): before ANY traffic through the rig, run TD-3's preflight on the rig interfaces' sw_if_index and never send packets through an unchecked interface; every af_packet delete with its veth down; one host test package at a time (D-087); `systemctl show vpp -p NRestarts` before/after every host run pasted — stop host runs and write it down if it rises
evidence: Playwright is not installed — use the headless Chrome approach from P07a/P07b/P08 (kept outside the product code) for screenshots (en + fa/RTL: the LDP tab: global form, neighbours, bindings, sync chip); paste `show mpls ldp neighbor`, `show mpls ldp binding`, `vppctl show mpls fib table <t>` and the hold-down log with timestamps
time box: 15 h — when exceeded: stop, commit WIP, write docs/status/tasks/F-mpls-ldp.md with what is left
WIP: commit at least every 45 min; keep docs/status/tasks/F-mpls-ldp-wip.md current
CI: `TMPDIR=/tmp/g-w<SLOT> tools/ci.sh --base main` — short TMPDIR (unix socket paths ≤ 108 chars); no host-wide CI lock: golangci-lint serializes itself since main fc0fe68 (D-106 rejected serialising whole gates). Ports 3000/8080/9101 and /run/vrx/agent.sock belong to the running product stack (tools/app) — never touch them
finish: `tools/ci.sh --base main` green in the worktree · docs/status/tasks/F-mpls-ldp.md with pasted real output · everything committed · final message = 10-line summary (branch, last commit, CI result, evidence, open questions, decisions taken with options)
cleanup: stop every process you started (API/agent/vite; ldpd and its children through the harness Stop, leftovers by verified PID) by PID · lab lock released · vrx_w<SLOT> dropped · no w<SLOT> MPLS tables/routes left (Retrieve + `show mpls fib table <t>` pasted; routes before the table) · no ns-w<SLOT>-* netns left · rig down · dist/ and apps/agent/bin removed
questions: docs/status/tasks/F-mpls-ldp-questions.md — write and keep going; never wait for a human
never: merge · restart/kill VPP · Docker · pkill · secrets in files · edit files you do not own
