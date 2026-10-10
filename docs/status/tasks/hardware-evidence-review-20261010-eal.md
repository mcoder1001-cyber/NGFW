# Independent EAL failure review — same hardware campaign

Scope: docs/source/private receipt inspection only. Reviewer performs no target or
product mutation. Current physical acceptance verdict: BLOCK pending a corrected
renderer/native artifact and actual guarded physical result. This is a real runtime
failure, not a deferred laboratory-only test.

Actual root-owned failure receipt f64ad15bd241d4bbed2e3219900b6836138dcaf13be17ce2f916df2bc9559de1
is18019B0600. Independent selected JSON assertions confirm no COMMITTED, rolled-back
marker present, no console/deadman marker; five distinct VPP starts failed EINVAL22.
Native log reports old configuration restored, VPP/management healthy at14:30:10
and locks released at14:30:11. The inactive/dead native unit Result=success reflects
successful rollback. Full private journal/config/network remain off Git.

Independent source command/output:

```text
Python urllib read exact FDio VPP c3200b88dc46bd380f00a49ca3392a102cc1980b files
src/plugins/dpdk/device/init.c SHA256=6e29b55f722f348f62b8aecf3dc543a4e381475410586bd51a296aab6cbff660
build/external/packages/dpdk.mk SHA256=0590711efcc03255442fbb826e29d2ada8acf6f7dc819d4dcf583da13b030c93
dpdk.mk default version=26.03; tarball SHA pinned
literal PCI blacklist => EAL -b at init.c1303–1310
configured nonblacklisted devices => EAL -a at init.c1485–1500
DPDK26.03 eal_common_options.c215–218 rejects nonempty allow and block lists
DPDK26.03 Linux eal.c556–560 maps that parser failure to EINVAL
actual failed rendered startup:17 dev rows + management04 blacklist
```

