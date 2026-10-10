# Hardware .37 WIP — 2026-10-10

Installation task state: **blocked on offline root-filesystem recovery**. Installation and hardware acceptance: **NOT RUN**. Management, routing, netplan, SSH and target disk contents were left unchanged. Hardware deployment role remains **awaiting resume**. Manager will resume the same branch/worktree when recovery is available.

Separate verified live role resumed by manager on 2026-10-10: independent R1 reviewer/T1 tester for the API native shared-library dependency correction `2045ab8`, with no target operations. Actual20 unchanged packaging/PPPoE fixtures, slot-check and exact-source assertion pass; staged Argon2 native dependencies independently inspected. Final API archive full-read, native-content and exact dependency regression assertion PASS; SHA256 `47683ec5b2194ea360c656648718f48943cb04480717cf59804e5ff6ec6309a0`. Final PR217 HEAD `bde83bc87ae817a61bbc70e4029f76109ae77c35` independently verified product-identical to2045. Mandatory quick run38033113750/job114157962736 pending completion; no PASS claimed. Review/test receipts checkpoint `4c2ac762f316a4147c008f4fc79c775dc1c1776d` was successfully published and read back from GitHub. Review/test receipts are owned in this worktree; no product code changes.

## Ownership and checkpoints

Branch `codex/hardware-37-20261010`; worktree `/root/ngfw-wt/hardware-37-20261010`; remote host `root@172.30.126.37`. Owned files are `docs/status/tasks/hardware-37-20261010*`.

Starting local SHA: `d2d55984d74fa1d06c32e8271886f11f16375407`. Last verified published checkpoint: local HEAD and remote `refs/heads/codex/hardware-37-20261010` both `9d968c3c7e6859ff0099566ae1675d0dc9788953` after successful `git push origin codex/hardware-37-20261010`. Each subsequent checkpoint publication is verified with `git ls-remote origin refs/heads/codex/hardware-37-20261010`; the output SHA, rather than an unverified promise, is publication evidence. The commit containing a subsequent status update identifies the next checkpoint without a self-referential SHA.

Actual repository verification: `tools/ci.sh check --base origin/main` printed `check PASSED (0m43s)` for the first checkpoint and `check PASSED (0m14s)` for the second. Full quick/integration gates have not been run by this worker; no product code has changed and installation acceptance remains NOT RUN.

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
