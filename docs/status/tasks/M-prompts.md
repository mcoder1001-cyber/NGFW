# Recovery note — 2026-10-04

The historical report below preserves previous commands and outcomes. Its historical PASS/BLOCKED claims do not certify current main or current host readiness. This recovery adapts names to NGFW; current results and deferred acceptance are recorded in eight-review-recovery-20261004.md.

# M-prompts — missing task prompts (MANAGER-PROMPT §6; docs only)

Branch `task/M-prompts` (slot 4, base main@55a18e0f). Row list: `prompts/ops/M-prompts.md` (16 rows). No product code, no board edit,
no existing prompt edited. Every prompt was written from its template after reading the row's board fields, the cited review /
questions / answer files and decisions, and the code on main (forks of this worker drafted four groups in parallel in this
worktree; every file was reviewed and corrected here, citations spot-checked, see "How verified").

## What

26 prompt files for the 16 rows. Four rows are proposed as splits (estimate check: > 10 h, or clearly larger than `est_hours`):

| id | prompt | files_owned (globs) | est h | host row? | web/ui-kit? | daemon-owner | deps found missing / board fixes |
|---|---|---|---|---|---|---|---|
| F-bruteforce-detectors-a | prompts/features/F-bruteforce-detectors-a.md | `packages/proto/ngfw/v1/dataplane.proto` (EventKind line, contract commit) + regenerated stubs · `docs/status/wave-BC-numbers.md` (own §) · `docs/contracts/proto.md` (own §) · `apps/agent/internal/detectors/**` · `apps/agent/internal/desired/autoblock*.go` · `apps/agent/internal/subsystems/autoblock*.go` · `apps/agent/internal/agent/rpc_events_autoblock*.go` · anchored lines in `apps/agent/internal/subsystems/subsystems.go` (Domains + register), `apps/agent/internal/agent/projection.go`, `apps/agent/internal/subsystems/reachability_test.go` · `apps/api/src/features/auto-block/**` · `test/topology/autoblock/**` · `docs/user/security/auto-block.md` · `docs/status/tasks/F-bruteforce-detectors-a*` | 9 (split of 8) | N (netns, slot nft, test sshd; Y only if a live charon step is granted) | N | none (strongswan optional) | EventKind **18** proposed — manager confirms in the envelope; the agent has no `security` domain yet (row adds it) |
| F-bruteforce-detectors-b | prompts/features/F-bruteforce-detectors-b.md | `apps/api/src/features/auto-block/**` · anchored hunks in `apps/api/src/agent/agent.client.ts`, `apps/api/src/testing/fake-agent.ts` · own proto section + regen · own §§ of `docs/status/wave-BC-numbers.md`, `docs/contracts/proto.md` · `apps/agent/internal/agent/rpc_autoblock*.go` · `apps/agent/internal/subsystems/autoblock*.go` · `apps/agent/internal/desired/autoblock*.go` · anchored hunks in `apps/agent/internal/desired/global_blocking.go`, `apps/agent/internal/agent/projection.go` · `test/topology/autoblock/**` · `docs/user/security/auto-block.md` · `docs/status/tasks/F-bruteforce-detectors-b*` | 6 (split) | Y (af_packet rig, `_gb.*` on VPP) | N | none | dep F-bruteforce-detectors-a; `AutoBlockSync` RPC names/numbers proposed by the worker, confirmed in the envelope; takes over TD-H2 |
| F-igp-followups-a | prompts/features/F-igp-followups-a.md | `packages/schema/src/domains/ext/{ospf,isis-rip}.ts` · anchored lines in `packages/schema/src/domains/{routing,ha}.ts`, `packages/schema/src/semantic/index.ts`, `packages/schema/src/index.ts` · `packages/schema/src/semantic/{ospf,isis-rip,ha}*.ts` · `packages/schema/examples/{ospf6,ripng,isis-auth,vrrp-state,cluster-sync}*.json` · `packages/proto/ngfw/v1/dataplane.proto` (additive) + stubs · `packages/api-client/**` (generated) · `docs/contracts/proto.md` (own §§) · `docs/status/tasks/F-igp-followups-a*` | 4 (split of 16) | N | N | none | — (only numbers already allocated: RoutingConfig 13/14, OspfInterface 9, IsisConfig 6/7, IsisInterface 6/7, RipConfig 6, RipInterface 2, EventKind 17/20/21, `VrrpState`, HaCluster 10) |
| F-igp-followups-b | prompts/features/F-igp-followups-b.md | `apps/agent/internal/renderers/frr/ospf/**` · `apps/agent/internal/renderers/frr/ospf6/**` · `apps/agent/internal/subsystems/ospf.go` · `apps/agent/internal/subsystems/frr.go` (EventOf OSPF cases) · one hunk each in `apps/agent/internal/desired/bgp.go`, `apps/agent/internal/agent/projection.go` · `test/topology/ospf/ospf6*` · `docs/agent/renderers/frr-ospf*.md` · `docs/user/routing/ospf.md` · `docs/status/tasks/F-igp-followups-b*` | 5 | Y (slot frrtest) | N | frr | dep -a |
| F-igp-followups-c | prompts/features/F-igp-followups-c.md | `apps/agent/internal/renderers/frr/{rip,isis,ripng}/**` · `apps/agent/internal/subsystems/isis_rip.go` · `apps/agent/internal/subsystems/frr.go` (EventOf IS-IS case) · one hunk each in `desired/bgp.go`, `agent/projection.go` · `test/topology/ripng/**` · `docs/agent/renderers/frr-{rip,ripng,isis}.md` · `docs/user/routing/isis-rip.md` · `docs/status/tasks/F-igp-followups-c*` | 5 | Y (slot frrtest) | N | frr | deps -a, -b (same FRR daemon + hunks) |
| F-igp-followups-d | prompts/features/F-igp-followups-d.md | `apps/agent/internal/agent/rpc_vrrp_state*.go` · `apps/agent/internal/subsystems/vrrp_events*.go` + one line in `subsystems.go` · `apps/api/src/features/vrrp-config-sync/**` · the `// wave-BC: F-vrrp-config-sync` lines in `apps/api/src/{app.module.ts,agent/agent.client.ts,testing/fake-agent.ts,infra/bus.ts,telemetry/relay.service.ts}` · `packages/api-client/**` (generated) · `docs/user/system/vrrp-config-sync.md` (live-state §) · `docs/contracts/proto.md` (VrrpState §) · `docs/status/tasks/F-igp-followups-d*` | 5 | Y (only in a granted VRRP window) | N | keepalived (optional) | dep -a |
| F-igp-followups-e | prompts/features/F-igp-followups-e.md | `apps/api/src/features/{ospf,isis-rip}/**` · the F-ospf / F-isis-rip anchor lines in `apps/api/src/{app.module.ts,infra/bus.ts,telemetry/relay.service.ts,testing/fake-agent.ts}` · `packages/api-client/**` (generated) · `apps/web/src/domains/routing/{ospf,isis-rip}/**` · `apps/web/src/domains/system/ha/{HaPage.tsx,queries.ts}` (role chips) · `apps/web/src/locales/{en,fa}/{routingIgp,ha}.json` · `docs/user/routing/{ospf,isis-rip}.md` + `docs/user/system/vrrp-config-sync.md` (live-state §§) · `docs/status/tasks/F-igp-followups-e*` | 6 | N | Y | none | deps -a, -b, -c, -d; M-origin-sync (web rule) |
| F-igp-followups-f | prompts/features/F-igp-followups-f.md | `apps/api/src/features/cluster-sync/**` · one line in `apps/api/src/app.module.ts` and per needed set in `apps/api/src/auth/route-guard.test.ts` · `packages/api-client/**` (generated) · `apps/web/src/domains/system/ha/cluster/**` + one mount line in `HaPage.tsx` · `apps/web/src/locales/{en,fa}/ha.json` (cluster keys) · `docs/user/system/vrrp-config-sync.md` (cluster §§) · `docs/status/tasks/F-igp-followups-f*` | 10 | N (two slot APIs, no VPP) | Y | none | deps -a, -e; M-origin-sync; may become its own row `F-config-sync`; security-sensitive (cluster key, TLS pinning) → 2 reviewers |
| F-pim-frrsync | prompts/features/F-pim-frrsync.md | `apps/agent/internal/renderers/frr/pim/**` · `apps/agent/internal/frrsync/pim/**` · `apps/agent/internal/subsystems/pim_frrsync*.go` + one line in `subsystems.go` · `apps/agent/internal/descriptors/mfib/**` (gap-only) · one hunk in `apps/agent/internal/desired/bgp.go` · `test/topology/pim/**` · `docs/agent/renderers/frr-pim.md` · `docs/agent/descriptors/mfib.md` (instance §) · `docs/user/routing/igmp-mfib.md` (PIM §) · `docs/status/tasks/F-pim-frrsync*` | 8 | Y | N | frr | board files_owned must be widened as listed; **no row owns the multicast config wiring** (`desired/igmp_mfib*.go`, IGMP events relay, `mfib.route` registration — owed by the F-igmp-mfib-host CONTINUE) |
| F-multiwan-wiring-a | prompts/features/F-multiwan-wiring-a.md | `apps/agent/internal/subsystems/wanmon/**` · `apps/agent/internal/subsystems/wanmon_register.go` · `apps/agent/internal/desired/multiwan*.go` · `apps/agent/internal/agent/rpc_wan*.go` · one anchored line in `apps/agent/internal/agent/projection.go` · `test/topology/multiwan/**` · `docs/user/network/multi-wan.md` · `docs/status/tasks/F-multiwan-wiring-a*` | 6 (split of 10) | Y | N | none | prober mechanism is a logged worker decision (VPP ping API is unusable in a loop) |
| F-multiwan-wiring-b | prompts/features/F-multiwan-wiring-b.md | `apps/agent/internal/desired/multiwan*.go` · `apps/agent/internal/subsystems/wanmon/**` · one hunk in `apps/agent/internal/desired/rpf_adl_pbr.go` · `packages/schema/src/domains/ext/rpf-adl-pbr.ts` (`wanGroup` only) · `packages/schema/src/semantic/multiwan.ts` · `packages/proto/ngfw/v1/dataplane.proto` (`PbrPath` 5 only) + `pnpm gen` outputs · `docs/status/wave-BC-numbers.md` (own §) · `test/topology/multiwan/**` · `docs/user/network/multi-wan.md` · `docs/status/tasks/F-multiwan-wiring-b*` | 8 (split) | Y (+ nat44 globals window) | N | none | dep -a; `PbrPath` field **5** proposed — manager confirms; contract row → 2 reviewers |
| F-pppoe-client-wiring | prompts/features/F-pppoe-client-wiring.md | `apps/agent/internal/desired/pppoe*.go` · `apps/agent/internal/subsystems/pppoe*.go` · `apps/agent/internal/descriptors/pppoe/**` · `apps/agent/internal/agent/ifstate.go` (pppoe hunk) · `apps/agent/internal/agent/projection.go` (one anchored line) · `apps/agent/internal/subsystems/reachability_test.go` (one line) · `test/topology/pppoe/**` · `docs/user/network/pppoe.md` · `docs/agent/descriptors/pppoe.md` · `docs/status/tasks/F-pppoe-client-wiring*` | 8 | Y (mirror on the shared VPP) | N | none | board files_owned + `docs/agent/descriptors/pppoe.md` + the reachability line; live dial owed until the owner installs `ppp`/`pppoe` (D-168) |
| F-dataplane-apply-flow | prompts/features/F-dataplane-apply-flow.md | `apps/api/src/features/dataplane/apply*` · `apps/api/src/features/dataplane/index.ts` · `apps/api/src/features/dataplane/dataplane.controller.ts` (applyAvailable hunk) + `dataplane.test.ts` · `apps/api/src/auth/route-guard.test.ts` (2 lines) · regenerated only: `packages/api-client/src/generated/**`, `apps/cli/internal/api/operations_gen.go`, `docs/user/cli/reference.md` · `deploy/vpp/apply-executor*` · executor lines in `deploy/debian/build.sh`, `deploy/debian/ngfw/debian/ngfw-agent.{install,tmpfiles}` · `apps/web/src/domains/system/dataplane/Apply*` + `DataplanePage.tsx` (button hunk) · `apps/web/src/locales/{en,fa}/dataplane.json` (`apply.*`) · `apps/agent/cmd/ngfw-startupgen/preview_parity_test.go` (new) · `docs/user/system/dataplane.md` (apply §) · `docs/agent/renderers/vppstartup.md` (one bullet) · `docs/status/tasks/F-dataplane-apply-flow*` | 8 | N (never runs a real apply; preview read-only) | Y | none | add dep **M-origin-sync** (web rule; it edits `apply-startup.sh`); widen files_owned; security + /etc → 2 reviewers |
| F-tunnels-host | prompts/features/F-tunnels-host.md | `test/topology/tunnels/**` · `apps/agent/internal/agent/tunnels_integration_test.go` · `docs/status/tasks/F-tunnels-host*` | 4 | Y | N | none | none; also carries S-tunnels-contract's owed real-engine run (`t1-slot.sh`); en/fa screenshot owed to T4 |
| F-capture-trace-host | prompts/features/F-capture-trace-host.md | `test/topology/capture-trace/**` · `apps/api/test/e2e/capture-trace*` · `apps/agent/internal/agent/capture_integration_test.go` · `docs/status/tasks/F-capture-trace-host*` | 4 | Y | N | none | dep S-capture-retention-stop (on board); the BPF item needs a manager VPP window (launch-queue §3); screenshot owed to T4 |
| S-capture-retention-stop | prompts/fixes/S-capture-retention-stop.md | board globs (`apps/agent/internal/actions/capture-trace/**` · `apps/agent/internal/agent/rpc_capture_trace*.go` · `apps/agent/internal/subsystems/capture_trace*.go` (+ one anchored line) · `apps/api/src/features/capture-trace/**` · `apps/api/src/agent/agent.client.ts` (capture hunk) · `apps/web/src/domains/tools/capture-trace/**` · `apps/web/src/locales/{en,fa}/capture-trace.json` · `docs/contracts/proto.md` (capture §) · `docs/user/tools/capture-trace.md` · `docs/status/tasks/F-capture-trace-contract.md` · `docs/status/tasks/S-capture-retention-stop*`) **+ `packages/api-client/src/generated/schema.d.ts`** (regenerated, `contract(api-client)` commit) | 5 | N | Y | none | add the api-client file to files_owned; R1 #6 already done by S-capture-file-safety (verify only) |
| S-vrrp-product-fixes | prompts/fixes/S-vrrp-product-fixes.md | `apps/agent/internal/descriptors/vrrp/**` · `apps/agent/internal/desired/vrrp.go` + `vrrp_test.go` · `apps/agent/internal/descriptors/core/core.go` (interface-ip Retrieve hook only) · `apps/agent/internal/subsystems/{vrrp,keepalived}.go` · `apps/agent/internal/renderers/keepalived/**` (stage/README only) · `docs/agent/descriptors/vrrp.md` · `docs/agent/renderers/keepalived.md` · `docs/status/tasks/S-vrrp-product-fixes*` | 5 (board 4) | Y (host checks only in a granted VRRP window) | N | keepalived (optional) | board glob `desired/ha_vrrp*.go` matches no file → `desired/vrrp.go`; add the other files listed; F3 stays with PENDING-agent-privileges |
| S-cli-ipsec | prompts/fixes/S-cli-ipsec.md | `apps/cli/internal/cli/cmd_op*.go` · `apps/cli/internal/cli/docs.go` (:110 sentence) · `docs/user/cli/reference.md` (regenerated by `make -C apps/cli docs`) · `docs/status/tasks/S-cli-ipsec*` | 2 | N | N | none | add `docs.go`; `reference.md` is generated (not "ipsec lines"); `Ipsec_sas`/`Ipsec_tunnels` already in `operations_gen.go` |
| TD-19 | prompts/fixes/TD-19.md | `scripts/{00-add-repos,20-install-build,40-install-lab}.sh` · `scripts/tests/td19*` · `tools/lab` (provision functions, now ~:453-564) · `test/topology/ngfw-{b,c}.yml` (`vpp.source` lines) · `docs/status/tasks/TD-19*` | 3.5 | N | N | none | provision hunk moved (board says :505-530); `/srv/ngfw-artifacts/vpp/` must be published before any real `provision --apply` |
| LAB-vpp-per-slot-a | prompts/fixes/LAB-vpp-per-slot-a.md | `tools/lab` (new `cmd_vpp`, dispatch/usage, `cmd_env` exports, vppctl helpers ~:147-160) · `apps/agent/internal/renderers/vppstartup/**` · `apps/agent/cmd/ngfw-startupgen/{main,main_test}.go` · `tools/slot-check.py` (one hunk) · `docs/lab/shared-host-rules.md` (new § only) · `docs/status/tasks/LAB-vpp-per-slot-a*` | 6 (split of 10) | Y (starts one slot VPP) | N | none | board note "parked: needs A+B" is stale (B applied 2026-09-29, A open → cap 2, ceiling 4, MemoryMax 1G, `tools/mem-canary.sh`); shared-host hazard → 2 reviewers; slot-12 CI wiring comes via its questions file |
| LAB-vpp-per-slot-b | prompts/fixes/LAB-vpp-per-slot-b.md | `apps/agent/internal/vpp/vpptest/vpptest.go` (+ test) · `apps/agent/internal/descriptors/natcommon/nattest/nattest.go` · `apps/agent/internal/descriptors/df6/df6test/host.go` · socket/vppctl hunks of the 24 + 30 test files the prompt's greps list (minus files open rows own) · `docs/status/tasks/LAB-vpp-per-slot-b*` | 4 (split) | Y | N | none | dep LAB-vpp-per-slot-a; excludes `test/topology/ipsec/run.sh` (P11-host), `test/topology/det44/helpers_test.go` (F-det44-cnat-fix), `descriptors/lcp/mfibguard_integration_test.go` (TD-lcp-leftover-local-path) while those rows are open |
| TEST-traffic-A | prompts/tests/TEST-traffic-A.md | `test/topology/traffic-a/**` · `docs/status/tasks/TEST-traffic-A*` | 6 | Y (manager window) | N | none | none (all 12 deps merged); check nobody holds nat44-ed (e.g. tools/app) before the EI step |
| TEST-traffic-B | prompts/tests/TEST-traffic-B.md | `test/topology/traffic-b/**` · `docs/status/tasks/TEST-traffic-B*` | 8 | Y (manager window) | N | frr + strongswan + kea (slot instances; the only root-netns zebra) | board deps not merged: F-ikev2-native, F-pki, F-bfd-redistribution; missing: **F-tunnels-host, F-isis-rip-host, F-det44-cnat-fix, P11-host** (phase 1a; P11-pkg held, D-223) |
| TEST-traffic-C | prompts/tests/TEST-traffic-C.md | `test/topology/traffic-c/**` · `docs/status/tasks/TEST-traffic-C*` | 6 | Y (manager window) | N | keepalived (+ frr only for the optional LDP step) | board dep not merged: F-ha-state-sync; missing: **S-vrrp-product-fixes, S-capture-retention-stop, F-capture-trace-host**, F-mpls-ldp-host (optional LDP step) |

