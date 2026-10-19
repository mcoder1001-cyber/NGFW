# PPPoE IPv6 PR196 recovery WIP

## Frozen source handoff — authoritative current status

Local/remote tested checkpoint `88fca3168e775fd0bbf699bdc9e82c2295091f00`, tree
`afbd6626295e20d290519ecbd10da4df932dfbef`, published on the named branch below.
Merged main `3ddb1680e475e94d43e8036cd3776bc60c87208b` cleanly; upstream delta
docs/board-only, no executable change or owned edits outside scope.
Frozen executable trees: apps `b85d6dcb068be6f1f22d931ac66f2790f6523f7d`, deploy
`3af47703e8d7d71133594209d0779aa580fc5b7d`, packages
`4e36d6a20a1163148072c5011b27a4b591c4e2e3`. Final WIP-only receipt SHA/tree
is resolved externally via `git rev-parse HEAD HEAD^{tree}`; verify publication
with `git ls-remote origin refs/heads/codex/resume-pppoe-20261007`.

Complete independent reports cbeea9a55/9ec1f1d17 mandatory findings addressed:
pidfd/starttime/command identity rejects bare/zero/foreign/reused PID; verified
refresher and direct DHCP child stop before handle/hook deletion; locked revocation
and generation-gated publication reject late writers; exec/sysctl/client failures
observable; packaged dhcpcd-base dependency; inherited contract status recorded.
Reconnect now serializes with existing transaction exclusion and clears both
families; mode-change regression fixed. No disagreement needing arbiter.
These are author fixes/controls, NOT an independent APPROVE verdict.

Final post-merge bounded focused race PASS: renderer 44.096s, descriptor 2.145s,
subsystem 3.150s. Covers noforeignsignal, childgone, TERM/KILL, late DHCP event,
collection barrier, abrupt pppd/link loss, replacement, remove/off/mode/credentials,
failure reporting and reconnect. Scoped golangci-lint PASS zero issues. Post-merge
`tools/ci.sh check --base origin/main` PASS 0m14s; golden/allowlist checks passed.
No full local quick, actual shared daemon/VPP/sysctl mutation or fresh live dial.

Remaining bounded lifecycle code: none identified by these controls. Exact-source
independent re-review and manager unchanged complete hosted quick remain REQUIRED.
Actual packaged dhcpcd privsep descendants and live dial/reconnect are untested;
fake direct-child controls do not certify them. Discovery and missing IPv4/IPv6 LAN
PPPoE encapsulation remain unsupported PRODUCT gaps, NOT laboratory-only acceptance
or full-feature DONE. Discovery conflict: plugin owns EtherType 0x8863, linux-cp
registration rejects it, global CP dispatch cannot resolve client broadcast PADI
through server learned-MAC state. Physical-interface address/FIB mirroring has no
PPPoE encapsulation DPO/session and lacks required AC/session metadata. Resolving
forwarding/coexistence needs separately authorized design, possibly VPP C/security
boundary; no generated binding change, plugin-disable or privilege workaround.
Existing namespace refusal and unsupported disclosure preserved.

Current blocker: independent current-source review/hosted gate; full product datapath
still unsupported. Do not declare PR196/all merged work complete or merge APPROVE.
Exact next command, if reviewer requests rerun:
`cd /root/ngfw-wt/resume-pppoe-20261007/apps/agent && ../../tools/heavy.sh timeout 300s go test -race -p 2 -count=1 -timeout 240s ./internal/renderers/pppoe ./internal/descriptors/pppoe ./internal/subsystems -run 'Pppoe|PPPoE|Client|ReadState|ReadIPv6|IPv6|DHCP6|Render|Apply|Secrets'`.
Manager next: re-review frozen source and run unchanged hosted quick on exact final
integration tree; preserve explicit discovery/transit product gaps.

New inventory additions to inherited list below: renderer `lifecycle.go`,
`lifecycle_test.go`, `templates/ipv6.tmpl`; shared renderer `ALLOWLIST.md`;
`deploy/debian/ngfw/debian/control`; `resume-pppoe-20261007-contract.md`.
Exact complete changed-file inventory: `git diff --name-only origin/main HEAD`.

## Active independent-review repair — supersedes readiness below

Published lifecycle checkpoint: local/remote `1cc677129ba564eb0fec1f2251856aba6fc13299`
(CLI push and ls-remote verified). New owned files are `lifecycle.go`,
`lifecycle_test.go`, `templates/ipv6.tmpl`, and the already-owned renderer README.
Focused race PASS: renderer 36.731s, descriptor 1.427s, subsystem 3.601s using
the exact next command below. Added abrupt-pppd-loss/child-gone race control plus
setup failures PASS 12.675s. Initial lint found only private executable fixture
permissions, then path/permission annotations; fixes are test-only. Final scoped
golangci-lint PASS, zero issues; check against fetched main PASS 0m15s.
No executable production change after this published checkpoint.

