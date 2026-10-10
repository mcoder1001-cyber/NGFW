# Combined review — hardware package preparation

Final PR217 source: `5bd7e8b545fc765fd2babd8dda15175d6f33af1b`.
Compiled source: `2045ab8b3d2f477bb23446fb5e58b7d9d3abea3c`.
Product trees are identical. Only product change is API `${shlibs:Depends}`.
Tested integration: `8a15d644c53cc3ef4abde339efd3b2a0331221a5`, parents current
main `d2d55984d74fa1d06c32e8271886f11f16375407` and final source5bd7e8b;
tree `a0d7b7cbc7f37dc2fadd9a93d41ce486dcda50d2`.

Applicable independent review receipts, published and read back:

- R1 correctness APPROVE, T1 PASS: `2debe48f2769de473e048544c60261b54f50370c`;
  [R1](https://github.com/mcoder1001-cyber/NGFW/blob/2debe48f2769de473e048544c60261b54f50370c/docs/status/tasks/hardware-37-20261010-review-R1.md),
  [T1](https://github.com/mcoder1001-cyber/NGFW/blob/2debe48f2769de473e048544c60261b54f50370c/docs/status/tasks/hardware-37-20261010-test-T1.md).
- R2 security APPROVE: `18f2e59869395d915c2aef12deef57a0c58f59dd`;
  [receipt](https://github.com/mcoder1001-cyber/NGFW/blob/18f2e59869395d915c2aef12deef57a0c58f59dd/docs/status/tasks/hardware-211-20261010-review-R2.md).
- R7 evidence APPROVE: `65ca4275cd7bdf161d88b4b9d34fc4c9b94f7427`;
  [receipt](https://github.com/mcoder1001-cyber/NGFW/blob/65ca4275cd7bdf161d88b4b9d34fc4c9b94f7427/docs/status/tasks/hardware-evidence-review-20261010-review-R7.md).
  Missing command/output evidence was corrected before final-SHA CI; only an
  optional whitespace nit on a verbatim old output line remains.
- R8 archive/native/control/helper/unit/paths/pins APPROVE:
  `c6ecdb6790b5d9511e1703b27c2b9b78ef9966d1`;
  [receipt](https://github.com/mcoder1001-cyber/NGFW/blob/c6ecdb6790b5d9511e1703b27c2b9b78ef9966d1/docs/status/tasks/hardware-review-20261010-artifact-review.md).
  Old archives stay BLOCKED; missing native requirements are enforced in fixed archives.

Actual final CI: mandatory quick38033766837/job114159912573 completed success
2026-10-10T07:36:18Z. Independent tester downloaded full60874-byte log;
actual checkout8a15d644 and literal `CI GATE PASSED` verified. All job steps
success; unchanged full gate (including35turbo tasks and149startup harness checks).
Packaging38033766825 (7+81 tests) and provisioning38033766876
(46+23+11+18 tests) completed success on final5bd7e8b. No test or workflow weakened.

Hardware package preparation build exit0; all41 mandatory Debian fixtures PASS.
All4fixed application archives and7selected VPP packages hashed/verified. No offline
dependency-closure or release acceptance claim.

Premerge entire-worktree gitleaks default/no-config scan returned10 matches in
preexisting test fixtures and generated copies; merge held for independent R2
triage. The unchanged repository-authoritative `.github/gitleaks.toml` configured
full no-git scan exited0: scanned114800082bytes in11.4s; no leaks found.
Independent R2 verified all10 matches each contain exactly one sanctioned synthetic
placeholder. Eight tracked source matches are byte-identical to main; two generated
matches correspond to unchanged TypeScript tests. Its independently rerun configured
full no-git scan also exited0:114800082bytes/15.7s/no leaks found. Existing allowlist
is byte-identical to main. No real leaked secret, new exemption or product mutation.
[Published R2 addendum](https://github.com/mcoder1001-cyber/NGFW/blob/fb4ebd5d5b88bbb13ee484f2028ba538eeeb448b/docs/status/tasks/hardware-211-20261010-review-R2.md)
is verified remotely atfb4ebd5d5b88bbb13ee484f2028ba538eeeb448b.

Combined source verdict: APPROVE, T1 PASS. Premerge R2 triage confirms the configured
result. All applicable source gates and reviews passed. This review does
not authorize filesystem repair or count unexecuted hardware tests as passed.

Both target roots have active structural ext4 errors and failed boot fsck.
No installation, NIC binding, firstboot, service changes, filesystem repair or
reboot performed. Management ports/routes/SSH remain intact. Physical import,
forwarding, restart/reboot and throughput acceptance remain NOT RUN. Console/rescue
information and off-host backup/offline recovery are prerequisites to resume.

## Integration readback

Expected-head GH merge, without admin/bypass, completed. Actual remote main
`4908716b4501312102382e6979b8fc1ded6f9311` has exact parents d2d55984 and5bd7e8b
and treea0d7b7 equal to the completely tested PR integration tree. PR217 merged
2026-10-10T07:46:33Z. No local main/user worktree was edited; the manager's own
branch was fast-forwarded to published main. Main push bare quick38035583209
subsequently COMPLETED SUCCESS; packaging38035583240 and provisioning38035583210 subsequently completedSUCCESS. Main-CI PASS independently confirmed in final receipt below.

## Configuration recovery preparation

Both read-only configuration snapshots are stored privately outside Git and the
candidate bundle. Published receipts: .37at0b96a5ff51aca239e2b1492456c37e2052f139ed,
.211at269d455f6aee29cc89007bbac4aa93d00c0fad7f. Independent R7 checked actual archive
readability, management-netplan presence, permissions and candidate exclusion;
no new blocker. Root parent permission0755 was tightened to0700 immediately; nested
host directories were0700 throughout, so no content exposure was observed.
Native nft gap remains explicit despite successful empty compatibility exports.
[Recovery runbook](hardware-manager-20261010-recovery.md) keeps full-data backup and
console/unmounted offline diagnosis/repair prerequisites separate from these
configuration snapshots. No filesystem repair/target mutation or hardware PASS.

## Final independent main-gate receipt

T1 final PASS published/read back
[4a8a82205d490068d865a1344d86afcaf93b8502](https://github.com/mcoder1001-cyber/NGFW/blob/4a8a82205d490068d865a1344d86afcaf93b8502/docs/status/tasks/hardware-37-20261010-test-T1.md).
Main38035583209/job114165198295 completedSUCCESS on4908716b,allstepssuccess,
completedAt2026-10-10T08:07:08Z(API timestamp). Independent58932-byte log has
actualcheckout4908716b/mainbranch,35/35tasks,149harnesschecks and
`CI GATE PASSED` at2026-10-10T08:07:02.2874218Z. SHA256
9c957c9fe1c469f1acbde3f434b2b021ba21e6a09a6c666c0d10f3503df2fb12.
Fresh remote main parents/tree independently reasserted unchanged.
Root also read exactmainSUCCESS/allstepsmetadata and both mainfixtureSUCCESS.
[Recovery R7 addendum](https://github.com/mcoder1001-cyber/NGFW/blob/0eb7036450d410b7540e05a786205c72bd814acf/docs/status/tasks/hardware-evidence-review-20261010-postmerge.md) APPROVE on0f3ab280f,
with privatebackup metadata/readability/hash and bundle exclusion evidence.

Combined final package/source verdict APPROVE; T1 PASS before and after merge.
Actual hardware installation/acceptance remains NOT RUN, blocked offline recovery.
