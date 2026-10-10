# Hardware .37 WIP — 2026-10-10

Current hardware role **active for recovery diagnostics/design**: user explicitly requests fixing disk/root filesystem. Installation remains **blocked on a verified offline recovery path and healthy storage**; installation/hardware acceptance are **NOT RUN**. Management, routing, netplan, SSH and target disk contents were left unchanged. Independent PR R1/T1 and main CI verification remain complete; that reviewer/tester role is finished. Resume uses the same branch/worktree/owned host. No recovery repair has run.

Fresh read-only recovery evidence and candidate RAM-root constraints are in owned `hardware-37-20261010-recovery.md`. Root still `/dev/sda2` rw ext4, errors count1319; physical health unknown, existing SMART/IPMI tools absent. Systemd259.5 supports soft-reboot, but no rescue root is staged, /run is noexec, networkd has a shutdown conflict, and17 userspace executables across4 namespaces reference root device8:2. Mounted-negative exclusive readonly block probe returns EBUSY. Existing protected network/config snapshots hash/mode/size verified unchanged. Exact next action: complete readonly geometry/e2image/effective-SSH/NSS investigation, publish evidence, then use only manager's independently reviewed RAM staging procedure. No target mutation until that procedure is ready.

Final independent package-source review: **R1 APPROVE / T1 PASS** on PR217 HEAD `5bd7e8b545fc765fd2babd8dda15175d6f33af1b`, actual hosted checkout `8a15d644c53cc3ef4abde339efd3b2a0331221a5`, reviewed/tested tree `a0d7b7cbc7f37dc2fadd9a93d41ce486dcda50d2`. Mandatory full unchanged quick run38033766837/job114159912573 completed success; actual REST log60874bytes independently obtained in protected `/tmp/hardware-37-20261010-ci/job-114159912573.log`, checked checkout and all steps, literal `CI GATE PASSED`. Source/archive/fixture/gate evidence is recorded in owned R1/T1 receipts. Previous pending entries below are checkpoint history. No installation or hardware acceptance is included in this PASS.