Latest verified published checkpoint before this source checkpoint: `9d63a56a8`
(DHCPv6 dependency and contract status); docs checkpoint `793837f80` discloses
unsupported discovery/transit. Read complete independent reports `cbeea9a55` and `9ec1f1d17`.
The family fixes at `17de38549` resolve the stale-family findings only; remaining
mandatory lifecycle/security/operation findings block final freeze/approval.
Current checkpoint adds required `dhcpcd-base` dependency to ngfw-agent and the
inherited PPPoE contract status record. New ownership is in the envelope.

Unfinished source checkpoint in this own worktree: replace bare-PID shell termination
with a fixed rendered Python3 helper (existing packaged runtime), verified starttime
and command identity after pidfd_open, pidfd-only signals, bounded TERM/KILL and
exit verification, session action/publication locks and generation-gated DHCP events.
Runtime stops and verifies old writers before deleting state/handles or replacing
hooks. Hook setup, client launch/exit and state-write failures fail closed and
use fixed non-secret error codes exposed in PPPoE status. Custom paths reject
shell expansion/control tokens. No new privilege model or VPP mutation.

First focused controls passed (before final action-lock/identity refinements):
`../../tools/heavy.sh timeout 240s go test -p 2 -count=1 -timeout 180s ./internal/renderers/pppoe -run 'TestIPv6Owned|TestIPv6Collection|TestIPv6PID|TestIPv6Setup|TestRenderIPv6|TestDHCP6' -v`
reported PASS, renderer 13.837s: verified child+refresher gone, TERM-ignoring child
KILL fallback, late DHCP event rejected, barrier-controlled collection revocation,
zero/reserved/bare/foreign PID refusal, missing/invalid client, sysctl/state-write
failure and asynchronous child-exit visibility. Tests use private sysctl/IP/client
substitutes, never shared host configuration or a real DHCP daemon.

Expanded renderer tests found two failures, recorded honestly: the golden was
stale after template refinements (regenerated); DHCPv6 -> SLAAC did not stop the
old generation because both peer files contain +ipv6. Fixed change detection to
include rendered helper and optional session files. Expanded rerun PASS: renderer
46.420s, shared renderer 0.079s (IPv6/ReadState/ReadIPv6/Render/Apply/DHCP6/Secrets/
AllowlistDocumented selection, timeout 240s). `tools/ci.sh check --base origin/main`
PASS 0m14s. Final race/lint and independent re-review remain pending.
Reconnect now uses existing transaction exclusion and runtime mutex, stops verified
IPv6 writers, withdraws tracked mirrors and invalidates both family states before
unit restart. Added a focused regression for exclusion and stale-state invalidation.
This checkpoint is NOT final/frozen/approved; remaining controls must finish.

Remaining: finish latest expanded controls, scoped race/lint/check, exact-delta
re-review, then publish final source freeze. Manager reports new main `3ddb1680e`
is docs/board-only; do not claim that as fresh product verification.
Exact next command: `cd /root/ngfw-wt/resume-pppoe-20261007/apps/agent && ../../tools/heavy.sh timeout 600s go test -race -p 2 -count=1 -timeout 240s ./internal/renderers/pppoe ./internal/descriptors/pppoe ./internal/subsystems -run 'Pppoe|PPPoE|Client|ReadState|ReadIPv6|IPv6|DHCP6|Render|Apply|Secrets'`.
Current failure: mandatory final lifecycle verification/re-review incomplete;
discovery/encapsulation remain explicit unsupported product behavior.

Branch/worktree: `codex/resume-pppoe-20261007`, `/root/ngfw-wt/resume-pppoe-20261007`.
Initial source: `992b264b2084a8adfcee755d2b5650b771a6a8f1`.
Local and verified remote code checkpoint: `17de38549129ab742fe18051b629cc93ba1be558`.
Tested code-checkpoint tree: `84fc03af656bba09adc38f966dae5db6a9903d23`.
Final recovery-doc commit is a descendant with documentation-only changes; its SHA
is `git log -1 --format=%H -- docs/status/tasks/resume-pppoe-20261007-wip.md` and its
tree is `git rev-parse HEAD^{tree}`. Verify publication with
`git ls-remote origin refs/heads/codex/resume-pppoe-20261007`.
Merged origin/main `0ec397e327123cadfd5d278a9a1cda37532fdc2c` cleanly; source history preserved.
Publication: CLI push succeeded to this named branch; original PR branch untouched.
Owned files: PPPoE IPv6 diff only plus `resume-pppoe-20261007-{envelope,wip}.md`.