Hours: the 16 board rows carry 110.5 h; the prompts need 141.5 h (+31 h: F-igp-followups 16 → 35, F-bruteforce-detectors 8 → 15,
F-multiwan-wiring 10 → 14, S-vrrp-product-fixes 4 → 5). PROGRESS.md's "% by hours" moves accordingly when the manager adds the rows.

### Proposed splits (the manager names the board rows; split ids carry a hyphen because the source ids end in a letter)
- **F-igp-followups** (16 h, board "split at prompt time", D-173) → -a contract 4 h → -b OSPFv3 + OSPF MD5 + EventKind 20, 5 h · -c RIPng,
  rip.version, RIP auth, IS-IS passwords/families, EventKind 21, 5 h (after -b) · -d VRRP live state + events, 5 h (parallel with -b/-c) →
  -e API state routes + grids + tabs + role chips + routing i18n, 6 h (after -a…-d) → -f HA config sync, 10 h (after -e).
- **F-bruteforce-detectors** (8 h) → -a detectors → `observe()`, 9 h → -b API → agent push of the auto-block set to `_gb.*`/`b4_bad` + TD-H2,
  6 h. Reason: nothing on main pushes the API's `auto_block` set to the data plane.
- **F-multiwan-wiring** (10 h) → -a monitor + prober + health-driven default route/ECMP, 6 h → -b per-member SNAT, dead-link clear,
  sticky, ABF member/group pinning (`PbrPath.wan_group`), 8 h.