Pinned [VPP parser](https://github.com/FDio/vpp/blob/c3200b88dc46bd380f00a49ca3392a102cc1980b/src/plugins/dpdk/device/init.c#L1303)
turns these clauses into incompatible EAL options. Its [DPDK build pin](https://github.com/FDio/vpp/blob/c3200b88dc46bd380f00a49ca3392a102cc1980b/build/external/packages/dpdk.mk#L24)
matches the independently read [DPDK conflict check](https://github.com/DPDK/dpdk/blob/v26.03/lib/eal/common/eal_common_options.c#L215)
and [Linux EINVAL path](https://github.com/DPDK/dpdk/blob/v26.03/lib/eal/linux/eal.c#L556).
This is a source-derived cause tied to the actual configuration and failure;
full live expanded EAL argv was not captured. No diagnostic VPP relaunch is required.

Appropriate narrow correction: when explicit physical dev rows exist, emit only
their closed allowlist; exclude protected management and blacklisted PCIs from
that allowlist through the existing rejecting model. Do not also emit literal
EAL blacklist clauses. With no physical dev rows, retain existing noPCI/blacklist
output unchanged. Canonical apply separately snapshots protected PCI identities
through management interface discovery; this must remain independent of rendered
blacklist syntax. Independent final product diff/regressions, full hosted quick,
corrected native helper identity and exact fresh render/dryrun precede any retry.

Root owns the isolated mandatory correction. Worker source diagnosis was published
at3363c2293510a907184b176e0d1ed44150faf690; reviewer independently used the primary
sources above and actual receipt, not just that worker conclusion. Current .211
VPP is rolled back to noPCI; buffers65536 remains stored but not enforced. Physical
enumeration/forwarding/reboot acceptance remains incomplete.

## Independent exact correction source review

PR226 candidate97ae88ee5b6aaf304f78547abed39e80bbeac5e1 is one commit on
d1f3f19d4837de3f7a36bfffcbd3c70593bea307, tree449ea85e5111ad946245200dbf8f6a3490c5353a.
Actual remote readback matches. All11 declared files read; product correction
source applicability APPROVE. The template branches on a nonempty validated
device list, emits the closed explicit data allowlist plus management-exclusion
comments, and uses literal blacklist/no-pci only without devices. buildDevices
still rejects every protected management PCI in whitelist/devices. Canonical apply
still independently discovers management interfaces/PCI and snapshots their drivers
at deploy/vpp/apply-startup.sh939–943. No contract, privilege or VPP-version change.

Exact independent focused command/output, executed read-only against root's
isolated correction source checkout, not presented as this reviewer's product edit:

```text
CWD=/dev/shm/ngfw-hardware-firstboot-integration-20261010/apps/agent
go test ./internal/renderers/vppstartup -run 'Test(DPDKDeviceSelectionDoesNotMixEALAllowAndBlockLists|ManagementFromHost|ManagementPaths|SixNICSample|Golden)$' -count=1
ok  ngfw/agent/internal/renderers/vppstartup 0.092s
git status --short
<empty after root's coherent commit>
git ls-remote origin refs/heads/codex/hardware-eal-fix-20261010 refs/heads/main
97ae88ee5b6aaf304f78547abed39e80bbeac5e1 refs/heads/codex/hardware-eal-fix-20261010
d1f3f19d4837de3f7a36bfffcbd3c70593bea307 refs/heads/main
gh run view 38061027648 --json status,conclusion,headSha
{"conclusion":"","headSha":"97ae88ee5b6aaf304f78547abed39e80bbeac5e1","status":"in_progress"}
```

R7-1 BLOCKER: eal-fix-wip.md10 claims the root's Go package tests passed0.435/0.274
without exact command/pasted stdout; the completed check14s also lacks pasted
output in the candidate. Mandatory R7 requires command/output evidence, not only
prose. Root accepted the finding and will append existing output once immutable
97ae native preparation completes; no new test is requested. R7 current verdict
BLOCK for this documentation item, separate from the source applicability APPROVE.
Pending complete hosted quick/native/runtime results are correctly kept pending.
Decision D246 records closed allowlist, rejected mixing and retained management
validation. Scope stays within the owner's existing hardware correction campaign.

## R7 evidence closure on final48af

Final PR22648afba4adfcfe7f0ef0d8585d7c1afc9ed5edde9 /
treeeea909008cc78e09b2d4cfabe793291c2955ce44: R7-1 CLOSED, **R7 APPROVE**.
Independent remote/archive/parent/diff assertions:

```text
git diff --name-only 97ae88ee5b6aaf304f78547abed39e80bbeac5e1 48afba4adfcfe7f0ef0d8585d7c1afc9ed5edde9
docs/status/tasks/hardware-manager-20261010-eal-fix-wip.md
git rev-list --count d1f3f19d4837de3f7a36bfffcbd3c70593bea307..48afba4adfcfe7f0ef0d8585d7c1afc9ed5edde9
1
git show --format='%P' --no-patch 48afba4adfcfe7f0ef0d8585d7c1afc9ed5edde9
d1f3f19d4837de3f7a36bfffcbd3c70593bea307
remote archive codex/archive-hardware-eal-97ae-20261010=97ae88ee5b6aaf304f78547abed39e80bbeac5e1
remote integration/main=48afba4adfcfe7f0ef0d8585d7c1afc9ed5edde9/d1f3f19d4837de3f7a36bfffcbd3c70593bea307
gh run list --branch codex/hardware-eal-fix-20261010 --limit6
CI gate38061529505 head48af status=in_progress
CI gate38061027648 head97ae status=completed conclusion=cancelled
```

Existing exact Go commands/output0.435/0.274 and lightweight check14s are now pasted
in the candidate WIP. It explicitly explains the check's precommit range0 and
does not substitute that check for the mandatory complete final-head gate. No
product/test delta since the independently approved97ae source; no rerun needed
for this evidence-only change. The clean native prepare remains truthfully compiled
from97ae, not relabeled48af. Independently read private prepare log409lines verifies
VPP26.06-release+ngfw3/all72fixtures and producer state; native dpkg build pending.

Additional actual root postrollback readonly receipt19981B0600,
SHA2562b3d67696d1c98967f1909c98821454d18bc90e18087513aa129bae88cdd06d5,
shows VPP72599active/NRestarts0 via systemctl0/empty stderr. A successful logging
query does not recover full failed EAL argv. R7 source approval does not change
physical failure status; corrected native/control/runtime/reboot acceptance pending.

## Native97ae archive and actual packaged generator review

Compiled source97ae88ee5b6aaf304f78547abed39e80bbeac5e1/version
0.1.0~dev+97ae88ee5b6a stays distinct from final48af documentation head.
Independent archive applicability **APPROVE**; no target installation or physical
retry is implied. Read-only original manifest4634B0600 SHA256
b2f3f6a78014ea76995c783919a375f4398ea4bf7ccbf56bfc32b3ae1472b893.
All four archive SHA/size and Package/Version/Architecture/Depends controls were
rederived with hashlib and dpkg-deb. Streaming dpkg-deb --ctrl-tarfile and
--fsys-tarfile checks independently compared all12 maintscripts byte-for-byte to
previous ee202 archives; all match. Shipped firstboot.sh and initial-dataplane.json
match pristine97ae source and previous ee202 payload bytes. Native agent
b3c7cc10671abcab6d11c7dce12323f284c64822a15cfda1c46ba78803283744 and startupgen
55e490bad1c302505231df105a1d516cb074678b38b85e28740c399020aac619 match manifest.
The private reviewer-only archive/index receipt1452557B SHA256
5600110043131c6d11555412c9a139d36f29ac5fbb46b2c7f72755b0278b458c
is not committed. An initial overstrict payload-path assertion stopped on the
API's relocated PNPM workspace; no initial complete PASS was claimed. The final
streaming audit resolves archive hardlinks and normalizes workspace paths for
content comparison. API dist files and native argon2 bytes are unchanged; remaining
differences are deployment/workspace metadata, CLI wrappers and changelog. Web and
meta payload differ only in changelog. Updated compiled Go helper hashes are
recorded rather than incorrectly asserted byte-identical to prior source.

Actual selected build-log commands/output (read only, not reviewer rerun):

```text
rg -n '^(Ran |OK$|dpkg-buildpackage:|.*dh_builddeb)' /root/Documents/Codex/2026-10-10/hardware/package-build-eal-97ae.log
dpkg-buildpackage: info: source version 0.1.0~dev+97ae88ee5b6a
Ran 16 tests in 0.833s
OK
Ran 12 tests in 0.401s
OK
Ran 6 tests in 20.517s
OK
Ran 3 tests in 0.002s
OK
Ran 4 tests in 0.309s
OK
dh_builddeb
dpkg-buildpackage: info: binary-only upload (no source included)
```

The actual native generator was extracted only into the reviewer's own private
scratch and executed with the same fixture facts as the real CLI regressions.
Every command used the exact argument prefix below; JSON document bytes were
passed on stdin, with `--current none` appended for firstboot:

```text
/dev/shm/evidence-review-eal-native-20261010/new/usr/lib/ngfw/bin/ngfw-startupgen --no-host --mgmt-pci 0000:0b:00.0 --plugin-dir /dev/shm/evidence-review-eal-native-20261010/plugins --online-cpus 0-31 --numa-nodes 2 --hugepages-mb 2048 --current /dev/shm/ngfw-hardware-firstboot-integration-20261010/apps/agent/internal/renderers/vppstartup/testdata/host-startup.conf
native six-nic-sample PASS exit0 golden_equal True closed_data_devices 6 blacklist_rows 0
native whitelist-only PASS exit0 golden_equal True closed_data_devices 2 blacklist_rows 0
native empty PASS exit0 ee202_bytes_equal True no_pci True management_blacklist_retained True
native firstboot PASS exit0 ee202_bytes_equal True no_pci True management_blacklist_retained True
native management-whitelist PASS refusal2 stdout0
native management-device PASS refusal2 stdout0
native missing-LCP PASS refusal2 stdout0
native_receipt 9 commands 2543 88314bf6e2c29b622fcf888f390b06efae91ddb23db030958c0d0803cb7e69d9
```

Inputs are exact cases/six-nic-sample.json, whitelist-only.json, empty.json and
the packaged initial-dataplane.json. Positive outputs equal the final corrected
goldens. Empty/firstboot are compared to the extracted prior ee202 generator
using the identical argument prefix; firstboot keeps all three policy plugins.
Negative documents are {"dataplane":{"pciWhitelist":["0000:0b:00.0"]}} and
{"dataplane":{"devices":{"0000:0b:00.0":{"name":"mgmt"}}}}. Missing-LCP
temporarily removes only the fake plugin file in the reviewer's scratch; refusal2
names linux_cp_plugin.so. No controller or target VPP/device/network operation.
These are offline fixture facts, not live hardware enumeration acceptance.

```text
readelf -d /dev/shm/evidence-review-eal-native-20261010/argon2.linux-x64-gnu.node
NEEDED libdl.so.2 libgcc_s.so.1 librt.so.1 libpthread.so.0 libc.so.6 ld-linux-x86-64.so.2
dpkg-deb -f ngfw-api_0.1.0~dev+97ae88ee5b6a_amd64.deb Depends
libc6 (>= 2.34), libgcc-s1 (>= 4.2), nodejs (>= 22), nodejs (<< 23), adduser, python3
```

## Same-campaign four-package phase applicability

Worker source1f29e159ccc4d64ff1554a343208d875bb7805d77c1f6bf11866ae846a9cc40c
at e98aabdd115373fb5e3d09b24abc2f5ce2980906 and preceding contract read in full.
ROOT-only source applicability APPROVE for its explicit phase boundaries. It
checks the immutable source/manifest/four archive identities and pristine current
native proof, clean root/trusted time/protected singleton management, full L3,
17 VFIO versus7 original igc, owner-bound cache and tracked private credential
file metadata, original firstboot state, all21 units and all16 sysctls. Only .37
hold-runtime stops API then agent. Policy101/persistent mask have finite fsynced
original/owned identities, actual init-system-helpers1.69 hash; no VPP stop/restart
in upgrade. Framed stdin and concurrent private spools retain controller argv-limit
and early-refusal fixes. Native install is only four exact Inst/0Remv after immediate
plan equality; no dpkg timeout kill. Restore requires actual successful install
proof and restores original absence while retaining recovery record.

Actual controller-only commands/output:

```text
python3 docs/status/tasks/hardware-211-20261010-upgrade-eal-four.py --host 211 inspect --validate-inputs-only --manifest /root/Documents/Codex/2026-10-10/hardware/runtime-eal-97ae/manifest.json --manifest-sha256 b2f3f6a78014ea76995c783919a375f4398ea4bf7ccbf56bfc32b3ae1472b893
host211 modeinspect target_contacted=false generated_AST_PASS=true data_devices17 archives_verified4 generated_source_bytes27833
python3 docs/status/tasks/hardware-211-20261010-upgrade-eal-four.py --host 37 hold-runtime --validate-inputs-only --manifest /root/Documents/Codex/2026-10-10/hardware/runtime-eal-97ae/manifest.json --manifest-sha256 b2f3f6a78014ea76995c783919a375f4398ea4bf7ccbf56bfc32b3ae1472b893
host37 modehold-runtime target_contacted=false generated_AST_PASS=true data_devices7 archives_verified4 generated_source_bytes26421
```

Actual ROOT-parent receipts independently hash/mode/structure verified:

| Receipt | Bytes | SHA256 |
| --- | ---: | --- |
| manager-eal-upgrade211-inspect-20261010T150636Z.json | 310484 | 02d145ee91805f4fc1f71275c7473c90ab078f76ce67eaabe81d585c7bce80ae |
| manager-eal-upgrade37-hold-runtime-20261010T150643Z.json | 240191 | cef25c5e18ce8de14ea7c92fd0b83b984c157dfb920df183882715bc0d00b43f |
| manager-eal-upgrade37-inspect-20261010T150657Z.json | 218745 | f00385b9a9218779029811865e59900bbb5c4d0185e854ce0c39d613520904a0 |

All0600/root, SSH0/outer stderr0, ioerr0x6 stable/no new storage events. .211
17VFIO/VPP72599/nginx9281 and .37 sevenigc/VPP7820/nginx9669 match. .37 stop
commands0 only API/agent; their actual after states inactive/PID0. Fresh inspect
original policy/mask absent, nr1024 and exact helper1.69. ROOT prepare/upload/sim
applicability APPROVE after these actual proofs. Installation remains held for
complete final48af quick green and actual exact four-package plan review. Neither
artifact/source nor inspection approval is physical/reboot acceptance.

## Historical operational gitleaks classification

Read the REDACTED report from root's failed scan against its historical490871 base,
then independently read exact public historical line contexts without printing
private values. Six generic-api-key findings are public operational prose, not
credentials or checksum findings. Five at b1f5bea6bb8a7964165b9ba314d09ecbd238750a
and1c149b1a28da457f81e256fdc914e279196a03a6 describe three environment fields
preserved, equal routing and no activation. Sixth at39a77d066cb491c36b7e4b0b4ecc9a29ff5bf2f0,
hardware-manager WIP1465, describes two held services and seventeen remaining
VFIO devices. An exact boolean comparison confirms that public NIC-state
sentence is the matched context. This classification retains the failed scan, adds no rule exclusion,
does not waive any required gate and does not call that historical check clean.
ROOT separately reports cleaned working-directory wording; final product48af quick
is unaffected and independently still in_progress at this checkpoint.

## Completed final gate and actual upgrade plan/failure

The prior in-progress gate is historical. Independent actual lookup now:

```text
gh run view 38061529505 --json status,conclusion,headSha,jobs
status=completed conclusion=success headSha=48afba4adfcfe7f0ef0d8585d7c1afc9ed5edde9
Mandatory quick gate: completed/success; Run repository gate: success
gh pr view 226 --json state,headRefOid,mergeCommit
state=MERGED headRefOid=48afba4adfcfe7f0ef0d8585d7c1afc9ed5edde9 mergeCommit=0c21e65eea8b2d2cfb61f57ec9f6b57a0e6c55f3
git fetch origin 0c21e65eea8b2d2cfb61f57ec9f6b57a0e6c55f3
git show -s --format='%H %T %P' 0c21e65eea8b2d2cfb61f57ec9f6b57a0e6c55f3
0c21e65eea8b2d2cfb61f57ec9f6b57a0e6c55f3 eea909008cc78e09b2d4cfabe793291c2955ce44 d1f3f19d4837de3f7a36bfffcbd3c70593bea307 48afba4adfcfe7f0ef0d8585d7c1afc9ed5edde9
gh run view 38062974117 --json status,conclusion,headSha
status=in_progress conclusion="" headSha=0c21e65eea8b2d2cfb61f57ec9f6b57a0e6c55f3
```

Both actual simulations independently hash/mode/shape verified in ROOT parent:
.211 manager-eal-upgrade211-simulate-20261010T150936Z.json312315B0600 SHA256
71377b9ad75e10e678d532049412a83e22878a69fecf0f32c088748293966b2f;
.37 manager-eal-upgrade37-simulate-20261010T151003Z.json220572B0600 SHA256
2ad25fd1090d783be01a0e3e791023f645d13e1a33fbe842898a65ff4da27af9.
Both exit0, manifestb2f3 exact, guards retained, ioerr6/no new storage events.
Each has the actual same four Inst rows and no Remv:

```text
Inst ngfw-meta [0.1.0~dev+ee2025007293] (0.1.0~dev+97ae88ee5b6a local-deb [all]) []
Inst ngfw-api [0.1.0~dev+ee2025007293] (0.1.0~dev+97ae88ee5b6a local-deb [amd64]) []
Inst ngfw-web [0.1.0~dev+ee2025007293] (0.1.0~dev+97ae88ee5b6a local-deb [all]) []
Inst ngfw-agent [0.1.0~dev+ee2025007293] (0.1.0~dev+97ae88ee5b6a local-deb [amd64])
```

Exact1f29 INSTALL phase was approved after green gate/plans, conditional on its
immediate re-sim/guard assertions. This reviewer missed Debian lexical git-suffix
version ordering. Actual .211 attempt then APT100 before package changes; preserved
manager-eal-upgrade211-install-20261010T151153Z.json338582B0600 SHA256
7a64a439ca5d6d0f63712efc8dc7211847ce44f9b19115d07f74d2e94efe61f9.
Independent parse verifies all four still configured ee202, old binaries,
dpkg-audit0/empty, protected_unchanged=true/no new storage events and guards retained.
Actual stderr and controller-only ordering proof:

```text
E: Packages were downgraded and -y was used without --allow-downgrades.
dpkg --compare-versions '0.1.0~dev+ee2025007293' gt '0.1.0~dev+97ae88ee5b6a'
exit0
```

.37 was not attempted. Bounded same four-artifact flag correction and exact
preservation of prior transaction stdout/stderr are pending source review before
retry. No relabeling source/artifact version or broad package downgrade permission.
Additional direct independent prior/current whole-network/sysctl comparison PASS
both hosts, canonical environment field-name set PASS. .211 complete postrollback
protection evidence is reviewed; fixed physical retry has not occurred.

## Reviewer checkpoint check failure preserved

Reviewer checkpoint542bf9146b304d39b17c8ba3d5a3cd8bb9bcb08f was published and
read back. Its actual post-commit check exited1 because this report copied two
historical operational prose triggers into lines248/249. The REDACTED report
records exactly two generic-api-key findings in that commit; no private value
output. This reviewer introduced that documentation error. Failed receipt remains
in logs/ci/hardware-evidence-review-20261010-20261010-151026-392017. Before amending
only this own latest checkpoint, remote archive
codex/archive-hardware-evidence-review-542bf-20261010 was pushed and exact542bf SHA
read back. The prose above now uses separated words; no gitleaks configuration,
rule, exclusion, product or main-history change and no waiver. The unchanged
post-commit check must be rerun on this amended receipt.

Actual amended51e60 post-commit check completed exit0, `check PASSED (0m14s)`;
gitleaks342.32KB/no leaks and board212 valid. Failed542bf archive/log remains.

## Source-only corrected .37 preflight and finite record

Initial preflight2a68c044ef6e1db1c20e073a5e289d16789a974f14a730e7324a84b7899eb651
set its final storage counter to a constant after observing it only at the start.
Independent concrete finding: that receipt cannot establish counter stability.
Root fixed before execution; CLOSED on source30f98252bcefafa48655659c5ab21e2572c2a205f0c66e9519cb42b27157d675
at897cf05f2c44c00da21b7896b29280024c06dcc5. Exact diff reads before and after,
requires equality and actual6, records actual values, and uses the reviewed
word-boundary UNC/ATA/sd classifier. Outer/REMOTE AST PASS. Readonly preflight source
APPROVE after actual new97ae upgrade and guard restoration. It renders exact
confirmed native7 document, demands seven data rows/no raw EAL blacklist/protected
0c exclusion/all3plugins, uses canonical dryrun only, and checks whole L3/DNS/
sysctl/NFT/held-service identity before and after.

Record sourcee2be408b0cd509e2a6611b79e5e807c6f971018190ff97a9035f32c607d1543c
conditionally APPROVE: exact actual preflight/doc/native7 equality, current VPP/
nginx PIDs and held service0 states, original protected7 devices/sysfs/empty route
facts, original startup UID/GID/mode and absent three owned file identities,
trusted root parents, finite private fsynced backups before driver/startup changes.
Previously reviewed binder0de/rollback00bed/unitd537 pins retained. No .37
preflight/record/bind/apply action or physical acceptance is claimed here.

## Bounded version-ordering correction and fresh-phase scope

Source6da8b8e09b87ca75b5bff651370fae04bb2907da60e9a3f69703e35caf731440,
published83f39e7b38b111a2f1dc9d06edbcb6909a698e63, independently APPROVE.
Read preceding45f2b6e4a contract and exact83f diff. Only the existing four pinned
local archives admit `--allow-downgrades`; old/new identities, zero removals,
only-upgrade/no-remove, immutable plan and immediate identical re-simulation stay.
No repository package downgrade or broadened dependency plan is admitted.

New explicit preserve-failed-attempt mode is limited to actual .211 failure7a64.
It proves old four identities, original binary, empty audit, unchanged protected
state/guards and receipt-equal268/72-byte logs. Both root-owned0600 single-link
RAM files are copied into a fresh private disk-record child; identities and both
copies are fsynced/read back before either original is removed. Descriptor and
path inode/device/size/mode/bytes are rechecked before each exact unlink. Archive
membership stays exactly four. Future attempt logs use unique private disk-record
names rather than polluting the archive directory. Failure100 remains historical.

Actual independent commands/output:

```text
sha256sum hardware-211-20261010-upgrade-eal-four.py
6da8b8e09b87ca75b5bff651370fae04bb2907da60e9a3f69703e35caf731440
Python AST: outer + REMOTE + UPLOAD_BOOTSTRAP PASS
Rehash/parse manager-eal-upgrade211-install-20261010T151153Z.json
SHA7a64a439ca5d6d0f63712efc8dc7211847ce44f9b19115d07f74d2e94efe61f9
upgrade_exit100; protected_unchanged=true; no_new_storage_errors=true
dpkg_audit exit0/empty; exact_four_configured=false
retained stdout268 bytes / stderr72 bytes
python3 /root/ngfw-wt/hardware-211-20261010/docs/status/tasks/hardware-211-20261010-upgrade-eal-four.py --host 211 preserve-failed-attempt --validate-inputs-only --baseline /root/Documents/Codex/2026-10-10/hardware/manager-eal-upgrade211-inspect-20261010T150636Z.json --baseline-sha256 02d145ee91805f4fc1f71275c7473c90ab078f76ce67eaabe81d585c7bce80ae --manifest /root/Documents/Codex/2026-10-10/hardware/runtime-eal-97ae/manifest.json --manifest-sha256 b2f3f6a78014ea76995c783919a375f4398ea4bf7ccbf56bfc32b3ae1472b893 --failed-proof /root/Documents/Codex/2026-10-10/hardware/manager-eal-upgrade211-install-20261010T151153Z.json --failed-proof-sha256 7a64a439ca5d6d0f63712efc8dc7211847ce44f9b19115d07f74d2e94efe61f9
exit0: target_contacted=false; controller_output_written=false
generated_AST_PASS=true; archives_verified4; data_devices17; generated_source_bytes651258
git ls-remote origin refs/heads/codex/hardware-211-20261010
3045ba1270d94daa8ca13401ad471fa2ac6042c1
```

ROOT phase applicability: preserve exact previous logs, then fresh simulations on both
hosts; inspect actual plans before installation. No reviewer target operation.

## Fresh physical retry source and .37 nginx identity

Worker3045ba12 retry fe9c4e094e9b551d155a69a77a336f716e85d702f076c9d1699f5d7a6f347437
is conditionally applicable for readonly inspect/private supplemental record
staging and one canonical launch after actual97ae installation/guard restoration.
Original73fd/d965/735B367e startup, failedb1f rendering/f64 native transaction and
native document/resource2 remain immutable. Corrected generator55e490 must render
the closed17 allowlist, management absent, no raw blacklist, three plugins and
65536 pool intent. Launch-to-observer whole kernel window is retained.

Concrete observer finding sent to worker/root: before native_committed17_PASS,
WORK mode must also observe native run-unit inactive/success and released
canonical locks; markers and inactive dead-man timer alone do not publish that
terminal-state proof. This same evidence is already implemented in reviewed ROOT
.37 physical observer. Source-only focused correction requested; no extra host
test/relaunch or new user approval. Inspect/stage scope remains separate.

Finite rollback d1ca0e834fef7d00c265ff64e4f1c6a42c834d96eecbe9bbdeb4db21f82afbaa
conditionally APPROVE: exact new supplement/staged self-source, sealed safely
terminal work, stopped VPP/API/agent, nonblocking canonical locks, original17
driver/override/name/master/up-down states and only three unchanged owned files.
No global ID, broad interface/network cleanup or service start; originals retained.

ROOT .37 native observer's literal .211 nginx PID9281 was a real inherited-source
finding before execution. CLOSED at994643a6c0778a1f65bfa049b8d6647271bf9051:
only that expression changes to RECORD.record_before.expected_unit_PIDs.nginx.
Exact new source e2a0d066f5635b69ae38e84dc99f8713aef5a6ab09302240ff5d10f2dd3d4ea1;
independent outer/REMOTE AST PASS, diff exactly one line, remote root b162adaea
includes correction. Conditional source APPROVE after actual sealed COMMITTED7,
new package/binary/record proof; no native/reboot execution acceptance inferred.

## Actual corrected upgrade and main gate completion

ROOT exact6da8 preservation and fresh both-host simulations independently PASS;
then actual both-host installation PASS. All receipts original controller parent,
root-owned0600/fsynced by producer, independently rehashed and parsed:

| Receipt basename | Bytes | SHA256 |
| --- | ---: | --- |
| manager-eal-upgrade211-preserve-failed-attempt-20261010T152246Z.json | 336122 | 63703fe4c2fb9a73502cd437c0f1ee9aaffd06d4945567aa6966a35a3a29319c |
| manager-eal-upgrade211-simulate-20261010T152320Z.json | 312569 | 7c0cf51e209f69c6ef9ae96eeca09c920b8f801c53d89670fa6cd1e0670bea30 |
| manager-eal-upgrade37-simulate-20261010T152400Z.json | 220600 | 78505d6449ad102d0d3f7c54653ff9d88f192d96888600bfcf5fce08b7a24ec0 |
| manager-eal-upgrade211-install-20261010T152426Z.json | 342361 | c4f2f2e2e60571dc8741968a71bfda718c1e444980b404047dd2d6c1c44440cf |
| manager-eal-upgrade37-install-20261010T152527Z.json | 246850 | ababf4cc5984a5a7b286c438a27d5d0bd57e1e62ffcebff0c44f059008e88437 |

Actual preservation requires both old log hashes, disk copy/fsync and only the two
owned input files removed. Guards/protection retained, storage errors empty. Fresh
plans each simulation0/exact same four Inst/zero Remv/allow-downgrades flag; no
extra package change. Both actual installations nativeAPT0, stdout retained and
empty stderr, all four exact0.1.0~dev+97ae88ee5b6a configured, dpkg-audit0/empty.
Installed agentb3c7 exact37049584B/root0755 and startupgen55e verified. Eleven
protected before/after sections are identical: whole network, original data
driver/group scope, files/cache/config/crypto/firstboot, unit identities,16sysctls,
DNS, NFT, loaded IDs and storage6. New storage events empty. .211 retains
VPP72599/nginx9281; .37 retains VPP7820/nginx9669; both API and agent held inactive.
Attempt logs are unique private disk-record files, old failure100 remains retained.
Restore-only source applicability APPROVE with each actual immutable install proof;
actual guard restoration and subsequent physical/native acceptance still pending.

Actual independently executed selected commands/output:

```text
sha256sum /root/Documents/Codex/2026-10-10/hardware/manager-eal-upgrade211-preserve-failed-attempt-20261010T152246Z.json /root/Documents/Codex/2026-10-10/hardware/manager-eal-upgrade211-simulate-20261010T152320Z.json /root/Documents/Codex/2026-10-10/hardware/manager-eal-upgrade37-simulate-20261010T152400Z.json /root/Documents/Codex/2026-10-10/hardware/manager-eal-upgrade211-install-20261010T152426Z.json /root/Documents/Codex/2026-10-10/hardware/manager-eal-upgrade37-install-20261010T152527Z.json
exit0; five exact SHA values in table above
gh run view 38062974117 --json headSha,status,conclusion,jobs --jq '{headSha,status,conclusion,jobs:[.jobs[]|{name,status,conclusion,steps:[.steps[]|select(.status!="completed" or .conclusion!="success")|{name,status,conclusion}]}]}'
{"conclusion":"success","headSha":"0c21e65eea8b2d2cfb61f57ec9f6b57a0e6c55f3","jobs":[{"conclusion":"success","name":"Mandatory quick gate","status":"completed","steps":[]}],"status":"completed"}
tools/ci.sh check --base origin/main  # own published8005bd09 checkpoint
check PASSED (0m14s); gitleaks348.54KB/no leaks; board212 valid
```

Exact merged product tree/main mandatory gate PASS is now observed separately
from final48af branch gate. Hardware physical/control/post-binding boot acceptance
remains unfinished; source CI and package configuration do not claim that result.

## Terminal-lock and reboot epoch findings

Worker observer cfac7c5e267ba7ce0f33205e1d39c325ee237471d7d628aeb0ff9330933e1f25
at4a3e0ab40895c8a4a4f0eda9f73031462fb006ec adds native unit inactive/success/0
and nonblocking existing-lock evidence. Its recursive private-parent check also
rejects the actual canonical /run/lock mode1777. ROOT supplied readonly metadata
cf9db50adf14a35c4cfcb66b3f72b2185128ad6973b7a3a39821f140866d2160/362B
(.211), ceea26988992ff431bf0efd3ae18e05894d0c103d1b976730092de74f1afeaa4/258B
(.37); both independently rehashed/parsed: /run root:root0755, /run/lock
root:root1777. Existing .211 lock files root:root0644/single link; .37 locks
absent before its first canonical physical launch. No chmod or lock-file creation
by reviewer. Worker accepted narrow sticky-parent source correction; acceptance
closure pending exact delta. Default inspect/stage/launch scope is unaffected.

ROOT boot source dd8baa0b75825fd2670e9264eb2b88f57753f7efc7fe364c058d3f7ed5d93803
full read/outer+REMOTE AST PASS. Conditional scope: after actual native physical
proof, inspect current21 units, enable only owned VPP/agent/API without --now,
durable private original states/offhost proof, then one owner-authorized normal
reboot; no foreign service/network/driver/startup operation. Observe-boot currently
requires ioerr6 across the kernel restart. Concrete finding: this device counter
has a fresh boot epoch, previously proven during both original-root returns.
New-boot actual value/stability plus new kernel storage-error scan are required;
old6 must remain only same-boot precondition, not a postboot equality assertion.
ROOT notified for narrow correction before observer acceptance. Fresh timezone
metadata, full protected management/runtime/PCI/native control after reboot remain
actual checks, with no extra console/full-image prerequisite introduced.
