# Hardware package preparation evidence

This is host-independent preparation, not target installation or appliance acceptance.
Source2045ab8b3d2f477bb23446fb5e58b7d9d3abea3c is preserved in remote
`codex/archive-hardware-manager-20261010`. The final PR product tree is identical;
subsequent changes address task documentation only. Outputs below are selected actual
log lines; omitted output is retained outside the repository in the task-owned directory
`/root/Documents/Codex/2026-10-10/hardware/`. All completed process commands listed here
were observed exit0. The final PR hosted quick completed SUCCESS; post-merge bare main quick also completed SUCCESS. Fixed native archive integrity/metadata
passed independent R8 inspection; its durable receipt is linked below when published.

## Source preparation

Command in the manager worktree at clean2045ab8:

```sh
GOMAXPROCS=4 tools/heavy.sh deploy/debian/ngfw/prepare.sh \
  /root/Documents/Codex/2026-10-04/debian-latest/outputs/NGFW-Debian-latest-development/VPP-26.06-release+ngfw3-amd64 \
  /dev/shm/ngfw-hardware-package-fixed-20261010
```

Selected actual output from package-prepare-fixed.log:

```text
ok   VERSION: v26.06 c3200b88dc46bd380f00a49ca3392a102cc1980b → 26.06-release (patched: 26.06-release+ngfw3), 11 packages, ship 7
ok   series: patches 4 · build-patches 1 (build/ only) · optional optional/trace-plugins-core.patch
ok   pydeps.lock: meson==0.57.2 pyelftools==0.33 setuptools==84.0.0 wheel==0.48.0 packaging==26.3 (sha256-pinned)
ok   scripts parse + shellcheck; no .deb/.whl/.build in git; .gitignore covers deploy/vpp/.build and *.deb
ok   tests/run.sh: 72 passed, 0 failed
ok   output /root/Documents/Codex/2026-10-04/debian-latest/outputs/NGFW-Debian-latest-development/VPP-26.06-release+ngfw3-amd64: 11 .deb, version 26.06-release+ngfw3, variant default, patches ['patches/0002-ikev2-safe-native-state.patch', 'patches/0003-lcp-multicast-reconcile.patch']; files/control fields/sha256/SHA256SUMS consistent; install gate passed
verify.sh: OK
```
```text
@ngfw/web:build: bundle-budget: initial   521.2 kB gzipped of   600.0 kB budget; 141 lazy chunk(s)
@ngfw/web:build: bundle-budget: OK
@ngfw/web:build: check-no-dev-routes: OK (143 files, no /dev routes, dev nav or demo chunks)
 Tasks:    14 successful, 14 total
Cached:    7 cached, 14 total
Done in 2.1s using pnpm v12.5.1
Prepared source package: /dev/shm/ngfw-hardware-package-fixed-20261010
```

The unchanged prepare script builds the seven Go helper binaries before publishing
the prepared-source marker. Its completed command also verifies VPP integrity and
all72 verifier tests. Dependency installation command and selected actual output:

```sh
pnpm install --frozen-lockfile --prefer-offline
```

```text
Packages: +604
Progress: resolved 604, reused 604, downloaded 0, added 604, done
Done in 2m 5.1s using pnpm v12.5.1
```

## Corrected native packages

Command in /dev/shm/ngfw-hardware-package-fixed-20261010:

```sh
GOMAXPROCS=4 /root/ngfw-wt/hardware-manager-20261010/tools/heavy.sh dpkg-buildpackage -us -uc -b
```

Selected actual output from package-build-fixed.log; all41 required tests ran, with
no test option, fixture, rule or workflow weakened:

```text
python3 tests/test_packaging.py
Ran 16 tests in 1.144s

OK
python3 tests/test_system_identity.py
Ran 12 tests in 0.431s

OK
python3 tests/test_firstboot.py
Ran 6 tests in 20.381s

OK
python3 tests/test_base_policy.py
Ran 3 tests in 0.001s

OK
python3 tests/test_pppoe_assets.py
Ran 4 tests in 0.319s

OK
dpkg-deb: building package 'ngfw-meta' in '../ngfw-meta_0.1.0~dev+2045ab8b3d2f_all.deb'.
dpkg-deb: building package 'ngfw-agent-dbgsym' in 'debian/.debhelper/scratch-space/build-ngfw-agent/ngfw-agent-dbgsym_0.1.0~dev+2045ab8b3d2f_amd64.deb'.
	Renaming ngfw-agent-dbgsym_0.1.0~dev+2045ab8b3d2f_amd64.deb to ngfw-agent-dbgsym_0.1.0~dev+2045ab8b3d2f_amd64.ddeb
 dpkg-genbuildinfo --build=binary -O../ngfw_0.1.0~dev+2045ab8b3d2f_amd64.buildinfo
 dpkg-genchanges --build=binary -O../ngfw_0.1.0~dev+2045ab8b3d2f_amd64.changes
dpkg-genchanges: info: binary-only upload (no source code included)
 dpkg-source --after-build .
dpkg-buildpackage: info: binary-only upload (no source included)
```

Command: `dpkg-deb -f runtime-fixed/ngfw-api_0.1.0~dev+2045ab8b3d2f_amd64.deb Package Version Architecture Depends`. Actual output:

```text
Package: ngfw-api
Version: 0.1.0~dev+2045ab8b3d2f
Architecture: amd64
Depends: libc6 (>= 2.34), libgcc-s1 (>= 4.2), nodejs (>= 22), nodejs (<< 23), adduser, python3
```

Command: `sha256sum runtime-fixed/*.deb`. Actual preserved package hashes:

```text
7e505cf9c6415a58c45a39366b512667d4ad79e2f6db340f7ef0d639ca6431bf  ngfw-agent_0.1.0~dev+2045ab8b3d2f_amd64.deb
47683ec5b2194ea360c656648718f48943cb04480717cf59804e5ff6ec6309a0  ngfw-api_0.1.0~dev+2045ab8b3d2f_amd64.deb
4a16c2e6b04da696876c2978a36af9feca238ab55ec120801ffdfb63c89c777d  ngfw-meta_0.1.0~dev+2045ab8b3d2f_all.deb
f124a576f14435903dfe62f69fd6853a75c2e609528a5c6e0733334a93d48d56  ngfw-web_0.1.0~dev+2045ab8b3d2f_all.deb
```

Old cc80be66edcf runtime/ archives are BLOCKED by the missing native dependencies;
their manifest explicitly records this. Fixed runtime-fixed/ archives passed local
SHA256SUMS readback; independent R8 control/native/helper/unit/payload review passed.
Its final publication receipt is recorded separately. They have not been
transferred to either target. No offline dependency-closure/release claim is made.

## Lightweight source gate and identity

Command: `tools/ci.sh check --base origin/main`. Selected actual final-source-check.log:

```text
ok: no secret-shaped strings
ok: gitleaks — scanned ~8156 bytes (8.16 KB) in 650ms no leaks found 
board valid: 212 tasks; read-only validation
== slot resource scheme (1..32, no collisions) ==
ok: 30 developer slots + CI slot 12; 964 ports, 32 id ranges, no collision; tools/lab env verified for 31 slots
check PASSED (0m14s)
```

Command: `git diff --exit-code 2045ab8b3d2f477bb23446fb5e58b7d9d3abea3c HEAD -- apps packages deploy tools .github pnpm-lock.yaml package.json`: exit0, empty output. This proves product/build/test/CI content equality, not hardware behavior.

## Independent immutable receipts

