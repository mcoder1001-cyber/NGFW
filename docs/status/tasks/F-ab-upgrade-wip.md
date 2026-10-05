# Current final integration readiness

Root-owned codex/integrate-ab-upgrade-20261005 /dev/shm/ngfw-integrate-ab-upgrade-20261005, pinned parent e5605d9c123e4c90a300e15ac0236fb49c4d62d5 after verified images173 merge. Frozen reviewedbe144source unchanged, allselectedR1/R2/R4/R7/R8 APPROVE; independent exactquick PASS9m49, actualprivate-loopstage/confirm/rollbackPASS. docs09 nowlistsactualruntimepackage deps; unrelatedcross-taskreports preservedinreviewedsourcearchive andexcludedfromfinalscope. Own tasksourcearchivebe144local/remote existsbeforeD112. Hardening and upgrade packaging assertions preserved together after three-way integration; finalexactcurrent-main/local+hosted gates pending, no mergeclaimed. Runtimefirmware/backup/reboothealthgenuineapplianceNOTRUN. Historicalchronology below.

# F-ab-upgrade WIP
Branch: codex/ready-ab-20261005; base e5dba658; last published local/remote9e31ece34beb81bf5b5e56a680629b3ece0e30e6 via GitHub connector (CLI push403).
Owned files: deploy/upgrade/**, docs/install/ab-upgrade.md, test/topology/ab-upgrade/** and own status.
Completed: signed Ed25519 rootfs verifier, bounded decompression/tar preflight, staging/lifecycle CLI, offline EFI provisioning, DB backup ordering, health probe and rollback units. Implementation checkpoint; not yet reviewed.
Tests: signed-bundle/lifecycle tests10/10 passed; topology go test -race passed26.961s; P14 loop image stage/confirm/rollback passed; exact host lsblk and grubenv SHA unchanged; no owned loops remain. Health script decisions13 tests rerun pending.
Remaining: final tampered loop rerun, packaging checks, unchanged complete quick gate, PR, independent review; real VM boot/DB/watchdog acceptance deferred.
Completed security fix: API can write only owned children; data parent and upgrade/shared state root-protected; actual dropped-UID regression14 packaging tests pass. Source-complete PR174.
Current failure: initial quick failed old baseline rsyslog timeout; current-main7b integration unchanged gate rerun /tmp/w15-ab-quick-current.log. Independent hardening and P11 findings fixed and verified APPROVE; reports durable here.
Previous failure: none; CLI push403 handled by authorized connector. Lab VM boot deferred; loop-device capability unverified.
Next command: poll /tmp/w15-ab-quick.log; independently review P11-host and hardening per manager while gate runs; then publish final gate/review evidence.

2026-10-05 security correction checkpoint: R2 demonstrated the offline builder
could package its signing key. Source is unfrozen for correction. Builder now
rejects a canonical key beneath the source root and any same-inode alias anywhere
in that root before creating output, and excludes all root/home operator content.
Regression covers contained key, external hardlink alias, and SSH/GnuPG home
credentials; upgrade suite 15 PASS (3.956s), packaging suite 14 PASS (1.282s).
Hosted packaging run 37272896483 failed solely because its non-root runner skipped
the actual UID boundary fixture. The fixture now re-executes only that test with
passwordless sudo and fails clearly if privilege is unavailable; strict gate is
unchanged. Hosted execution and renewed R2 review pending. Mandatory unchanged
quick continues with disk TMPDIR; last completed stage topology all PASS, currently
VPP fake-host harness. Next: publish checkpoint, R2 targeted re-review, inspect
quick completion and hosted fixture run; then resume authorized independent RA.

R1 correction: probe FileNotFoundError/PermissionError now remain within the
180-second retry deadline and trigger rollback/reboot. Unreadable initial status
still exits without mutating unknown boot state. Regression covers both missing
and nonexecutable probe plus status failure. Upgrade17 PASS4.149s. Prior unchanged
complete quick at ac421 passed12m23s; new product fixes require exact-tree hosted
gate and renewed R1 verification before merge. Next: publish this correction,
request R1 targeted review, monitor hosted exact-tree mandatory quick and fixtures.
