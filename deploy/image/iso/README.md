# VRX installer ISO

This directory contains the P14 installer builder and its offline unit checks. A complete ISO and an unattended VM installation have not yet been validated.

`build-iso.sh --help` lists input paths and stages. Build outputs, downloaded media and chroot files belong under the worktree's `.scratch/`; never install build tools on the appliance host.

Required inputs:

- Ubuntu 26.04 live-server amd64 ISO with adjacent `SHA256SUMS` and `SHA256SUMS.gpg`. The builder verifies the pinned Ubuntu CD signer before accepting the media.
- Signed VRX APT repository containing `vrx-meta` and source-built VPP packages, together with the independently trusted VRX signer fingerprint.
- Matching VPP package manifest supplied with `--vpp-manifest`.
- Ubuntu and FRR public archive keyrings. Public signer fingerprints are trust pins, not credentials.
- A build chroot with cloud-init, squashfs-tools and the packages listed by the builder's `--make-chroot` path.
- An external ISO signing key home. Keep private keys outside the repository.

The dependency resolver downloads a package closure, indexes and signs the embedded pool, and attempts an isolated pool-only APT simulation. The autoinstall renderer places the seed on the ISO, and xorriso replays the base media's boot configuration. Those stages require real inputs and remain to be executed. Unit checks of fixture repositories do not prove that the production dependency closure is complete.

To run focused checks without network, mounts or daemons:

```sh
mkdir -p .scratch/tests
TMPDIR="$PWD/.scratch/tests" tools/heavy.sh bash deploy/image/iso/tests/run.sh
```

If the checkout does not include the shared-host semaphore helper, use the manager-provided path to that helper. The command requires Python 3 with PyYAML, standard shell utilities, and dpkg fixture tools for pool checks. Shellcheck runs when installed.

The installer exposes destructive disk-install and explicit reinstall GRUB entries. VM boot, actual offline installation, BIOS/UEFI behavior and console credential lifecycle on the installed appliance require a disposable owner-provided VM; none are established by renderer tests. Secure Boot signing and the A/B upgrade mechanism are outside this builder's scope.

The early installer refuses disk inventory errors before disk changes, including
failed lsblk commands with partial output. Explicit reinstall permits a known
existing VRX installation but does not override unreadable disk inventory.
The read-only guard has five offline stub-lsblk regression cases; the aggregate
suite includes them. Tests create only scratch files and never enumerate or
modify the host's actual disks.
