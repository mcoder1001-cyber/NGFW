# PPPoE IPv6 PR196 recovery WIP

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
