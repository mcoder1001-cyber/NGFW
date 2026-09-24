# TASK ENVELOPE — F-ikev2-native
id: F-ikev2-native   branch: task/F-ikev2-native   worktree: /root/ngfw-wt/F-ikev2-native   base: main@<BASE>   started: <STARTED>
title: Wave B (day 10-12): VPP native IKEv2 responder path
prompt: prompts/features/F-ikev2-native.md   (template: prompts/FEATURE-TEMPLATE.md; refreshed on task/prep-rest against main + P08 + the P11 envelope)   wbs: D6.3
scope: `vpn.ipsec.tunnels.<name>` with `engine: "vpp-ikev2"` → DF-5's `ikev2.profile` (+ responder, transforms, traffic selectors, route-based `ipsec<N>` tunnel interface, route via core `ip.route`), SA state RPC, initiate/rekey/delete actions, the native-tunnel status column + SA drawer, docs. Responder first; initiator = one explicit action. No strongSwan, no SPD/SA descriptors of your own.
merged deps you can rely on: P08, DF-5, P11
  - P08: desired/ (Sink, `interface/<name>` aliases), subsystems.Register/Domains, `Wiring.IKEv2Options()` (persisted BootStore + D-096 keyer + globals flag — already written and tested in subsystems/vpn_test.go), `Wiring.VPNKeyer()`, projection.go
  - DF-5: descriptors/ikev2 (`ikev2.profile`, write-only `ikev2.responder-hostname`, owner-only `ikev2.local-key` / `ikev2.liveness` / `ikev2.sleep-interval`, `SAs()` state helper with zeroed derived keys, `InitiateSAInit` / `DeleteIKESA` / `DeleteChildSA` / `RekeyChildSA`), descriptors/ipsec (`ipsec.itf`, `ipsec.tunnel-protect`; P11 gap-owner), descriptors/vpn (Resolver interface, MapResolver fixture, Keyer, vpntest.Agent)
  - P11: the strongSwan builder (desired/ipsec*.go — it skips `engine: vpp-ikev2` tunnels), `Domains["vpn"]`, the IpsecState RPC + SA EventKind (12), the vpn page IPsec tab, the stock-strongSwan unpack harness (RF-2 swantest), and whatever it merged for PENDING-secret-channel
  - also on main: W-seed (anchors, `subsystems.Env` event sink, the vpn page shell + tabs.ts), TD-3 (V19 sanitizer — it runs inside `ipsec.itf` Create), TD-2
read first: prompts/features/F-ikev2-native.md · docs/status/wave-A-hotspots.md (§0 rules, A1 A2 A4 A7 C1–C7 P1 P4 P5 P6 W3) · docs/status/wave-BC-numbers.md (section F-ikev2-native) · docs/agent/descriptors/ikev2.md (first), ipsec.md · apps/agent/internal/descriptors/vpn/doc.go · docs/status/tasks/DF-5.md + DF-5-questions.md (Q2 id truncation, Q9 initiator hostnames) · docs/status/tasks/P11.md (its secret-channel and renderer-stage decisions) · docs/decisions/PENDING-secret-channel.md · docs/decisions/LOG.md D-051, D-063, D-065, D-067, D-069, D-071, D-076, D-080, D-082, D-083, D-089, D-096, D-099, D-109
slot: <SLOT> → VRX_SLOT=<SLOT> VRX_TEST_PREFIX=w<SLOT> VRX_HTTP_PORT=3000+100·<SLOT> VRX_WEB_PORT=5000+100·<SLOT> VRX_METRICS_PORT=9100+10·<SLOT>+1 VRX_AGENT_SOCKET=/run/vrx-test/w<SLOT>/agent.sock VRX_PG_DATABASE=vrx_w<SLOT> VRX_VALKEY_DB=<SLOT> VRX_VPP_TABLE_BASE=<SLOT>000 VRX_LAB_LOCK=/run/lock/vrx-lab.lock
  - source of truth: `eval "$(tools/lab env <SLOT>)"`
  - rig prefix w<SLOT> → 10.<SLOT>.{1,2}.0/24; inner networks inside 10.<SLOT>.0.0/16; profile names start with w<SLOT>; IKE ids are fqdn (`w<SLOT>-<name>.vrx.test`), never an IPv4 id with a zero inner octet (DF-5 Q2); `ipsec<N>` instances and tables in <SLOT>000–<SLOT>999; ipsec-over-udp ports 20000+100·<SLOT>+1/+2 (DF-5 port scheme)
  - test agents run with VRX_GLOBALS_OWNER=0 (D-071)
  - slots 1–11 only; 12 is CI
