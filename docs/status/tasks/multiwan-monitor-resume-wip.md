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
