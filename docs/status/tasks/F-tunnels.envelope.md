# TASK ENVELOPE — F-tunnels
id: F-tunnels   branch: task/F-tunnels   worktree: /root/ngfw-wt/F-tunnels   base: main@<BASE>   started: <STARTED>
title: Wave B (day 10-12): GRE, IPIP/6RD, VXLAN(-GPE), GTP-U, L2TPv3, PPPoE
prompt: prompts/features/F-tunnels.md   (template: prompts/FEATURE-TEMPLATE.md; refreshed on task/prep-rest against main + P08)   wbs: D6.6
scope: the `tunnels` domain end to end on top of the merged DF-6 descriptors: builder + assembler per kind (gre, ipip + 6RD, vxlan, vxlan-gpe first; gtpu, l2tpv3, pppoe last), `interface/<name>` aliases, tunnel addresses/MTU/VRF, L2 tunnels into F-bridge-l2's bridge domains, the four new schema kinds, a TunnelState RPC, API module, Tunnels page, docs. No second tunnel descriptor, no bridge-domain objects.
merged deps you can rely on: P08, DF-6, F-bridge-l2
  - P08: desired/ (Sink, Ptr, `interface/<name>` aliases, Assemble), subsystems.Register/Domains + Wiring.IfaceClaims()/BootStore(), projection.go, the InterfaceState state-RPC pattern (tunnels already appear in `/state/interfaces`), core `interface-ip` / `interface.mtu` / `interface-ip.table` descriptors
  - DF-6: descriptors/{gre,ipip,vxlan,vxlan_gpe,gtpu,l2tp,pppoe} + shared df6 (claims = iface.ClaimStore, keyed claims, bypass toggles, write-only singletons, dump-first guards); TD-3's sanitizer runs in df6/ifdesc.go
  - F-bridge-l2: the L2 container + bridge-domain builder (numeric BD ids, the ids `tunnels.*.bridgeDomain` already uses) on DF-1's l2.bridge-domain / bridge-domain-member
  - also on main: W-seed (anchors/seams), TD-3 (V19 sanitizer + cmd/vrx-vpp-preflight), TD-2
read first: prompts/features/F-tunnels.md · docs/status/wave-A-hotspots.md (§0 rules, A1 A2 A6 A7 C2–C7 P1 P4 P5 W1–W3) · docs/status/wave-BC-numbers.md (section F-tunnels) · docs/agent/descriptors/df6.md (first), gre.md, ipip.md, vxlan.md, vxlan_gpe.md, gtpu.md, l2tp.md, pppoe.md · docs/status/tasks/DF-6.md + DF-6-questions.md (Q1, Q5, Q10) · docs/status/tasks/F-bridge-l2.md · docs/status/vertical-slice.md · docs/vpp-code-track.md V8, V14, V19, V21 · docs/decisions/LOG.md D-063, D-064, D-065, D-069, D-071, D-074, D-076, D-080, D-082, D-095, D-101, D-113
slot: <SLOT> → VRX_SLOT=<SLOT> VRX_TEST_PREFIX=w<SLOT> VRX_HTTP_PORT=3000+100·<SLOT> VRX_WEB_PORT=5000+100·<SLOT> VRX_METRICS_PORT=9100+10·<SLOT>+1 VRX_AGENT_SOCKET=/run/vrx-test/w<SLOT>/agent.sock VRX_PG_DATABASE=vrx_w<SLOT> VRX_VALKEY_DB=<SLOT> VRX_VPP_TABLE_BASE=<SLOT>000 VRX_LAB_LOCK=/run/lock/vrx-lab.lock
  - source of truth: `eval "$(tools/lab env <SLOT>)"`
  - rig prefix w<SLOT> → 10.<SLOT>.{1,2}.0/24; tunnel src/dst on w<SLOT> loopbacks inside 10.<SLOT>.0.0/16 (IPv6 only inside fd00:<SLOT hex>::/32)
  - tunnel `instance` numbers, VNIs, GTP-U TEIDs, PPPoE session ids and L2TPv3 session ids from <SLOT>000–<SLOT>999 (VPP-wide interface names gre<n>/ipip<n>/vxlan_tunnel<n>); VRFs/tables and bridge-domain ids in <SLOT>000–<SLOT>999
  - test agents run with VRX_GLOBALS_OWNER=0 (D-071)
  - slots 1–11 only; 12 is CI
