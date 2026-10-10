# T1 verification — API native-library package dependencies

Date: 2026-10-10. Independent tester `/root/host_37`; no slot required; no target operations. Source under review `2045ab8b3d2f477bb23446fb5e58b7d9d3abea3c`; final PR SHA/run still pending. Lightweight unchanged fixtures ran at own checkout `86a2f06eafc6406e3e5395769d923a09bbfa60aa`, with test-file identity against reviewed source confirmed by git diff.

| Scenario | Expected | Observed | Result |
|---|---|---|---|
| API source dependency correction | API consumes generated shlibs; all other control fields identical | Exact source/parent assertion succeeds | PASS |
| Existing packaging fixtures | Real permissions/idempotency/unit constraints remain valid | 16 tests pass | PASS |
| Existing PPPoE package receipts | Attested helper bytes preserved | 4 tests pass | PASS |
| Slot scheme | No resource collisions | 964 ports,32 id ranges valid | PASS |
| Fixed actual API archive | Generated libc6/libgcc-s1 appear in Depends | Fixed dpkg build still running | PENDING |
| Final PR unchanged mandatory quick | Full job success and `CI GATE PASSED`, exact final source/integration SHA | Final PR/run not yet available | PENDING |
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

No complete local quick gate was run: manager envelope directs hosted final-HEAD evidence and prohibits duplicating a broad build on the nearly full root disk. Next: independently inspect final fixed archive native dependency metadata, confirm final PR/source tree, retrieve completed mandatory hosted quick run and its real output before revising the verdict. No prior green run is substituted for the pending final-SHA gate.

Verdict: **BLOCKED-ENV — awaiting final artifact and mandatory hosted quick completion**. No failing test is being waived or reported as PASS.