- **LAB-vpp-per-slot** (10 h) → -a `tools/lab vpp up|down|status` + startupgen lab settings + env exports, 6 h → -b migrate the tests that
  hard-code `/run/vpp/*` and `vppctl`, 4 h.

### Board-field changes for the manager (not applied: workers do not edit the board)
1. Add the 12 split rows above (deps as listed), retire or re-point the four source rows; `prompt:` = the file paths above.
2. files_owned: F-pim-frrsync, S-vrrp-product-fixes, F-pppoe-client-wiring, F-dataplane-apply-flow, S-cli-ipsec, TD-19,
   S-capture-retention-stop — replace with the table's globs.
3. deps: F-dataplane-apply-flow += M-origin-sync; TEST-traffic-B += F-tunnels-host, F-isis-rip-host, F-det44-cnat-fix, P11-host;
   TEST-traffic-C += S-vrrp-product-fixes, S-capture-retention-stop, F-capture-trace-host (+ F-mpls-ldp-host if LDP is wanted).
4. est_hours: S-vrrp-product-fixes 4 → 5; the split rows as listed.
5. LAB-vpp-per-slot note: "parked: needs A+B" is stale (option B applied; A still open; the prompt ships capped).
6. **Narrow F-mpls-ldp-host's `apps/agent/internal/**` (D-174)** before any agent row here runs beside it: it overlaps 15 of the new rows.
7. A missing row for the multicast config wiring (`desired/igmp_mfib*.go` projection, IGMP events relay, `mfib.route` registration).

