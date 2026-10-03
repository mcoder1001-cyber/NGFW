# NGFW unattended installer ISO

The ISO builder combines Ubuntu Server 26.04 amd64 installation media, a signed
offline package pool and a NGFW autoinstall seed. The source and focused fixture
checks exist; a complete production signed ISO and an unattended VM installation
have **NOT RUN** acceptance as of 2026-10-03. This guide describes implemented
behavior and the evidence still required, not a production-ready release.

## Disk selection and boot behavior

Both Install and Reinstall erase the **largest disk selected by the storage
layout**, with a minimum fixed-disk size check of 96 GiB. Reinstall does not select
the disk containing an existing NGFW installation. An existing installation on a
smaller disk does not protect a larger unrelated disk. Use disposable disks for
acceptance and check the VM's attached disks before booting the media.

The default GRUB entry is unattended Install, with a five-second menu timeout.
Reinstall adds `ngfw.reinstall=1`. The early guard refuses disk inventory errors,
including partial failed inventory output, even with that flag. Without the flag,
finding the `ngfw-rootA` label or partition name refuses installation. The installer
prints its reason and attempts to power off after 60 seconds. The Ubuntu interactive
installer remains available in a separate submenu.

The layout includes BIOS GRUB space, the `NGFW-EFI` ESP, `ngfw-rootA`, reserved
`ngfw-rootB`, `ngfw-log`, `ngfw-pg` and `ngfw-data`. RootB reserves space for later A/B
work; this installer does not implement an upgrade mechanism. UEFI boots target the
ESP; the early script adjusts GRUB's target for BIOS. The builder replays the base
ISO's boot configuration with xorriso. Actual BIOS and UEFI boot remain NOT RUN.
Secure Boot signing is outside this builder's scope.

## Prepare trusted inputs

Run builds in a dedicated build environment, using an isolated checkout and its
`.scratch` directories. The script requires root for chroot/file-ownership work.
Do not install build dependencies on the appliance host. Arrange the coordinator's
shared resource scheduling before any build; this guide does not authorize host or
VM changes.

Required inputs are:

- An Ubuntu live-server amd64 ISO with adjacent `SHA256SUMS` and `SHA256SUMS.gpg`.
  The current default filename is `ubuntu-26.04.1-live-server-amd64.iso`.
  `--base-iso` supplies another explicit path; its basename must appear in the signed
  sums. The builder checks its pinned Ubuntu CD signer and SHA-256.
- P10's signed repository containing `ngfw-meta` and the shipped VPP package set.
  Supply `--ngfw-repo` and an independently trusted 40-hex `--ngfw-key-fpr`; obtaining
  the fingerprint from the same untrusted repository is not independent trust.
- The matching VPP producer v2 `manifest.json`, supplied with `--vpp-manifest`.
  Its package set and ship flags must match `deploy/vpp/VERSION`; shipped archives
  must match version, control identity, architecture and hash. The builder rejects
  omissions, duplicates, non-shipped VPP artifacts and unsafe paths.
- Ubuntu archive/CD and FRR public keyrings through `--ubuntu-keyring` and
  `--frr-keyring`. The builder uses pinned signer fingerprints.
- A build chroot containing cloud-init and unsquashfs. `--make-chroot` prepares an
  Ubuntu `resolute` chroot through debootstrap; it refuses an existing chroot path.
- An external GPG signing-key home via `--gpg-home`. Keep private keys outside the
  checkout. `--gen-key` can create a dedicated key if none is present; choose key
  custody and distribution through the release process before using that option.

The external package/pool builder must deliver the signed NGFW repository and
matching VPP manifest. The ISO resolver additionally needs access to Ubuntu/FRR
archives to download the complete dependency closure, including Recommends.
This build-time access is distinct from the intended offline installation.
Fixture tests cannot establish completeness of the production pool.

The script checks for xorriso, gpg/gpgv, systemd-nspawn, dpkg-deb,
dpkg-scanpackages, apt-get and Python 3. Chroot creation and base downloading also
need debootstrap/unshare and curl respectively. Check space for media, chroot,
downloaded packages and outputs before scheduling; the historical budget was
8–10 GB with at least 40 GB free, not a measurement of current production inputs.

## Stage and inspect a build

