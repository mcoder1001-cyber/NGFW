# TASK ENVELOPE — F-ra-vpn
id: F-ra-vpn   branch: task/F-ra-vpn   worktree: /root/ngfw-wt/F-ra-vpn   base: main@<BASE>   started: <STARTED>
title: S5: remote-access VPN — IKEv2 + EAP, client pools
prompt: prompts/features/F-ra-vpn.md   (template: prompts/FEATURE-TEMPLATE.md; refreshed on task/prep-rest against main + the P11 and F-pki envelopes)   wbs: D6.9
scope: `vpn.remoteAccess.<name>` → strongSwan remote-access connections (pools with DNS, split tunnel, EAP-MSCHAPv2 / EAP-TLS / pubkey / EAP-RADIUS) rendered by the merged P11 renderer through NEW files; EAP/RADIUS secrets in a 0600 file; connected users (VICI `list-sas`) + disconnect (VICI `terminate`); API, Remote-access tab, docs. IKE in strongSwan, ESP in VPP (kernel-vpp). No new VPP descriptors.
merged deps you can rely on: P11, F-pki (+ P08 through them)
  - P11: vrx-strongswan (kernel-vpp) build and package, the strongSwan renderer (RF-2 + P11: `Model.Pools` / `Conn.Pools` / `Authorities` already render `pools { <name> { addrs; dns } }` and `authorities`, owner-prefix scoping `WithOwnerPrefix`, restart requests + `AckRestart`), its singleton scheduler descriptor under `Domains["vpn"]`, `apps/agent/internal/charon` (stop → sweep → start → AckRestart, SA state), the IpsecState RPC and SA EventKind (12), whatever it merged for PENDING-secret-channel
  - F-pki: `apps/agent/internal/pki` materialiser (x509/, x509ca/, private/ files by name) and its hook in the renderer, the internal CA actions (issue the EAP-TLS client cert at test time)
  - P08 (via P11): desired/, subsystems/, projection.go patterns; also on main: W-seed (anchors, event sink, vpn page shell + tabs.ts), TD-2/TD-3/TD-4
read first: prompts/features/F-ra-vpn.md · docs/status/wave-A-hotspots.md (§0 rules, A1 A2 A4 A7 C1–C7 P1 P4 P5 P6 W3) · docs/status/wave-BC-numbers.md (section F-ra-vpn) · docs/agent/renderers/strongswan.md (RF-2 + P11 + F-pki sections) · apps/agent/internal/renderers/strongswan/README.md · docs/status/tasks/P11.md + F-pki.md · docs/status/tasks/DF-5-questions.md Q10, Q13 · docs/decisions/PENDING-secret-channel.md · docs/vpp-code-track.md V6 · docs/decisions/LOG.md D-040, D-051, D-067, D-072, D-079, D-083, D-089, D-096, D-109
slot: <SLOT> → VRX_SLOT=<SLOT> VRX_TEST_PREFIX=w<SLOT> VRX_HTTP_PORT=3000+100·<SLOT> VRX_WEB_PORT=5000+100·<SLOT> VRX_METRICS_PORT=9100+10·<SLOT>+1 VRX_AGENT_SOCKET=/run/vrx-test/w<SLOT>/agent.sock VRX_PG_DATABASE=vrx_w<SLOT> VRX_VALKEY_DB=<SLOT> VRX_VPP_TABLE_BASE=<SLOT>000 VRX_LAB_LOCK=/run/lock/vrx-lab.lock
  - source of truth: `eval "$(tools/lab env <SLOT>)"`
  - rig prefix w<SLOT> → 10.<SLOT>.{1,2}.0/24; client pools inside 10.<SLOT>.0.0/16 (e.g. 10.<SLOT>.200.0/24) and fd00:<SLOT hex>::/32; connection/pool/user names start with w<SLOT> (≤ 60 chars, D-089); IKE/NAT-T on P11's slot ports 20000+100·<SLOT>+20/+21; tables in <SLOT>000–<SLOT>999
  - test agents run with VRX_GLOBALS_OWNER=0 (D-071)
  - slots 1–11 only; 12 is CI
