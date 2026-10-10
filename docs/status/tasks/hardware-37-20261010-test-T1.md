# T1 verification — API native-library package dependencies

Date: 2026-10-10. Independent tester `/root/host_37`; no slot required; no target operations. Source under review `2045ab8b3d2f477bb23446fb5e58b7d9d3abea3c`; final [PR217](https://github.com/mcoder1001-cyber/NGFW/pull/217) SHA `5bd7e8b545fc765fd2babd8dda15175d6f33af1b`, mandatory quick [run38033766837/job114159912573](https://github.com/mcoder1001-cyber/NGFW/actions/runs/38033766837/job/114159912573) pending completion. Lightweight unchanged fixtures ran at own checkout `86a2f06eafc6406e3e5395769d923a09bbfa60aa`, with test-file and full product/CI tree identity against final reviewed source confirmed by git diff. PR integration candidate readback `8a15d644c53cc3ef4abde339efd3b2a0331221a5`. Earlier bde83bc/run38033113750 was superseded/canceled after a documentation-only amendment and is not counted as PASS or as a product-test failure.

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
| Final PR unchanged mandatory quick | Full job success and `CI GATE PASSED`, exact final source/integration SHA | Final HEAD verified; repository gate currently running in exact final-HEAD run | PENDING |
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

These fixture results do not substitute for the pending mandatory quick run38033766837, whose metadata still reports `in_progress` and no conclusion.

No complete local quick gate was run: manager envelope directs hosted final-HEAD evidence and prohibits duplicating a broad build on the nearly full root disk. Next: retrieve completed mandatory hosted quick run and its real output before revising the verdict. No prior green run is substituted for the pending final-SHA gate.

Verdict: **BLOCKED-ENV — awaiting mandatory hosted quick completion**. No failing test is being waived or reported as PASS.
