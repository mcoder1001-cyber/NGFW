# Dashboard resume checkpoint — 2026-10-02

Branch: `codex/dashboard-resume-20261002`; worktree: `/workspace/scratch/de92de7d9874/ngfw-dashboard`.
Base: `53a43ce5b91e71f3fedc282f9c5c54ba22fc9fb8`. Local recovered product commit: `6563e1073b76b0e439d19b99731f5715bdf3a838`, tree `d2eaaac3d9877884befbe8560dd8f9505016878b`.
Original remote checkpoint remains unchanged at `origin/codex/dashboard-recovered-20261002` (`148a7cc8`). New branch publication is pending manager authorization verification; a tree object was uploaded, but no new remote commit, branch or PR was created by this worker.

Scope: recover the reviewed dashboard stats source, listener lifecycle, desired-state projection and subsystem wiring, plus existing MPLS Persian settings regression test. No new product changes, no board edits. Squashed inherited delta and rebased cleanly onto current main, retaining CI DAG fix. `git diff --exit-code origin/codex/dashboard-recovered-20261002 HEAD -- apps/agent apps/web docs/user/dashboard` returned 0.

Actual verification with pinned Node 22.23.2, pnpm 12.5.1, Go 1.26.0:

- `go test -race -count=1 ./internal/promexport ./internal/desired -run 'Test(Govpp|Prometheus|Handler)'`: PASS, promexport 1.013s and desired 1.072s. Live VPP test remains opt-in/skipped, not acceptance.
- Broader `go test -race -count=1 ./internal/promexport ./internal/desired ./internal/subsystems -run 'Test(Govpp|Prometheus|Handler|Listener)'`: desired PASS 1.066s; listener-serving, listener-stop and subsystem lifecycle tests BLOCKED at localhost TCP bind with `socket: operation not permitted`. Do not record the broader command as passing.
- Unchanged `bash tools/ci.sh quick --base origin/main` with pinned tools and writable caches: contract guard PASS (no contract changes); tools PASS; frozen dependency install FAILED fetching npm string-width 4.2.3 with network socket `Operation not permitted`. Later quick stages did not run. Full log `/workspace/scratch/de92de7d9874/dashboard-quick.log`; step log in `/workspace/scratch/de92de7d9874/dashboard-ci/ngfw-dashboard-20261002-173440-5`.

Historical panel reports remain in `F-dashboard-prom-alarms-host-review*.md`, `dashboard-review-R1.md`, and `dashboard-delta-review-R1-R6-R7.md`. New integration delta is rebase onto main CI-only changes plus this checkpoint/envelope. Manager should obtain independent integration/evidence review and complete hosted quick on published head. No historical code approval is represented as a new test run.

Remaining: publish new branch/PR without overwriting archived predecessor; hosted unchanged complete quick gate; fresh integration review; manager-owned sequential merge only after gates. All real VPP traffic, alarms/webhooks, API/agent restart and browser acceptance remains NOT RUN/deferred in `docs/status/DEFERRED-ACCEPTANCE.md`.

Exact next local gate command (network/bind-capable runner required): `bash tools/ci.sh quick --base origin/main` using pinned toolchain. Manager owns remote publication and PR creation while external-destination approval is checked.

## Captured verification output

Initial restricted focused source/projection test:

```text
go test -race -count=1 ./internal/promexport ./internal/desired -run 'Test(Govpp|Prometheus|Handler)'
ok  ngfw/agent/internal/promexport  1.013s
ok  ngfw/agent/internal/desired     1.072s
```

Initial restricted broad test failed at localhost binds (excerpt):

```text
--- FAIL: TestListenerServes (0.00s)
    promexport_test.go:144: promexport: listen 127.0.0.1:0: listen tcp 127.0.0.1:0: socket: operation not permitted
--- FAIL: TestListenerStopCancelsActiveRequestAfterGracePeriod (0.00s)
    promexport_test.go:192: promexport: listen 127.0.0.1:0: listen tcp 127.0.0.1:0: socket: operation not permitted
ok  ngfw/agent/internal/desired  1.066s
--- FAIL: TestPrometheusListenerLifecycle (0.00s)
    dashboard_prom_alarms_test.go:22: listen tcp 127.0.0.1:0: socket: operation not permitted
FAIL
```

Initial restricted full quick installation failure (excerpt):

```text
bash tools/ci.sh quick --base origin/main
  × installing dependencies
  ├─▶ Failed to fetch https://registry.npmjs.org/string-width/-/string-width-
  │   4.2.3.tgz: error sending request: client error (Connect): tunnel error:
  │   failed to create underlying connection: tcp open error: Operation not
  │   permitted (os error 1)
```

The same broad focused test was rerun with approved unrestricted socket access, unchanged tests and pinned Go 1.26.0:

```text
go test -race -count=1 ./internal/promexport ./internal/desired ./internal/subsystems -run 'Test(Govpp|Prometheus|Handler|Listener)'
ok  ngfw/agent/internal/promexport  2.071s
ok  ngfw/agent/internal/desired     1.254s
ok  ngfw/agent/internal/subsystems  1.228s
```

`bash tools/ci.sh check --base origin/main` also completed:

```text
ok: gitleaks — scanned ~60050 bytes (60.05 KB) in 253ms no leaks found
ok: no packet trace (trace add / show trace / clear trace / tracedump API) outside docs and the generated bindings
ok: 30 developer slots + CI slot 12; 964 ports, 32 id ranges, no collision; tools/lab env verified for 31 slots
check PASSED (0m03s)
```

Remote publication update: manager's automatic approval review rejected publication to the external GitHub destination; no retry via another transport is authorized. Deliver local checkpoint for concrete fresh publication approval. Unrestricted local testing was separately approved and does not publish code.

Unrestricted complete quick at checkpoint: **RUNNING**, log `/workspace/scratch/de92de7d9874/dashboard-quick-unrestricted.log`. Install, generation (13/13 tasks), generated-output and forbidden-pattern stages passed; full Turbo lint/typecheck/test/build running. This is not full gate PASS. Manager will record final exit/result before any merge.

Publication authorization update: after the review block, the user explicitly confirmed permission in this active conversation; manager authorized connector publication on 2026-10-02. Earlier rejection remains historical evidence. R2 and R7 independent reports are now included; unchanged full local quick is still running and hosted quick remains a merge prerequisite.
