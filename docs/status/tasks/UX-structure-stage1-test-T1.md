# UX-structure-stage1 — independent final T1 evidence

- Tester branch: `codex/ux-structure-t1-20261007`; worktree `/root/ngfw-wt/ux-structure-t1-20261007`.
- Immutable product source tested: `f6d8315b6f290aa32de488820308b213d7e13657`.
- Date: 2026-10-07 UTC; no slot, live integration, host-service changes, or product/test/tooling edits.
- Owned file: this report only. Product scope/envelope: `UX-structure-stage1.envelope.md`.
- Local/remote checkpoint: this report commit; publication result recorded by the manager from successful push output. Product source remains the SHA above.

## Commands and evidence

Unchanged complete quick gate, using the hosted scheduling bound and no disabled checks:

```text
NGFW_CI_TASK_CONCURRENCY=2 tools/ci.sh quick --base origin/main
```

First stdout log: `/root/ngfw-wt/logs/ux-structure-final-T1-20261007.log`.
Step logs: `/root/ngfw-wt/logs/ci/ux-structure-t1-20261007-20261007-162353-3203517/`.

```text
no contract files changed in the 6 commit(s) of HEAD since origin/main (19052bb13)
Tasks:    35 successful, 35 total Cached:    24 cached, 35 total Time:    7m56.881s
Test Files  109 passed (109)
     Tests  644 passed (644)
  Duration  396.56s (transform 14.74s, setup 38.30s, collect 295.84s, tests 1050.73s, environment 108.78s, prepare 20.49s)
go vet ./...
0 issues.
go test -race -count=1 ./...
ngfw/agent/internal/actions/ha-state-sync: open /tmp/go-build4047746496/b805/importcfg: no space left on device
ngfw/agent/internal/actions/unbound-chrony-syslog: mkdir /tmp/go-build4047746496/b813/: no space left on device
CI GATE FAILED — apps/agent lint/test/build failed
```

Contract/generated-output/forbidden-pattern/gitleaks/board/slot guards passed. All web lint, typecheck, build and unit checks passed as part of 35/35 Turbo tasks. The complete web suite includes the existing App tests and the new navigation tests.

The first run exited 1 because Go race-test compilation could not allocate temporary files. Immediately after Go cleanup, `df -h /tmp /root` showed `/tmp` 32G total/27G available, root filesystem 197G total/2.6G available. This is observed ENOSPC, not an inferred product assertion failure. Agent build and Go test modules under `test/` were not reached. The gate itself was unchanged.

## Recovery checkpoint

Identical full quick-gate retry is running, stdout `/root/ngfw-wt/logs/ux-structure-final-T1-20261007-retry.log`; no source edits or host cleanup performed. Exact next action: wait for this command, inspect its final status and step logs, then replace this provisional section with the actual aggregate result and publish the report.

| Scenario | Expected | Observed | Status |
|---|---|---|---|
| Complete Turbo lint/typecheck/test/build | All tasks succeed | 35/35; web 109 files, 644 tests pass | PASS |
| Agent vet and lint | No findings | vet succeeds; 0 issues | PASS |
| Full mandatory quick gate | CI GATE PASSED | First run ENOSPC at agent race compile; retry pending | BLOCKED-ENV pending retry |
| Live integration / browser T4 | Separate applicable execution | Not part of T1 quick gate | NOT RUN |

Provisional verdict: BLOCKED-ENV — complete retry pending; this report is not merge approval.


## Second recovery checkpoint

- Provisional report checkpoint `8591e3834` was successfully published to `origin/codex/ux-structure-t1-20261007`.
- Identical complete retry also passed all 35 Turbo tasks, then reproduced two `ra_vpn` readback failures. The unchanged targeted pair failed twice with default `/tmp`.
- Go 1.26 `testing.TempDir` explicitly uses `GOTMPDIR` (`/usr/lib/go-1.26/src/testing/testing.go:1420,1460`), overriding separate secure `TMPDIR`. Both readback tests passed twice when both variables pointed to a root-owned protected directory; no source/test/security-guard edits.
- Complete bounded secure retry (`GOFLAGS=-p=2`, `GOMAXPROCS=4`, same concurrency2) passed Turbo35/35 and `ra_vpn`, but the long hidden `.t1-tmp` path caused Unix socket `bind: invalid argument` and existing SNMP clean-path rejection. Agent, chrony, snmpd, strongswan, unbound and subsystem packages consequently failed. Logs: `/root/ngfw-wt/logs/ux-structure-final-T1-20261007-secure-bounded.log`; step directory `ux-structure-t1-20261007-20261007-164312-3253370`.
- Manager authorized task-owned short secure `/root/ux-t1-20261007` (verified absent before creation; created root-owned0700) for both temp variables. Targeted unchanged affected-package rerun is active, log `/root/ngfw-wt/logs/ux-structure-final-T1-shorttmp-targeted.log`. Exact next action: finish targeted affected-package checks, then at most one final complete unchanged gate if they pass; publish final report regardless of outcome.
- Neither the protected-parent security check nor Unix socket/path guards were weakened. Complete gate still pending; no merge authorization.


