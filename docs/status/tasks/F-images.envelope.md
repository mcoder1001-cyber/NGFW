# TASK ENVELOPE — F-images
id: F-images   branch: task/F-images   worktree: /root/ngfw-wt/F-images   base: main@<BASE>   started: <STARTED>
title: S5 system (day 16-18): VM and cloud image builds (qcow2, ova, vhdx + AWS/Azure/GCP profiles)
prompt: prompts/features/F-images.md   (template: prompts/FEATURE-TEMPLATE.md; refreshed on task/prep-rest against main@11a175b: host tools, `dpdk { no-pci }`, per-task loop-device checks)   wbs: D11.2, D11.1
scope: `deploy/image/vm/build.sh` (debootstrap → raw sparse image with the §7 layout → vrx-meta from the published repo → GRUB UEFI+BIOS, cloud-init NoCloud/ConfigDrive, serial console, no default password, empty machine-id, no SSH host keys), conversions (qcow2, vmdk/ova, vhdx) with SHA256SUMS + manifest, cloud overlays + documented (not executed) import commands, offline inspection, docs. Booting and cloud import are deferred
merged deps you can rely on: P10, P14 (+ F-vpp-debs, F-startup-gen)
  - P10: packages + the repo the manager published to `/srv/vrx-artifacts/apt/`, firstboot (startup.conf via `vrx-startupgen` → `dpdk { no-pci }`), `inet vrx_base`
  - P14: `deploy/image/common/` (package list, first-boot banner, bootstrap password), partition labels (`VRX-EFI`, `vrx-rootA`, `vrx-rootB`, `vrx-log`, `vrx-pg`, `vrx-data`), appliance marker, kernel cmdline defaults
read first: prompts/features/F-images.md · docs/status/wave-BC-numbers.md (section "S5 system": SY7, SY9 + "P14", "F-images") · docs/09-os-packages.md §4/§7 · docs/install/iso.md + docs/install/bare-metal.md · deploy/vpp/README.md · docs/decisions/LOG.md D-001, D-002, D-059, D-092
slot: <SLOT> → VRX_SLOT=<SLOT> VRX_TEST_PREFIX=w<SLOT> VRX_HTTP_PORT=3000+100·<SLOT> VRX_WEB_PORT=5000+100·<SLOT> VRX_METRICS_PORT=9100+10·<SLOT>+1 VRX_AGENT_SOCKET=/run/vrx-test/w<SLOT>/agent.sock VRX_PG_DATABASE=vrx_w<SLOT> VRX_VALKEY_DB=<SLOT> VRX_VPP_TABLE_BASE=<SLOT>000 VRX_LAB_LOCK=/run/lock/vrx-lab.lock
  - source of truth: `eval "$(tools/lab env <SLOT>)"`
  - raw images, mounts, the build chroot and outputs live under /root/ngfw-wt/F-images/.scratch/ (names carry w<SLOT>); loop devices only on your own `.scratch/` files; nspawn machine name `w<SLOT>-img`, always `--private-network`
  - long builds (> 8 min): `nohup … > /root/ngfw-wt/logs/F-images-build.log 2>&1 &`, polled
  - slots 1–11 only; 12 is CI
