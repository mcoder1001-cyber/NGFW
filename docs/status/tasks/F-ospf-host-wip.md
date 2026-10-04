# F-ospf-host — WIP

- 2026-09-28 19:04 frrtest step done: `apps/agent/internal/renderers/frr/ospf/integration_test.go` (TestOSPFLive) PASS on FRR 10.7.1,
  evidence `docs/status/tasks/F-ospf-host-evidence/frrtest-ospf.txt`, NRestarts 2 → 2.
- 2026-09-28 19:09-19:14 topology test (netns mode) written and run once: commit ok, no adjacency in 90 s, diag added; second
  run could not reach VPP — the shared VPP wedged at ~19:12:27 (main thread 100 %, CLI/API/stats hang). Quota outage 20:50.
- 2026-09-29 07:55 CONTINUE: no w11 leftovers; VPP still wedged (questions.md Q1). API-400 driver `test/topology/ospf/api400.sh`
  run: undefined area → 400 with pointer (done); backbone stub → 503 agent-unavailable (blocked on VPP). Status file written,
  main merged (NO-TESTS), compile-only gate run.
- left (after the manager restarts VPP): `test/topology/ospf/run.sh` in both modes, api400.sh step 2.
