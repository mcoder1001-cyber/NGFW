# PPPoE IPv6 PR196 recovery WIP

Branch/worktree: `codex/resume-pppoe-20261007`, `/root/ngfw-wt/resume-pppoe-20261007`.
Initial source: `992b264b2084a8adfcee755d2b5650b771a6a8f1`.
Local and verified published checkpoint: `620aec2927bf0956b2ed070ae1ecca32c2e057b5`.
Merged origin/main `0ec397e327123cadfd5d278a9a1cda37532fdc2c` cleanly; source history preserved.
Publication: CLI push succeeded to this named branch; original PR branch untouched.
Owned files: PPPoE IPv6 diff only plus `resume-pppoe-20261007-{envelope,wip}.md`.

Completed: read instructions, task prompts and complete original IPv6/secret WIP;
recovered source and inspected PR196 (original mandatory quick SUCCESS).
Tests on recovered integration: not yet run. Original test evidence is historical only.
Current audit findings: IPv4 down retains negotiated addresses, and combined IPv6-up
phase can therefore keep a withdrawn IPv4 address mirrored; disabled IPv6 stale state
can also make the watcher consider an IPv4-down session up. Refresher lifecycle under review.
Remaining code: add failing focused regressions, repair in scope, run focused race tests,
publish final checkpoint and report frozen SHA/tree to manager.
Exact next command: `cd /root/ngfw-wt/resume-pppoe-20261007/apps/agent && ../../tools/heavy.sh go test -count=1 -timeout 120s ./internal/renderers/pppoe ./internal/descriptors/pppoe ./internal/subsystems -run 'PppoeIPv6|ReadState|IPv6'`.

Residual acceptance: historical product dial-up blocked by VPP discovery and LAN transit
by missing encapsulation; these are unresolved product failures, not passing lab evidence.
No shared host changes performed in this recovery. Globals-owner IPv6/systemd and live
IPv6 traffic/reconnect/rollback acceptance require an authorized lab rerun.
