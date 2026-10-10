# T1 verification — API native-library package dependencies

Date: 2026-10-10. Independent tester `/root/host_37`; no slot required; no target operations. Source under review `2045ab8b3d2f477bb23446fb5e58b7d9d3abea3c`; final [PR217](https://github.com/mcoder1001-cyber/NGFW/pull/217) SHA `5bd7e8b545fc765fd2babd8dda15175d6f33af1b`, mandatory quick [run38033766837/job114159912573](https://github.com/mcoder1001-cyber/NGFW/actions/runs/38033766837/job/114159912573) completed success. Lightweight unchanged fixtures ran at own checkout `86a2f06eafc6406e3e5395769d923a09bbfa60aa`, with test-file and full product/CI tree identity against final reviewed source confirmed by git diff. Actual hosted checkout `8a15d644c53cc3ef4abde339efd3b2a0331221a5`, exactly the independently verified PR integration tree. Earlier bde83bc/run38033113750 was superseded/canceled after a documentation-only amendment and is not counted as PASS or as a product-test failure.

| Scenario | Expected | Observed | Result |
|---|---|---|---|
| API source dependency correction | API consumes generated shlibs; all other control fields identical | Exact source/parent assertion succeeds | PASS |
| Existing packaging fixtures | Real permissions/idempotency/unit constraints remain valid | 16 tests pass | PASS |
| Existing PPPoE package receipts | Attested helper bytes preserved | 4 tests pass | PASS |
| Slot scheme | No resource collisions | 964 ports,32 id ranges valid | PASS |
| Fixed actual API archive | Generated libc6/libgcc-s1 appear in Depends | Final archive full-read; metadata/native-content/SHA256 regression assertion passes | PASS |
| Git integration candidate tree | Merge of current main and final head has exactly the reviewed tree | Remote Git tree equals local final-head tree; both parents verified | PASS |
| Latest-head hosted packaging fixtures | Completed job and actual no-skip fixture output | Run38033766825 success;7+81 tests OK | PASS |
| Latest-head hosted provisioning fixtures | Completed job and actual fixed-suite output | Run38033766876 success;46+23+11+18 tests OK | PASS |
| Final PR unchanged mandatory quick | Full job success and `CI GATE PASSED`, exact final source/integration SHA | Exact-head run completed success; actual checkout8a15d644 and literal final gate output read back | PASS |
| Target installation, reboot and hardware acceptance | Healthy storage and protected management prerequisite | Offline filesystem recovery still required; no target operations | NOT RUN |

Actual output from commands run in own worktree:

```text
$ python3 deploy/debian/ngfw/tests/test_packaging.py
Ran 16 tests in 1.195s
OK
$ python3 deploy/debian/ngfw/tests/test_pppoe_assets.py
Ran 4 tests in 0.223s
OK
$ python3 tools/slot-check.py
ok: 30 developer slots + CI slot 12; 964 ports, 32 id ranges, no collision; tools/lab env verified for 31 slots
PASS source 2045ab8b3d2f477bb23446fb5e58b7d9d3abea3c: sole control change consumes API shlibs:Depends; all other stanzas identical
```

Independent actual final archive regression probe output (`dpkg-deb -f/-c` and Python assertion/SHA256 in own worktree):

```text
Package: ngfw-api
Version: 0.1.0~dev+2045ab8b3d2f
Architecture: amd64
Depends: libc6 (>= 2.34), libgcc-s1 (>= 4.2), nodejs (>= 22), nodejs (<< 23), adduser, python3
sha256 47683ec5b2194ea360c656648718f48943cb04480717cf59804e5ff6ec6309a0
PASS final archive: libc6/libgcc-s1 declared; original archive reproduces missing dependencies; native x64 Argon2 present; archive full contents readable
```

Archive: `/dev/shm/ngfw-api_0.1.0~dev+2045ab8b3d2f_amd64.deb`, preserved by manager in `/root/Documents/Codex/2026-10-10/hardware/runtime-fixed/`. Original archive reproducer: `/root/Documents/Codex/2026-10-10/hardware/runtime/ngfw-api_0.1.0~dev+cc80be66edcf_amd64.deb`. Final product/source identity checked with `git diff --exit-code 2045ab8 5bd7e8b -- . ':!docs/status/tasks/hardware-manager-20261010*'`, exit0/no output. Manager evidence appendix's API SHA256 was read independently and matches this actual archive hash.

Independent Git-tree assertion uses `git rev-parse 5bd7e8b^{tree}` and `gh api repos/mcoder1001-cyber/NGFW/git/commits/8a15d644c53cc3ef4abde339efd3b2a0331221a5`, asserting equal tree IDs and exact expected parent SHAs:

```text
PASS final PR head tree == candidate integration tree a0d7b7cbc7f37dc2fadd9a93d41ce486dcda50d2
Integration commit 8a15d644c53cc3ef4abde339efd3b2a0331221a5 parents d2d55984d74fa1d06c32e8271886f11f16375407, 5bd7e8b545fc765fd2babd8dda15175d6f33af1b
```

Independently read `gh run view <id> --repo mcoder1001-cyber/NGFW --json headSha,event,status,conclusion` and `--log` for latest-head [packaging38033766825](https://github.com/mcoder1001-cyber/NGFW/actions/runs/38033766825) and [provisioning38033766876](https://github.com/mcoder1001-cyber/NGFW/actions/runs/38033766876). Both metadata reports `headSha=5bd7e8b545fc765fd2babd8dda15175d6f33af1b`, `event=pull_request`, `status=completed`, `conclusion=success`. Actual log excerpts:

```text
RUN 38033766825
Verify strict gate exit policy: Ran 7 tests in 0.002s
OK
Run all fixtures without skips: Ran 81 tests in 9.514s
OK
RUN 38033766876
Run all fixed suites without skips: Ran 46 tests in 33.909s
OK
Validate offline Debian bundle fixtures: Ran 23 tests in 10.313s
OK
Validate trusted offline installer fixtures: Ran 11 tests in 8.019s
OK
Validate portable Debian export fixtures: Ran 18 tests in 34.868s
OK
```

These fixture results were not substituted for mandatory quick run38033766837. The required complete gate has now independently completed success on the same final source SHA.

## Completed mandatory quick evidence

Commands ran in this independent tester's own worktree: `gh api repos/mcoder1001-cyber/NGFW/actions/jobs/114159912573/logs` (captured into root-only `/tmp/hardware-37-20261010-ci/job-114159912573.log`) and `gh run view 38033766837 --repo mcoder1001-cyber/NGFW --json headSha,status,conclusion,jobs,url`. Actual REST result: `REST job log download exit=0 bytes=60874`. Actual metadata: `headSha=5bd7e8b545fc765fd2babd8dda15175d6f33af1b`, `status=completed`, `conclusion=success`; job114159912573 and every step success, completion `2026-10-10T07:36:18Z`.

Actual log excerpts, preserving checkout identity and complete gate summary:

```text
2026-10-10T07:15:22.6958486Z HEAD is now at 8a15d644 Merge 5bd7e8b545fc765fd2babd8dda15175d6f33af1b into d2d55984d74fa1d06c32e8271886f11f16375407
2026-10-10T07:15:22.7055712Z 8a15d644c53cc3ef4abde339efd3b2a0331221a5
2026-10-10T07:15:59.4316703Z branch    HEAD @ 8a15d644   (base: origin/main)
2026-10-10T07:25:54.8326964Z Tasks:    35 successful, 35 total Cached:    6 cached, 35 total Time:    8m50.903s  
2026-10-10T07:36:11.4197063Z apply-startup harness: green (2 shards; 149 checks passed in the parallel run)
2026-10-10T07:36:11.4209437Z == summary (quick) ==
2026-10-10T07:36:11.4210542Z   contract guard: HEAD vs origin/main                0m00s
2026-10-10T07:36:11.4211397Z   tools (golangci-lint, gitleaks)                    0m01s
2026-10-10T07:36:11.4212208Z   install (pnpm --frozen-lockfile --prefer-offline)   0m05s
2026-10-10T07:36:11.4212843Z   generate + generated-output gate                   0m53s
2026-10-10T07:36:11.4213323Z   forbidden patterns (+ gitleaks)                    0m03s
2026-10-10T07:36:11.4214117Z   packet-trace ban on the shared VPP (D-128)         0m01s
2026-10-10T07:36:11.4214647Z   ip classify reset after every shell interface create (D-185)   0m01s
2026-10-10T07:36:11.4215180Z   slot resource scheme (1..32, no collisions)        0m00s
2026-10-10T07:36:11.4215897Z   lint · typecheck · unit tests · build (turbo)   8m51s
2026-10-10T07:36:11.4216361Z   apps/agent: make lint test build                   7m52s
2026-10-10T07:36:11.4216806Z   apps/cli: make lint test build                     0m13s
…
2026-10-10T07:36:11.4227158Z   deploy/vpp: shellcheck + apply-startup fake-host harness   1m43s
…
2026-10-10T07:36:11.4241589Z CI GATE PASSED
```

The omitted test/Go-modules summary line reports unit-mode vet/tests/build across every listed test module in0m29s; integration is not run by this quick gate. All gates and fixtures are unchanged from the reviewed source; no test option or workflow was weakened.

No complete local quick gate was run: manager envelope directs hosted final-HEAD evidence and prohibits duplicating a broad build on the nearly full root disk. Completed exact final-head hosted gate and its real output were independently obtained instead. This PASS applies to the package/source correction, not target installation or physical data forwarding.

Verdict: **PASS** on final source `5bd7e8b545fc765fd2babd8dda15175d6f33af1b`, actual hosted integration checkout `8a15d644c53cc3ef4abde339efd3b2a0331221a5`, tree `a0d7b7cbc7f37dc2fadd9a93d41ce486dcda50d2`. No failing test was waived. Hardware installation/forwarding/reboot acceptance remains NOT RUN pending offline disk recovery.

## Post-merge main verification — gate pending

Manager extended T1 after merging PR217. Independent `gh api repos/mcoder1001-cyber/NGFW/git/commits/4908716b4501312102382e6979b8fc1ded6f9311`, `git rev-parse 5bd7e8b^{tree}` and `git ls-remote origin refs/heads/main` assertions completed successfully in this tester's own worktree. Actual output:

```text
PASS actual remote main 4908716b4501312102382e6979b8fc1ded6f9311 tree a0d7b7cbc7f37dc2fadd9a93d41ce486dcda50d2 equals reviewed/tested integration tree
Parents d2d55984d74fa1d06c32e8271886f11f16375407, 5bd7e8b545fc765fd2babd8dda15175d6f33af1b
```

PR217 metadata independently reports `state=MERGED`, expected `headRefOid=5bd7e8b545fc765fd2babd8dda15175d6f33af1b`, actual `mergeCommit=4908716b4501312102382e6979b8fc1ded6f9311`, `mergedAt=2026-10-10T07:46:33Z`.

`gh run view 38035583209 --repo mcoder1001-cyber/NGFW --json headSha,status,conclusion,event,jobs,url` independently reports main push `headSha=4908716b4501312102382e6979b8fc1ded6f9311`, `status=in_progress`, empty conclusion; [Mandatory quick gate job114165198295](https://github.com/mcoder1001-cyber/NGFW/actions/runs/38035583209/job/114165198295) is executing `Run repository gate`. Its workflow/source is the unchanged reviewed tree. Tree/parent identity is PASS; completed post-merge main CI is **pending**, not yet PASS. Keep PR PASS distinct from this new run. Next: retrieve completed actual job log, verify main checkout and complete quick summary/literal gate pass, then publish the final receipt. No target operations.