daemon-owner: none (nothing runs on the host; no hypervisor is used)
HOST BOOT SAFETY: every GRUB call takes explicit `--boot-directory=<mnt>/boot --efi-directory=<mnt>/boot/efi --no-nvram` (or `--target=i386-pc` with the chroot's modules) into the image; never `update-grub` on the host; paste `sha256sum /boot/grub/grubenv` before/after
obligations:
  - reuse, do not fork: P14's `deploy/image/common/` and package list are read-only for you; a change → questions file
  - no secrets or build-host material in any image: no `/etc/ssh/ssh_host_*`, empty `/etc/machine-id`, no bootstrap password baked in (printed once on first boot as in P14), no apt signing private key, no `.scratch` paths (inspection output pasted)
  - VPP boots with `dpdk { no-pci }` (P10's firstboot; there is no `dpdk { disable }` stanza); cloud DPDK defaults are an open question
  - D-002: no libvirt/virt-install/KVM (libvirt is installed on this host by mistake — never use it); cloud import commands are documented, never executed; no cloud credentials anywhere
  - D-059: marketplace publishing, cloud route-table HA, secure boot and image signing are have-nots — list them
files you own exclusively:
  - deploy/image/vm/** (builder, conversion, OVF template), deploy/image/cloud/** (aws/azure/gcp overlays + import notes)
  - docs/install/images.md
  - test/topology/images/** (offline inspection tests; root/loop parts behind `VRX_INTEGRATION=1`, skipped in unit mode)
  - docs/status/tasks/F-images*
shared hotspots (append-only, conflicts resolved by the manager at merge; ids from docs/status/wave-BC-numbers.md "S5 system"):
  - protocol: docs/status/wave-A-hotspots.md §0. List every hunk under "Shared hunks" in docs/status/tasks/F-images.md
  - SY7 root .gitignore `/.scratch/`: seeded by the manager; if absent, never `git add -A`
  - SY9 tools/ci.sh: ask for a `deploy/image` shellcheck step; never edit it
contract numbers: none (docs/status/wave-BC-numbers.md "F-images": names only)
files you must not touch:
  - everything else
  - never: /root/ngfw (main), other worktrees, /etc, /boot, the disks of this host, /root/vpp, host packages or services
  - deploy/image/iso/**, deploy/image/common/**, deploy/image/build-iso.sh (P14), deploy/upgrade/** (F-ab-upgrade), deploy/debian/**, deploy/apt/**, deploy/systemd/**, deploy/firstboot/** (P10), deploy/vpp/**
  - apps/**, packages/**, tools/*, plan/tasks.yaml, docs/decisions/LOG.md, docs/status/PROGRESS.md
host facts (verified 2026-09-24; no host package installs):
  - present: debootstrap (`resolute`), qemu-img (qcow2, vmdk streamOptimized, vhdx), sfdisk, losetup, mkfs.ext4, grub-install (**x86_64-efi only**), zstd, gpg, systemd-nspawn
  - absent: mmdebstrap, virt-inspector/guestfish (fallback: `losetup -P` + read-only mount), mkfs.vfat/mtools, GRUB i386-pc (BIOS), cloud-init, ovftool → inside a build chroot under `.scratch/` (packages from the mirror `repo.amnafzar.ir`) or `apt-get download` + `dpkg -x`; the `.ova` is a tar of `.ovf` + `.vmdk` + `.mf`
  - debootstrap and the chroot need the mirror; if unreachable → PENDING-network in the questions file and build what you can
  - disk: a raw 20+ GB sparse image plus three converted formats; check `df -BG /` ≥ 40 G free before each build, delete intermediates as you go
coordination:
  - F-ab-upgrade (parallel): same layout and labels; rootB stays reserved in your images
  - F-hardening-lite (parallel): its baseline is not applied by you; if it merges first, a follow-up applies it to the images (note it)
  - P14: a missing piece in `deploy/image/common/` → questions file, never a copy
evidence: `ls -l`, `qemu-img info`/`check` for every output, SHA256SUMS + manifest (package versions incl. VPP), offline inspection (partition table, fstab, `dpkg-query --admindir` for vrx-meta + VPP, cloud-init datasources, grub.cfg, units present, no host keys, empty machine-id), host-unchanged listings (`findmnt | grep F-images` empty, `losetup -a` filtered to `.scratch/`, grubenv sha256), the deferred boot/import test plan
time box: 15 h — when exceeded: stop, commit WIP, write docs/status/tasks/F-images.md with what is left
WIP: commit at least every 45 min; keep docs/status/tasks/F-images-wip.md current
CI: `TMPDIR=/tmp/g-w<SLOT> tools/ci.sh --base main` — short TMPDIR (unix socket paths ≤ 108 chars); no host-wide CI lock: golangci-lint serializes itself since main fc0fe68 (D-106 rejected serialising whole gates). Ports 3000/8080/9101 and /run/vrx/agent.sock belong to the running product stack (tools/app) — never touch them
finish: `tools/ci.sh --base main` green in the worktree · docs/status/tasks/F-images.md with pasted real output · everything committed · final message = 10-line summary (branch, last commit, CI result, evidence, open questions, decisions taken with options)
cleanup: no mounts or loop devices of yours, no `w<SLOT>-img` container, build chroot and raw intermediates deleted, final artefacts listed with paths + sha256 in F-images.md (they stay in `.scratch/` for the manager), processes stopped by PID
questions: docs/status/tasks/F-images-questions.md — write and keep going; never wait for a human
never: merge · restart/kill VPP · Docker · pkill · secrets in files · edit files you do not own · install, remove or upgrade a package on this host
