# TD-7 — verify (verifier, slot 6, 2026-09-24)

Branch `task/TD-7` @ 902a935, base `task/TD-6@621d1e2` (D-114/D-116). All checks below ran in fake-host mode
(`VRX_TEST_ROOT` fixture, scratch `TMPDIR=/tmp/g-td7verify`). Nothing under `/etc/vpp` was read or written, VPP was
never touched, no real `--apply` ran.

## Scope
`git diff 621d1e2...task/TD-7 --stat` (621d1e2 is the actual fork point per `TD-7.envelope.md`; `task/TD-6`'s branch
tip has since advanced past it, so diffing against the live branch pointer is misleading and was not used):
```
 deploy/vpp/apply-startup.sh        |  59 ++++++--
 deploy/vpp/test-apply-startup.sh   |  74 +++++++++-
 docs/agent/renderers/vppstartup.md |  40 +++++-
 docs/status/tasks/TD-7-wip.md      |  18 ++
 docs/status/tasks/TD-7.envelope.md |  14 ++
 docs/status/tasks/TD-7.md          | 275 +++++++++
```
Only `deploy/vpp/**`, `docs/agent/renderers/vppstartup.md` and `docs/status/tasks/TD-7*` changed. `tools/ci.sh` is
untouched (confirmed by this diff, not just asserted in `TD-7.md`) — correct, since `test-apply-startup.sh`'s `scen()`
selects scenarios 42/43 by number/modulo, needing no registration.

## F1 — per-apply lock cannot deadlock the dead-man's own rollback
Read `stage_rollback` (`apply-startup.sh:999-1030`), `cancel_deadman` (`:637-640`), `finish_hint` (`:669-681`),
`secure_locks` (`:509-523`), and the fd-hygiene call sites (`tmo`/`svc`/`vppcheck`/`sysfs_write`/`tcp_connect`
at `:192-196`, `:294-296`, and the two `flock -w` in `secure_locks:517`).

