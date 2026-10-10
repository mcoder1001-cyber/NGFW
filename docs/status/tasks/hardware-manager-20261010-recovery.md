# Hardware recovery and installation resume

This runbook is a preparation record, not proof of repair, installation or hardware
acceptance. Both /dev/sda2 roots have active structural ext4 errors and failed boot
fsck. Reboot alone may leave a manual-fsck/emergency prompt with no SSH. No offline
recovery path has been supplied yet. Existing authorization to reboot when needed
does not provide that missing access information.

## Preserve management and existing data

Protected .37:172.30.126.37/24 onenp12s0,default172.30.126.1,
PCI0000:0c:00.0,IOMMUgroup58. Protected .211:172.30.110.211/24 onenp4s0,
default172.30.110.1,PCI0000:04:00.0,IOMMUgroup28.
These interfaces/devices/groups must remain in the kernel; preserve addresses,
all routing tables/rules, DNS and netplan, and current firewall allowances.

Read-only configuration snapshots were successfully collected into controller-private
/root/Documents/Codex/2026-10-10/hardware/recovery-private/host-{37,211}.
They contain network configuration and snapshots, not full root/data backup. Keep
0700 directories/0600 files; no public content, Git commit or candidate-tar inclusion.
Actual command exits/readability/hashes and gaps are preserved in published receipts:
[.37](https://github.com/mcoder1001-cyber/NGFW/blob/0b96a5ff51aca239e2b1492456c37e2052f139ed/docs/status/tasks/hardware-37-20261010-wip.md),
[.211](https://github.com/mcoder1001-cyber/NGFW/blob/269d455f6aee29cc89007bbac4aa93d00c0fad7f/docs/status/tasks/hardware-211-20261010-recovery-receipt.md).
Independent R7 inspected permissions/manifests/archive readability without publishing
contents: parent/host directories0700, all24/.37 and26/.211 regular files private,
both configuration archives5members with management netplan included; all regular
payload lengths readable. The23-member candidate tar contains0private-backup members.
Native nft capture is unavailable on both hosts (toolmissing,exit127); successful
empty IPv4/IPv6 iptables-save exports do not establish empty native nft state.
Inspect actual native firewall after clean recovery and before activation.
Do not infer a complete backup from a tar file merely existing.
Secure a trusted off-host full backup/image or agreed data-recovery plan before
any potentially destructive offline repair. The configuration snapshot alone is
insufficient to certify preservation of unrelated existing data.

## Offline diagnosis and repair

1. Obtain verified usable physical/serial/IPMI/KVM/hypervisor console or LiveISO
   rescue, and confirm it can select a rescue boot and show the recovery shell.
2. With that console and backup plan, boot rescue. Identify the actual disk and root
   partition by device model/identity, partition table and filesystem UUID. Do not
   assume rescue names match current /dev/sda2. Inspect available drive-health data.
3. Verify that the identified root is unmounted everywhere. Do not run repair on
   a live/mounted root or force online repair/remount shortcuts.
4. Run a diagnostic only on that verified unmounted root, for example
   `e2fsck -fn /dev/sda2` if identity was confirmed. Record complete output/exit code;
   exit4 (uncorrected errors) is a failure to repair, not a successful health check.
5. Review diagnostics and backup/recovery readiness before interactive repair.
   Kernel reports requested directory rebuilding; a possible reviewed command is
   `e2fsck -fD /dev/sda2` with operator-visible prompts. Do not use blind `-y`, erase
   affected directories, or declare repair successful from a canceled command.
6. Repeat the unmounted check until clean, preserve actual repair/check outputs,
   then boot normal and independently confirm fresh SSH via each original
   management address, exact protected NIC/default route/alltables/rules/DNS,
   filesystem state and successful boot fsck. Investigate any new storage errors.

If owner/operator repairs independently, receive actual clean offline-check output,
normal-boot filesystem/management evidence, and backup/recovery confirmation before
resuming package writes. No repair or reboot has run in this task.

## Resume the existing installation tasks

Resume codex/hardware-37-20261010 and codex/hardware-211-20261010 in their existing
isolated worktrees; read durable WIP/envelope. Use only corrected runtime-fixed/
archives and the seven selected VPP26.06+ngfw3 packages; old runtime/ is BLOCKED.
Check archive hashes and independently reviewed provenance before transfer/install.
Package metadata source2045ab8 equals merged tested product tree; no offline
system-dependency closure or release certification is claimed.

After clean-storage preflight: synchronize trusted time; inspect dependency service
start behavior and temporarily suppress automatic starts for controlled installation.
Protect SSH/routing/firewall before starting the application. Complete canonical
firstboot (only its allowed overrides), preserve management blacklist and VPPno-pci,
connect agent, then enable hardware seeding only after provisioning and BEFORE any
data PCI binding/operator revision or candidate edit. Verify authoritative original
names and physical markers for exact7/.37 and17/.211 nonmanagement NICs, then export
seeded dataplane and use guarded startup with those same names. Physical af_packet
import is unsupportedD105. True IOMMU is required; never bypass with no-IOMMU or
fabricated DB/API import markers. Remove obsolete .211 data bridge membership only
as a reviewed data-port topology change; preserve management/routing state.

Actual installation/service/API/TLS/storage/live inventory/forwarding tests must
then pass. Check route/SSH preservation after each activation. Perform required
restart/reboot persistence only with verified recovery available; reconnect and
verify exact interface inventory and routing again. Record unexecuted tests as
NOT RUN, and measured throughput only if an actual traffic test runs.
