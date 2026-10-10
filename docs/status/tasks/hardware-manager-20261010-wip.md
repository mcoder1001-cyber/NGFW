# Hardware installation WIP — 2026-10-10 07:07 UTC

Branch: codex/hardware-manager-20261010; base origin/main
`d2d55984d74fa1d06c32e8271886f11f16375407`.
Last published/read-back integration checkpoint: `bde83bc87ae817a61bbc70e4029f76109ae77c35`.
Compiled source checkpoint: `2045ab8b3d2f477bb23446fb5e58b7d9d3abea3c`.
Integration history also preserved remotely as
`codex/archive-hardware-manager-20261010-integration` at bde83bc87.
Actual command/output excerpts and immutable independent receipts:
[hardware-manager-20261010-evidence.md](hardware-manager-20261010-evidence.md).
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
Corrected unchanged dpkg-buildpackage -us -uc -b completed exit0; all41 required
fixtures passed. Four fixed runtime-fixed/ archives/buildinfo/changes/hash manifest
are preserved. API final Depends now enforces libc6>=2.34/libgcc-s1>=4.2 plus Node22.
R8 independently inspected all fixed archives/helper bytes/units and APPROVE
integrity/metadata; its publication receipt follows separately. Logs/proposed configs:
/root/Documents/Codex/2026-10-10/hardware/. Root free space~565MiB; tmpfs~1.3GiB.
Do not delete other work or modify shared-host VPP. No target packages transferred.

Live roles: root manages/builds this correction; install_review R8 archive review;
host_37 resumed only for independent R1/T1; evidence_review is independent R7.
host_211 completed R2 (published da9426156320e5a95fd1b26351af43bb102a5f1a) and is
awaiting resume. No host agent is installing. Their target preflight work remains awaiting offline recovery.
Published preflight .37=86a2f06eafc6406e3e5395769d923a09bbfa60aa;
.211=a4059c73b4a2e3346b7cf1cf705c0a5b908c317d. See separate role receipts as they finish.
Reviewer25 Python fixtures PASS; its prior Go command printed ok but was stopped
while stalled afterward, so it is not reported as a completed PASS.

Current blocker: offline repair requires verified console/rescue and backups first.
Owner console information requested asynchronously, still unanswered. Do not run
fsck on mounted root, force fsck/reboot, or erase directories to hide corruption.
Remaining: final applicable R1/R7/R8/T1 receipts and complete unchanged hosted
quick on amended final PR217 HEAD, then expected-head single-commit merge; then offline recovery, guarded
install, application import and real packet/API/service/reboot persistence tests.
Hardware acceptance/throughput and aggregate release claims remain unverified.

Exact next command: `gh pr checks 217`; keep all complete hosted quick steps
unchanged. Final amended docs correct R7's command/output evidence gap without
changing product/test/build/CI files or rebuilding packages. Source history is
archived remotely before amendment. Recheck main and expected PR head before merge. After owner supplies verified console/rescue, resume each
existing host branch/worktree from its durable envelope for offline recovery.

Source-derived installation ordering (independent R8; NOT live-tested): firstboot
canonical api.env must complete first. Start VPP with no-pci and agent, keep database
without operator revisions/candidate edits. Enable NGFW_SEED_DEFAULT_NICS=1 only
after provisioning and BEFORE any data PCI binding; API seeds revision1 containing
original netdev names and physical markers. Unbound physical NICs are intentionally
not applied yet; dataplane is stored/notApplied. Verify exact7/17 rows and management
exclusions, export authoritative seeded dataplane, then guarded apply-startup uses
the same names. Binding first would erase kernel names and change seed fallback
names; ordinary API edits cannot add physical markers. Do not fake them with import
or write DB rows directly. Real seed/startup/retrieval/forwarding verification remains.
