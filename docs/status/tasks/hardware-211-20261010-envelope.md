# Hardware installation and acceptance: 172.30.110.211

- Owner: worker `/root/host_211`; manager `/root`.
- Branch: `codex/hardware-211-20261010`.
- Worktree: `/root/ngfw-wt/hardware-211-20261010`.
- Product base: `d2d55984d74fa1d06c32e8271886f11f16375407` (`origin/main`).
- Owned files: `docs/status/tasks/hardware-211-20261010*`; task-specific deployment scripts only if later required and approved by manager.
- Owned remote target: `root@172.30.110.211`; no changes to the other target or shared development host.
- Authorization: owner requests package installation, tests, all interfaces except management/routing, and reboot if necessary. Management access and routing must remain functional.
- Constraints: exact manager-provided product payload; independent installation review; no replacing OS, mounted filesystem repair, blind nftables baseline/flush, management PCI rebinding, unsafe VFIO/no-IOMMU, secrets in evidence, or developer-host VPP changes.
- Current phase: actual RAM soft-reboot and positive offline guard/audit completed; persistentPTY and freshSSH2222/network preserved. Read-only fsck aborts12; metadata-Q fails1, but native scoped metadata plus five raw blocks now durably preserved and capped undo preflight passes. Corrective session is active after applicable R7 approval/fresh audit0; live single-byte driver fix pending narrow review. Normal return reboot and installation remain pending.
- Runtime publication: commit coherent evidence and publish immediately to own branch; report actual remote SHA to manager.
- Acceptance: SSH and exact management routes preserved; packages installed from verified payload; firstboot/services/HTTPS/auth; 17 data interfaces persisted through API and present in VPP; routing/NAT/ACL/FRR and recovery/reboot tests with actual evidence where hardware links allow.

## Independent review assignment — 2026-10-10

Manager resumed this worker as R2 security reviewer of metadata candidate `2045ab8b3d2f477bb23446fb5e58b7d9d3abea3c` on `codex/hardware-manager-20261010`. Own only `hardware-211-20261010-review-R2.md` and own envelope/WIP. No product edits or target operations. Review exact source diff, secrets and security boundaries, confirm unchanged policy/CI, then compare supplied final squash/PR tree and publish applicability. Narrow history scans only; no heavy `node_modules` scan or broad tests.

## Read-only private recovery capture — 2026-10-10

Manager resumed original host task solely for off-host configuration/network-state preservation. Own controller subdirectory `/root/Documents/Codex/2026-10-10/hardware/recovery-private/host-211` (0700), its private files (0600), and public task recovery receipt/WIP/envelope. Only selected `/etc/netplan`, `/etc/systemd/network`, `/etc/resolv.conf` are archived; live addresses/links/all-table routes/rules and PCI/name/IOMMU maps captured read-only. No target backup files, configuration/service/package/FS changes. Existing firewall save binaries may be read as fallback; no installation. Never commit private backup bytes or disclose contents; retain capture failures privately. Configuration backup only, not complete system/data backup or authorization to repair mounted root. Independent R7 checks metadata/completeness; offline recovery remains gated.

## Owner-requested root repair assessment — 2026-10-10

Owner now explicitly requests fixing disk/root filesystem; original installation/testing objective remains after recovery. Worker resumes same host/branch/worktree for read-only fresh filesystem/disk identity/health, platform/rescue/BMC/tool/capability checks and private-backup verification. Assess RAM maintenance preserving SSH and truly unmounting old root. No credential reads or diagnostic package installs; do not stage target files, change services/network/bootloader/mounts, repair mounted root, stop services, reboot, kexec or pivot without a concrete independently reviewed path. Manager coordinates any transition/repair one host at a time. Own public maintenance-preflight evidence plus existing task docs and private host-211 diagnostics only.


## Reversible RAM staging authorization

Manager authorizes only dedicated executable RAM tmpfs `/run/ngfwrescue`, isolated key-only management listener2222 and complete matching minimal rescue tree/runtime tests. Keep `/run/nextroot` absent. Original SSH/network/boot/disk configuration remains unchanged. SMART package may be downloaded/extracted exclusively into RAM only after official cached signature/index/package hash verification, with read-only smartctl -x only. No apt update/install, SMART test/enable/write control, soft-reboot, KeepConfiguration application, transition or fsck is authorized. Independent reviewer receives concrete tree/unit/procedure hashes before manager coordinates next action.

## Released phase — transition and read-only diagnosis only

