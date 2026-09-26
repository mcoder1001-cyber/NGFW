# TASK ENVELOPE — F-srv6
id: F-srv6   branch: task/F-srv6   worktree: /root/ngfw-wt/F-srv6   base: main@<BASE>   started: <STARTED>
title: Wave C (day 13-15): SRv6 policies, network programming (local SIDs), steering — service-chaining proxies and SRv6-mobile not buildable/cut
prompt: prompts/features/F-srv6.md   (template: prompts/FEATURE-TEMPLATE.md; refreshed on task/prep-rest against main, task/P08 and task/W-seed)   wbs: D6.7
scope: the new `routing.srv6` model (contract: RoutingConfig 17), projection onto DF-6's merged `sr.*` descriptors (local SIDs END…END.DT4/DT6, policies with weighted SID lists, L2/L3 steering, encap source / hop limit globals on the globals owner only), `Srv6State` (local SIDs with counters, policies, steering), API, UI (an SRv6 tab on the VPN page), docs. NOT the End.AD/AM/AS proxies (no binary API in VPP 26.06 — see host facts), NOT SRv6-mobile (D-074/D-085), uSID or path tracing
merged deps you can rely on: P08, DF-6, W-seed (+ the wave-B/C anchor pass, docs/status/wave-BC-numbers.md "Pack rules")
  - DF-6: apps/agent/internal/descriptors/sr/ + docs/agent/descriptors/sr.md — `sr.localsid/<sid>`, `sr.policy/<bsid>`, `sr.steering/l2/<if>` | `sr.steering/<ipv4|ipv6>/<table>/<prefix>`, globals `sr.encap-source` / `sr.encap-hop-limit` (write-only; `sr.Register` builds setters only with `df6.WithGlobalsOwner(true)`, require variants otherwise); ownership only through ClaimStore records of our own Create; `sr.ErrPolicyInUse`, `df6.ErrNoSuchTable`
  - P08: desired/ + subsystems/ + projection patterns, `Wiring.IfaceClaims()` (= `df6.WithClaims` store), `Wiring.BootStore()`, `subsystems.SlotIDRange()` (W-seed), the vpn page shell + `vpnTabs` registry (W-seed)
  - also on main: TD-2, TD-3 (V19 sanitizer + preflight), TD-5 if merged
read first: prompts/features/F-srv6.md · docs/status/wave-BC-numbers.md (Pack rules, section F-srv6) · docs/status/wave-A-hotspots.md (§0 rules; ids A1 A2 A6 C1–C7 P1 P4 P5 W1–W3 A7) · docs/agent/descriptors/{sr,df6}.md · docs/status/tasks/DF-6.md + DF-6-questions.md (Q6) · docs/vpp-code-track.md V14, V15, V19, V22 · docs/decisions/LOG.md D-063, D-071, D-074, D-076, D-080, D-082, D-085, D-087, D-094, D-095, D-101
slot: 4 → VRX_SLOT=4 VRX_TEST_PREFIX=w4 VRX_HTTP_PORT=3000+100·4 VRX_WEB_PORT=5000+100·4 VRX_METRICS_PORT=9100+10·4+1 VRX_AGENT_SOCKET=/run/vrx-test/w4/agent.sock VRX_PG_DATABASE=vrx_w4 VRX_VALKEY_DB=4 VRX_VPP_TABLE_BASE=4000 VRX_LAB_LOCK=/run/lock/vrx-lab.lock
  - source of truth: `eval "$(tools/lab env 4)"`
  - rig prefix w4 → 10.4.{1,2}.0/24; SIDs, BSIDs and steered prefixes inside one slot-unique IPv6 /48 (e.g. fd00:<SLOT hex>::/48, the natcommon.Scope convention); VRFs/tables in 4000–4999
  - test agents run with VRX_GLOBALS_OWNER=0 (D-071)
  - slots 1–11 only; 12 is CI
