# TASK ENVELOPE — F-rpf-adl-pbr
id: F-rpf-adl-pbr   branch: task/F-rpf-adl-pbr   worktree: /root/ngfw-wt/F-rpf-adl-pbr   base: main@task/W-seed@8b7558e (SPECULATIVE, D-114/D-120: P08 fix round 2 still running — do NOT merge main until the manager tells you P08 has landed)   started: 2026-09-24T17:27
title: Wave A (day 7-9): uRPF strict/loose, ADL, ABF policy-based routing
prompt: prompts/features/F-rpf-adl-pbr.md   (template: prompts/FEATURE-TEMPLATE.md; checked against main + P08 + TD-3 in wave-A prep)   wbs: D2.4, D2.7 (ABF part), D3.9
scope: glue on top of DF-2/DF-4/TD-3 + P08: projection of `urpf.*`, `adl.*` (allow-list write-only) and `abf.*` (policies with ACLs by name + attachments), a new `auto_sdl` global descriptor (globals owner only), the V23 (a) fix in `adl.interface` Retrieve, `/state/pbr`, PBR + ADL/Auto-SDL screens, per-interface uRPF/ADL fields, docs. Classify tables and ip-session-redirect get NO schema/API/UI (prompt fence).
merged deps you can rely on: P08, DF-2, DF-4, TD-3
  - P08: desired/ (Sink, `interface/<name>` aliases), subsystems.Register/Domains, Wiring.KeyedClaims("acl") (= the persisted df2 claim store), Wiring.ClassifyStore(), Wiring.BootStore(), Env.GlobalsOwner, projection.go, InterfaceState state-RPC pattern, interface drawer = SchemaForm over `interfaces.<if>` grouped by `withUi` group
  - DF-2: descriptors/urpf (`urpf.Register`), descriptors/adl (`adl.Register` = adl.interface, `adl.RegisterWriteOnly` = adl.allowlist), descriptors/abf (`abf.Register(r, c, owner, ids *df2.IDRange, …)`, `NormalizePolicy`), descriptors/{classify,ip_session_redirect} (not projected here), helper descriptors/df2; docs/agent/descriptors/{urpf,adl,abf,classify,ip_session_redirect}.md
  - DF-4: ACL key `acl.acl/<name>` (`acl.KeyACL`), `acl.LookupIndex`, D-066 naming, ACL stats (hit counters) if enabled
  - TD-3: apps/agent/internal/vpp/ifsanitize (Acquire on interface create, BeforeDelete), cmd/vrx-vpp-preflight, classify unbind-before-delete; it owned descriptors/{classify,adl} until its merge (D-104)
  - also on main: TD-2 (API auth/users follow-ups)
read first: prompts/features/F-rpf-adl-pbr.md · docs/status/wave-A-hotspots.md (§0 rules, §2 numbers, A1 A2 C1–C7 P1 P4 P5 W1–W3 W5) · docs/agent/descriptors/{urpf,adl,abf,classify}.md · docs/status/tasks/TD-3.md (V23 section) · docs/vpp-code-track.md V19, V23 · docs/status/vertical-slice.md · docs/decisions/LOG.md D-063, D-064, D-065, D-066, D-071, D-076, D-080, D-082, D-094, D-095, D-101, D-104
slot: 10 → VRX_SLOT=10 VRX_TEST_PREFIX=w10 VRX_HTTP_PORT=3000+100·10 VRX_WEB_PORT=5000+100·10 VRX_METRICS_PORT=9100+10·10+1 VRX_AGENT_SOCKET=/run/vrx-test/w10/agent.sock VRX_PG_DATABASE=vrx_w10 VRX_VALKEY_DB=10 VRX_VPP_TABLE_BASE=10000 VRX_LAB_LOCK=/run/lock/vrx-lab.lock
  - source of truth: `eval "$(tools/lab env 10)"`
  - rig prefix w10 → 10.10.{1,2}.0/24; uRPF/ADL/ABF objects only on w10-prefixed interfaces; ABF policy ids and path/ADL tables in 10000–10999; ACLs tagged `w10:<name>` (DF-4 / `df2test.ACL` pattern)
  - test agents run with VRX_GLOBALS_OWNER=0 (D-071)
  - slots 1–11 only; 12 is CI
