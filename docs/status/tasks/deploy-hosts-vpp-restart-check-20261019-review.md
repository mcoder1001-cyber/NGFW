# PR #247 — independent review (2026-10-20, reviewer agent, read-only)

Head `f4d085c9b`, base `cb148aafb`. **Verdict: APPROVE** (no BLOCKER/MAJOR; merge after PR #246).

Correct: compares LoadState, ActiveState, ActiveEnterTimestampMonotonic and NRestarts by property name (MainPID not needed); parsing verified against systemd 259 (property order, sockets without NRestarts, not-found services still printing NRestarts=0); no false positives on the normal path (values survive daemon-reload; the agent restart only sends no-op start jobs to vpp/nftables/firstboot; the agent never starts sockets; D-253 postinst starts nothing; socket listening↔running keeps its timestamp; nginx not watched); only a restart after the final read is missed (documented); no new remote arguments, constant unit names, regex-checked values; fake systemctl models the reads. `bash -n`/shellcheck clean (also the extracted remote heredoc); fixture suite 34 tests OK (789 s); hosted packaging 126 tests pass incl. `test_watched_units_must_be_unchanged_by_the_upgrade`; quick gate pass.

Findings (all MINOR/NIT):
1. 465, 518–529: the pre-install warning tells the operator to `systemctl start <socket>`; doing so during the run turns inactive→active and fails the run blaming the packages, with no socket hint. Say "after this run completes" or report inactive→active as a warning; align D-254/doc wording.
2. 512–516: the after-record is read only after the `is-active` loop and the counter check, so when VPP is left stopped or the agent crash-loops there is no `unit-state-after.txt` and no VPP hint; earlier failures (apt, nginx -t, health) also leave none. Write it before the loop or from the EXIT trap (`unit_states > … || true`) and add vpp/nftables to the trap's `systemctl status`.
3. Untested paths: an nftables change (hint at 528), a failed/unexpected `systemctl show` (445, 454), a socket going not-found→loaded.
4. 454/518: on appliances whose packages predate the sockets (before `db46e75ae`, 2026-10-07) the before-record says `not-found` and the after-record `loaded inactive` → "no longer listening" false positive. Treat not-found-before as a warning or document.
5. Merge order/evidence: depends on D-253 (PR #246) — state "merge after #246"; DEFERRED-ACCEPTANCE's "not reproducible without deliberately building pre-D-253 packages" is wrong: any current main build is pre-D-253 (checked `/var/tmp/ngfw-build-main-cb148aafb-20261019`: ngfw-meta and ngfw-agent preinst stop units on upgrade).
6. NIT 508–509: stale comment "nginx and VPP are only required to be active".
7. NIT 527: VPP hint names only maintainer scripts; a startup apply or a crash can also restart it ("or another actor").
[other R7] the host-37 receipt said `ngfw-meta.prerm`; the stop is in the preinst on upgrade (prerm stops only on remove) — corrected in `fdc7fc470`.
