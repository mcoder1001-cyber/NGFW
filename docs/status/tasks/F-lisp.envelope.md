# TASK ENVELOPE — F-lisp
id: F-lisp   branch: task/F-lisp   worktree: /root/ngfw-wt/F-lisp   base: main@<BASE>   started: <STARTED>
title: Wave C (day 13-15): LISP/LISP-GPE
prompt: prompts/features/F-lisp.md   (template: prompts/FEATURE-TEMPLATE.md; refreshed on task/prep-rest against main, task/P08, task/W-seed and the F-tunnels envelope)   wbs: D6.8
scope: the minimal LISP / LISP-GPE set, kept thin (T3): the new `tunnels.lisp` model (contract: TunnelsConfig 10), projection onto DF-6's merged lisp descriptors (enable/GPE/PITR globals on the globals owner only, locator sets, local EIDs, map-resolvers/servers, remote mappings, adjacencies, EID-table maps, GPE forwarding entries), `LispState`, API, UI (a LISP tab on the VPN page), docs with the V13/V14 limits; fake-client evidence first — the host run is opt-in in a manager window
merged deps you can rely on: P08, DF-6, W-seed (+ the wave-B/C anchor pass, docs/status/wave-BC-numbers.md "Pack rules")
  - DF-6: apps/agent/internal/descriptors/lisp/ + docs/agent/descriptors/lisp.md — `lisp.locator-set/<name>`, `lisp.locator/…`, `lisp.local-eid/<vni>/<eid>`, `lisp.map-resolver/…`, `lisp.map-server/…`, `lisp.remote-mapping/…`, `lisp.adjacency/…`, `lisp.eid-table-map/l3|l2/<vni>`, `lisp-gpe.fwd-entry/…` (write-only, V13); globals `lisp.enable`, `lisp-gpe.enable`, `lisp.pitr` (setters only with `df6.WithGlobalsOwner(true)`, require variants otherwise); `lisp.SafeToDisable`; the opt-in host test (`VRX_DF6_LISP_HOST=1`, stage limiter `VRX_DF6_LISP_UPTO`)
  - P08: desired/ + subsystems/ + projection patterns, `Wiring.IfaceClaims()` (= `df6.WithClaims` store), `Wiring.BootStore()`, `subsystems.SlotIDRange()` (W-seed), the vpn page shell + `vpnTabs` registry (W-seed)
  - also on main: TD-2, TD-3 (V19 sanitizer + preflight), TD-5 if merged
read first: prompts/features/F-lisp.md · docs/status/wave-BC-numbers.md (Pack rules, section F-lisp) · docs/status/wave-A-hotspots.md (§0 rules; ids A1 A2 A6 C1–C7 P1 P4 P5 W1–W3 A7) · docs/agent/descriptors/{lisp,df6}.md · docs/status/tasks/DF-6.md + DF-6-questions.md (Q5, Q8, Q9) · docs/status/tasks/F-tunnels.envelope.md (shared `tunnels` domain) · docs/vpp-code-track.md V13, V14 · docs/decisions/LOG.md D-063, D-064, D-071, D-074, D-076, D-080, D-082, D-087, D-094, D-095, D-101
slot: <SLOT> → VRX_SLOT=<SLOT> VRX_TEST_PREFIX=w<SLOT> VRX_HTTP_PORT=3000+100·<SLOT> VRX_WEB_PORT=5000+100·<SLOT> VRX_METRICS_PORT=9100+10·<SLOT>+1 VRX_AGENT_SOCKET=/run/vrx-test/w<SLOT>/agent.sock VRX_PG_DATABASE=vrx_w<SLOT> VRX_VALKEY_DB=<SLOT> VRX_VPP_TABLE_BASE=<SLOT>000 VRX_LAB_LOCK=/run/lock/vrx-lab.lock
  - source of truth: `eval "$(tools/lab env <SLOT>)"`
  - rig prefix w<SLOT> → 10.<SLOT>.{1,2}.0/24; VNIs, VRFs/tables and bridge-domain ids in <SLOT>000–<SLOT>999; EIDs/RLOCs inside 10.<SLOT>.0.0/16; locator-set names prefixed w<SLOT>
  - test agents run with VRX_GLOBALS_OWNER=0 (D-071)
  - slots 1–11 only; 12 is CI