Completed: read instructions, task prompts and complete original IPv6/secret WIP;
recovered source and inspected PR196 (original mandatory quick SUCCESS).
Completed code: clear IPCP addresses/DNS on IPv4 down even if IPv6CP remains up;
read only configured families in runtime observation/status; fence each IPv6 refresher
with its own PID-file ownership; mark down on pppd/link loss; remove state on hook
withdrawal; preserve replacement state; terminate/reap the owned DHCPv6 child on exit.
Golden regenerated through the renderer's `-update` test flag. Support docs explicitly
state unsupported product discovery and both IPv4/IPv6 LAN transit.

Actual tests on this integration tree (no NGFW_INTEGRATION, no shared host mutation):
- Focused subsystem regressions: `go test -count=1 -timeout 120s ./internal/subsystems -run 'TestPppoeIPv6(MirrorFollowsHookState|StaleStateIgnoredWhenOff)$'`: ok, 6.293s.
- Renderer lifecycle/shell/state: `go test -p 2 -count=1 -timeout 120s ./internal/renderers/pppoe -run 'TestIPv6RefresherLifetime|TestIPv6ScriptsParse|TestReadState' -v`: PASS, 10.996s; all three lifecycle cases passed (pppd exit, hook withdrawal, replacement).
- `../../tools/heavy.sh timeout 600s go test -race -p 2 -count=1 -timeout 180s ./internal/renderers/pppoe ./internal/descriptors/pppoe ./internal/subsystems -run 'Pppoe|PPPoE|Client|ReadState|ReadIPv6|IPv6|DHCP6|Render|Apply|Secrets'`: all ok (renderer 10.096s, descriptor 1.609s, subsystem 7.966s).
- `tools/ci.sh check --base origin/main`: check PASSED (0m16s), contract guard and gitleaks passed. This is not the whole quick gate.
- `git diff --check`: clean.
- `../../tools/heavy.sh timeout 180s golangci-lint run --timeout 150s ./internal/renderers/pppoe/... ./internal/descriptors/pppoe/... ./internal/subsystems/...`: exit 0, `0 issues.`
TypeScript schema/drawer tests were not rerun in this recovery (worktree has no
node_modules); inherited changes retain historical evidence, with final hosted quick owed.
Original quick/test evidence remains historical; manager must run unchanged hosted quick
on the final integration tree and arrange independent applicable review.

Remaining code in the bounded IPv6 state/mirror scope: none identified by this audit.
Published developer candidate is ready for independent review and the manager's final
integration/hosted gate, not an unconditional merge approval. Whole PPPoE feature is NOT DONE.
Current failure: product discovery and LAN transit, detailed below; no focused code-test failure.
Exact next recovery command: `cd /root/ngfw-wt/resume-pppoe-20261007 && git status --short && git rev-parse HEAD HEAD^{tree} && git ls-remote origin refs/heads/main refs/heads/codex/resume-pppoe-20261007`.
Final main read-only check: origin/main remains `0ec397e327123cadfd5d278a9a1cda37532fdc2c`.
No main merge, new PR, squash or original PR branch write performed.

## Discovery/transit diagnosis (read-only source audit)

`/root/vpp/src/plugins/pppoe/pppoe.c:739-743` registers 0x8864 to
`pppoe-input` and 0x8863 to `pppoe-cp-dispatch` at plugin initialization.
`/root/vpp/src/plugins/linux-cp/lcp_interface.c:1246-1247` refuses an EtherType
already registered to another node with `VNET_API_ERROR_INVALID_REGISTRATION`.
Thus the existing generated `LcpEthertypeEnable` API cannot override this conflict.
The CP dispatch sends physical-side discovery to the single `pem->cp_if_index`,
not the physical interface's linux-cp pair (`pppoe_cp_node.c:174`). On its CP-side
path it looks up destination MAC in the learned server link table (`:132-144`);
broadcast client PADI has no learned AC destination. `PppoeAddDelCp` is a globals-only,
server-side singleton in existing `descriptors/pppoe/cp.go`, not a multi-WAN client fix.
The historical product run records `Timeout waiting for PADO packets` and dial-up FAIL.
Disabling pppoe_plugin and enabling global linux-cp EtherTypes is only the historical
diagnostic workaround; it removes the server path and is not shipped here.

The client mirror programs /32 or /128 addresses and ordinary physical-interface FIB
paths; it never creates a PPPoE encapsulation interface or selects an encapsulation DPO.
Existing server `pppoe_add_del_session` requires a client MAC learned in the AC link
table (`pppoe.c:308-315`) and the current hook state does not provide a client session
ID/AC MAC for such a mapping. There is no bounded IPv6 mirror fix for LAN transit.
Resolution requires a separately reviewed client forwarding mechanism and discovery
coexistence design, potentially VPP C changes (prohibited here) or host/kernel data-path
work beyond this envelope. Manager should record these findings in the VPP code track;
that file is outside this worker's ownership. Generated bindings and global registration
are untouched. Existing `pppoe.netns-unavailable`, globals-owner and table-policy refusals
remain unchanged. No product acceptance failure is reclassified as laboratory-only.

