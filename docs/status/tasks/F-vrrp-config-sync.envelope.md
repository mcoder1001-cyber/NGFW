# TASK ENVELOPE — F-vrrp-config-sync
id: F-vrrp-config-sync   branch: task/F-vrrp-config-sync   worktree: /root/ngfw-wt/F-vrrp-config-sync   base: main@<BASE>   started: <STARTED>
title: Wave C (day 13-15): VRRPv3 (VPP plugin + keepalived path), config sync, cluster UI
prompt: prompts/features/F-vrrp-config-sync.md   (template: prompts/FEATURE-TEMPLATE.md; refreshed on task/prep-rest against main + P08 + the P12 envelope)   wbs: D9.1, D9.2, D9.5
scope: `ha.vrrp` → DF-7's VPP vrrp descriptors (`engine: vpp`, primary) and RF-4's keepalived renderer with P12's linux-cp mapper (`engine: keepalived`), VRRP state + events, API-to-API config sync of the running revision minus node-local pointers, cluster state + force-sync, the System → High availability page (VRRP list, cluster view, a panel registry for F-ha-state-sync), docs. No NAT/IPsec state sync, no LCP pair management.
merged deps you can rely on: P08, DF-7, RF-4, P12
  - P08: desired/ (Sink, `interface/<name>` aliases), subsystems.Register/Domains + Wiring.BootStore(), projection.go
  - DF-7: descriptors/vrrp (`vrrp.vr`, `vrrp.vr-peers`, `vrrp.vr-track-interface`, `vrrp.vr-state`; `WatchEvents` over `want_vrrp_vr_events`, `States`; V20 peer dump per VR) + df7 helpers (SetBootStore installed by P08)
  - RF-4: renderers/keepalived (renders only `engine: keepalived`; `InterfaceMapper` defaults to `NoMapper` = every instance rejected; notify helper state files; SIGHUP reload / SIGJSON dump; `TestPaths`; D-086 stand-ins `ha.keepalived.*`, `ha.vrrp.<name>.keepalived.*`; VRRP with auth renders VRRPv2)
  - P12: apps/agent/internal/lcpmap (the linux-cp interface mapper), the LCP pair model, its renderer-stage choice (singleton scheduler descriptor, D-109 d)
  - P06/TD-2/TD-4 (on main): commit engine, `config_revision.kind` (text), secrets service, the `commit.events` bus topic, RBAC, audit; also W-seed (anchors, `subsystems.Env` event sink)
read first: prompts/features/F-vrrp-config-sync.md · docs/status/wave-A-hotspots.md (§0 rules, A1 A2 A6 A7 C1–C7 P1 P4 P5 P6 W1–W3) · docs/status/wave-BC-numbers.md (section F-vrrp-config-sync) · docs/agent/descriptors/vrrp.md · docs/agent/renderers/keepalived.md + apps/agent/internal/renderers/keepalived/README.md · docs/status/tasks/DF-7.md, RF-4.md + RF-4-questions.md (Q2 stand-ins), P12.md · docs/vpp-code-track.md V20, V22 · docs/decisions/LOG.md D-053, D-063, D-067, D-071, D-076, D-080, D-086, D-087, D-089, D-090, D-091, D-097, D-100, D-102, D-109
slot: <SLOT> → VRX_SLOT=<SLOT> VRX_TEST_PREFIX=w<SLOT> VRX_HTTP_PORT=3000+100·<SLOT> VRX_WEB_PORT=5000+100·<SLOT> VRX_METRICS_PORT=9100+10·<SLOT>+1 VRX_AGENT_SOCKET=/run/vrx-test/w<SLOT>/agent.sock VRX_PG_DATABASE=vrx_w<SLOT> VRX_VALKEY_DB=<SLOT> VRX_VPP_TABLE_BASE=<SLOT>000 VRX_LAB_LOCK=/run/lock/vrx-lab.lock
  - source of truth: `eval "$(tools/lab env <SLOT>)"`
  - rig prefix w<SLOT> → 10.<SLOT>.{1,2}.0/24; virtual addresses inside 10.<SLOT>.0.0/16; VRs only on w<SLOT> interfaces
  - cluster node B lives inside YOUR slot: API on 3000+100·<SLOT>+50, database `vrx_w<SLOT>b` (`deploy/dev/pg-test.sh create w<SLOT>b` / `drop w<SLOT>b`), `VRX_VALKEY_PREFIX=vrx:w<SLOT>b:` on Valkey db <SLOT>, agent socket /run/vrx-test/w<SLOT>/b/agent.sock, object prefix `w<SLOT>b` (≤ 6 chars), tables <SLOT>500–<SLOT>999 (node A keeps <SLOT>000–<SLOT>499); throwaway TLS material under /run/vrx-test/w<SLOT>/
  - test agents run with VRX_GLOBALS_OWNER=0 (D-071)
  - slots 1–11 only; 12 is CI