### Serialisation the manager must keep (from the prompts; overlap check below)
- FRR daemon, one row at a time: F-igp-followups-b → -c, F-pim-frrsync, F-mpls-ldp-host, TEST-traffic-B (they also share `desired/bgp.go` hunks).
- VRRP window rows: F-igp-followups-d, S-vrrp-product-fixes, TEST-traffic-C (VPP VRRP engine alone, launch-queue §3).
- Manager windows with the shared VPP otherwise idle: TEST-traffic-A/B/C; globals windows: F-capture-trace-host (BPF), F-multiwan-wiring-b (nat44).
- Shared files carry one anchored hunk per row (`// wave-BC: <row>`), unioned at merge: `agent/projection.go` (multiwan-a, pppoe, bruteforce-a/-b,
  igp-b/-c), `subsystems/subsystems.go` (bruteforce-a, igp-d, pim), `subsystems/reachability_test.go` (pppoe, bruteforce-a), `agent.client.ts`
  (capture, igp-d, bruteforce-b), `fake-agent.ts` (igp-d/-e, bruteforce-b), `route-guard.test.ts` (dataplane-apply, igp-f), `tools/lab` (TD-19
  provision vs LAB-a vpp/env/helpers), `dataplane.proto` (igp-a, multiwan-b, bruteforce-a/-b, F-pki). Generated outputs (`packages/api-client`,
  `operations_gen.go`, `docs/user/cli/reference.md`) are regenerated by whichever row lands second.