daemon-owner: none
obligations:
  - D-104: use, do not rebuild — the DF-6 descriptors and df6 helpers; edit a DF-6 package only for a proven defect (name the test)
  - D-074: every delete path checks existence first (V8: gtpu crashes VPP on a failed add/delete — keep DF-6's dump-first guard; never send a duplicate add)
  - D-071 globals: `l2tp.lookup-key` and `pppoe.cp` are VPP-wide → only the globals owner sets them; slot agents require or skip
  - D-063/D-076/D-080: write-only kinds (`ipip.sixrd`, bypass toggles, singletons) re-apply once per VPP boot identity from the persisted BootStore / IfaceClaims (Wiring), never an in-memory store; Retrieve never echoes desired state
  - V21: vxlan bypass = disable before enable (DF-6 does it; keep it)
  - V14: L2TPv3 has no delete → create only behind `VRX_DF6_L2TP_CREATE=1`-style opt-in, rollback documented as the V14 exception
  - D-065/D-069: every tunnel is also `interface/<name>` (logical name) so addresses, MTU, VRF, VRRP/LLDP/IPFIX refs resolve through the alias
  - L2 membership: `tunnels.<kind>.<name>.bridgeDomain` → one DF-1 `l2.bridge-domain-member` object per tunnel interface (depends on F-bridge-l2's `l2.bridge-domain/<id>`); a semantic rule refuses the same tunnel also being a member through F-bridge-l2's per-interface leaf (never two programmers)
  - GTP-U forwarding entries (VPP-wide per type, DF-6 review M2): default drop from the model — write the question
files you own exclusively:
  - gap-only (DF-6 built them): apps/agent/internal/descriptors/{gre,ipip,vxlan,vxlan_gpe,gtpu,l2tp,pppoe}/**, docs/agent/descriptors/{gre,ipip,vxlan,vxlan_gpe,gtpu,l2tp,pppoe}.md
  - apps/agent/internal/desired/tunnels*.go (builders + assembler), apps/agent/internal/subsystems/tunnels*.go (Register options, Wiring helpers), apps/agent/internal/agent/rpc_tunnels*.go (`TunnelState`)
  - apps/agent/internal/descriptors/core/coretest/tunnels*.go (A6: new file)
  - packages/schema/src/domains/tunnels.test.ts, packages/schema/src/domains/ext/tunnels*.ts (new kinds' sub-schemas), packages/schema/src/semantic/tunnels.ts + tunnels.test.ts (rule ids `tunnels.…`; F-lisp keeps its rules in its own `semantic/lisp*.ts`)
  - packages/schema/examples/tunnels-*.json, packages/proto/test/fixtures/tunnels-*.json
  - apps/api/src/features/tunnels/** (index.ts exports {controllers, providers}; `TunnelsController`; GET /state/tunnels; real fake behaviour in fake.ts), apps/api/test/e2e/tunnels*
  - apps/web/src/domains/vpn/tunnels/** (route /vpn/tunnels — `tunnels` is its own root key in the vpn nav group, not a VpnPage tab), apps/web/src/locales/{en,fa}/tunnels.json
  - docs/user/vpn/tunnels.md, test/topology/tunnels/**, docs/status/tasks/F-tunnels*
shared hotspots (append-only, conflicts resolved by the manager at merge):
  - protocol: docs/status/wave-A-hotspots.md §0. Insert only directly below your own anchor `// wave-BC: F-tunnels` if the manager seeded one, else at the end of the block. Never reorder or reformat other lines. List every hunk under "Shared hunks" in docs/status/tasks/F-tunnels.md
  - A1 apps/agent/internal/subsystems/subsystems.go: the new `Domains["tunnels"]` key — shared with F-lisp (wave-BC-numbers.md): whoever lands first adds the key, the other appends — + one Register line calling your subsystems/tunnels.go (df6.WithClaims(<Wiring.IfaceClaims()>) and the BootStore per DF-6 family)
  - C1 packages/schema/src/domains/tunnels.ts (P02c-created; you are its main toucher): in-place edits of the existing gre/vxlan/ipip schemas (6RD under ipip, `TUNNEL_KINDS`, the header comment that still says "Only P02c edits this file") are yours alone; new keys go under `// wave-BC: F-tunnels` in `TunnelsSchema` — F-lisp adds `lisp` under its own anchor below yours, never touch it
  - A2 apps/agent/internal/agent/projection.go: one call in project(), one in assemble() (after desired.Assemble); replaces the `tunnels` unsupported-field warning
  - A7 docs/vpp-code-track.md: `### V-new (F-tunnels)` only for a new gap; the manager numbers it
  - C2 packages/schema/src/semantic/index.ts: only if you add a new rule file (tunnels.ts is already spread) · C3 packages/schema/src/index.ts: new exports only
  - C4 packages/schema/examples/ + packages/proto/test/fixtures/: your `tunnels-*.json` (the existing tunnels-gre-vxlan-ipip.json / tunnels-semantic-source-not-configured.json are yours to extend) plus new files; all-domains.json, group-a-full.json and invalid-tunnels-vxlan-vni.json are read-only
  - C5 packages/proto/vrx/v1/dataplane.proto: rpc `TunnelState` under the service anchor; the new kind messages (VxlanGpeTunnel, GtpuTunnel, L2tpv3Tunnel, PppoeSession, IpipSixrd, TunnelState*) in a `// ----- F-tunnels -----` section at the end
  - allocated numbers (docs/status/wave-BC-numbers.md, a merge blocker if reused): **TunnelsConfig 4 `vxlan_gpe`, 5 `gtpu`, 6 `l2tpv3`, 7 `pppoe`** (8–9 unallocated, 10 = F-lisp — the `TunnelsConfig` block is an anchor site shared with F-lisp), **IpipTunnel 14 `sixrd`**; GreTunnel 14 and VxlanTunnel 17 only for a proven gap; no ActionRequest, no EventKind
  - C6 docs/contracts/proto.md: `### F-tunnels: TunnelState` · C7 generated, regenerated and never hand-edited: apps/agent/gen/**, packages/proto/gen/ts/**, packages/api-client/src/generated/**, apps/cli/internal/api/operations_gen.go, docs/user/cli/reference.md (`packages/proto/gen.sh`, `pnpm gen`, `make -C apps/cli gen docs`)
  - P1 apps/api/src/app.module.ts · P4 apps/api/src/agent/agent.client.ts (`tunnelState`) · P5 apps/api/src/testing/fake-agent.ts (UNIMPLEMENTED stub under the anchor on the contract commit)
  - W1 apps/web/src/router.tsx (/vpn/tunnels) · W2 apps/web/src/nav/nav.ts + nav.test.ts (`tunnels` into BUILT_DOMAINS) · W3 apps/web/src/i18n.ts
  - D1 docs/user/interfaces/basics.md: one see-also line at the end
contract: commit `contract(schema): tunnels vxlan-gpe/gtpu/l2tpv3/pppoe/6rd` and `contract(proto): tunnel kinds + TunnelState` as separate commits on YOUR branch first (the ci.sh contract guard checks the subject), plus docs/status/tasks/F-tunnels-contract.md; `TUNNEL_KINDS` gets the new VPP name prefixes. Tell the manager in the questions file and keep building. No own branches (the prompt's old `contract/F-tunnels` wording is void)
files you must not touch:
  - everything else
  - never: /root/ngfw (main), other worktrees, /etc (incl. /etc/vpp), /root/vpp
  - apps/agent/binapi (P04/manager-owned), tools/lab, tools/ci.sh, plan/tasks.yaml, docs/decisions/LOG.md, docs/status/PROGRESS.md, packages/ui-kit/**
  - apps/agent/internal/descriptors/df6/** (shared helpers — read-only → questions file), descriptors/{l2,l3xc,mactime}/** and desired/l2*.go (F-bridge-l2), descriptors/{interface,core,dfkit}/**, descriptors/{ipsec,vpn,wireguard,sr,sr_mpls,lisp,mpls,span}/** (P11/F-wireguard/F-srv6/F-lisp/F-mpls-srmpls/F-loopback)
  - packages/schema/src/semantic/tunnels-common*.ts (shared by the vpn/services/ha rules — read-only)
  - agent core (A5): apps/agent/internal/agent/{agent,service,state,ifstate}.go, apps/agent/cmd/**; apps/agent/internal/desired/interfaces.go (A3); apps/agent/internal/vpp/ifsanitize/** (TD-3); apps/api/src/state/** (P08)
host rules:
  - VPP SAFETY (D-064): `systemctl show vpp -p NRestarts` before and after every host run (pasted); on a rise, disable the test behind an opt-in env var at once and record the message (A7)
  - the ONE host integration check covers gre + ipip + vxlan + vxlan-gpe only; gtpu stays opt-in (`VRX_DF6_GTPU_HOST=1` pattern, V8), l2tp create opt-in (V14), pppoe host steps opt-in (needs a learnt client MAC) — fakes cover them
  - V19 SAFETY (D-095): before ANY packet through the rig or a tunnel, `go -C apps/agent run ./cmd/vrx-vpp-preflight` must exit 0 (TD-3). Stop if it does not
  - delete order: bridge membership, addresses and routes before the tunnel; tunnels before the loopbacks/tables carrying src/dst (D-095c, V15)
  - D-101: bring the veth down before any af_packet delete (TD-5 may not be merged); af_packet rings per D-113
  - with VRX_INTEGRATION=1, one Go package at a time; hold `flock -s` on the lab lock only during a run (D-094)
coordination: F-bridge-l2 owns bridge domains (you only emit members for tunnel interfaces) · P11 (may be running) names `tunnels.ipip.<name>` in `routeBased.ipipInterface` — the ipip projection is yours; GRE-over-IPsec is a docs pointer only · F-loopback-bvi-gso-lldp-span owns ERSPAN mirror sessions (you create the ERSPAN GRE tunnel) · F-srv6, F-lisp, F-mpls-srmpls own their DF-6 families; F-lisp (may run in parallel) shares `Domains["tunnels"]`, `TunnelsSchema` and `TunnelsConfig` with you through anchors
evidence: Playwright is not installed. Take the screenshots (en + fa/RTL: Tunnels page per kind, advanced toggle) with the headless Chrome approach from P07a/P07b/P08 (`test/topology/interfaces/shots_test.go`, kept outside the product code), and say so
time box: 15 h — when exceeded: stop, commit WIP, write docs/status/tasks/F-tunnels.md with what is left (gtpu/l2tp/pppoe are the first to drop)
WIP: commit at least every 45 min; keep docs/status/tasks/F-tunnels-wip.md current
CI: `TMPDIR=/tmp/g-w<SLOT> tools/ci.sh --base main` — short TMPDIR (unix socket paths ≤ 108 chars); no host-wide CI lock: golangci-lint serializes itself since main fc0fe68 (D-106 rejected serialising whole gates). Ports 3000/8080/9101 and /run/vrx/agent.sock belong to the running product stack (tools/app) — never touch them
finish: `tools/ci.sh --base main` green in the worktree · docs/status/tasks/F-tunnels.md with pasted real output · everything committed · final message = 10-line summary (branch, last commit, CI result, evidence, open questions, decisions taken with options)
cleanup: stop every process you started (API/agent/vite), by PID · lab lock released · vrx_w<SLOT> dropped · no w<SLOT> tunnels, bypass toggles, addresses or tables left (dumps pasted; L2TPv3 leftovers listed as V14) · rig down · dist/ and apps/agent/bin removed
questions: docs/status/tasks/F-tunnels-questions.md — write and keep going; never wait for a human
never: merge · restart/kill VPP · Docker · pkill · secrets in files · edit files you do not own