Manager durably releases exact owned `/run/nextroot -> /run/ngfwrescue` and ordinary original22 soft-reboot after final independently applicable RAM audit helper, durable checkpoint, persistentPTY/otherhostSSH/controllercapacity gates. Keep PTY71683. After actual newRAMPID1/runtimehelper/SSH/network proof, enumerate process/device/namespace references and require exact guard0; only then read-only `e2fsck -f -n` and measured privateRAM e2image-Q plus verified compressed durable offhost metadata. No corrections/returnreboot/packageactivation/original-filedeletion. Namespace helper child-only setns reference inspection does not substitute for exclusive guard. Never commit confidential archives/logs.


## Active corrective session and single-byte answer correction

Previous coherent checkpoint local/verified remote `74a0db35428e380ab0323ee503aa41c6d89110ee`. R7 formal correction APPROVE and manager conditional release became executable after fresh complete offline audit:262 processes/93FDs/nsfs0/races0/failures0/finalguard0; private pre-correction-offline-audit.json24506B SHA256 `cbfebefb4c3c1f6dd6f5a0974ae81a84c8f391bc64390fd40012109d06fecd20`. Interactive capped fsck started, controller session66248, persistent RAMPTY71683 remains intact. Full transcript private0600/fsync; actual first three journal invalid extent questions answered y.

Actual driver defect discovered while still in known scope: previous driver0186e5ab wrote y plus newline. Matching upstream [ask_yn](https://raw.githubusercontent.com/tytso/e2fsprogs/v1.47.2/e2fsck/util.c) disables ICANON and reads one byte; the newline accepted the following default. Consequent journal accounting and known cache/config missing dot reconstruction received implicit yes. All actual affected operations remain classified by parent/R7, but these were not all separate explicit answers. The previous claim of one prompt per answer is corrected. Hold at known directory259602 missing dot Fix; no unknown prompt accepted. Preserve live fsck/undo rather than restart.

Future driver now sends exactly answer.encode(), SHA256 `dc3219514cc7be54a07d83120b704811dfe3d8c6a0566a06c158ac545ab939d7`. Focused controller-only fsck-single-byte.py SHA256 `e6b6d00e27f6c86ba53679c33b0f708c4d02fa133458738dc0afce6584de34e0` identifies unique exact owned driver/cwd/executable/uid/transcript FD and exact SSH child/PPID/executable/starttime/uid; proves its writable FIFO equals SSH stdin by device/inode and flags. Opens only that pipe WRONLY|NONBLOCK|CLOEXEC, rechecks identities/fstat/current child input, writes exactly one selected y/n byte and records a private fsynced ledger. It closes only its own opened FD. Actual probe writes zero bytes and passes; private repair-single-byte-answers.jsonl722B/0600 SHA256 `162de6b45b9c177fa254baa32e189e86668836e7ea51d85fab314a6aeaca8e4d`. Python AST passes for both sources. Focused independent applicability requested before the first helper answer. No target process restart/signals/descriptor closure or unknown repair.

Current active worker, not awaiting-resume: existing fsck held at classified prompt while this coherent source correction is published. Exact next command after focused applicable review: `python3 docs/status/tasks/hardware-211-20261010-fsck-single-byte.py y`, then inspect the new private prompt before another single byte. Parent/R7 will not write concurrently. Continue known individual repairs; new inode/data/deletion/health/undo issue holds for classification. Normal return reboot, product installation and hardware acceptance remain NOT RUN.


## New directory259603 prompt held; narrow raw preservation prepared

Single-byte controller helper applicability approved by R7 on published87cc97e7. Actual independent y bytes repaired known config259602 self/parent entries; next NEW directory259603 block0 offset0 salvage question is held. Read-only debugfs stat/ncheck0 gives directory0700/root,4096B/links2/data block15503875; no recoverable pathname from corrupt parents. Private new-inode-259603-readonly.json1120B/0600 SHA256 `4a2409d4de11c01a88109c9526b9c9802444dfcc433823656d9499e7def6fd04`. This new directory was not included in prior salvage approval.

Parent releases only read-only preservation of actual new metadata block15503875 while existing fsck remains paused; expected device BUSY from this own writer is not a new offline-positive claim. Rawreader allowlist extended ONLY15503875, source SHA256 `c5c0649f6150b869042e8890977bc84f5d2e8f18f736451cfe833cd0e5355216`; static binary821424B SHA256 `050316c810573137bd76d313415d9e5877d5dd92f841cdbcbbc09d671905d4e5`, static-Werror compile and six nonallowlisted refusal tests2/empty pass. Existing five-block reader history preserved in previous commits. R7 focused applicability precedes v2RAM stage/read/private durable4096B preservation and actual health comparison. No new salvage answer, fsck restart or other disk writer authorized. Verify own paused fsck identity/deviceFD/current prompt, no other new writer. Continue only after actual captured block/type evidence and specific salvage classification.
