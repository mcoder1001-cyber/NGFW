# T1 verification — API native-library package dependencies

Date: 2026-10-10. Independent tester `/root/host_37`; no slot required; no target operations. Source under review `2045ab8b3d2f477bb23446fb5e58b7d9d3abea3c`; final [PR217](https://github.com/mcoder1001-cyber/NGFW/pull/217) SHA `bde83bc87ae817a61bbc70e4029f76109ae77c35`, mandatory quick [run38033113750/job114157962736](https://github.com/mcoder1001-cyber/NGFW/actions/runs/38033113750/job/114157962736) pending completion. Lightweight unchanged fixtures ran at own checkout `86a2f06eafc6406e3e5395769d923a09bbfa60aa`, with test-file and full product/CI tree identity against final reviewed source confirmed by git diff. PR integration candidate readback `883838639ef92a0ea224012ca89254f3ec166334`.

| Scenario | Expected | Observed | Result |
|---|---|---|---|
| API source dependency correction | API consumes generated shlibs; all other control fields identical | Exact source/parent assertion succeeds | PASS |
| Existing packaging fixtures | Real permissions/idempotency/unit constraints remain valid | 16 tests pass | PASS |
| Existing PPPoE package receipts | Attested helper bytes preserved | 4 tests pass | PASS |
| Slot scheme | No resource collisions | 964 ports,32 id ranges valid | PASS |
| Fixed actual API archive | Generated libc6/libgcc-s1 appear in Depends | Final archive full-read; metadata/native-content/SHA256 regression assertion passes | PASS |
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

Archive: `/dev/shm/ngfw-api_0.1.0~dev+2045ab8b3d2f_amd64.deb`. Original archive reproducer: `/root/Documents/Codex/2026-10-10/hardware/runtime/ngfw-api_0.1.0~dev+cc80be66edcf_amd64.deb`. Final product/source identity checked with `git diff --exit-code 2045ab8 bde83bc -- . ':!docs/status/tasks/hardware-manager-20261010-review-plan.md' ':!docs/status/tasks/hardware-manager-20261010-wip.md'`, exit0/no output.

No complete local quick gate was run: manager envelope directs hosted final-HEAD evidence and prohibits duplicating a broad build on the nearly full root disk. Next: retrieve completed mandatory hosted quick run and its real output before revising the verdict. No prior green run is substituted for the pending final-SHA gate.

Verdict: **BLOCKED-ENV — awaiting mandatory hosted quick completion**. No failing test is being waived or reported as PASS.