daemon-owner: none
obligations:
  - D-104: use, do not rebuild — DF-6 lisp descriptors are gap-only; `descriptors/df6/**` read-only
  - D-071: LISP enable / GPE enable / PITR are VPP-global — setters only on the globals owner; never disabled while any owner still has LISP objects (`lisp.SafeToDisable`); slot agents require them (clear error when LISP is off)
  - D-064 + V14: every enable/disable cycle leaks a `<remote-N>` locator set and `lisp_gpe*` interfaces no API deletes → the host run is opt-in only (`VRX_DF6_LISP_HOST=1`), alone, in a manager VPP window; ask for it in the questions file; without it: fake-client evidence and "not run on the shared VPP" in F-lisp.md
  - D-063/D-076/D-080 + V13: `lisp-gpe.fwd-entry` is write-only with an existence probe; re-applied once per boot identity from a store taken from `Wiring`; never echo desired state from Retrieve
  - D-074: delete order adjacencies → mappings → EIDs → locator sets → EID-table maps; every delete checks existence first
files you own exclusively:
  - apps/agent/internal/descriptors/lisp/** and docs/agent/descriptors/lisp.md (gap-only)
  - apps/agent/internal/descriptors/core/coretest/lisp*.go (A6: new file only)
  - apps/agent/internal/desired/lisp*.go (builder + assembler), apps/agent/internal/subsystems/lisp*.go (`lisp.Register` with `df6.WithClaims(<Wiring.IfaceClaims()>)` + `df6.WithGlobalsOwner(env.GlobalsOwner)`), apps/agent/internal/agent/rpc_lisp*.go (`LispState`)
  - packages/schema/src/domains/ext/lisp*.ts, packages/schema/src/semantic/lisp*.ts (rule ids `tunnels.lisp-…`), packages/schema/examples/lisp-*.json, packages/proto/test/fixtures/lisp-*.json
  - apps/api/src/features/lisp/** (index.ts exports {controllers, providers}; `LispController`; `GET /api/v1/state/lisp`; real fake behaviour in fake.ts), apps/api/test/e2e/lisp*
  - apps/web/src/domains/vpn/lisp/**, apps/web/src/locales/{en,fa}/lisp.json
  - docs/user/vpn/lisp.md, test/topology/lisp/**, docs/status/tasks/F-lisp*
shared hotspots (append-only, conflicts resolved by the manager at merge):
  - protocol: docs/status/wave-A-hotspots.md §0. Insert only directly below your own anchor `// wave-BC: F-lisp` (manager's wave-B/C anchor pass; in tunnels.ts / `TunnelsConfig` it sits below F-tunnels' anchor); if missing, say so in the questions file and insert at the end of that anchor block. dataplane.proto: keep the one-blank-line framing. Never reorder or reformat other lines. List every hunk under "Shared hunks" in docs/status/tasks/F-lisp.md
  - A1 apps/agent/internal/subsystems/subsystems.go: the `Tunnels` domain const and `Domains["tunnels"]` entry — shared with F-tunnels: whoever lands first adds the key, the other appends — with the lisp descriptor names (package constants); one call into your subsystems/lisp.go at the end of `Register()`
  - A2 apps/agent/internal/agent/projection.go: one builder call in project(), one assembler call in assemble()
  - C1 packages/schema/src/domains/tunnels.ts: `TunnelsSchema.lisp` key line (sub-schema in ext/lisp.ts; the rest of tunnels.ts is F-tunnels') · C2 semantic/index.ts · C3 schema/src/index.ts · C4 new fixture files only (never all-domains.json / group-a-full.json / the tunnels-*.json files)
  - C5 packages/proto/vrx/v1/dataplane.proto: `TunnelsConfig` 10, the `LispState` rpc under the service anchor; new messages in `// ----- F-lisp -----`
  - C6 docs/contracts/proto.md (`### F-lisp: LispState`) · C7 generated, never hand-edited (apps/agent/gen/**, packages/proto/gen/ts/**, packages/api-client/src/generated/**, apps/cli/internal/api/operations_gen.go, docs/user/cli/reference.md)
  - P1 apps/api/src/app.module.ts · P4 apps/api/src/agent/agent.client.ts · P5 apps/api/src/testing/fake-agent.ts (UNIMPLEMENTED stub on the contract commit; real fake in features/lisp/fake.ts)
  - vpn page: apps/web/src/domains/vpn/tabs.ts (`vpnTabs`, W-seed shell — one `lisp` entry under your anchor, labelled "advanced" per the prompt) · W1 apps/web/src/router.tsx only if the tab needs a sub-route · W2 apps/web/src/nav/nav.ts + nav.test.ts (`'vpn'` into BUILT_DOMAINS only if P11/F-wireguard have not added it) · W3 apps/web/src/i18n.ts
  - A7 docs/vpp-code-track.md: `### V-new (F-lisp)` only for a new gap (V13/V14 exist); the manager numbers it
  - allocated numbers (docs/status/wave-BC-numbers.md, a merge blocker if reused): **TunnelsConfig 10 `lisp`** (4–7 are F-tunnels', 8–9 unallocated) — nothing else (no EventKind, no ActionRequest)
contract: commit `contract(schema): tunnels lisp` and `contract(proto): tunnels lisp, LispState` as separate commits on YOUR branch first + docs/status/tasks/F-lisp-contract.md; tell the manager in the questions file and keep building. Never a `contract/` branch. Config home is decided: `tunnels.lisp` (wave-BC-numbers.md) — say so, do not wait
files you must not touch:
  - everything else
  - never: /root/ngfw (main), other worktrees, /etc (incl. /etc/vpp), /root/vpp
  - apps/agent/binapi + tools/binapi-gen.sh, tools/lab, tools/ci.sh, plan/tasks.yaml, docs/decisions/LOG.md, docs/status/PROGRESS.md, packages/ui-kit/** (a need → questions file)
  - F-tunnels' files: descriptors/{gre,ipip,vxlan,vxlan_gpe,gtpu,l2tp,pppoe}/**, desired/tunnels*.go, subsystems/tunnels*.go, rpc_tunnels*.go, ext/tunnels*.ts, semantic/tunnels.ts (+ tunnels-common*.ts, read-only), tunnels.ts beyond your key line, apps/web/src/domains/vpn/tunnels/**
  - apps/agent/internal/descriptors/{df6,dfkit}/**, sr/** (F-srv6), every other descriptor package; apps/agent/internal/renderers/**
  - apps/web/src/domains/vpn/VpnPage.tsx and the other vpn tab directories; agent core (A5): apps/agent/internal/agent/{agent,service,state,ifstate}.go, apps/agent/cmd/**; apps/agent/internal/desired/interfaces.go (A3); apps/api/src/state/**
host facts:
  - `lisp_plugin.so` loaded (on disk; its unittest plugin is default-disabled); binapi `lisp`, `lisp_gpe`, `lisp_types` on main (`one` exists too — out of scope)
  - V13: the GPE fwd-entry path dump replies with the wrong message id (DF-6 doc numbers it V9 locally — use the code-track ids); V14: enable/disable leaks (see obligations)
  - no data NICs bound (D-026): af_packet rig only (D-010); no packet-level test required
coordination: (1) F-tunnels (parallel) shares the `tunnels` domain: `Domains["tunnels"]` (first lands adds the key), `TunnelsSchema` / `TunnelsConfig` anchors (yours below theirs), and keeps its rules in semantic/tunnels.ts while yours stay in semantic/lisp*.ts · (2) F-srv6 also adds a `vpnTabs` entry · (3) bridge domains for L2 EID tables are F-bridge-l2's — reference them by id only
V19/V24 SAFETY (D-095, D-101): before ANY packet through the rig (none is required), run TD-3's preflight on the rig interfaces' sw_if_index and never send packets through an unchecked interface; every af_packet delete with its veth down; one host test package at a time (D-087); `systemctl show vpp -p NRestarts` before/after every host run pasted — stop host runs and write it down if it rises
evidence: Playwright is not installed — use the headless Chrome approach from P07a/P07b/P08 (kept outside the product code) for screenshots (en + fa/RTL: LISP tab — Locators / EIDs / Mappings / Resolvers); say whether the screen ran against a fake-backed agent; paste `vppctl show lisp locator-set` / `show lisp eid-table` only from the opt-in window
time box: 15 h — when exceeded: stop, commit WIP, write docs/status/tasks/F-lisp.md with what is left
WIP: commit at least every 45 min; keep docs/status/tasks/F-lisp-wip.md current
CI: `TMPDIR=/tmp/g-w<SLOT> tools/ci.sh --base main` — short TMPDIR (unix socket paths ≤ 108 chars); no host-wide CI lock: golangci-lint serializes itself since main fc0fe68 (D-106 rejected serialising whole gates). Ports 3000/8080/9101 and /run/vrx/agent.sock belong to the running product stack (tools/app) — never touch them
finish: `tools/ci.sh --base main` green in the worktree · docs/status/tasks/F-lisp.md with pasted real output · everything committed · final message = 10-line summary (branch, last commit, CI result, evidence, open questions, decisions taken with options)
cleanup: stop every process you started (API/agent/vite) by PID · lab lock released · vrx_w<SLOT> dropped · after an opt-in host run: no w<SLOT> LISP objects left except the documented V14 leaks (Retrieve + show output pasted) · rig down · dist/ and apps/agent/bin removed
questions: docs/status/tasks/F-lisp-questions.md — write and keep going; never wait for a human
never: merge · restart/kill VPP · Docker · pkill · secrets in files · edit files you do not own