Post-merge **T1 PASS**: actual remote main `4908716b4501312102382e6979b8fc1ded6f9311`, merged at `2026-10-10T07:46:33Z`, has exactly the expected main/final-source parents and tree `a0d7b7cbc7f37dc2fadd9a93d41ce486dcda50d2`, identical to the tested PR integration. Main push complete unchanged quick [run38035583209/job114165198295](https://github.com/mcoder1001-cyber/NGFW/actions/runs/38035583209/job/114165198295), `headSha=4908716b4501312102382e6979b8fc1ded6f9311`, completed success with every job step success; GitHub `completedAt=2026-10-10T08:07:08Z`. Actual REST log58932bytes is protected at `/tmp/hardware-37-20261010-ci/job-114165198295.log`, SHA256 `9c957c9fe1c469f1acbde3f434b2b021ba21e6a09a6c666c0d10f3503df2fb12`. It prints the full exact main checkout,35 successful tasks,149 harness checks, complete quick summary and literal `2026-10-10T08:07:02.2874218Z CI GATE PASSED`. Fresh remote main/parents/tree readback passed after log verification. Actual API timestamps are preserved; no duration is inferred across clock sources. No required source/CI review work remains. Hardware installation and physical forwarding remain NOT RUN.

Separate verified live role resumed by manager on 2026-10-10: independent R1 reviewer/T1 tester for the API native shared-library dependency correction `2045ab8`, with no target operations. Actual20 unchanged packaging/PPPoE fixtures, slot-check and exact-source assertion pass; staged Argon2 native dependencies independently inspected. Final API archive full-read, native-content and exact dependency regression assertion PASS; SHA256 `47683ec5b2194ea360c656648718f48943cb04480717cf59804e5ff6ec6309a0`. Final PR217 HEAD `bde83bc87ae817a61bbc70e4029f76109ae77c35` independently verified product-identical to2045. Mandatory quick run38033113750/job114157962736 pending completion; no PASS claimed. Review/test receipts checkpoint `4c2ac762f316a4147c008f4fc79c775dc1c1776d` was successfully published and read back from GitHub. Review/test receipts are owned in this worktree; no product code changes.

Manager subsequently announced one additional documentation-only final HEAD for PR217; R7 requires the actual command/output evidence appendix inside the PR. Therefore bde83bc/run38033113750 are historical evidence only if superseded, and must not be accepted as final T1 PASS. Await new final HEAD/run, recheck product-tree equality with2045 and require completed unchanged quick on the latest SHA. No package rebuild is needed for the documentation-only replacement. Actual API archive receipt checkpoint `4dc424692b2be76801a2146bea45c0bef3157f48` was published/read back successfully; lightweight check14s passed.

Latest manager/PR217 final HEAD now independently verified `5bd7e8b545fc765fd2babd8dda15175d6f33af1b`; only evidence/review-plan/WIP manager docs differ from2045, product/tests/CI identical. Fresh mandatory quick run38033766837/job114159912573 in progress; integration candidate `8a15d644c53cc3ef4abde339efd3b2a0331221a5`. Old run38033113750 canceled/superseded. Exact latest-head R1/T1 review remains pending complete fresh gate; no target actions. Latest-head waiting receipt `fa7fb0f9106ac45ac0e18cb460f78d84a5a89d6b` published/read back successfully with check14s passed.

Latest review checkpoint `0ac4701ba689278b7e609fee72efcf464dced700` published/read back successfully. Candidate integration tree independently asserted equal to final source tree `a0d7b7cbc7f37dc2fadd9a93d41ce486dcda50d2`, exact main/final-source parents verified. Latest-head hosted packaging38033766825 and provisioning38033766876 completed success; actual logs7+81 and46+23+11+18 tests OK independently retrieved. Complete mandatory quick38033766837 still pending; no overall T1 PASS or R1 APPROVE claimed.

Final preceding checkpoint `00dbea93801932b1b7555ea383c815af803ca99f` published/read back successfully with check14s passed. Final R1/T1 approval receipts supersede its pending verdict only after completed exact-head mandatory quick was independently read back.

## Ownership and checkpoints

Branch `codex/hardware-37-20261010`; worktree `/root/ngfw-wt/hardware-37-20261010`; remote host `root@172.30.126.37`. Owned files are `docs/status/tasks/hardware-37-20261010*`.

Starting local SHA: `d2d55984d74fa1d06c32e8271886f11f16375407`. Last verified published checkpoint before this recovery-resume update: local HEAD and remote `refs/heads/codex/hardware-37-20261010` both `4a8a82205d490068d865a1344d86afcaf93b8502` after successful `git push origin codex/hardware-37-20261010`. Each subsequent checkpoint publication is verified with `git ls-remote origin refs/heads/codex/hardware-37-20261010`; the output SHA, rather than an unverified promise, is publication evidence. The commit containing a subsequent status update identifies the next checkpoint without a self-referential SHA.

Actual repository verification: `tools/ci.sh check --base origin/main` printed `check PASSED (0m43s)` for the first checkpoint and `check PASSED (0m14s)` for subsequent checkpoints. No broad local quick/integration run was duplicated; the exact final PR complete hosted quick was independently verified as described above. No product code has changed in this worker's branch and installation acceptance remains NOT RUN.

## Actual read-only preflight

Commands ran through `ssh -o BatchMode=yes -o ConnectTimeout=15 root@172.30.126.37`. Nothing has been installed, restarted, rebound, rebooted or written on the target.

- Ubuntu 26.04 LTS amd64; kernel `7.0.0-22-generic`; root `/dev/sda2`, ext4, still mounted read/write; 59 GiB capacity, 40 GiB available; 15 GiB RAM, no swap.
- Product packages `vpp`, `ngfw-agent`, `ngfw-api`, `ngfw-web`, `ngfw-meta` are not installed.
- SSH and systemd-networkd active/enabled. `ip route get 172.30.126.195` returns `dev enp12s0 src 172.30.126.37`; remote peer `.211` goes through `172.30.126.1` on the same management interface.
- Management `enp12s0`: `172.30.126.37/24`, default gateway `172.30.126.1`, PCI `0000:0c:00.0`, Intel `8086:125c`, kernel driver `igc`, IOMMU group 58, MAC `00:04:e1:e0:00:3e`. This interface/PCI and every existing route are excluded from dataplane ownership.
- Other physical NICs: `enp10s0` / PCI `0000:0a:00.0` / group 56; `enp11s0` / `0000:0b:00.0` / 57; `enp13s0` / `0000:0d:00.0` / 59; `enp14s0` / `0000:0e:00.0` / 60; `enp15s0` / `0000:0f:00.0` / 61; `enp16s0` / `0000:10:00.0` / 62; `enp17s0` / `0000:11:00.0` / 63. All Intel `8086:125c`, `igc`, currently no carrier; no global IP addresses or IPv4 routes; some have IPv6 link-local addresses/routes.
- `bridge link` returns no members. Existing br0..br5, br7..br9 have no addresses or route ownership. `/etc/netplan/00-installer-config.yaml` contains stale names for different hardware and `172.30.110.223`; `/etc/netplan/90-ngfw-management.yaml` supplies actual `enp12s0` configuration. Do not apply/replace netplan blindly.
- DMAR initialized; each of the eight NICs has a different IOMMU group. Actual `/sys/kernel/iommu_groups/{56..63}/devices/*` inspection confirms each group contains exactly its one listed NIC and no other device. Revalidate this before binding. VFIO is not loaded; no unsafe no-IOMMU setting was enabled.
- Target clock reports 2026-04-18, `NTP=no`, `NTPSynchronized=no`, timezone Asia/Tehran; this is an observed incorrect clock, not the acceptance date. Check trusted time synchronization before certificate/browser tests.
- Utilities python3 and apt-get available; lspci, curl, jq and ar absent. No host utility packages were installed merely for discovery.

## Blocking filesystem evidence

`systemctl status systemd-fsck-root.service --no-pager`:

```text
Active: failed (Result: exit-code)
/dev/sda2 contains a file system with errors, check forced.
/dev/sda2: Inode 259596 has an invalid extent node (blk 15505493, lblk 0)
/dev/sda2: UNEXPECTED INCONSISTENCY; RUN fsck MANUALLY.
```

`tune2fs -l /dev/sda2` relevant output:

```text
Filesystem state: clean with errors
Errors behavior: Continue
FS Error count: 1187
Last error function: ext4_find_extent
Last error inode #: 259596
Last error err: EFSCORRUPTED
```

`dmesg` repeatedly reports directory block checksum failures for `/root/.cache` inode 259599 and `/root/.config` inode 259602; kernel says `Please run e2fsck -D`. Opening a journal file also reports `Input/output error`. The error is broader than missing writable caches. Package installation and persistence/reboot claims cannot be trusted while this remains.

## Private recovery configuration snapshot

Manager authorized one read-only configuration/network snapshot while main CI runs. Private controller path `/root/Documents/Codex/2026-10-10/hardware/recovery-private/host-37` is mode0700; every snapshot, diagnostic and manifest is mode0600. Contents stay outside git and outside the candidate package bundle. Target writes/configuration/service/driver changes: none. This is **configuration backup only, not a full-system backup**. Root-filesystem corruption is unresolved.

Each remote command used exactly `ssh -o BatchMode=yes -o StrictHostKeyChecking=yes -o ConnectTimeout=15 root@172.30.126.37 <command>`, with stdout streamed directly into its private local file, stderr captured separately into an equally protected file, and actual SSH exit recorded. Commands were:

```sh
tar -C / -cf - -- etc/netplan etc/systemd/network etc/resolv.conf
ip -j -details address show
ip -j -details link show
ip -j -4 route show table all
ip -j -6 route show table all
ip -j -4 rule show
ip -j -6 rule show
nft list ruleset
```

The PCI mapping command was a remote `python3` stdin script, reading only `/sys/class/net/*/device`: resolve each physical device's PCI address, driver and IOMMU-group symlinks; enumerate the full group's `devices` membership; read vendor/device IDs; emit one JSON object per physical interface. Its exact executed script/commands and all exit/hash metadata are preserved privately in `snapshot-manifest.json`; no secrets were requested.

| Private file | Actual exit | Bytes | SHA256 |
|---|---:|---:|---|
| network-config.tar | 0 | 10240 | ddfcd92c7df9dc4f67f85445f1793b6280984d1f7acda89cbcbce5c1e4cbce1f |
| addresses.json | 0 | 22173 | d5ca56087afd73968b4f722c6acb0cc16b2b88f95e81892ad4518028fa7035d5 |
| links.json | 0 | 21520 | a8c8710f74b2baa0e5e3607d8e111a131be37b691565058e56d15dc211d74b10 |
| routes-ipv4-all.json | 0 | 899 | 2b8296f4e16250ca95f06f9458856297dd09cd0408a6029d13675ae6be896a05 |
| routes-ipv6-all.json | 0 | 2016 | 1452733be3168ea2efb95f9885d30f628dbd8127ca7494bfe409a1ba812a567a |
| rules-ipv4.json | 0 | 140 | e79a76eff86a78de4ce5d38c6277dbafbab227656214e1e116d9ac63597dbb9d |
| rules-ipv6.json | 0 | 91 | 78fc521d26d8f02af636ce8e1c74bb782cbfd272b8a81529bea0adb9a68ed0ea |
| nft-ruleset.txt | 127 | 0 | e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855 |
| pci-name-iommu.jsonl | 0 | 1288 | 75502003e9a032bdb3fc7b6d5b583bf683dedaa0a47d71a06f92b43f5a703a2d |

Private manifest SHA256: `147795190a78e36cc0c2ae3d9cb66458ee55525bef37fee4b951156b0b02b26d`. Successful commands had empty stderr. `nft list ruleset` failed exit127 because nft is absent; its37-byte diagnostic was classified privately without printing contents. **Firewall ruleset is not backed up**; no tool was installed to complete it. The readable 5-member archive represents all three requested configuration paths, including a regular resolver file, and every archived regular-file payload was read successfully. All six JSON network snapshots validate; eight PCI/IOMMU mappings validate and management's PCI/igc/group58 identity matches preflight. No configuration contents were printed or committed.

Manager subsequently authorized read-only fallback commands through the same strict-known-host SSH prefix. `iptables-save` and `ip6tables-save` both exist and each completed exit0 with empty stdout/empty stderr. Outputs are protected `iptables-save.txt` and `ip6tables-save.txt`, each0bytes and SHA256 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`. Their private `firewall-fallback-manifest.json` SHA256 is `9eda7af0f63314aac1aeed4d03b9ce1924e2d281a229e5076beb41b75c53263e`; the original snapshot manifest is preserved unchanged. Every added file/diagnostic/manifest is mode0600. **Compatibility firewall outputs were captured; native nft ruleset remains unavailable**. Empty compatibility exports do not prove that arbitrary native nft rules do not exist. After clean offline recovery and before activating the package/firewall, inspect the complete active native firewall and preserve its state using available approved tooling. No target tool installation or configuration change was made for backup.

## Prepared installation and testing plan

1. Owner/manager provides console or recovery access. Export protected network configuration and routes without secrets; ensure data backup/recovery is agreed before offline repair. Reboot alone is insufficient because automatic fsck already refused the filesystem; arbitrary repair while root is mounted is prohibited.
2. Once filesystem is repaired offline and management returns, repeat root-filesystem/dmesg/fsck preflight. Compare management IP/routes/driver/netplan to above. Verify disk health with available approved tooling and synchronize clock.
3. Receive manager's current-main payload, verify provenance, version, sha256 and supported Ubuntu/architecture. Inspect final package maintainer scripts and unit enable/start behavior with independent reviewer. Snapshot existing route/rule/address/firewall/netplan/service state and make protected configuration backup; arm an explicit management recovery mechanism before changing services.
4. Install using verified local payload and a service-start policy boundary. Preserve root SSH, exact netplan and every management route; do not load appliance firewall blindly. Complete firstboot with secure generated credentials stored only in mode0600 target files; never report them in evidence. Start services in verified order and test SSH from a second session.
5. Existing product prohibits physical `af_packet` (D-105, `apps/agent/internal/subsystems/netdev.go`). Use approved DPDK with real IOMMU protection for these isolated data NICs after confirming entire groups and Intel PMD availability. Management PCI must remain blacklisted and `igc`-bound. Never use no-IOMMU or bypass the physical-af_packet guard. Startup changes go through `ngfw-startupgen` and guarded apply, with the target's own handover/configuration evidence.
6. Persist seven data NICs and their logical names through source-of-truth product config API; confirm candidate/diff/commit/rollback and state/physical inventory. Do not count ad hoc vppctl interfaces as product configuration.
7. Verify active VPP/agent/API/web/PostgreSQL/Valkey and HTTPS login, ownership and permissions, startup/daemon logs, FRR reload and routes; run actual local dataplane rig tests for routing/NAT/ACL and report physical wire tests separately because all data ports currently lack carrier.
8. Only after storage is healthy and management recovery exists: restart VPP and stack/reboot as needed, reconnect over management, compare routing and all seven persisted data NICs, test repeat installation without overwriting data/secrets. Record actual results and limitations.

## Remaining work and exact next command

No installation or dataplane tests have run. Blocked by root filesystem corruption and pending manager payload/reviewer verdict. Continue preparing reviewed payload and recovery instructions without target mutations.

Required recovery input is either (a) owner-provided usable physical/serial/IPMI/KVM console or recovery-boot access, with the root filesystem unmounted for an agreed offline repair, or (b) owner/operator confirmation that offline repair has completed, including the actual filesystem-check result and restored management reachability. Merely authorizing a reboot does not provide console recovery from the existing manual-fsck failure. No console/recovery path has been supplied at handoff.

After manager reports offline recovery complete, exact first command:

```sh
ssh -o BatchMode=yes -o ConnectTimeout=15 root@172.30.126.37 'findmnt -no SOURCE,FSTYPE,OPTIONS /; systemctl status systemd-fsck-root.service --no-pager; tune2fs -l /dev/sda2 | sed -n "/Filesystem state/p;/FS Error count/p;/Last error/p"; dmesg | tail -60; ip -br addr; ip route show table all; ip route get 172.30.126.195'
```