## How verified

### Convention check over every new prompt (scratch script; template placeholders, out-of-scope fence, files you own, rules block)
```
$ python3 <scratchpad>/check_prompts.py
modified tracked files vs main (must be empty): none
  prompts/features/F-bruteforce-detectors-a.md: 105 lines; web-mention=True; PROBLEMS: touches web but no M-origin-sync rule
  prompts/features/F-bruteforce-detectors-b.md: 94 lines; web-mention=True; PROBLEMS: touches web but no M-origin-sync rule
  prompts/features/F-capture-trace-host.md: 76 lines; web-mention=False; OK
  prompts/features/F-dataplane-apply-flow.md: 105 lines; web-mention=True; OK
  prompts/features/F-igp-followups-a.md: 78 lines; web-mention=False; OK
  prompts/features/F-igp-followups-b.md: 73 lines; web-mention=False; PROBLEMS: placeholder <id>
  prompts/features/F-igp-followups-c.md: 71 lines; web-mention=False; PROBLEMS: placeholder <id>
  prompts/features/F-igp-followups-d.md: 69 lines; web-mention=False; OK
  prompts/features/F-igp-followups-e.md: 68 lines; web-mention=True; OK
  prompts/features/F-igp-followups-f.md: 74 lines; web-mention=True; OK
  prompts/features/F-multiwan-wiring-a.md: 105 lines; web-mention=True; PROBLEMS: touches web but no M-origin-sync rule
  prompts/features/F-multiwan-wiring-b.md: 97 lines; web-mention=True; PROBLEMS: touches web but no M-origin-sync rule
  prompts/features/F-pim-frrsync.md: 76 lines; web-mention=False; OK
  prompts/features/F-pppoe-client-wiring.md: 100 lines; web-mention=True; PROBLEMS: touches web but no M-origin-sync rule
  prompts/features/F-tunnels-host.md: 77 lines; web-mention=False; OK
  prompts/fixes/LAB-vpp-per-slot-a.md: 52 lines; web-mention=False; OK
  prompts/fixes/LAB-vpp-per-slot-b.md: 41 lines; web-mention=False; OK
  prompts/fixes/S-capture-retention-stop.md: 73 lines; web-mention=True; OK
  prompts/fixes/S-cli-ipsec.md: 36 lines; web-mention=False; OK
  prompts/fixes/S-vrrp-product-fixes.md: 41 lines; web-mention=False; OK
  prompts/fixes/TD-19.md: 49 lines; web-mention=False; OK
  prompts/tests/TEST-traffic-A.md: 60 lines; web-mention=False; PROBLEMS: placeholder <id>
  prompts/tests/TEST-traffic-B.md: 60 lines; web-mention=False; OK
  prompts/tests/TEST-traffic-C.md: 60 lines; web-mention=False; OK
rc=1
```
The remaining flags are false positives, checked by hand: the five "touches web" files name `apps/web/**`/`packages/ui-kit/**` only in
their Out-of-scope fence (no web file is owned); `<id>` in F-igp-followups-b/-c and TEST-traffic-A is literal syntax
(`ip ospf message-digest-key <id> md5 …`, the `NGFW_TEST_PSK_<id>_<n>` naming rule, `show bridge-domain <id> detail`).