daemon-owner: strongswan — slot test instances only: the vrx-strongswan charon (kernel-vpp) from P11's package unpacked under /run/vrx-test/w<SLOT>/ in a slot namespace, and the STOCK strongSwan client (debs incl. the EAP client plugins, unpacked the same way, D-083) in ns-w<SLOT>-wan; VICI sockets under /run/vrx-test/w<SLOT>/swan/; strongSwan is NOT installed on the host and must stay so; stop every charon you started by PID. Not concurrently with another strongswan owner (F-ikev2-native) unless the manager says slot-private instances may share (D-089)
obligations:
  - keep P11's S2S behaviour and every P11/RF-2/F-pki test green; you extend the renderer, you do not re-own it (the board row's `renderers/strongswan/**` is narrowed here to new files + named hunks). P11's envelope glob `renderers/strongswan/**` is sequential ownership: P11 is merged before you spawn (board dep) and F-ikev2-native does not touch the renderer, so nobody else edits it while you run
  - D-051/D-040: users' `passwordRef` and RADIUS `secretRef` are refs; EAP secrets and the RADIUS shared secret are rendered only into 0600 files, never into world-readable files, logs, Retrieve/DryRun output, errors, GET, fixtures or status files (grep evidence; test literals `VRX_TEST_PSK_FRAVPN_*`)
  - PENDING-secret-channel (D-109 b): if P11 did not merge a channel, the product path refuses secret-bearing RA profiles in DryRun with a clear error; tests use the slot-local `vpn.MapResolver` fixture; only the end-to-end secret step waits
  - D-089: owner-prefix scoping on the shared charon — you load/unload only `w<SLOT>`-prefixed connections, pools and secrets; never flush another prefix's
  - D-096 / DF-5 Q10: P11's charon id range and restart ordering (stop → Sweep → start → AckRestart) stay as they are; a connection change never leaves orphaned SAs (RF-2 review M1 — close or document)
  - D-072 spirit: verify on the rig whether kernel-vpp installs the client-pool routes in VPP. If it does, add nothing; if the agent must, it becomes the single programmer through core `ip.route` (desired/ra_vpn*.go) — write which one you found
  - D-067: IKEv1/XAuth and GCM IKE proposals stay rejected
  - V6: use the strongSwan version P11 pinned; if an EAP plugin you need (eap-identity, eap-mschapv2 + md4, eap-tls, eap-radius) is missing from P11's configure line, add it with one named hunk in P11's build config and rebuild from P11's pinned tarball (never from the GitHub archive)