daemon-owner: none
obligations:
  - D-104: use, do not rebuild — DF-2's urpf/adl/abf descriptors and DF-4's ACL lookup as they are; only `descriptors/auto_sdl` is new; `descriptors/df2`, `descriptors/acl` and `internal/vpp/ifsanitize` are read-only
  - V23 (a) (TD-3 open question, assigned here): `feature_is_enabled` answers true for every VPP error and `adl.interface` Retrieve trusts it → confirm a "true" with a control query that cannot be on, or report unknown; fake + host test; update the V23 row (A7-style append) with what you did. `classify.output-acl` has the same flaw but is not projected here → note it in the questions file, do not fix it
  - persisted claims only: urpf/adl/abf get `df2.WithClaims(<Wiring.KeyedClaims("acl")>)` (the store F-acl also uses); no in-memory default in the product agent (D-080)
  - ID range for `abf.Register`: `subsystems.SlotIDRange()` if the manager's anchor commit provides it; otherwise a private, slug-named helper (`rpfAdlPbrIDRange`, so it never collides with F-neighbors-ra's) in your owned `subsystems/rpf_adl_pbr.go` reading `VRX_VPP_TABLE_BASE` (set → [base, base+999]; unset → nil = all ids, the product agent). Never edit agent.go (A5)
  - D-063/D-076/D-080: `adl.allowlist` only through `adl.RegisterWriteOnly`; its Create must be idempotent or keep an applied-once record keyed by the boot identity in `Wiring.BootStore()`; unit fakes model duplicate-add; the restart simulation shows it re-applied exactly once; counted in `vrx_agent_retrieve_unsupported_*`
  - D-071/D-082: `auto_sdl_config{enable, threshold (default 5), remove_timeout (default 300)}` is a VPP global with no getter — registered only when `Env.GlobalsOwner`; slot agents warn and skip; a host test only behind your own opt-in env var with `flock -x /run/lock/vrx-globals.lock`, and it cannot restore a previous value it cannot read → skip-unless-supported, run only in a manager VPP window, say so. If it needs the session layer / SDL backend that vrx-a does not enable (startup.conf = handover-gated), keep it fake-tested and write it in the questions file — never enable the session layer
  - services domain: if you merge before F-loopback-bvi-gso-lldp-span you are the first to add `Domains["services"]` (its envelope claims the same) — add the key under your anchor and emit agent.unsupported-field for every other non-empty services.* leaf
  - D-095c + TD-3: delete order in tests, rollback and restart simulation — ABF attachments before policies, uRPF/ADL before the interface, ACLs last; V23 (b): ABF attachments are not sanitized on interface delete (no crash path known) — leave it
  - UI: the P08 drawer already renders new `interfaces.<if>` leaves through SchemaForm by `withUi` group (group name per wave-A-hotspots C1) — no drawer rewrite; absent/default must mean "off" (drawer saves write defaults back; P08's `dropPhantomOptionals` drops only unchanged optional objects)
files you own exclusively:
  - apps/agent/internal/descriptors/auto_sdl/**, docs/agent/descriptors/auto_sdl.md (new, based on descriptors/dfkit, D-077)
  - apps/agent/internal/desired/rpf_adl_pbr*.go (builder + assembler; `desired/interfaces.go` stays P08's)
  - apps/agent/internal/subsystems/rpf_adl_pbr*.go (registration helper, ID range), apps/agent/internal/agent/rpc_rpf_adl_pbr*.go (`PbrState`, only if `/state/pbr` needs live counters)
  - apps/agent/internal/descriptors/core/coretest/rpf_adl_pbr*.go (A6: new file; existing coretest files are read-only)
  - apps/api/src/features/rpf-adl-pbr/** (index.ts exports {controllers, providers}; `RpfAdlPbrController`; `GET /state/pbr`; real fake behaviour in fake.ts), apps/api/test/e2e/rpf-adl-pbr*
  - apps/web/src/domains/routing/rpf-adl-pbr/**, apps/web/src/locales/{en,fa}/rpf-adl-pbr.json
  - packages/schema/src/domains/ext/rpf-adl-pbr*.ts, packages/schema/src/semantic/rpf-adl-pbr*.ts (rule ids `interfaces.rpf-adl-pbr-…`, `routing.rpf-adl-pbr-…`, `services.rpf-adl-pbr-…`), packages/schema/examples/rpf-adl-pbr-*.json, packages/proto/test/fixtures/rpf-adl-pbr-*.json
  - docs/user/routing/rpf-adl-pbr.md, test/topology/rpf-adl-pbr/**, docs/status/tasks/F-rpf-adl-pbr*
  - gap-only (edit only for a proven defect or a missing helper, name the test; V23 (a) in adl is expected): apps/agent/internal/descriptors/{urpf,adl,abf,classify,ip_session_redirect}/**, docs/agent/descriptors/{urpf,adl,abf,classify,ip_session_redirect}.md — keep TD-3's sanitizer contract intact
shared hotspots (append-only, conflicts resolved by the manager at merge):
  - protocol: docs/status/wave-A-hotspots.md §0. Insert only directly below your own anchor `// wave-A: F-rpf-adl-pbr`. Never reorder or reformat other lines. List every hunk under "Shared hunks" in docs/status/tasks/F-rpf-adl-pbr.md
  - A1 apps/agent/internal/subsystems/subsystems.go: urpf/adl/abf (+ auto_sdl for the globals owner) registrations; descriptor names in `Domains` where each config leaf lives (urpf/adl → Interfaces, abf → Routing, auto_sdl → Services; one domain per descriptor)
  - A2 apps/agent/internal/agent/projection.go: one call in project(), one in assemble() (after `desired.Assemble`)
  - A4 apps/agent/internal/agent/server.go: nothing expected (the `PbrState` method lives in your rpc file)
  - C1 packages/schema/src/domains/{interfaces,routing,services}.ts: one key line each (`urpf`, `adl`; `pbr`; `autoSdl` with `removeTimeoutSec`), sub-schemas in your ext file · C2 packages/schema/src/semantic/index.ts: one spread line · C3 packages/schema/src/index.ts if you export anything · C4 new fixture files only
  - C5 packages/proto/vrx/v1/dataplane.proto: `PbrState` (if any) under the service anchor; new messages in a `// ----- F-rpf-adl-pbr -----` section at the end
  - allocated numbers (wave-A-hotspots §2, a merge blocker if reused): **Interface 18 `urpf`, 19 `adl`** · Subinterface 16–17 only if you mirror the leaves on sub-interfaces · **RoutingConfig 11 `pbr`** · **ServicesConfig 8 `auto_sdl`**; nothing else
  - C6 docs/contracts/proto.md: `### F-rpf-adl-pbr: PbrState` (if added) · C7 generated, regenerated and never hand-edited: apps/agent/gen/**, packages/proto/gen/ts/**, packages/api-client/src/generated/**, apps/cli/internal/api/operations_gen.go, docs/user/cli/reference.md (`packages/proto/gen.sh`, `pnpm gen`, `make -C apps/cli gen docs`)
  - P1 apps/api/src/app.module.ts · P4 apps/api/src/agent/agent.client.ts (`pbrState`, if added) · P5 apps/api/src/testing/fake-agent.ts (UNIMPLEMENTED stub under the anchor on the contract commit, if you add an RPC)
  - W1 apps/web/src/router.tsx · W2 apps/web/src/nav/nav.ts + nav.test.ts ("Policy routing" and "ADL / Auto-SDL" NavItems, labelKeys in your namespace) · W3 apps/web/src/i18n.ts · W5 apps/web/src/domains/interfaces/{InterfacesPage,InterfaceDrawer}.tsx: one line in the manager's registry, only if uRPF/ADL need more than their SchemaForm group
  - docs/vpp-code-track.md: append to the V23 row / `### V-new (F-rpf-adl-pbr)` for anything new; the manager numbers it
contract: commit `contract(schema): urpf, adl, pbr, autoSdl` and (if needed) `contract(proto): PbrState` as separate commits on YOUR branch first (the ci.sh contract guard checks the subject), plus docs/status/tasks/F-rpf-adl-pbr-contract.md; tell the manager in the questions file and keep building. No own branches (P08 pattern)
files you must not touch:
  - everything else
  - never: /root/ngfw (main), other worktrees, /etc (incl. /etc/vpp), /root/vpp
  - apps/agent/binapi + tools/binapi-gen.sh (P04/manager-owned), tools/lab, tools/ci.sh, plan/tasks.yaml, docs/decisions/LOG.md, docs/status/PROGRESS.md, packages/ui-kit/** (a need → questions file)
  - agent core (A5): apps/agent/internal/agent/{agent,service,ifstate}.go, apps/agent/cmd/**; apps/agent/internal/desired/interfaces.go (A3); apps/agent/internal/vpp/ifsanitize/** and descriptors/{interface,policer,ipfix}/** (TD-3 results); apps/agent/internal/descriptors/{df2,acl}/** (read-only)
  - apps/api/src/actions/** (F-vrf-static-ecmp), apps/api/src/state/** (P08), apps/api/src/{auth,users}/** (TD-4)
  - every other descriptor package (core/svs, ip_neighbor/ip6_nd/arp, nat*, bond/l2/l3xc/mactime/lldp/span/gso/nsim, af_packet = TD-5); apps/web/src/domains/firewall/acl/** (F-acl); apps/web/src/locales/*/{nav,common,interfaces}.json (W4)
host rules:
  - plugins `urpf`, `adl`, `abf`, `acl` are loaded on vrx-a (confirm once with `vppctl show plugins`); `auto_sdl` host test opt-in only (see obligations); no data NICs needed; a packet test is not required — if you send one it runs on the af_packet rig (path af_packet, D-010)
  - V19 SAFETY (D-095, crash on first packet): interfaces you create go through TD-3's sanitizer (DF-1/core Create does it); before ANY packet, `go -C apps/agent run ./cmd/vrx-vpp-preflight` must exit 0. Stop if it does not
  - run `systemctl show vpp -p NRestarts` before and after every host run; stop and write the questions file if it rises (D-064)
  - D-101: bring the veth down before any af_packet delete (TD-5 may not be merged)
  - with VRX_INTEGRATION=1, one Go package at a time; hold `flock -s` on the lab lock only during a run (D-094)
coordination: F-acl runs in parallel on the same `KeyedClaims("acl")` store; ABF resolves ACLs by name and F-acl's D5.5 only links to your ADL page · F-neighbors-ra appends to `Interface` (15–17) and `RoutingConfig` (10) and needs the same slot ID range · F-loopback-bvi-gso-lldp-span (a follow-on after F-bridge-l2) also registers the `services` domain; its envelope now proposes Interface 20/21 and ServicesConfig 9, so there is no number collision with yours · F-kea-dhcp-relay / F-unbound-chrony-syslog (wave B) also append to `Domains["services"]` · F-bonding / F-bridge-l2 / F-neighbors-ra also add interface-drawer fields
evidence: Playwright is not installed. Take the screenshots (en + fa/RTL: PBR policy list + editor, attachments, uRPF/ADL fields in the interface drawer, ADL/Auto-SDL page) with the headless Chrome approach from P07a/P07b/P08 (`test/topology/interfaces/shots_test.go`, kept outside the product code), and say so.
time box: 15 h — when exceeded: stop, commit WIP, write docs/status/tasks/F-rpf-adl-pbr.md with what is left
WIP: commit at least every 45 min; keep docs/status/tasks/F-rpf-adl-pbr-wip.md current
CI: `TMPDIR=/tmp/g-w10 tools/ci.sh --base main` — short TMPDIR (unix socket paths ≤ 108 chars); no host-wide CI lock: golangci-lint serializes itself since main fc0fe68 (D-106 rejected serialising whole gates). Ports 3000/8080/9101 and /run/vrx/agent.sock belong to the running product stack (tools/app) — never touch them
finish: `tools/ci.sh --base main` green in the worktree · docs/status/tasks/F-rpf-adl-pbr.md with pasted real output · everything committed · final message = 10-line summary (branch, last commit, CI result, evidence, open questions, decisions taken with options)
cleanup: stop every process you started (API/agent/vite), by PID · lab lock released · vrx_w10 dropped · no w10 uRPF/ADL/ABF/ACL objects left in VPP (Retrieve + `vppctl show abf policy` / `show abf attach <if>` / `show interface features <if>` pasted) · any global you changed restored · rig down · dist/ and apps/agent/bin removed
questions: docs/status/tasks/F-rpf-adl-pbr-questions.md — write and keep going; never wait for a human
never: merge · restart/kill VPP · Docker · pkill · secrets in files · edit files you do not own