daemon-owner: strongswan — test peer only: the STOCK charon (debs unpacked under /run/vrx-test/w<SLOT>/ by the RF-2/P11 harness, D-083) as the IKEv2 initiator in ns-w<SLOT>-wan, VICI socket under /run/vrx-test/w<SLOT>/swan/; strongSwan is NOT installed on the host and must stay so; stop every charon you started by PID. Not concurrently with another strongswan owner (F-ra-vpn) unless the manager says slot-private instances may share (D-089)
obligations:
  - D-096: PSK/key references reach `ikev2.profile` as keyed `hmac:<hex>` fingerprints through `Wiring.IKEv2Options()` (the prompt's old `sha256:` wording is void); never plaintext in Value, logs, errors, GET, fixtures or status files (test literal `VRX_TEST_PSK_FIKEV2_*`)
  - PENDING-secret-channel (D-109 b): if P11 did not merge a channel, the product path resolves no secret — DryRun refuses a native tunnel with a clear error; tests use a slot-local `vpn.MapResolver` fixture; only the end-to-end secret step waits, nothing else
  - D-071/D-082: `ikev2.liveness`, `ikev2.local-key`, `ikev2.sleep-interval` are plugin-wide → registered as setters only for the globals owner; slot agents require the sleep interval (read under `flock -s /run/lock/vrx-globals.lock`) and never touch liveness / local key
  - D-063/D-076/D-080: `ikev2.responder-hostname` is write-only, applied once per VPP boot identity via the persisted store in IKEv2Options(); Retrieve never echoes desired state
  - D-065/D-069: the tunnel interface is `interface/<name>`; the route to it uses core `ip.route` (one programmer, D-072 spirit)
  - D-067: IKEv1 and GCM IKE proposals stay rejected; key-id ids are refused (VPP 26.06 supports ip4/ip6/fqdn/rfc822 only)
  - native path is route-based only (policy-based native tunnels are out of scope)
files you own exclusively:
  - gap-only (DF-5 built it; edit only for a proven defect, name the test): apps/agent/internal/descriptors/ikev2/**, docs/agent/descriptors/ikev2.md
  - apps/agent/internal/desired/ikev2*.go (builder + assembler for `engine: vpp-ikev2`), apps/agent/internal/subsystems/ikev2*.go (Register call with IKEv2Options(), resolver wiring), apps/agent/internal/agent/rpc_ikev2*.go (`Ikev2Sas`)
  - apps/agent/internal/actions/ikev2-native/** (initiate / rekey child / delete IKE SA)
  - apps/agent/internal/descriptors/core/coretest/ikev2*.go (A6: new file, only if the agent-level fake needs it)
  - packages/schema/src/semantic/ikev2-native*.ts (rule ids `vpn.ikev2-native-…`), packages/schema/src/domains/ext/ikev2-native*.ts (only with the settings answer, see numbers)
  - packages/schema/examples/ikev2-native-*.json, packages/proto/test/fixtures/ikev2-native-*.json
  - apps/api/src/features/ikev2-native/** (index.ts exports {controllers, providers}; `Ikev2NativeController`; GET /state/ipsec/ikev2/sas, POST /actions/ipsec/ikev2/{tunnel}/{initiate|rekey|delete-sa}; real fake behaviour in fake.ts), apps/api/test/e2e/ikev2-native*
  - apps/web/src/domains/vpn/ikev2-native/** (native status column + SA drawer), apps/web/src/locales/{en,fa}/ikev2-native.json
  - docs/user/vpn/ikev2-native.md, test/topology/ikev2-native/**, docs/status/tasks/F-ikev2-native*
shared hotspots (append-only, conflicts resolved by the manager at merge):
  - protocol: docs/status/wave-A-hotspots.md §0. Insert only directly below your own anchor `// wave-BC: F-ikev2-native` if the manager seeded one, else at the end of the block. Never reorder or reformat other lines. List every hunk under "Shared hunks" in docs/status/tasks/F-ikev2-native.md
  - A1 apps/agent/internal/subsystems/subsystems.go: append ikev2 descriptor names to `Domains["vpn"]` (P11 added the key) + one Register line calling your subsystems/ikev2.go
  - A2 apps/agent/internal/agent/projection.go: one call in project(), one in assemble()
  - P11's apps/agent/internal/desired/ipsec*.go (dep-chained): only if it reports `engine: vpp-ikev2` tunnels as unsupported — replace that one case by a call into your builder (one hunk); never change its strongSwan path
  - A4 apps/agent/internal/agent/server.go: one case `ikev2_sa` in the Action type switch; RPC methods live in your rpc_ikev2*.go
  - A7 docs/vpp-code-track.md: `### V-new (F-ikev2-native)` for the IKE id NUL truncation (DF-5 Q2); the manager numbers it
  - C1 packages/schema/src/domains/vpn.ts: one key line only if the liveness/sleep-interval answer needs fields (sub-schema in your ext file) · C2 packages/schema/src/semantic/index.ts: one spread line · C3 packages/schema/src/index.ts if you export anything
  - C4 packages/schema/examples/ + packages/proto/test/fixtures/: new files only; vpn-site-to-site.json, vpn-*.json and all-domains.json are read-only
  - C5 packages/proto/vrx/v1/dataplane.proto: rpc `Ikev2Sas` under the service anchor; messages (Ikev2Sa*, Ikev2SaAction) in a `// ----- F-ikev2-native -----` section at the end
  - allocated numbers (docs/status/wave-BC-numbers.md, a merge blocker if reused): **ActionRequest 11 `ikev2_sa`**; IpsecSettings 3–4 only if the globals-owner liveness/sleep-interval answer is "configure them"; IpsecTunnel 30–31 only for a proven gap (27–29 are left for P11); SA events reuse P11's EventKind 12 with attribute `engine=vpp-ikev2` — no new EventKind
  - C6 docs/contracts/proto.md: `### F-ikev2-native: Ikev2Sas, ikev2_sa action` · C7 generated, regenerated and never hand-edited: apps/agent/gen/**, packages/proto/gen/ts/**, packages/api-client/src/generated/**, apps/cli/internal/api/operations_gen.go, docs/user/cli/reference.md (`packages/proto/gen.sh`, `pnpm gen`, `make -C apps/cli gen docs`)
  - P1 apps/api/src/app.module.ts · P4 apps/api/src/agent/agent.client.ts (`ikev2Sas`; actions reuse F-vrf-static-ecmp's generic Action stream method) · P5 apps/api/src/testing/fake-agent.ts (UNIMPLEMENTED stub under the anchor on the contract commit) · P6 apps/api/src/infra/bus.ts: none expected (P11's SA topic carries native SAs too)
  - W3 apps/web/src/i18n.ts · the vpn page tab registry apps/web/src/domains/vpn/tabs.ts (W-seed shell): one line only if the SA view is its own tab · P11's IPsec tab (apps/web/src/domains/vpn/ipsec/**, dep-chained): one named hunk that mounts your status column / SA drawer for native tunnels — the engine selector already comes from the schema-driven form
contract: commit `contract(proto): ikev2 SA state + ikev2_sa action` (and `contract(schema): …` only with the settings answer) as separate commits on YOUR branch first (the ci.sh contract guard checks the subject), plus docs/status/tasks/F-ikev2-native-contract.md; tell the manager in the questions file and keep building. No own branches (the prompt's old `contract/F-ikev2-native` wording is void)
files you must not touch:
  - everything else
  - never: /root/ngfw (main), other worktrees, /etc (incl. /etc/vpp, /etc/swanctl), /root/vpp
  - apps/agent/binapi (P04/manager-owned), tools/lab, tools/ci.sh, plan/tasks.yaml, docs/decisions/LOG.md, docs/status/PROGRESS.md, packages/ui-kit/**
  - apps/agent/internal/descriptors/{ipsec,vpn,wireguard,lcp}/** (P11 gap-owner / DF-5 shared helpers / F-wireguard / P12 — a change goes to the questions file), apps/agent/internal/renderers/strongswan/**, apps/agent/internal/charon/**, apps/agent/internal/secrets/** (P11), apps/agent/internal/pki/** (F-pki), P11's desired/subsystems/rpc ipsec files beyond the one hunk above
  - apps/api/src/{secrets,commit}/** (use the secrets service, do not edit it), apps/api/src/features/{ipsec,wireguard,pki,ra-vpn}/**
  - agent core (A5): apps/agent/internal/agent/{agent,service,state,ifstate}.go, apps/agent/cmd/**; apps/agent/internal/vpp/ifsanitize/** (TD-3)
host rules:
  - VPP SAFETY (D-064): `systemctl show vpp -p NRestarts` before and after every host run (pasted); stop host runs and write it down if it rises
  - UDP 500/4500: adding the first profile makes VPP bind IKE inside its own UDP stack (refcounted, not a host socket); the stock initiator in ns-w<SLOT>-wan talks to your rig address only
  - stock strongSwan debs: unpacked under /run/vrx-test/w<SLOT>/ by the harness (non-persistent). If they are gone (reboot) and not re-fetchable offline, the re-download needs the network (D-089 manual step) — then write it in the questions file, skip the packet step with the reason and finish everything else
  - cert (rsa-sig) auth needs `ikev2.local-key` = a VPP-wide global with no getter → host step only as opt-in (`VRX_FIKEV2_GLOBALS=1`) in a manager window under `flock -x /run/lock/vrx-globals.lock`; PSK is the default host path
  - V19 SAFETY (D-095): before ANY ESP/ping through the rig, `go -C apps/agent run ./cmd/vrx-vpp-preflight` must exit 0 (TD-3); never send packets through an unchecked interface
  - delete order: SAs (delete IKE SA action) and route before the tunnel interface, profile last; D-101: bring the veth down before any af_packet delete; af_packet rings per D-113
  - `vppctl show ikev2 profile` / `show ikev2 sa` print the PSK and keys → every pasted output goes through a redaction filter
  - with VRX_INTEGRATION=1, one Go package at a time; hold `flock -s` on the lab lock only during a run (D-094)
coordination: P11 (merged) owns the strongSwan path, `descriptors/ipsec` and the IpsecState RPC — native SAs are yours, strongSwan SAs are P11's; one IPsec page with an engine column · F-pki (may run in parallel) materialises certificate/key files — the rsa-sig key path for `ikev2_set_local_key` comes from its materialiser (soft dep, D-099); until it merges, cert auth uses a throwaway key under /run/vrx-test/w<SLOT>/ in the opt-in step only · F-wireguard/F-ra-vpn share `Domains["vpn"]` and the vpn page · F-ha-state-sync documents "re-key on failover" for native SAs
evidence: Playwright is not installed. Take the screenshots (en + fa/RTL: IPsec list with engine + native status, SA drawer) with the headless Chrome approach from P07a/P07b/P08 (`test/topology/interfaces/shots_test.go`, kept outside the product code), and say so. `tcpdump` on the inter-namespace veth must show ESP only
time box: 15 h — when exceeded: stop, commit WIP, write docs/status/tasks/F-ikev2-native.md with what is left
WIP: commit at least every 45 min; keep docs/status/tasks/F-ikev2-native-wip.md current
CI: `TMPDIR=/tmp/g-w<SLOT> tools/ci.sh --base main` — short TMPDIR (unix socket paths ≤ 108 chars); no host-wide CI lock: golangci-lint serializes itself since main fc0fe68 (D-106 rejected serialising whole gates). Ports 3000/8080/9101 and /run/vrx/agent.sock belong to the running product stack (tools/app) — never touch them
finish: `tools/ci.sh --base main` green in the worktree · docs/status/tasks/F-ikev2-native.md with pasted real output (PSK redacted) · everything committed · final message = 10-line summary (branch, last commit, CI result, evidence, open questions, decisions taken with options)
cleanup: stop every process you started (API/agent/vite/charon), by PID · lab lock released · vrx_w<SLOT> dropped · no w<SLOT> IKEv2 profiles, SAs, ipsec<N> interfaces or routes left (Retrieve + `show ikev2 profile` redacted, pasted) · globals left as found · unpacked debs and key files under /run/vrx-test/w<SLOT>/ removed · rig down · dist/ and apps/agent/bin removed
questions: docs/status/tasks/F-ikev2-native-questions.md — write and keep going; never wait for a human
never: merge · restart/kill VPP · Docker · pkill · secrets in files · edit files you do not own