files you own exclusively:
  - NEW files in P11's renderer: apps/agent/internal/renderers/strongswan/ra*.go (+ ra*_test.go), apps/agent/internal/renderers/strongswan/templates/vrx-ra*.tmpl (prefer a separate conf.d `vrx-ra.conf`, a 0600 `vrx-ra-secrets.conf` and a strongswan.d `eap-radius.conf` over editing P11's templates), apps/agent/internal/renderers/strongswan/testdata/ra-*
  - apps/agent/internal/desired/ra_vpn*.go (only for pool routes), apps/agent/internal/subsystems/ra_vpn*.go, apps/agent/internal/agent/rpc_ra_vpn*.go (`RemoteAccessSessions`), apps/agent/internal/actions/ra-vpn/** (disconnect)
  - packages/schema/src/domains/ext/ra-vpn*.ts (only for gap fields), packages/schema/src/semantic/ra-vpn*.ts (rule ids `vpn.ra-vpn-…`; most RA rules already exist in semantic/vpn.ts — test them, add only what is missing)
  - packages/schema/examples/ra-vpn-*.json, packages/proto/test/fixtures/ra-vpn-*.json
  - apps/api/src/features/ra-vpn/** (index.ts exports {controllers, providers}; `RaVpnController`; GET /state/vpn/remote-access/sessions (paged), POST /actions/vpn/remote-access/sessions/{id}/disconnect — not a DELETE under /state, which is read-only (architecture rule 8); real fake behaviour in fake.ts), apps/api/test/e2e/ra-vpn*
  - apps/web/src/domains/vpn/ra-vpn/** (profile wizard, connected-users grid, per-OS client hints), apps/web/src/locales/{en,fa}/ra-vpn.json
  - docs/user/vpn/ra-vpn.md, test/topology/ra-vpn/**, docs/status/tasks/F-ra-vpn*
shared hotspots (append-only, conflicts resolved by the manager at merge):
  - protocol: docs/status/wave-A-hotspots.md §0. Insert only directly below your own anchor `// wave-BC: F-ra-vpn` if the manager seeded one, else at the end of the block. Never reorder or reformat other lines. List every hunk under "Shared hunks" in docs/status/tasks/F-ra-vpn.md
  - P11's renderer files (dep-chained; P11 + F-pki merged, nobody else edits them now): apps/agent/internal/renderers/strongswan/{model,plan,renderer,settings,paths}.go and templates/* — named hunks only: the call that builds RA connections into the model, the new files in the rendered file set, the EAP plugins in `DaemonConfig.Plugins`, the EAP auth fields on `Auth` (`eap_id`, `send_certreq`). Keep F-pki's hook hunk
  - docs/agent/renderers/strongswan.md: append a "Remote access (F-ra-vpn)" section; README.md in the renderer: one paragraph
  - P11's build config deploy/strongswan/** or deploy/debian/vrx-strongswan/**: one configure-line hunk only if an EAP plugin is missing (see obligations)
  - A1 apps/agent/internal/subsystems/subsystems.go: only if you add a descriptor or Wiring (expected: none — RA rides P11's renderer descriptor)
  - A2 apps/agent/internal/agent/projection.go: only for pool routes
  - A4 apps/agent/internal/agent/server.go: one case `remote_access_disconnect` in the Action type switch; RPC methods live in your rpc_ra_vpn*.go
  - A7 docs/vpp-code-track.md: `### V-new (F-ra-vpn)` only for a new kernel-vpp gap; the manager numbers it
  - C1 packages/schema/src/domains/vpn.ts: key lines only for gap fields (per-user static IP, RADIUS accounting) · C2 packages/schema/src/semantic/index.ts: one spread line · C3 packages/schema/src/index.ts if you export anything
  - C4 packages/schema/examples/ + packages/proto/test/fixtures/: new files only; vpn-remote-access.json, vpn-*.json and all-domains.json are read-only
  - C5 packages/proto/vrx/v1/dataplane.proto: rpc `RemoteAccessSessions` under the service anchor; messages (RemoteAccessSession*, RemoteAccessDisconnectAction) in a `// ----- F-ra-vpn -----` section at the end
  - allocated numbers (docs/status/wave-BC-numbers.md, a merge blocker if reused): **ActionRequest 12 `remote_access_disconnect`**; RemoteAccessProfile 17–18, RemoteAccessUser 3 and EventKind 16 `EVENT_KIND_REMOTE_ACCESS_SESSION` only if needed; nothing else
  - C6 docs/contracts/proto.md: `### F-ra-vpn: RemoteAccessSessions, remote_access_disconnect` · C7 generated, regenerated and never hand-edited: apps/agent/gen/**, packages/proto/gen/ts/**, packages/api-client/src/generated/**, apps/cli/internal/api/operations_gen.go, docs/user/cli/reference.md (`packages/proto/gen.sh`, `pnpm gen`, `make -C apps/cli gen docs`)
  - P1 apps/api/src/app.module.ts · P4 apps/api/src/agent/agent.client.ts (`remoteAccessSessions`; the disconnect reuses F-vrf-static-ecmp's generic Action stream method) · P5 apps/api/src/testing/fake-agent.ts (UNIMPLEMENTED stub under the anchor on the contract commit) · P6 apps/api/src/infra/bus.ts: one RA session topic only with EventKind 16
  - W3 apps/web/src/i18n.ts · the vpn page tab registry apps/web/src/domains/vpn/tabs.ts: one line
contract: commit `contract(proto): remote-access sessions + disconnect` (and `contract(schema): …` only for gap fields) as separate commits on YOUR branch first (the ci.sh contract guard checks the subject), plus docs/status/tasks/F-ra-vpn-contract.md; tell the manager in the questions file and keep building. No own branches (the prompt's old `contract/F-ra-vpn` wording is void)
files you must not touch:
  - everything else
  - never: /root/ngfw (main), other worktrees, /etc (incl. /etc/swanctl, /etc/strongswan*, /etc/vpp), /root/vpp (read-only)
  - apps/agent/binapi (P04/manager-owned), tools/lab, tools/ci.sh, plan/tasks.yaml, docs/decisions/LOG.md, docs/status/PROGRESS.md, packages/ui-kit/**
  - apps/agent/internal/{charon,secrets,pki}/** (P11 / F-pki), apps/agent/internal/descriptors/{ipsec,ikev2,vpn,wireguard}/**, P11's desired/subsystems/rpc ipsec files
  - apps/api/src/{secrets,commit,auth,users}/** (use the secrets service; admin login RADIUS is F-aaa's), apps/api/src/features/{ipsec,ikev2-native,pki,wireguard}/**, apps/web/src/domains/vpn/{ipsec,ikev2-native,pki,wireguard}/**
  - agent core (A5): apps/agent/internal/agent/{agent,service,state,ifstate}.go, apps/agent/cmd/**; apps/agent/internal/vpp/ifsanitize/** (TD-3)
host rules:
  - VPP SAFETY (D-064): `systemctl show vpp -p NRestarts` before and after every host run (pasted); stop host runs and write it down if it rises
  - the kernel-vpp charon talks to the shared VPP: it may bind SPDs/SAs and add routes only on your slot's interfaces and tables <SLOT>000–<SLOT>999; one host test package at a time (D-087)
  - stock strongSwan client debs (incl. the package that carries the EAP client plugins): unpacked under /run/vrx-test/w<SLOT>/ (non-persistent). If they are gone and not re-fetchable offline, the re-download needs the network (D-089 manual step) — write it in the questions file, skip the packet step with the reason and finish everything else
  - EAP-RADIUS: no RADIUS server is installed on the host and no package installs are allowed → rendered config + golden files + unit tests only; the host step is skipped with that reason
  - V19 SAFETY (D-095): before ANY ping/ESP through the rig, `go -C apps/agent run ./cmd/vrx-vpp-preflight` must exit 0 (TD-3); never send packets through an unchecked interface
  - D-101: bring the veth down before any af_packet delete; af_packet rings per D-113
  - `vppctl show ipsec sa` and charon logs are pasted only through a redaction filter (no keys, no EAP secrets)
  - with VRX_INTEGRATION=1, one Go package at a time; hold `flock -s` on the lab lock only during a run (D-094)
coordination: P11 (merged) owns S2S and the charon lifecycle · F-pki (merged) owns certificate files — you reference names only · F-ikev2-native owns native (VPP) IKEv2 · F-aaa owns RADIUS/TACACS for admin login · F-ha-state-sync documents SA behaviour on failover
evidence: Playwright is not installed. Take the screenshots (en + fa/RTL: profile wizard, connected-users grid with disconnect) with the headless Chrome approach from P07a/P07b/P08 (`test/topology/interfaces/shots_test.go`, kept outside the product code), and say so. `tcpdump` on the inter-namespace veth must show ESP only; paste VICI `list-conns` / `get-pools` after rollback
time box: 15 h — when exceeded: stop, commit WIP, write docs/status/tasks/F-ra-vpn.md with what is left
WIP: commit at least every 45 min; keep docs/status/tasks/F-ra-vpn-wip.md current
CI: `TMPDIR=/tmp/g-w<SLOT> tools/ci.sh --base main` — short TMPDIR (unix socket paths ≤ 108 chars); no host-wide CI lock: golangci-lint serializes itself since main fc0fe68 (D-106 rejected serialising whole gates). Ports 3000/8080/9101 and /run/vrx/agent.sock belong to the running product stack (tools/app) — never touch them
finish: `tools/ci.sh --base main` green in the worktree · docs/status/tasks/F-ra-vpn.md with pasted real output (secrets redacted) · everything committed · final message = 10-line summary (branch, last commit, CI result, evidence, open questions, decisions taken with options)
cleanup: stop every process you started (API/agent/vite/both charons), by PID · lab lock released · vrx_w<SLOT> dropped · no w<SLOT> SAs/SPDs/routes left in VPP (dumps pasted, keys redacted) · unpacked debs, rendered files and client certs under /run/vrx-test/w<SLOT>/ removed · rig down · dist/ and apps/agent/bin removed
questions: docs/status/tasks/F-ra-vpn-questions.md — write and keep going; never wait for a human
never: merge · restart/kill VPP · Docker · pkill · secrets in files · edit files you do not own
