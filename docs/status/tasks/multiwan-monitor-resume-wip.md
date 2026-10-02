# Multi-WAN monitor recovery checkpoint

Branch `codex/multiwan-monitor-resume-20261002`; isolated worktree `/workspace/scratch/de92de7d9874/ngfw-multiwan-monitor`. Recovered product local SHA `1017bbffa78aa0dd6ab4c1b96084a15c02ab96c1`, from unchanged remote checkpoint `2ca15b6f` on `codex/network-manager-20261002`. All agent files byte-identical to checkpoint. No new remote publication yet.

Bounded task envelope: monitor slice only. Own inherited runtime/probe and tests, agent watcher/lifecycle/RPC/projection changes and task docs. No main/board or contract edits; no route installation, NAT cleanup, VRF, dynamic gateway or ABF implementation. Current scope is default-namespace/default-VRF LCP-bound HTTP/DNS/IPv4 ICMP observations. Active stays empty until separate forwarding controller confirms installed routes. No full F-multiwan-host completion or lab success claimed.

Read AGENTS, shared architecture/contribution/decision/review instructions, feature prompt F-multiwan and inherited F-multiwan-host-runtime-wip. Reviewed bounded generation replacement, probe cancellation/deadlines, binding/no fallback, DNS/ICMP response matching, durably applied config watcher and owner-scoped RPC. New independent review remains pending.

Actual pinned Go1.26.0 evidence:

```text
go test -race -count=3 ./internal/multiwan ./internal/agent -run 'Test(Runtime|HTTPProbe|DNSProbe|ICMPProbe|DeviceProbe|Wan)'
ok  ngfw/agent/internal/multiwan  1.563s
ok  ngfw/agent/internal/agent     6.101s

go vet ./internal/multiwan ./internal/agent
(exit 0, no diagnostics)
```

Unit/protocol fake peers only; real interface binding, network namespace and VPP/browser acceptance NOT RUN. No full local quick run to avoid concurrent broad test contention. Main advanced to `31355cef` during focused tests; pending rebase is required before publication. Temporarily frozen to prioritize manager-requested dashboard PR62 rebase. Next command: `git rebase origin/main`, then check gate, independent R1/R2/R3/R4/R5/R7/R8 review and draft PR with unchanged hosted quick. No known product failure identified by focused checks.

Resumed after dashboard rebase: cleanly rebased onto main `31355cef80b4e8ba3aaa6f95a47c7e7afc055fb6`. Existing monitor product files remain byte-identical to archived network-manager checkpoint. Added an explicit implementation-status notice to user docs so the existing aspirational forwarding description does not falsely certify this monitor-only slice. No schema/proto/UI change. Parent assigns independent review; draft PR is a review checkpoint, not full feature completion.

## Published draft checkpoint

Draft PR https://github.com/mcoder1001-cyber/NGFW/pull/65 created successfully. Remote branch `codex/multiwan-monitor-resume-20261002`, commit `987c2c9f1511b8694c5332783a9fd7eafdae8ae2`, one commit atop `31355cef80b4e8ba3aaa6f95a47c7e7afc055fb6`. Remote tree `b7c62b157300d9f588a2c5784685480eb1044004` exactly equals local `4a7b0b50eba79f79c199c4b0d423cd9312efbf75` tree. Independent review and full hosted quick pending; no merge.

```text
bash tools/ci.sh check --base origin/main
ok: 30 developer slots + CI slot 12; 964 ports, 32 id ranges, no collision; tools/lab env verified for 31 slots
check PASSED (0m02s)
```

This publication-evidence follow-up is local until coordinated with manager, so the active hosted gate is not needlessly restarted. Next: manager dispatch independent review and monitor draft PR65 full hosted quick. All forwarding/route/NAT/dynamic gateway/VRF/ABF follow-ups and laboratory NOT RUN limitations remain unchanged.

## Correctness review fix: persisted monitor input

R1 reproduced a blocker: Apply updated in-memory desired state before persistence; after a failed authoritative save, the watcher could activate an uncommitted monitor. Reproduction was confirmed locally before the fix:

```text
go test -race -count=1 ./internal/agent -run TestReviewWANFailedPersistenceMustNotStart
--- FAIL: TestReviewWANFailedPersistenceMustNotStart (0.03s)
    rpc_wan_durable_test.go:28: WAN watcher activated a monitor whose desired-state save failed (Apply DEGRADED)
FAIL ngfw/agent/internal/agent 0.077s
```

The state now keeps a separate immutable WAN-input snapshot (only interfaces and WAN groups), advanced after the authoritative atomic write and initialized from loaded state. Mirror failure still publishes because the authoritative record succeeded. The watcher never reads speculative desired/storedIfs for monitors; each probe generation captures the same saved interface mapping as its groups, including while old probes drain. Apply/rollback/DEGRADED semantics and unrelated subsystem snapshots remain unchanged. Forwarding remains out of scope.

Added regressions for failed Apply starting no monitor, failed replacement/removal retaining prior saved input, mirror failure retaining durable new input, snapshot alias isolation, restart and confirmed rollback. Actual focused checks after the fix:

```text
go test -race -count=3 ./internal/multiwan ./internal/agent -run 'Test(ReviewWAN|Wan|Runtime|HTTPProbe|DNSProbe|ICMPProbe|DeviceProbe)'
ok ngfw/agent/internal/multiwan 1.565s
ok ngfw/agent/internal/agent    6.226s
```

R1 verification and refreshed hosted quick on the published fix are required. No lab result changed.