## Final result — PASS

The single final complete gate exited 0 with all original checks enabled. Product source is still exactly `f6d8315b6f290aa32de488820308b213d7e13657`; report-only checkpoint HEAD during this run was `94675f651`. Generated/product paths have no diff against that source. Test fixture/temp artifacts were removed only from this worktree and the explicitly authorized task-owned directory.

Exact final command:

```text
TMPDIR=/root/ux-t1-20261007 GOTMPDIR=/root/ux-t1-20261007 GOFLAGS=-p=2 GOMAXPROCS=4 NGFW_CI_TASK_CONCURRENCY=2 tools/ci.sh quick --base origin/main
```

These environment values bound scheduling and provide protected, short fixture paths; they do not disable tests, assertions, race detection, lints, generation, guards, builds, or the unchanged gate.

Full stdout: `/root/ngfw-wt/logs/ux-structure-final-T1-20261007-final.log`.
Step logs: `/root/ngfw-wt/logs/ci/ux-structure-t1-20261007-20261007-165315-3261513/`.

Actual final output:

```text
Tasks:    35 successful, 35 total Cached:    28 cached, 35 total Time:    57.225s
apply-startup harness: green (4 shards; 149 checks passed in the parallel run)
  generate + generated-output gate                   0m57s
  lint · typecheck · unit tests · build (turbo)   0m58s
  apps/agent: make lint test build                   6m45s
  apps/cli: make lint test build                     0m27s
  test/ Go modules, unit mode (...)                  1m59s
  deploy/vpp: shellcheck + apply-startup fake-host harness   5m08s
  mode quick · wall time 16m31s
CI GATE PASSED
```

All 27 Go modules under `test/` passed gofmt, vet and unit execution. Agent vet, golangci-lint (0 issues), unchanged `go test -race -count=1 ./...` and build passed. Existing integration tests intentionally skipped because `NGFW_INTEGRATION` is unset by the quick-gate contract; this does not claim live integration or browser T4 acceptance. Full web suite ran on immutable final product source in the first invocation: 109/109 files and 644/644 tests PASS, including existing App and new navigation tests. Later full gates replayed those successful package caches while regenerating/guarding the tree.

Short protected fixture recovery also independently passed unchanged affected tests twice: `ra_vpn`, chrony, snmpd, strongswan, unbound packages and the five previously failing agent/subsystem cases. Their logs are `/root/ngfw-wt/logs/ux-structure-final-T1-shorttmp-targeted.log` and `ux-structure-final-T1-shorttmp-agent-targeted.log`. The original ENOSPC and long/default-temp failures remain preserved above and in the logs. No production/security/test code was changed to recover.

The final gate warned about only the task-owned `.t1-tmp` leftovers and pre-existing documented control-plane test ALLOW lines. The temp leftovers were removed after completion; final tracked/generated product paths are clean.

| Scenario | Expected | Observed | Status |
|---|---|---|---|
| Final source web suite | All assertions pass | 109 files / 644 tests pass | PASS |
| All workspace lint/typecheck/test/build | Every Turbo task green | 35/35 | PASS |
| Agent/CLI vet/lint/race/build | Every mandatory check succeeds | Both gate steps complete | PASS |
| Test-module formatting/vet/unit | Every module green | 27 modules complete | PASS |
| Startup shell/harness guards | Every unchanged shard green | 149 checks / 4 shards | PASS |
| Complete unchanged quick gate | CI GATE PASSED | exit0,16m31s | PASS |
| Hosted CI / live lab / browser T4 | Separate execution evidence | Not established by this T1 run | NOT VERIFIED |

Final verdict: PASS (T1 quick/unit scope only). No merge or deployment authorized or performed.

Publication: checkpoint `8591e3834` was successfully pushed. Interim `94675f651` publication was rejected twice with GitHub Internal Server Error; final report publication outcome must be taken from actual subsequent push output, not assumed.
