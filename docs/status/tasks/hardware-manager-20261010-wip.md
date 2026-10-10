# Hardware installation WIP — 2026-10-10 06:58 UTC

Branch: codex/hardware-manager-20261010; base origin/main
`d2d55984d74fa1d06c32e8271886f11f16375407`.
Last published/read-back source checkpoint: `2045ab8b3d2f477bb23446fb5e58b7d9d3abea3c`.
Reviewed history preserved in local refs/archive/hardware-manager-20261010 and
remote codex/archive-hardware-manager-20261010, independently read back at that SHA.
The final D112 single-commit HEAD is discoverable with `git rev-parse HEAD` and
`git ls-remote origin refs/heads/codex/hardware-manager-20261010`; publication is
reported only after push/read-back. Owned files: this task's docs, the API native
Depends field in deploy/debian/ngfw/debian/control, and external build output.

Completed: remote main/branches/PR/CI and board inspected at startup. Board212 tasks:
205 merged,7 parked,0 running/ready/review. No existing PR at startup. No board state
was invented for this additional hardware request. Prior complete hosted quick
run37903333143 PASS on reviewed product source30de26ee; final corrected source still
needs its own unchanged complete hosted quick gate before merge.

Target preflight: strict-known-host BatchMode SSH to both PASS; no NGFW/VPP installed.
Both ext4 root /dev/sda2 filesystems have active checksum/journal errors and failed
boot fsck (invalid inode259596 extent; UNEXPECTED INCONSISTENCY). Independent review
confirmed. Package install, data NIC binding, firstboot, service start, route edits,
filesystem repair and reboot have NOT RUN. Fresh SSH still PASS, addresses/routes
intact. Management exclusions: .37 enp12s0/0000:0c:00.0/group58/default172.30.126.1;
.211 enp4s0/0000:04:00.0/group28/default172.30.110.1.

Host inventories independently verified: .37 seven data NICs; .211 seventeen.
External proposed-dataplane-{37,211}.json schema checks PASS, exact PCI/port mappings
and matching whitelists reviewed. Management groups excluded; all groups singleton.
Proposals are unapplied. Existing .211 data bridges need reviewed topology handling.
Physical af_packet import is unsupported (D105); guarded DPDK and persistent
physical-interface rows/validated-confirmed commit/live retrieval remain necessary.

Build evidence: VPP26.06+ngfw3 full verifier72 tests/manifest/hashes/install gate PASS.
pnpm frozen604-package install PASS; fresh14/14 TypeScript build tasks PASS including
web bundle budget; all seven Go helper builds and production API deployment PASS.
Initial cc80be66edcf dpkg-buildpackage exit0,41 unchanged required fixtures PASS.
Independent native archive review found missing generated libc6>=2.34/libgcc-s1>=4.2
API Depends. Old runtime/ artifacts are BLOCKED. Source correction consumes existing
shlibs:Depends; no compiler sources, test expectations, rules or CI changed.
R8 independently APPROVE source2045ab8; receipt3009a40ead3d73e0c1ed277d334cfc739f2d67a7.

Corrected clean prepare from2045ab8 completed exit0, version0.1.0~dev+2045ab8b3d2f,
at /dev/shm/ngfw-hardware-package-fixed-20261010. Full verifier/build/deploy PASS again.
Corrected unchanged dpkg-buildpackage -us -uc -b RUNNING; its41 required fixtures
passed already, native archive/control inspection remains. Logs and proposed configs:
/root/Documents/Codex/2026-10-10/hardware/. Root free space~938MiB; tmpfs~1.4GiB.
Do not delete other work or modify shared-host VPP. No target packages transferred.

Live roles: root manages/builds this correction; install_review R8 archive review;
host_37 resumed only for independent R1/T1; host_211 resumed only for R2. Neither
host agent is installing. Their target preflight work remains awaiting offline recovery.
Published preflight .37=86a2f06eafc6406e3e5395769d923a09bbfa60aa;
.211=a4059c73b4a2e3346b7cf1cf705c0a5b908c317d. See separate role receipts as they finish.
Reviewer25 Python fixtures PASS; its prior Go command printed ok but was stopped
while stalled afterward, so it is not reported as a completed PASS.

Current blocker: offline repair requires verified console/rescue and backups first.
Owner console information requested asynchronously, still unanswered. Do not run
fsck on mounted root, force fsck/reboot, or erase directories to hide corruption.
Remaining: independent fixed archive/R1/R2/R7/R8/T1 receipts, unchanged complete
hosted quick and expected-head single-commit merge; then offline recovery, guarded
install, application import and real packet/API/service/reboot persistence tests.
Hardware acceptance/throughput and aggregate release claims remain unverified.

Exact next command: `tail -40 /root/Documents/Codex/2026-10-10/hardware/package-build-fixed.log`;
when build exits0, preserve only task-owned fixed artifacts in external runtime-fixed/,
record hashes and ask the independent reviewer to inspect final native dependencies.
Prepare the final one-commit PR and wait for its complete hosted quick; recheck main
and PR heads before merge. After owner supplies verified console/rescue, resume each
existing host branch/worktree from its durable envelope for offline recovery.