Residual acceptance: historical product dial-up blocked by VPP discovery and LAN transit
by missing encapsulation; these are unresolved product failures, not passing lab evidence.
No shared host changes performed in this recovery. Globals-owner IPv6/systemd and
live reconnect/rollback acceptance require an authorized lab rerun. IPv6 traffic
acceptance first requires resolution of the discovery/encapsulation product blockers.

## Recovery changes

New edits since the manager-created source: renderer `state.go`, `ipv6_test.go`,
`templates/hook6.tmpl`, regenerated `testdata/two-sessions.golden`, renderer README;
subsystem `pppoe.go`, `pppoe_watch.go`, `pppoe_ipv6_test.go`; PPPoE user and descriptor
docs; this WIP and its envelope. No P12, RA, main, board, bindings or new contract edits.

Full integration changed-file inventory is reproduced by:
`git diff --name-only 0ec397e327123cadfd5d278a9a1cda37532fdc2c HEAD`.
It includes the inherited PPPoE IPv6 contracts/schema tests, generated YANG description,
API fake, UI test, renderer/mirror/watcher source and tests, package/support docs and
original Claude evidence. The complete inventory follows for recovery without chat.

- `apps/agent/internal/descriptors/pppoe/client.go`
- `apps/agent/internal/descriptors/pppoe/client_mirror_ipv6_test.go`
- `apps/agent/internal/renderers/ALLOWLIST.md`
- `apps/agent/internal/renderers/pppoe/README.md`
- `apps/agent/internal/renderers/pppoe/ipv6_test.go`
- `apps/agent/internal/renderers/pppoe/paths.go`
- `apps/agent/internal/renderers/pppoe/pppoe_test.go`
- `apps/agent/internal/renderers/pppoe/renderer.go`
- `apps/agent/internal/renderers/pppoe/state.go`
- `apps/agent/internal/renderers/pppoe/state6.go`
- `apps/agent/internal/renderers/pppoe/state_test.go`
- `apps/agent/internal/renderers/pppoe/supervisor.go`
- `apps/agent/internal/renderers/pppoe/templates/dhcp6.tmpl`
- `apps/agent/internal/renderers/pppoe/templates/dhcpcd.tmpl`
- `apps/agent/internal/renderers/pppoe/templates/hook6.tmpl`
- `apps/agent/internal/renderers/pppoe/testdata/two-sessions.golden`
- `apps/agent/internal/subsystems/pppoe.go`
- `apps/agent/internal/subsystems/pppoe_ipv6_test.go`
- `apps/agent/internal/subsystems/pppoe_watch.go`
- `apps/api/src/testing/fake-agent.ts`
- `apps/web/src/domains/interfaces/PppoeDrawer.test.tsx`
- `docs/09-os-packages.md`
- `docs/agent/descriptors/pppoe.md`
- `docs/status/tasks/claude-pppoe-ipv6-evidence/01-go-race.txt`
- `docs/status/tasks/claude-pppoe-ipv6-evidence/02-ipv6-tests-verbose.txt`
- `docs/status/tasks/claude-pppoe-ipv6-evidence/03-probe-slaac.txt`
- `docs/status/tasks/claude-pppoe-ipv6-evidence/04-probe-dhcpv6.txt`
- `docs/status/tasks/claude-pppoe-ipv6-evidence/05-ci-quick.txt`
- `docs/status/tasks/claude-pppoe-ipv6-evidence/10-diag-slaac-run-first.txt`
- `docs/status/tasks/claude-pppoe-ipv6-evidence/11-diag-slaac-run.txt`
- `docs/status/tasks/claude-pppoe-ipv6-evidence/12-diag-dhcpv6-run.txt`
- `docs/status/tasks/claude-pppoe-ipv6-evidence/13-product-dhcpv6-run.txt`
- `docs/status/tasks/claude-pppoe-ipv6-evidence/driver-ipv6.patch`
- `docs/status/tasks/claude-pppoe-ipv6-evidence/probe-inner.sh.txt`
- `docs/status/tasks/claude-pppoe-ipv6-wip.md`
- `docs/status/tasks/resume-pppoe-20261007-envelope.md`
- `docs/status/tasks/resume-pppoe-20261007-wip.md`
- `docs/user/network/pppoe.md`
- `packages/schema/src/domains/ext/pppoe.ts`
- `packages/schema/src/semantic/pppoe.test.ts`
- `packages/schema/src/semantic/pppoe.ts`
- `packages/yang/generated/ngfw-interfaces.yang`