Run from the checkout root. This example uses placeholders that must be replaced;
none of these commands were executed to validate this guide.

```sh
deploy/image/iso/build-iso.sh --help

deploy/image/iso/build-iso.sh \
  --base-iso "$PWD/.scratch/base/ubuntu-26.04.1-live-server-amd64.iso" \
  --ngfw-repo /path/to/signed-ngfw-repository \
  --ngfw-key-fpr TRUSTED_40_HEX_FINGERPRINT \
  --vpp-manifest /path/to/matching/manifest.json \
  --ubuntu-keyring /path/to/ubuntu-archive-keyring.gpg \
  --frr-keyring /path/to/frrouting.gpg \
  --gpg-home /path/outside/checkout/iso-signing \
  --stage verify
```

Prepare the chroot separately or add `--make-chroot` to the first staged invocation.
`--fetch-base` downloads media and signed sums from the configured Ubuntu release
URL; availability is not established here. `--mirror` and `--frr-uri` select the
build-time archives. Every invocation recreates the owned work directory, so
stages rerun prerequisite work rather than resume it. Preserve evidence and lock
files outside that directory before starting another invocation.

| Stage | Implemented work and evidence |
| --- | --- |
| `verify` | Signed base ISO and repository checks, base package status extraction, VPP manifest parity |
| `pool` | Dependency download, sorted pool manifest, signed embedded repository, isolated pool-only APT simulation |
| `tree` | Autoinstall render/schema validation, installer/common files, build-info and GRUB tree |
| `iso` | xorriso output, SHA-256 file and detached ASCII GPG signature; default stage |

Repeat the same invocation with the appropriate `--stage` after reviewing the
preceding evidence. Outputs default to `.scratch/out`; evidence is below its
`evidence` directory. The final files are `ngfw-<version>.iso`, `.iso.sha256` and
`.iso.asc`, with `build-info.json` and `pool.manifest`. Build-info records base and
repository hashes, VPP provenance/build inputs and selected options.
Use `--lock /path/to/saved/pool.manifest` for exact-version dependency rebuilding.
Identical package manifests are the required reproducibility evidence; byte-identical
ISO output is not established by fixed package versions alone.

Optional flags include `--source ubuntu-server` (default minimal),
`--serial-console` (ttyS0, 115200n8), `--hostname`, `--version`,
`--ssh-authorized-keys` for root public keys, and `--apt-uri` for an optional
installed online repository using HTTPS or file URI. `--scratch`, `--work`,
`--out` and `--chroot` override working paths subject to the builder's path guards.
`--luks` encrypts log/PostgreSQL/data volumes with a per-install key and automatic
boot unlock; it does not add a passphrase prompt or encrypt the root volumes.

## Installation and first boot

The seed requests no interactive sections, disables installer refresh and uses
English/US keyboard/UTC. Late commands use only the signed embedded pool to install
NGFW packages, purge unwanted packages and apply appliance/kernel settings.
There is no default OS user or password SSH login. Optional root authorized keys
are embedded only when explicitly supplied. The API bootstrap administrator's
random password is generated on the target, not baked into the ISO.

The console banner shows management address/HTTPS URL and bootstrap credentials.
After firstboot is complete, a periodic database check removes the console password
copy once no unused bootstrap administrator remains. A database failure preserves
the copy for retry. Real first-login/banner cleanup acceptance remains NOT RUN.
Installer logs include `/var/log/ngfw-early.log` and `/var/log/ngfw-late.log`; late logs
are copied to the installed system's `/var/log/installer`.

## Acceptance still required

Before declaring a releasable image, record actual evidence for signed production
inputs, pool-only dependency simulation, cloud-init schema validation, two identical
package manifests, xorriso file listings/boot metadata and ISO signature verification.
Then use an owner-provided disposable amd64 VM for separate BIOS/UEFI boots,
network-disconnected installation, default/reinstall safety, disk layout, firstboot
and credential removal. Remove installation media before ordinary boot. Confirm
builder-owned mounts/containers/temporary resources are cleaned up.

These build and VM cases are NOT RUN here. Focused offline checks in
`deploy/image/iso/tests/run.sh` cover fixture behavior, not production dependency
closure or installed-appliance acceptance. Do not mark the whole P14 task complete
from those checks alone.