daemon-owner: keepalived — slot test instances only (RF-4 `TestPaths("w<SLOT>", <binDir>, "ns-w<SLOT>-a")`; /run is noexec, so the notify helper and checks live in a bin dir outside /run, RF-4 README); `keepalived.service` stays disabled and stopped; nothing under /etc/keepalived is written; stop every keepalived you started by PID
obligations:
  - D-053/D-067: `ha.vrrp` stays a record keyed by name (map field 2); `ha.cluster` shape unchanged; everything new is additive
  - D-086: the keepalived stand-ins become real contract fields (`ha.keepalived.scripts`, `ha.keepalived.syncGroups`, `ha.vrrp.<name>.keepalived.trackScripts`); `check` stays an allow-listed name, never a path or script text; VRRP with auth renders VRRPv2; 8-char test keys in the accepted D-086 form (e.g. `FVRtpsk1`), never a real secret
  - keepalived path: pass P12's lcpmap as `InterfaceMapper`; without an LCP pair the instance is skip-with-reason (DryRun warning), never silently dropped
  - D-063/D-076/D-080: keep DF-7's write-only/applied-once handling of VR start/stop; persisted stores only (Wiring)
  - events: VRRP state from `vrrp.WatchEvents` + keepalived notify files → agent events through the W-seed `subsystems.Env` event sink (never edit agent.go, A5); one EventKind (numbers below)
  - config sync is API-to-API, never agent-to-agent: subscribe to the `commit.events` bus topic (no edit of the commit engine), push the running revision minus `syncExclude` + the fixed node-local set (`/ha/cluster/nodeName`, management addresses, `system.hostname`, `management.users` — password hashes live only in app_user, D-097/D-102) to each peer; the peer applies it as a normal commit with `config_revision.kind = 'cluster-sync'` and a comment naming the origin (text column, no migration); a peer with local unsynced edits refuses and reports
  - transport as the prompt specifies (implementing it is not a decision): HTTPS with the peer's certificate pinned + the cluster key from the secrets store (`key/<name>`, D-051); never plain HTTP (D-100 spirit)
  - secrets are NOT synced in this build: re-encrypting secret material for a peer is a secret-storage/security-boundary decision (decision-policy #4) — write the options in the questions file (the manager files a PENDING); meanwhile a sync whose document references a secret missing on the peer fails with a clear error that lists the refs (never material)
  - create an appendable panel registry on the HA page (e.g. `clusterPanels.ts`) — F-ha-state-sync adds its "State sync" panel there with one line
files you own exclusively:
  - gap-only (DF-7 / RF-4 built them; edit for a proven defect or the mapper/stand-in wiring, name the test): apps/agent/internal/descriptors/vrrp/**, docs/agent/descriptors/vrrp.md, apps/agent/internal/renderers/keepalived/**, docs/agent/renderers/keepalived.md
  - apps/agent/internal/desired/vrrp*.go (builders + assembler for both engines), apps/agent/internal/subsystems/{vrrp,keepalived}*.go (Register, the keepalived singleton descriptor, event publishers), apps/agent/internal/agent/rpc_vrrp*.go (`VrrpState`)
  - apps/agent/internal/descriptors/core/coretest/vrrp*.go (A6: new file, only if needed)
  - packages/schema/src/domains/ext/vrrp-config-sync*.ts, packages/schema/src/semantic/vrrp-config-sync*.ts (rule ids `ha.vrrp-config-sync-…`)
  - packages/schema/examples/vrrp-config-sync-*.json, packages/proto/test/fixtures/vrrp-config-sync-*.json
  - apps/api/src/features/vrrp-config-sync/** (index.ts exports {controllers, providers}; `VrrpConfigSyncController`; GET /state/ha/vrrp, GET /state/ha/cluster, POST /actions/ha/sync, the peer-facing sync endpoint, the sync service; real fake behaviour in fake.ts), apps/api/test/e2e/vrrp-config-sync*
  - apps/web/src/domains/system/vrrp-config-sync/** (route /system/ha: VRRP list with live role chips, cluster view, force-sync, the panel registry), apps/web/src/locales/{en,fa}/vrrp-config-sync.json
  - docs/user/system/vrrp-config-sync.md, test/topology/vrrp-config-sync/**, docs/status/tasks/F-vrrp-config-sync*
shared hotspots (append-only, conflicts resolved by the manager at merge):
  - protocol: docs/status/wave-A-hotspots.md §0. Insert only directly below your own anchor `// wave-BC: F-vrrp-config-sync` if the manager seeded one, else at the end of the block. Never reorder or reformat other lines. List every hunk under "Shared hunks" in docs/status/tasks/F-vrrp-config-sync.md
  - A1 apps/agent/internal/subsystems/subsystems.go: a new `Domains["ha"]` key (you add it; F-ha-state-sync appends later) + one Register line calling your subsystems/vrrp.go
  - A2 apps/agent/internal/agent/projection.go: one call in project(), one in assemble(); replaces the `ha` unsupported-field warning for the leaves you implement
  - A7 docs/vpp-code-track.md: `### V-new (F-vrrp-config-sync)` only for a new gap; the manager numbers it
  - C1 packages/schema/src/domains/ha.ts (P02c's): key lines for `cluster.syncExclude`, `keepalived`, `vrrp.<name>.keepalived` (sub-schemas in your ext file) · C2 packages/schema/src/semantic/index.ts: one spread line · C3 packages/schema/src/index.ts: one export line
  - C4 packages/schema/examples/ + packages/proto/test/fixtures/: new files only; ha-vrrp.json, ha-semantic-duplicate-vrid.json, invalid-ha-vrid.json and all-domains.json are read-only
  - C5 packages/proto/vrx/v1/dataplane.proto: rpc `VrrpState` under the service anchor; new messages (HaKeepalived, VrrpKeepalived, VrrpState*) in a `// ----- F-vrrp-config-sync -----` section at the end
  - allocated numbers (docs/status/wave-BC-numbers.md, a merge blocker if reused): **HaConfig 4 `keepalived`**, **VrrpInstance 15 `keepalived`**, **HaCluster 10 `sync_exclude`**, **EventKind 17 `EVENT_KIND_VRRP_STATE_CHANGED`**; no ActionRequest (force-sync is API-only); nothing else
  - C6 docs/contracts/proto.md: `### F-vrrp-config-sync: VrrpState` · C7 generated, regenerated and never hand-edited: apps/agent/gen/**, packages/proto/gen/ts/**, packages/api-client/src/generated/**, apps/cli/internal/api/operations_gen.go, docs/user/cli/reference.md (`packages/proto/gen.sh`, `pnpm gen`, `make -C apps/cli gen docs`)
  - P1 apps/api/src/app.module.ts · P4 apps/api/src/agent/agent.client.ts (`vrrpState`) · P5 apps/api/src/testing/fake-agent.ts (UNIMPLEMENTED stub under the anchor on the contract commit) · P6 apps/api/src/infra/bus.ts + telemetry relay: one `vrrp.events` topic + its EventKind case
  - W1 apps/web/src/router.tsx (/system/ha) · W2 apps/web/src/nav/nav.ts + nav.test.ts (`ha` into BUILT_DOMAINS) · W3 apps/web/src/i18n.ts
  - apps/agent/internal/renderers/ALLOWLIST.md: rows only for a new exec (the keepalived rows exist)
  - SY1 apps/api/src/auth/route-guard.test.ts: the test calls every route without credentials and fails on any unlisted `@Public()` route. If the peer-facing sync endpoint authenticates with the cluster key + pinned certificate instead of a user token, add it to `PUBLIC` under your anchor and prove in an e2e test that a request without the cluster key gets 401 problem+json. If it uses a service user's API key, no entry is needed (prep-rest critic)
contract: commit `contract(schema): ha keepalived + syncExclude` and `contract(proto): ha leaves + VrrpState + VRRP event` as separate commits on YOUR branch first (the ci.sh contract guard checks the subject), plus docs/status/tasks/F-vrrp-config-sync-contract.md; tell the manager in the questions file and keep building. No own branches (the prompt's old `contract/F-vrrp-config-sync` wording is void)
files you must not touch:
  - everything else
  - never: /root/ngfw (main), other worktrees, /etc (incl. /etc/keepalived, /etc/vpp), /root/vpp; keepalived.service
  - apps/agent/binapi (P04/manager-owned), tools/lab, tools/ci.sh, plan/tasks.yaml, docs/decisions/LOG.md, docs/status/PROGRESS.md, packages/ui-kit/**
  - apps/agent/internal/lcpmap/**, apps/agent/internal/descriptors/lcp/**, apps/agent/internal/renderers/frr/** (P12 — read-only), apps/agent/internal/descriptors/{df7,bfd}/** (shared helpers / F-bfd-redistribution)
  - apps/api/src/{commit,secrets,auth,users,datastore,db}/**, apps/api/migrations/** (no migration: `kind` is a text column; a real need → questions file, the manager numbers migrations)
  - F-ha-state-sync's future files (descriptors/hasync/**, features/ha-state-sync/**, domains/system/ha-state-sync/**)
  - agent core (A5): apps/agent/internal/agent/{agent,service,state,ifstate}.go, apps/agent/cmd/**; apps/agent/internal/vpp/ifsanitize/** (TD-3)
host rules:
  - V22b (D-087, D-090 (3)): VPP vrrp host tests crashed the shared VPP on the packet path → every VPP-engine host step is opt-in (`VRX_FVRRP_HOST=1`), runs ALONE in a manager window (VPP otherwise idle), with `systemctl show vpp -p NRestarts` before and after (pasted); outside a window the VPP engine is covered by fakes and the keepalived path
  - failover fixture: write the design with options in the questions file before the first host run — (a) VPP VR (priority 200) on host-w<SLOT>l0 vs a keepalived VR (priority 100) in ns-w<SLOT>-lan on the same segment, pinging the VIP from ns-w<SLOT>-wan (no extra af_packet); (b) two VPP VRs on two slot af_packet interfaces joined by a slot Linux bridge. Default (a): fewer af_packet objects (V24)
  - V19 SAFETY (D-095): before ANY packet through the rig, `go -C apps/agent run ./cmd/vrx-vpp-preflight` must exit 0 (TD-3)
  - D-101: bring the veth down before any af_packet delete; af_packet rings per D-113
  - node B processes (API, agent) are yours: start them on the slot-derived ports above, stop them by PID; never ports 3000/8080/9101
  - with VRX_INTEGRATION=1, one Go package at a time; hold `flock -s` on the lab lock only during a run (D-094)
coordination: P12 (merged) owns LCP pairs and the mapper · F-ha-state-sync (after you) adds the state-sync panel through your registry and appends to `Domains["ha"]` · F-bfd-redistribution owns BFD-driven tracking · F-backup-restore / F-aaa must not be broken by the `cluster-sync` revision kind (say so in the contract note)
evidence: Playwright is not installed. Take the screenshots (en + fa/RTL: VRRP list with role chips, cluster view with revision per node) with the headless Chrome approach from P07a/P07b/P08 (`test/topology/interfaces/shots_test.go`, kept outside the product code), and say so. Record which failover step ran in a manager window and which was skipped with the reason
time box: 24 h — when exceeded: stop, commit WIP, write docs/status/tasks/F-vrrp-config-sync.md with what is left (VRRP both engines first, config sync next, cluster UI last — D-099: a leftover becomes a follow-up row)
WIP: commit at least every 45 min; keep docs/status/tasks/F-vrrp-config-sync-wip.md current
CI: `TMPDIR=/tmp/g-w<SLOT> tools/ci.sh --base main` — short TMPDIR (unix socket paths ≤ 108 chars); no host-wide CI lock: golangci-lint serializes itself since main fc0fe68 (D-106 rejected serialising whole gates). Ports 3000/8080/9101 and /run/vrx/agent.sock belong to the running product stack (tools/app) — never touch them
finish: `tools/ci.sh --base main` green in the worktree · docs/status/tasks/F-vrrp-config-sync.md with pasted real output · everything committed · final message = 10-line summary (branch, last commit, CI result, evidence, open questions, decisions taken with options)
cleanup: stop every process you started (both API/agent pairs, vite, keepalived), by PID · lab lock released · vrx_w<SLOT> and vrx_w<SLOT>b dropped · Valkey keys `vrx:w<SLOT>b:*` removed · no w<SLOT>/w<SLOT>b VRs left in VPP (`show vrrp vr` pasted) · rendered files and TLS material under /run/vrx-test/w<SLOT>/ removed · rig down · dist/ and apps/agent/bin removed
questions: docs/status/tasks/F-vrrp-config-sync-questions.md — write and keep going; never wait for a human
never: merge · restart/kill VPP · Docker · pkill · secrets in files · edit files you do not own