- [Host37 preflight](https://github.com/mcoder1001-cyber/NGFW/blob/86a2f06eafc6406e3e5395769d923a09bbfa60aa/docs/status/tasks/hardware-37-20261010-wip.md): management enp12s0 protected; seven data NICs; ext4/boot fsck blocker; no target writes.
- [Host211 preflight](https://github.com/mcoder1001-cyber/NGFW/blob/a4059c73b4a2e3346b7cf1cf705c0a5b908c317d/docs/status/tasks/hardware-211-20261010-wip.md): management enp4s0 protected; seventeen data NICs; bridges and fsck blocker; no target writes.
- [R2 final source](https://github.com/mcoder1001-cyber/NGFW/blob/da9426156320e5a95fd1b26351af43bb102a5f1a/docs/status/tasks/hardware-211-20261010-review-R2.md): APPROVE, native dependency change and targeted secret scans.
- [R8 original archive/source](https://github.com/mcoder1001-cyber/NGFW/blob/3009a40ead3d73e0c1ed277d334cfc739f2d67a7/docs/status/tasks/hardware-review-20261010-artifact-review.md): old archive BLOCK, narrow source fix APPROVE, independent helper/unit checks.
- [R1/T1 preliminary evidence](https://github.com/mcoder1001-cyber/NGFW/blob/4c2ac762f316a4147c008f4fc79c775dc1c1776d/docs/status/tasks/hardware-37-20261010-test-T1.md): lightweight checks; complete hosted quick pending, no T1 PASS claimed yet.

PR217 complete unchanged hosted quick38033766837 passed on final5bd7e8b; see combined
review for immutable final receipts and literal CI GATE PASSED. A pending or
superseded run was not counted as a merge PASS. No target firstboot/API/forwarding/reboot tests
ran, and no persistent runner or live installation worker is claimed.

## Final source, independent reviews and merge

See [combined review](hardware-manager-20261010-review.md) for final immutable
R1/R2/R7/R8/T1 receipts. Source metadata correction APPROVE; T1 PASS.

Command: `gh run view 38033766837 --json status,conclusion,headSha,url,jobs`.
Actual root readback: completed,success,headSha5bd7e8b545fc765fd2babd8dda15175d6f33af1b;
job114159912573 completed2026-10-10T07:36:18Z,allstepssuccess. Independent T1 downloaded
60874-byte full log and verified:

```text
HEAD is now at 8a15d644 Merge 5bd7e8b545fc765fd2babd8dda15175d6f33af1b into d2d55984d74fa1d06c32e8271886f11f16375407
CI GATE PASSED
```

Command: `gh pr merge 217 --merge --match-head-commit 5bd7e8b545fc765fd2babd8dda15175d6f33af1b --body-file <task-owned merge-body.txt>`
under /run/lock/ngfw-main.lock; no admin/bypass. Readback `gh pr view217`:
MERGED,mergeCommit4908716b4501312102382e6979b8fc1ded6f9311,mergedAt2026-10-10T07:46:33Z.
Fresh `git fetch origin main`, `git ls-remote origin refs/heads/main`,
`git show -s --format='%H%n%P%n%T%n%s' origin/main` actual output:

```text
4908716b4501312102382e6979b8fc1ded6f9311
d2d55984d74fa1d06c32e8271886f11f16375407 5bd7e8b545fc765fd2babd8dda15175d6f33af1b
a0d7b7cbc7f37dc2fadd9a93d41ce486dcda50d2
Merge pull request #217 from mcoder1001-cyber/codex/hardware-manager-20261010
```

Actual merged tree equals independently tested integration tree. Main bare quick
38035583209,packaging38035583240,provisioning38035583210 observed IN PROGRESS;
This original in-progress snapshot was superseded by final main PASS below.

Premerge entire-worktree canonical-config scan command:
`gitleaks detect --no-git -s /root/ngfw-wt/hardware-manager-20261010 --config .github/gitleaks.toml --redact --no-banner`
(report paths outside repo). Actual exit0: scanned114800082bytes in11.4s; no leaks
found. Default scan without existing config initially flagged10sanctioned fixture
markers; independent R2 reviewed every redacted location and existing conventions,
confirmed sources unchanged frommain, then independently configured scan exit0
114800082bytes/15.7s/noleaks. No exception/config change.

## Final read-only target readback

Command for each supplied address at07:38UTC:

```sh
ssh -o BatchMode=yes -o StrictHostKeyChecking=yes -o ConnectTimeout=15 root@<address> 'ip -br -4 addr; ip -4 route show default; findmnt -no SOURCE,FSTYPE,OPTIONS /; systemctl is-failed systemd-fsck-root.service; tune2fs -l /dev/sda2 | sed -n "/Filesystem state/p;/FS Error count/p"'
```

Both command processes exit0; `is-failed` printsfailed. Selected actual output:

```text
172.30.126.37:
enp12s0 UP 172.30.126.37/24
default via 172.30.126.1 dev enp12s0 proto static
/dev/sda2 ext4 rw,relatime
failed
Filesystem state: clean with errors
FS Error count:1258

172.30.110.211:
enp4s0 UP 172.30.110.211/24
default via 172.30.110.1 dev enp4s0 proto static
/dev/sda2 ext4 rw,relatime
failed
Filesystem state: clean with errors
FS Error count:1248
```

Storage errors remain active; no target mutation/install/reboot. Package/API/live
physical inventory/forwarding/throughput/reboot acceptance all remain NOT RUN.

## Private configuration backup evidence

Host37 published readback0b96a5ff51aca239e2b1492456c37e2052f139ed; host211 published
readback269d455f6aee29cc89007bbac4aa93d00c0fad7f. Their public receipts record actual
commands/exits/hashes without configuration values. Network configtar and address/
link/all-table IPv4+IPv6 routes/rules/PCI maps captured read-only to controller,
not to target disks. Native nft list ruleset exits127 (utilitymissing);
iptables-save/ip6tables-save each exit0/empty on both hosts. No native nft emptiness
claim. Configuration snapshots alone are not full-data/systembackup.

Root command `chmod0700 <task-owned recovery-private>` then `stat -c '%a %n'`
verified parent and both host dirs0700. Independent R7 metadata-only checks exit0:
24/.37 and26/.211 regular files with zero unsafe permission bits; each5-member
configarchive includes management netplan and all regular payload lengths readable.
Candidate tar23members has0private-backup members and0unsafepaths. No privatebackup
content is committed/published. See recovery runbook/publichostreceipts.

## Completed bare main gate and final route check

Root command `gh run view38035583209 --json status,conclusion,headSha,url,jobs`
actual readback: completed,success,headSha4908716b4501312102382e6979b8fc1ded6f9311;
job114165198295 completed2026-10-10T08:07:08Z,allstepssuccess. Independent T1
downloaded full58932-byte log/SHA2569c957c9fe1c469f1acbde3f434b2b021ba21e6a09a6c666c0d10f3503df2fb12
and published actual excerpts in immutable final receipt4a8a82205d490068d865a1344d86afcaf93b8502:

```text
HEAD is now at 4908716b Merge pull request #217 from mcoder1001-cyber/codex/hardware-manager-20261010
CI GATE PASSED
```

Main bare quick, without PR base argument, ran unchanged complete gate. All35turbo
tasks and149fake-host checks passed; actual physical integration NOT RUN. Both
main packaging38035583240/provisioning38035583210 readbackcompletedSUCCESS/exact490.
Fresh gitfetch/lsremote/show and productdiff2045→originmain confirmed exactmerge/
parents/tree and product equality. `gh pr list --state open` returned[]. Fresh
board YAML count212,statesmerged205/parked7; no otherstate.

Final08:03readonlySSH command for each address:
`ssh -o BatchMode=yes -o StrictHostKeyChecking=yes -o ConnectTimeout=15 root@<address> 'ip -br -4 addr; ip -4 route show default; systemctl is-failed systemd-fsck-root.service'`
Both exits0,actualoutputs:

```text
enp12s0 UP 172.30.126.37/24
default via 172.30.126.1 dev enp12s0 proto static
failed
enp4s0 UP 172.30.110.211/24
default via 172.30.110.1 dev enp4s0 proto static
failed
```

No targetmutation/install/reboot or liveacceptance. Rootfilesystem recovery remains
required; configuration backups do not replace fullsystem/data protection.
