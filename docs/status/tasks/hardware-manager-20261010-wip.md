# Hardware installation WIP — 2026-10-10 07:42 UTC

Branch: codex/hardware-manager-20261010.
Worktree: /root/ngfw-wt/hardware-manager-20261010.
At this status edit localHEAD/origin-main:4908716b4501312102382e6979b8fc1ded6f9311;
last published own-branch checkpoint:5bd7e8b545fc765fd2babd8dda15175d6f33af1b.
This status checkpoint will be committed/pushed immediately; discover its current
local/remote SHA with `git rev-parse HEAD` and
`git ls-remote origin refs/heads/codex/hardware-manager-20261010`. Publication is
reported on PR217 only after successful push/readback. Owned files are task docs,
API native Depends field in deploy/debian/ngfw/debian/control, external build output.
No edits to another worktree or local main. User workspace remains untouched.

Compiled source:2045ab8b3d2f477bb23446fb5e58b7d9d3abea3c, remotely preserved in
codex/archive-hardware-manager-20261010. Prior integrationbde83bc preserved in
codex/archive-hardware-manager-20261010-integration. Final D112 one-commit source
5bd7e8b has identical product content. PR217 merged expected head without bypass;
remote main4908716b has exact expected parents/current main d2d55984 plus5bd7e8b
and fully tested treea0d7b7cbc7f37dc2fadd9a93d41ce486dcda50d2.

Completed host-independent code: API package now consumes existing shlibs:Depends,
enforcing actual native libc6>=2.34/libgcc-s1>=4.2 plus Node22 bounds. No other
product/test/build/CI change. Corrected clean prepare completed14TS tasks,
seven Go helpers, production API deployment. Unchanged dpkg-buildpackage exited0,
all41 mandatory fixtures PASS. Current VPP verifier72tests/manifests/hashes/install
gate PASS; four corrected application archives and seven selected VPP packages
preserved with hashes. Old runtime/archives remain explicitly BLOCKED.
Final hosted unchanged mandatory quick38033766837 SUCCESS: literal CI GATE PASSED
read independently, actual checkout8a15d644/fully tested treea0d7b7. Both additional
hosted fixture gates success. All R1/R2/R7/R8 APPROVE, T1PASS; immutable receipts in
[combined review](hardware-manager-20261010-review.md) and
[actual evidence](hardware-manager-20261010-evidence.md).
Premerge default full gitleaks found10 preexisting sanctioned test placeholders;
independent R2 triage confirmed no real secret. Unchanged canonical-config full
no-git scans both exit0/noleaks. No allowlist/test was modified to hide findings.
Main push quick38035583209 IN PROGRESS; post-merge PASS still unverified.

Hardware task: BLOCKED, installation and hardware acceptance NOT RUN. Both ext4
/dev/sda2 roots have structural checksum/extent/journal errors and failed boot fsck:
UNEXPECTED INCONSISTENCY at inode259596. Fresh strict-known-host SSH07:38UTC PASS;
.37enp12s0UP172.30.126.37/24/default172.30.126.1/group58/PCI0000:0c:00.0;
.211enp4s0UP172.30.110.211/24/default172.30.110.1/group28/PCI0000:04:00.0.
Both roots mountedrw, clean-with-errors; counts1258/.37,1248/.211.
No package transfer/install, firstboot/service change, NIC binding, route/config
change, filesystem repair or reboot performed. Management PCI/groups stay excluded.
Seven/seventeen data NICs have independently verified original names/PCI/group
maps; proposed configs are schema-validated and NOT APPLIED. .211 data bridges need
reviewed membership changes only after clean storage. Physical af_packet declarative
import unsupportedD105; guarded DPDK/true IOMMU, no no-IOMMU bypass.

Required recovery input: verified console/rescue (physical/serial/IPMI/KVM/LiveISO)
and off-host backup/offline repair path, or evidence owner already repaired root
unmounted and management returned. Asynchronous owner question remains unanswered.
Do not fsck mounted root, force unattended repair/reboot, or delete corrupt dirs.
After recovery: repeat root/fsck/device-health/SSH/route preflight; fix trusted time;
inspect package/service dependencies and preserve existing routing/firewall/netplan.
Complete canonical firstboot with VPPno-pci/managementblacklist, start agent/API,
seed builtin physical rows BEFORE any data PCI binding and before operator revisions.
Verify exact7/17 original names/markers and management exclusions; export authoritative
seeded dataplane, guarded apply-startup uses same names, reconverge and verify stored
and live inventory. Ordinary API import cannot fabricate physical markers. Then real
API/TLS/forwarding/restart/reboot persistence tests. No live forwarding/throughput,
offline dependency closure or release acceptance claimed.

Remote preflight .37=86a2f06eafc6406e3e5395769d923a09bbfa60aa;
.211=a4059c73b4a2e3346b7cf1cf705c0a5b908c317d. Resume existing owned host branches,
never silently rebuild. Live roles at07:42:rootmanager,host_37independentmainT1;
R2/R7/R8 finished awaitingresume; no active installers/persistent supervisor.
Board212tasks:205merged,7parked,0running/ready/review. Adhoc hardware request has no
invented WBS state. This cycle has one new product merge(PR217), no stale-row fixes.
Root disk~500MiB available; do not duplicate broad local builds or delete others' work.
Outputs/logs:/root/Documents/Codex/2026-10-10/hardware/.

Current failure: active target root-filesystem errors; verified rescue input missing.
Remaining: main bare quick completion/readback; final docs checkpoint/publication;
then offline recovery and actual installation/hardware testing when inputs arrive.
Exact next command: `gh run view 38035583209 --json headSha,status,conclusion,jobs,url`.