### Ownership overlap check (new rows × new rows that may run in parallel, new rows × open board rows; braces expanded)
```
$ python3 <scratchpad>/overlap2.py   # NEW = the files_owned of the 26 prompts, OPEN = files_owned of every unmerged board row on main
new x new, parallel-capable pairs: 57 shared paths, by kind: {('gen', 'gen'): 16, ('section', 'section'): 8, ('hunk', 'hunk'): 33}
own/own overlaps (both rows own the path outright): none
open rows: F-mpls-ldp-host apps/agent/internal/** overlaps 15 new rows: F-bruteforce-detectors-a, F-bruteforce-detectors-b, F-capture-trace-host, F-igp-followups-b, F-igp-followups-c, F-igp-followups-d, F-multiwan-wiring-a, F-multiwan-wiring-b, F-pim-frrsync, F-pppoe-client-wiring, F-tunnels-host, LAB-vpp-per-slot-a, LAB-vpp-per-slot-b, S-capture-retention-stop, S-vrrp-product-fixes
open rows: F-pki 'packages/proto' (additive proto section) shared with: F-bruteforce-detectors-a, F-bruteforce-detectors-b, F-igp-followups-a, F-multiwan-wiring-b
open rows: any other overlap: none
```
Declared sequential pairs (split deps, S-capture-retention-stop → F-capture-trace-host) are excluded from the first line; every remaining
shared path is an anchored hunk, a per-row section of a shared doc, or a regenerated output.