daemon-owner: none
obligations:
  - D-104: use, do not rebuild — DF-6 `sr.*` descriptors are gap-only; `descriptors/df6/**` read-only
  - D-074: every encap policy carries its own source address (`encap_src`): the projection fills it from `routing.srv6.encapSource` (or a per-policy override if you model one) — encap without a source is a validation error; insert policies forbid it; every delete checks existence first
  - D-071/D-082: `sr.encap-source` / `sr.encap-hop-limit` are VPP-global → setters only on the globals owner; slot agents require them; a test that changes them runs only behind an opt-in (`VRX_FSRV6_GLOBALS=1`) under `flock -x /run/lock/vrx-globals.lock`, restoring the previous value (write-only: record it before the change)
  - D-063/D-076/D-080: the globals are write-only (applied once per boot identity, store from `Wiring`); local SIDs / policies / steering have dumps — Retrieve covers them, never echo desired state
  - V15/V22a: steering before policies before SIDs before the VRF on every delete path (the scheduler's dependency order; `sr.ErrPolicyInUse` guards a policy with steering); never a table flush; prove "no leaked routes" in `show ip6 fib table <t>`
  - End.AD/AM/AS proxies: not built (no binary API) — write `### V-new (F-srv6)` and list them in F-srv6.md as have-not; SRv6-mobile (binapi `sr_mobile`): not modelled (D-074), noted as a vpp-code-track candidate
files you own exclusively:
  - apps/agent/internal/descriptors/sr/** and docs/agent/descriptors/sr.md (gap-only)
  - apps/agent/internal/descriptors/core/coretest/srv6*.go (A6: new file only)
  - apps/agent/internal/desired/srv6*.go (builder + assembler), apps/agent/internal/subsystems/srv6*.go (`sr.Register` with `df6.WithClaims(<Wiring.IfaceClaims()>)` + `df6.WithGlobalsOwner(env.GlobalsOwner)`), apps/agent/internal/agent/rpc_srv6*.go (`Srv6State`)
  - packages/schema/src/domains/ext/srv6*.ts, packages/schema/src/semantic/srv6*.ts (rule ids `routing.srv6-…`), packages/schema/examples/srv6-*.json, packages/proto/test/fixtures/srv6-*.json
  - apps/api/src/features/srv6/** (index.ts exports {controllers, providers}; `Srv6Controller`; `GET /api/v1/state/srv6`; real fake behaviour in fake.ts), apps/api/test/e2e/srv6*
  - apps/web/src/domains/vpn/srv6/**, apps/web/src/locales/{en,fa}/srv6.json
  - docs/user/vpn/srv6.md, test/topology/srv6/**, docs/status/tasks/F-srv6*
shared hotspots (append-only, conflicts resolved by the manager at merge):
  - protocol: docs/status/wave-A-hotspots.md §0. Insert only directly below your own anchor `// wave-BC: F-srv6` (manager's wave-B/C anchor pass); if missing, say so in the questions file and insert at the end of that anchor block. dataplane.proto: keep the one-blank-line framing. Never reorder or reformat other lines. List every hunk under "Shared hunks" in docs/status/tasks/F-srv6.md
  - A1 apps/agent/internal/subsystems/subsystems.go: `sr.LocalSidName`, `sr.PolicyName`, `sr.SteeringName`, `sr.EncapSourceName`, `sr.EncapHopLimitName` into `Domains[Routing]`; one call into your subsystems/srv6.go at the end of `Register()`
  - A2 apps/agent/internal/agent/projection.go: one builder call in project(), one assembler call in assemble()
  - C1 packages/schema/src/domains/routing.ts: `RoutingSchema.srv6` key line (sub-schema in ext/srv6.ts) · C2 semantic/index.ts · C3 schema/src/index.ts · C4 new fixture files only
  - C5 packages/proto/vrx/v1/dataplane.proto: `RoutingConfig` 17, the `Srv6State` rpc under the service anchor; new messages in `// ----- F-srv6 -----`
  - C6 docs/contracts/proto.md (`### F-srv6: Srv6State`) · C7 generated, never hand-edited (apps/agent/gen/**, packages/proto/gen/ts/**, packages/api-client/src/generated/**, apps/cli/internal/api/operations_gen.go, docs/user/cli/reference.md)
  - P1 apps/api/src/app.module.ts · P4 apps/api/src/agent/agent.client.ts · P5 apps/api/src/testing/fake-agent.ts (UNIMPLEMENTED stub on the contract commit; real fake in features/srv6/fake.ts)
  - vpn page: apps/web/src/domains/vpn/tabs.ts (`vpnTabs`, W-seed shell — one `srv6` entry under your anchor; shared with P11, F-wireguard, F-ikev2-native, F-lisp, F-tunnels) · W1 apps/web/src/router.tsx only if the tab needs a sub-route · W2 apps/web/src/nav/nav.ts + nav.test.ts (`'vpn'` into BUILT_DOMAINS only if P11/F-wireguard have not added it) · W3 apps/web/src/i18n.ts
  - A7 docs/vpp-code-track.md: `### V-new (F-srv6)` (SRv6 proxies without binary API); the manager numbers it
  - allocated numbers (docs/status/wave-BC-numbers.md, a merge blocker if reused): **RoutingConfig 17 `srv6`** — nothing else (no EventKind, no ActionRequest)
contract: commit `contract(schema): routing srv6` and `contract(proto): routing srv6, Srv6State` as separate commits on YOUR branch first + docs/status/tasks/F-srv6-contract.md; tell the manager in the questions file and keep building. Never a `contract/` branch. Config home is decided: `routing.srv6` (wave-BC-numbers.md) — say so, do not wait
files you must not touch:
  - everything else
  - never: /root/ngfw (main), other worktrees, /etc (incl. /etc/vpp), /root/vpp
  - apps/agent/binapi + tools/binapi-gen.sh, tools/lab, tools/ci.sh, plan/tasks.yaml, docs/decisions/LOG.md, docs/status/PROGRESS.md, packages/ui-kit/** (a need → questions file)
  - apps/agent/internal/descriptors/{df6,dfkit}/** (shared helpers), sr_mpls/** (F-mpls-srmpls), lisp/** (F-lisp), every other descriptor package; apps/agent/internal/renderers/**
  - apps/web/src/domains/vpn/VpnPage.tsx and the other vpn tab directories (ipsec, wireguard, ikev2-native, lisp, tunnels); agent core (A5): apps/agent/internal/agent/{agent,service,state,ifstate}.go, apps/agent/cmd/**; apps/agent/internal/desired/interfaces.go (A3); packages/schema/src/domains/routing.ts beyond the anchored key line; apps/api/src/state/**
host facts:
  - SRv6 is VPP core (`sr`); binapi `sr` has `sr_localsid_add_del`, `sr_localsids_dump`, `sr_localsids_with_packet_stats_dump` (the counters), `sr_policy_add_v2` / `sr_policy_mod_v2` / `sr_policy_del`, `sr_policies_v2_dump`, `sr_steering_add_del`, `sr_steering_pol_dump`, `sr_set_encap_source`, `sr_set_encap_hop_limit` — check every name in apps/agent/binapi/sr/
  - source (VPP 26.06 src/plugins/srv6-{ad,am,as}): the proxy plugins are on disk and loaded but have **no `.api` file** — their behaviours exist only as CLI (`sr localsid … behavior end.ad …`) and are not in `sr_types.api`'s behaviour enum; no binapi can be generated, and CLI is forbidden (the only CLI use is D-090's fixed-string lb cleanup). The board note "D-074: manager generates srv6_ad/am/as binapi" cannot be done
  - no data NICs bound (D-026): af_packet rig only (D-010); a packet-level SRv6 test is not required
coordination: (1) F-lisp and F-tunnels also add VPN-page tabs — one `vpnTabs` line each under their anchors · (2) F-mpls-srmpls (parallel) owns SR-MPLS (`descriptors/sr_mpls`); both use `df6` read-only · (3) BGP/IS-IS SRv6 signalling is P12/F-isis-rip territory and out of scope
V19/V24 SAFETY (D-095, D-101): before ANY packet through the rig, run TD-3's preflight on the rig interfaces' sw_if_index and never send packets through an unchecked interface; every af_packet delete with its veth down; one host test package at a time (D-087); `systemctl show vpp -p NRestarts` before/after every host run pasted — stop host runs and write it down if it rises
evidence: Playwright is not installed — use the headless Chrome approach from P07a/P07b/P08 (kept outside the product code) for screenshots (en + fa/RTL: SRv6 tab — Local SIDs / Policies / Steering, SID-list editor, counters column); paste `vppctl show sr localsids`, `show sr policies`, `show sr steering-policies`, `show ip6 fib table <t>`
time box: 15 h — when exceeded: stop, commit WIP, write docs/status/tasks/F-srv6.md with what is left
WIP: commit at least every 45 min; keep docs/status/tasks/F-srv6-wip.md current
CI: `TMPDIR=/tmp/g-w4 tools/ci.sh --base main` — short TMPDIR (unix socket paths ≤ 108 chars); no host-wide CI lock: golangci-lint serializes itself since main fc0fe68 (D-106 rejected serialising whole gates). Ports 3000/8080/9101 and /run/vrx/agent.sock belong to the running product stack (tools/app) — never touch them
finish: `tools/ci.sh --base main` green in the worktree · docs/status/tasks/F-srv6.md with pasted real output · everything committed · final message = 10-line summary (branch, last commit, CI result, evidence, open questions, decisions taken with options)
cleanup: stop every process you started (API/agent/vite) by PID · lab lock released · vrx_w4 dropped · no w4 SIDs/policies/steering/tables left (Retrieve + `show sr localsids` + `show ip6 fib table <t>` pasted) · globals restored to the recorded previous values (if you had the opt-in) · rig down · dist/ and apps/agent/bin removed
questions: docs/status/tasks/F-srv6-questions.md — write and keep going; never wait for a human
never: merge · restart/kill VPP · Docker · pkill · secrets in files · edit files you do not own

## Manager addendum (2026-09-25 10:30, ngfw-46) — rules that landed after this envelope was written
- **TD-11b (on main):** every descriptor you register declares `RecordsNoOwnership()` (finds its objects by key/tag) or `CheckPersistent() error` (records claims over the persisted Wiring store); the agent refuses to start otherwise. Creates are claim-first (dfkit ClaimFirst).
- **TD-11c (approved, merging):** every interface creator registers `iface.RegisterKind` or provides the `interface/<name>` KeyProvider and removes itself from the guard allowlist (mpls-tunnel → F-mpls-srmpls; gre/ipip/vxlan… → F-tunnels).
- **TD-8 seams (on main):** use Env.Publish / RequestResync / dynamic desired sources / metrics collector / Wiring.IDRange — never fork them; the agent refuses to start without an ID range (TD-8b).
- **TD-23 (approved, merging):** fake-agent actions via `registerActionHandler(kind, fn)`; coretest hooks via `RegisterExtension(name, fn)` (and `RegisterFeatureIsEnabled` for feature_is_enabled) — until it is on main, keep your hook in your own file so it becomes one registration line.
- **D-132:** no UI timer < 30 s on anything that walks VPP; Refresh button; the agent serialises full walks (one in flight, UNAVAILABLE after 3 s).
- **D-137/D-139:** never call VPP dns.api; **D-128:** never `show trace`/`trace add`/`clear trace`; **D-126:** no classify sweeps by index.
- **WEB-1 (approved, merging):** SchemaForm presence toggle — never call `dropPhantomOptionals`.
- **Host runs:** af_packet creates on the shared VPP fail until TD-25 merges (approved, first in the merge queue). Do code, unit tests, fake-VPP tests and CI first; the manager tells you when host runs are open. VPP NRestarts is 2 (baseline).
- **Usage limit:** if you are stopped by it, the manager resumes you; commit early and often.