- The lock is `exec 7<"$WORK"; flock -n 7` — **non-blocking**, opened fresh per process. A dead-man invocation (via
  the timer or `systemctl start <unit>.service`) that is the *only* rollback of that work dir always acquires it
  immediately: there is no code path where a process can block on, or already hold, its own fd. Self-deadlock is
  architecturally impossible (flock is scoped to the fd/process, not the apply). Verified live: scenarios 32/33/34/35/36
  (single-rollback paths, no contention) all still pass on this branch (12/12 in my own re-run, see below; 19/19 in
  the worker's `VRX_TEST_ONLY="32 40 42 43"` run).
- Ordering in `stage_rollback` is: take lock (:1008-1013) → check `finished` (:1015) → `cancel_deadman` (:1016,
  *before* `touch deadman-fired`) → `secure_locks`/`kill_run`/VPP touched. So the timer is disarmed before VPP is
  touched, satisfying the F1 requirement, and confirmed by scenario 43's own check that the "stop $DM.timer" call
  precedes "stop vpp" in the call log (`line_since_mark`, `c < s`) — reproduced below.
- fd hygiene: every child that can outlive the stage (`timeout`-wrapped `tmo`/`svc`/`vppcheck`/`sysfs_write`/
  `tcp_connect`, and both `flock -x -w … 8/9` waits in `secure_locks:517`) closes fd 7. `log_to_work`'s `tee`
  (`:855`) runs before `exec 7<"$WORK"` (:1008), so it never inherits fd 7 either. No other flock/background/
  systemd-run call sits inside the rollback path (`systemd-run` only appears in `stage_run`, before fd 7 exists).
  A killed rollback (SIGKILL) drops fd 7 with the process, so the lock cannot be stuck held by a dead holder.
- Refusal exits 0 (`return 0` at :1012, propagated through `main "$@"` at the end of the script with nothing after
  the `case` to override it) — matches the documented "second rollback exits 0" contract and the pasted evidence.

**No deadlock risk found.** This matches the review's suggested fix (F1, TD-6-review.md:61-65) essentially verbatim,
using the work dir itself as the lock file instead of a `rollback.lock` inside it (documented trade-off in
`TD-7.md`'s open questions — equivalent effect, no write needed so a full disk cannot stop the dead-man).

## Independent re-run of the new scenarios (my own commands, not copy-pasted from TD-7.md)
Built the generator from this tree; pulled `621d1e2:deploy/vpp/apply-startup.sh` (TD-6) and
`main:deploy/vpp/apply-startup.sh` (pre-TD-6) as reference scripts; ran against `TMPDIR=/tmp/g-td7verify`.

**Scenario 42 against TD-6's script — FAILS as claimed:**
```
$ TMPDIR=/tmp/g-td7verify/tmp VRX_TEST_ONLY="42" VRX_TEST_APPLY_SCRIPT=/tmp/g-td7verify/ref/td6-apply-startup.sh \
  deploy/vpp/test-apply-startup.sh /tmp/g-td7verify/gen/vrx-startupgen
  ok   the apply installed and has no result; its holder is gone
  FAIL the second rollback, started while the first is inside its VPP stop, is refused at once and touches nothing (exit 1)
  ok   the first one rolls back alone (exit 1): original file, VPP active, locks released
  FAIL VPP stopped and started exactly once (stops=2 starts=2); the locks taken exclusively, never FORCED
apply-startup tests: 2 passed, 2 failed
RC=1
```
Second rollback went FORCED and both processes stopped/started VPP — exactly the F1 defect.

**Scenario 40 against main's pre-TD-6 script — FAILS as claimed:**
```
$ TMPDIR=/tmp/g-td7verify/tmp VRX_TEST_ONLY="40" VRX_TEST_APPLY_SCRIPT=/tmp/g-td7verify/ref/main-apply-startup.sh \
  deploy/vpp/test-apply-startup.sh /tmp/g-td7verify/gen/vrx-startupgen
    elapsed 29s at load 13.82, 6 `ip neigh` read(s): ...
  FAIL the poll is bounded by time, not by count: 6 hanging `ip neigh` read(s) ... — independent of the host load
apply-startup tests: 1 passed, 2 failed
RC=1
```
6 reads (pre-V8 count-bounded poll), caught independent of the load-scaled time bound — the F2 gap.

**Scenarios 40/42/43 on this branch's own script — all pass:**
```
$ TMPDIR=/tmp/g-td7verify/tmp VRX_TEST_ONLY="40 42 43" deploy/vpp/test-apply-startup.sh /tmp/g-td7verify/gen/vrx-startupgen
apply-startup tests: 12 passed, 0 failed
RC=0
```
40: 1 `ip neigh` read. 42/43: `stops=1 starts=1`, second rollback refused with `a rollback of <dir> is already running
(pid N) — nothing to do` (exit 0), never FORCED; 43 additionally shows the timer-cancel call preceding the VPP-stop
call.

These reproduce the worker's pasted results in `TD-7.md` (32/40/42/43 fail-before/pass-after tables) independently.

## shellcheck
```
$ shellcheck --version | sed -n 2p; shellcheck -x -P SCRIPTDIR deploy/vpp/*.sh && echo SHELLCHECK-CLEAN
version: 0.11.0
SHELLCHECK-CLEAN
```

## Doc correction (F2's "always below the unbounded case" sentence)
`docs/agent/renderers/vppstartup.md`'s Tests paragraph no longer makes a blanket claim. It now names exactly the six
load-scaled `bounded` checks (scenarios 7, 21, 22, 23, 26, 29; idle limits 15–30 s, largest case 30 s × 5.8 = 174 s <
300 s) and states scenario 40 has no time bound any more, counting `ip -j neigh show` reads instead, independent of
load. Matches the code.

## Not re-verified in this pass (time box)
- The full 4-shard harness and `tools/ci.sh --base main` were not re-run here (would restart the harness under
  current host load and risk racing other agents sharing the cache dir); the worker's pasted CI GATE PASSED /
  138-checks-green evidence in `TD-7.md` was read and is internally consistent with the scenario-level reproduction
  above, but is trusted rather than independently re-executed.
- `systemctl start <dead-man>.service` against a real `systemd-run --on-active` transient timer/service was not
  exercised (no real apply/systemd unit allowed in this scope); this is disclosed as unverified in `TD-7.md`'s open
  questions, with a documented manual fallback. Not a blocker — F1's requirement (the refusal message points at the
  dead-man unit, and a second rollback of the same apply is refused) is met and proven under the fake-host harness.
- Two pre-existing tech-debt items TD-7 correctly declines to fix (rollback's final verification never checks the
  boot identity really changed; harness `kill_recorded` may signal reaped PIDs) are out of F1/F2 scope, as stated.

## Verdict
F1: the per-apply lock is non-blocking and per-process, so it cannot deadlock the dead-man's own rollback when no
manual rollback holds it; the timer is disarmed before VPP is touched; the refusal names the dead-man's own unit.
Reproduced scenario 42 failing on TD-6's script and passing here myself. F2: scenario 40 now counts `ip neigh`
invocations instead of a load-scaled time bound; reproduced it failing on main's pre-TD-6 script (6 reads) and
passing here myself. The vppstartup.md sentence is corrected to name exactly which checks are load-scaled. Scope is
exactly `deploy/vpp/**` + the doc + TD-7's own status files. Shellcheck is clean.

**APPROVE**