### Citation spot-check (file:line references the prompts give workers, against main@55a18e0f)
```
auto-block.service.ts:128 observe()                        128
infra/bus.ts onAgentEvent (cited :85-90)                   88
DesiredState.security = 14                                 1064
wanmon/monitor.go Prober (cited :31-35)                    34
binapi/arping exists                                       arping.ba.go arping_rpc.ba.go 
descriptors/vpn/secret.go Resolver :68                     68
desired/vrrp.go:348 AssembleVrrp                           348
descriptors/vrrp/vrrp.go:451 VRDescriptor.Delete           451
dataplane.controller.ts:99 applyAvailable: false           99
cli cmd_op.go:41-48 ipsec stub                             46
operations_gen.go:82-83 Ipsec_sas/tunnels                  82 83 
tools/lab provision ~:453/:525                             453 525 
tools/lab vppctl helpers ~:147-160                         149 159 
seams.go DynamicSource / AddDynamicSource (:228-363)       287 321 
ext/igmp-mfib.ts:127 PimSchema                             127
routing.ts anchors F-ospf :567/:723                        567 723 
capture.go retain :664 / Recover :688 / Delete :804        664 688 804 
subsystems.go:510 Wiring.BootStore()                       510
S-tunnels-contract t1-slot.sh exists                       t1-slot.sh
```

### Branch diff (docs only)
```
$ git diff --stat 55a18e0f c3b8d31d | tail -3
 prompts/tests/TEST-traffic-B.md              |  60 +++++++++
 prompts/tests/TEST-traffic-C.md              |  60 +++++++++
 27 files changed, 1950 insertions(+)
```
27 files, all new: 26 prompt files under prompts/{features,fixes,tests}/ plus this row's status, WIP and envelope files. No tracked file modified.

