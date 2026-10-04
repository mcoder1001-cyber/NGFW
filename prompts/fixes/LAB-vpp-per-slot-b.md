# LAB-vpp-per-slot-b — tests stop hard-coding the shared VPP's sockets (migration half of LAB-vpp-per-slot)
Source: REVIEW-2026-09-24 item **6.5** ("tests that hard-code /run/vpp/*.sock migrate"; `docs/status/review-2026-09-24-verdicts.md` row 99,
D-125) and `docs/decisions/PENDING-vpp-host-hardening.md` option F. Split by M-prompts: **-a** built `tools/lab vpp up|down|status` and the
`tools/lab env` exports `NGFW_VPP_API_SOCKET`, `NGFW_VPP_CLI_SOCKET`, `NGFW_VPP_STATS_SOCKET`, `NGFW_AGENT_VPP_{API,STATS}_SOCKET`, `NGFW_VPPCTL`
(read `docs/status/tasks/LAB-vpp-per-slot-a.md` and its § in `docs/lab/shared-host-rules.md` first). This row makes every integration and
topology test follow those variables, defaulting to today's `/run/vpp/*` when they are unset (behaviour on the shared VPP unchanged).
## Do
1. **One helper**: `apps/agent/internal/vpp/vpptest/vpptest.go` gains `APISocket()`, `StatsSocket()`, `CLISocket()` and a `VPPCtl(ctx, args…)`
   exec helper (`vppctl -s <CLISocket>`, `timeout`-bounded) reading the variables above; `natcommon/nattest/nattest.go` and
   `descriptors/df6/df6test/host.go` (already env-aware) delegate to it. Unit test: unset → `/run/vpp/*`, set → the slot paths.
2. **Socket constants** → the helper, in the files `grep -rln "/run/vpp/" --include=*_test.go apps test` lists at your base (24 today: the
   agent integration tests in `internal/{agent,actions/vrf-static-ecmp,promexport,subsystems}` and `descriptors/{core,svs}`,
   `test/integration/smoke`, and 14 `test/topology/*` harnesses). **Not** these three, whose `/run/vpp/` strings are rendered or fake
   paths, not connections: `descriptors/memif/memif_test.go`, `renderers/vppstartup/render_test.go`, `cmd/ngfw-startupgen/main_test.go`.
3. **vppctl calls** → `vpptest.VPPCtl` (Go) or `${NGFW_VPPCTL:-vppctl}` (shell) in the files `grep -rln '"vppctl"' test apps/agent --include=*.go`
   and `grep -rln 'vppctl ' test/topology --include=*.sh` list (25 Go + 5 shell today). Harnesses that start `ngfw-agent` with a clean
   environment pass `NGFW_AGENT_VPP_API_SOCKET`/`NGFW_AGENT_VPP_STATS_SOCKET` through (as they already pass `NGFW_VPP_TABLE_BASE`).
4. **Exclusions owned by other open rows** (do not edit; list them in your status file as follow-ups): `test/topology/ipsec/run.sh`
   (P11-host), and — unless their rows have merged when you start (then include them) — `test/topology/det44/helpers_test.go`
   (F-det44-cnat-fix) and `descriptors/lcp/mfibguard_integration_test.go` (TD-lcp-leftover-local-path); likewise any other listed file
   that a row still open when you start owns (check `files_owned` on the board; e.g. new harnesses of the -host rows).
5. **Evidence** (host row, your slot, under `flock -s /run/lock/ngfw-lab.lock`, one package at a time): `go vet` of every touched package;
   `grep` before/after (only helper defaults + the three exclusions left); three migrated packages — `internal/descriptors/core`,
   `test/topology/interfaces` (with `tools/lab rig up w<N>`), `internal/promexport` — run once against the shared VPP (variables unset) and
   once against your slot VPP (`tools/lab vpp up <N>`, `eval "$(tools/lab env <N>)"`), both green; NRestarts of `vpp.service` before/after
   unchanged; `tools/lab vpp down <N>` at the end. Evidence `.txt` under `docs/status/tasks/LAB-vpp-per-slot-b-evidence/` (D-175).
## Out of scope
Changing what any test asserts or which objects it creates; `tools/lab`, `tools/ci.sh`, the renderer (-a / the manager); running the whole
integration suite (D-210); new tests beyond the helper's unit test; the shared VPP's configuration.
## Rules
- Files you own: `apps/agent/internal/vpp/vpptest/vpptest.go` (+ `_test.go`), `apps/agent/internal/descriptors/natcommon/nattest/nattest.go`,
  `apps/agent/internal/descriptors/df6/df6test/host.go`, and the socket/vppctl hunks of the files the three greps above list (minus the
  exclusions) — paste the final file list in your status file; `docs/status/tasks/LAB-vpp-per-slot-b*`. Everything else read-only; a needed
  edit elsewhere → `docs/status/tasks/LAB-vpp-per-slot-b-questions.md`.
- Shared VPP: never restart or kill it; prefixed objects only; `timeout 10` on every vppctl; packet trace banned (D-128). Stop the slot VPP
  and every process you started before finishing.
- D-210a: the helper's unit test and the three packages above pass (paste output); no full suite, no lint, no other packages' tests; run them
  through `tools/heavy.sh` (D-224), e.g. `../../tools/heavy.sh go test ./internal/vpp/vpptest/...` from apps/agent.
- No contract change.
- Finish: `tools/ci-slot.sh --base main` green (compile-only gate, D-220/D-222; never tools/ci.sh directly), commit on your branch,
  `docs/status/tasks/LAB-vpp-per-slot-b.md` with pasted real output.
