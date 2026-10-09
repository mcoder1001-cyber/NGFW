# R8 operability and packaging review — PPPoE lifecycle

Initial review covered `3fae79eba5e19f367df3a16a6c3197c8a72b3f8d`. Final verify round covered HEAD `f7aadb6ab7357a73ef259623d7f0e2a2e01f1803`, including removal recovery fix `37de7f5`, late-parent fix `7debd6a` and its regression. Final SHA was rechecked after the commands below. Source remains local pending owner approval; publication was rejected by automatic review. The exact reported source of this limitation is `docs/status/tasks/pppoe-lifecycle-20261008-wip.md`: repository payload/destination authorization and trust/privacy were not established. This reviewer did not attempt publication or bypass that rejection.

## Resolved MAJOR — removed-unit daemon reload retry

`apps/agent/internal/renderers/pppoe/supervisor.go:56-74,127-130,179-193`

Scenario: successfully applied session is removed; StopIPv6 fences it, systemctl stop succeeds, session/unit files are deleted, then daemon-reload fails. Identical retry discovers stale sessions only from extant unit files, which are already gone. The persistent per-session pending marker is inspected only for desired sessions. Removing the final session therefore causes retry Apply(nil) to skip daemon-reload and return success with systemd's loaded unit state never refreshed. Removing one of several sessions has the same gap when remaining sessions are unchanged. Partial removal failure after deleting the unit file can also lose the discovery handle.

Required fix: persist removal/reload intent before destructive unit removal, rediscover it on retry independently of installed units, and clear it only after successful daemon-reload (and completed cleanup). Preserve admission fence for delayed old hooks. Add remove-all and remove-one fault injection followed by identical retry; successful retry must execute daemon-reload and fully clean owned stale files. Kept-session pending repair is sound but does not cover removed sessions.

Other R8 assessment: kept-session transitions now fence helper admission, stop old pppd, invalidate stale operational files, replace files, reload units, rotate admission and restart before clearing pending. Parent pidfd represents original process lifetime. Credential changes now flow through supervisor pending restart ownership. Systemd unit enablement remains untouched. These source improvements do not substitute for real process or service acceptance.

Actual commands executed:

```text
git diff --check
(no output; exit 0)

go -C apps/agent test -race -count=1 ./internal/renderers/pppoe ./internal/subsystems
/bin/bash: line 1: go: command not found
(exit 127)
```

The removal scenario above is source inspection evidence, not an executed Go regression. No real services/sysctls/VPP mutation occurred. Full unchanged hosted quick, source/security review and real lifecycle acceptance remain owed; discovery and LAN encapsulation remain unresolved product gaps.

Verify-round evidence: `installedHostIfs` now includes durable `.ipv6.pending` markers even after unit files disappear. Removal retries skip stop only when the unit file is already absent, repeat owned cleanup, require daemon-reload, then clear pending. This covers final-session and unchanged-retained-session removal with the same control flow. New `TestApplyRemovalRetryReloadsDeletedUnit` injects reload failure after unit deletion, requires retry reload, pending clearance and subsequent idempotence. The source failure above is resolved. Late unavailable-parent up hooks now return before replacement state is overwritten; down checks the recorded writer parent before stopping it.

Additional actual command:

```text
go -C apps/agent test -race -count=1 -run 'TestApplyRemovalRetryReloadsDeletedUnit|TestApplyIdenticalRetryCompletesFailedTransition' ./internal/renderers/pppoe
/bin/bash: line 1: go: command not found
(exit 127)

git diff --check
(no output; exit 0)

git rev-parse HEAD
f7aadb6ab7357a73ef259623d7f0e2a2e01f1803
```

No Go regression success is claimed. Hosted execution, mandatory quick, independent reviews and real lifecycle acceptance remain required.

Verdict: **APPROVE** for R8 source scope; previous MAJOR resolved, executable gates outstanding.


## Final narrow source verification — pre-unit installation retry

Reviewed local `a673d340a7a2b470a033ea96b59e176e579ed094`, tree `9f1c79084ba9edc9bb8be14ac468e128c520c984`. The manager reports remote `a939827119cf6645f77fa9555296e6a070df551b`; this reviewer did not query or publish the remote. This narrow pass covers only the kept-session missing-unit retry delta and its regression. Earlier publication limitation above describes historical evidence, not this later manager-reported publication.

A pending desired session discovered through transition inventory can exist before the first unit file was written. The kept-session stop step now checks that unit file exists; missing files skip stop while other stat errors refuse explicitly. The retry still fences admission, installs the files, reloads and restarts, then acknowledges transition completion. Existing installed-unit transitions retain their stop-before-replacement ordering.

`TestApplyNewSessionRetriesBeforeUnitWasWritten` induces a real directory collision after admission/pending but before unit installation, verifies retained intent and absent unit, repairs only the collision, then retries identical desired state. Its recording runner rejects attempts to stop a nonexistent unit, and expected retry commands are exactly daemon-reload then restart. This is a meaningful regression for the reported recovery issue.

Executed `git show HEAD` and `git rev-parse HEAD^{tree}` to inspect exact delta/identity; `git diff --check` returned no output, exit 0. An initial abbreviated parent lookup was unavailable and was replaced by actual HEAD diff inspection. No CI was run or polled, honoring the owner's instruction to run the combined final CI once all tasks finish. Focused Go execution remains pending a supplied trusted toolchain path; no success is claimed here.

Narrow R8 verdict: **APPROVE** source. Final combined CI and executable acceptance remain outstanding.


### Focused executable recovery verification

The manager supplied an official SHA256-verified Go 1.26.0 executable. This reviewer executed the following focused command in the task worktree (not CI):

```bash
PATH=<trusted-go-bin>:$PATH \
GOTOOLCHAIN=local GOMAXPROCS=2 GOFLAGS='-p=2 -mod=readonly' \
GOCACHE=<scratch-build-cache> \
GOMODCACHE=<scratch-module-cache> \
<trusted-go-bin>/go -C apps/agent test -race -count=1 -v \
-run '^TestApply(NewSessionRetriesBeforeUnitWasWritten|IdenticalRetryCompletesFailedTransition|RemovalRetryReloadsDeletedUnit|RefusesUnavailableTransitionInventory)$' \
./internal/renderers/pppoe
```

Actual output: all four named tests PASS, including daemon-reload/restart retry subtests; no skips. Final package result:

```text
PASS
ok ngfw/agent/internal/renderers/pppoe 1.038s
```

Exit 0. Initial dependency downloads succeeded. Final SHA rechecked `a673d340a7a2b470a033ea96b59e176e579ed094`; `git diff --check` remained clean. This supersedes the pending focused execution statement above, while prior unavailable-Go records remain historical evidence. No source changes, service operations, CI runs, or CI polling were performed.

Final narrow R8 verdict: **APPROVE** source and focused recovery verification. Combined final CI and live lifecycle acceptance remain required.