### CI gate
```
$ TMPDIR=/tmp/g-w4 tools/ci-slot.sh --base main      # run once on c3b8d31d, 17:47:35–17:59:24 UTC
ci-slot: /run/lock/ngfw-ci-2.lock taken after 0s (MemAvailable 14 GB) — running tools/ci.sh --base main
branch    task/M-prompts @ c3b8d31d   (base: main)
WARN TESTS OFF (plan/NO-TESTS, D-210): compile-only gate — typecheck · build · go vet; no lint, no unit/integration tests, no apply-startup harness
== contract guard: HEAD vs main ==
no contract files changed in the 3 commit(s) of HEAD since main (55a18e0f)
…
== summary (quick) ==
  contract guard: HEAD vs main                       0m00s
  tools (golangci-lint, gitleaks)                    0m03s
  install (pnpm --frozen-lockfile --prefer-offline)   1m13s
  generate + generated-output gate                   5m28s
  forbidden patterns (+ gitleaks)                    0m08s
  packet-trace ban on the shared VPP (D-128)         0m01s
  ip classify reset after every shell interface create (D-185)   0m02s
  slot resource scheme (1..32, no collisions)        0m07s
  typecheck · build (turbo — tests OFF)           3m25s
  apps/agent: go build + go vet (tests OFF)          0m50s
  apps/cli: make build (tests OFF)                   0m05s
  test/ Go modules, unit mode (…)                    gofmt ok · go vet ok · go test skipped (tests OFF) for all 21 modules
  warnings: TESTS OFF (D-210) · 3 pre-existing 'ALLOW:' lines in apps/api/src/features/mgmt-tls/mgmt-tls.test.ts · deploy/vpp harness skipped
  mode quick · wall time 11m49s · logs /root/ngfw-wt/logs/ci/M-prompts-20261001-211735-251302

CI GATE PASSED
```
The commit after the gate only fills this status file (docs). `/tmp/g-w4` could not be removed: the permission layer refused the
delete, which is final per the envelope; it is recorded in `docs/status/tasks/M-prompts-questions.md` (1.6 MB, owner/manager action).

## Out of scope / not done
Product code, plan/tasks.yaml, P11-pkg (held, D-223), S6 rows, editing existing prompts or templates. Not run: any test of the rows
(nothing to run for docs). `docs/status/tasks/M-prompts-questions.md` holds one item: the refused `/tmp/g-w4` cleanup.

## Open questions (for the manager)
1. Confirm the proposed numbers in the envelopes: EventKind 18 (F-bruteforce-detectors-a), `PbrPath` 5 (F-multiwan-wiring-b), the
   `AutoBlockSync` RPC (F-bruteforce-detectors-b; the worker writes its own § in wave-BC-numbers.md first).
2. TD-19: the scope says "containerlab from a pinned .deb", 00-CONTEXT forbids containerlab/Docker. The prompt follows 00-CONTEXT
   (remove containerlab, docker.io, docker-compose-v2, the libvirt group; note it in TD-19-questions.md). Confirm or re-scope.
3. HOST-FOLLOWUP-TEMPLATE step 3 still says `flock -x /run/lock/ngfw-lab.lock` for global steps, its own evidence-rules section and D-167
   say "globals lock inside the shared lab lock". The two host prompts here follow D-167; the template itself was not edited (out of scope).
4. F-igp-followups-f: promote to its own row `F-config-sync`? It is security-sensitive (cluster key HMAC, peer TLS pinning) → 2 reviewers.
5. High-risk review classes (second reviewer per MANAGER §2): contract/schema — F-igp-followups-a, F-multiwan-wiring-b,
   F-bruteforce-detectors-a/-b, S-capture-retention-stop (api-client regen); security — F-dataplane-apply-flow, F-bruteforce-detectors-a/-b,
   F-igp-followups-f; shared-host/VPP-global — LAB-vpp-per-slot-a/-b, TEST-traffic-A/B/C, F-capture-trace-host (BPF), F-multiwan-wiring-b (nat44),
   F-igp-followups-d and S-vrrp-product-fixes (VRRP engine window).

## Decisions (for the LOG)
- M-prompts-1: split ids are `<id>-a/-b/…` (the source ids end in a letter, `<id>a` would read as part of the word); rename freely on the board.
- M-prompts-2: every prompt carries D-224 (`tools/heavy.sh`; from apps/agent `../../tools/heavy.sh go test …`) although it landed on main after
  this branch's base, and the D-220/D-222 gate `tools/ci-slot.sh --base main` instead of the templates' `tools/ci.sh --base main`.
- M-prompts-3: host rows' UI screenshots are "owed to T4 on the main stack after merge" (D-175); while `plan/NO-TESTS` exists they are listed as owed.
- M-prompts-4: S-capture-retention-stop prefers an API-side abort of the held Action call for "stop" (no contract change); a `CaptureStop` RPC only if
  the abort cannot work. F-capture-trace-host keeps captures ≤ 30 s on the slot rig interface (VPP holds one pcap capture for the host).
- M-prompts-5: F-tunnels-host bans creating L2TPv3 tunnels on the shared VPP (VPP 26.06 cannot delete them) and adopts S-tunnels-contract's owed driver.
- M-prompts-6: LAB-vpp-per-slot-a adds `tools/mem-canary.sh` (D-224) to the start refusals besides the MemAvailable floor.
