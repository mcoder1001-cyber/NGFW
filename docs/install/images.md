# NGFW VM and cloud disk images

The image builder reuses the ISO's package list, partition labels, kernel defaults,
bootstrap-password generator and console banner from `deploy/image/iso/common`.
The appliance packages and VPP producer manifest must come from the published,
signed release repository. Image source tests do not establish appliance boot or
cloud import acceptance.

Allocate at least **4 vCPUs, 16 GiB RAM and 96 GiB disk**; use 32 GiB RAM or more
for a full BGP table and large NAT state. GPT contains a 1 MiB BIOS boot partition,
1 GiB EFI partition, 20 GiB rootA, reserved 20 GiB rootB, 20 GiB `/var/log`,
20 GiB PostgreSQL partition and the remaining `/data`. RootB is unmounted and
reserves space for a separate A/B upgrade mechanism. Secure Boot is not supported.

## Prepare and build

Use an isolated worktree and dedicated image build host with at least 40 GiB free.
Do not install packages on the shared development host. The builder requires root
and `NGFW_INTEGRATION=1`; it creates loop devices only for its own scratch image
and mounts them in a private mount namespace. GRUB runs from target packages,
against only the image loop device, with explicit boot/EFI directories and
`--no-nvram`. No bootloader installation targets the build host.

Inputs:

- A signed `resolute` dependency pool produced by P14, containing the same
  `ngfw-meta` package set and matching VPP packages. Extend its dependency closure
  with `linux-image-generic`, `grub-pc-bin`, `grub-efi-amd64-bin`, `grub2-common`,
  `cloud-init`, `openssh-server`, `netplan.io` and `systemd-resolved`. Dosfstools
  is included by debootstrap inside the disposable build root.
- The public pool signing key and independently supplied 40-hex fingerprint.
  The key's presence in the repository does not establish trust.
- The matching `ngfw.vpp-debs.manifest/v2`; all shipped VPP files, control
  identities and hashes must match the producer contract before any disk write.
- Ubuntu archive public keyring carrying the pinned archive signer used by P14.
  Debootstrap verifies signed Ubuntu `resolute` archives. Build-time mirror access
  is required; appliance package installation then runs with a private network
  and only the signed local pool. No missing dependency falls back to the network.
- The **explicit management NIC name expected in the guest**, e.g. `ens160` for
  VMware, `ens3` for a particular virtio PCI topology, or `eth0` for a cloud guest.
  Names depend on virtual hardware and cloud policy: verify them on acceptance
  hardware. The generated netplan enables DHCP only for this NIC and disables
  cloud-init's network renderer. The firewall accepts management traffic only
  from the same explicit NIC. An incorrect name leaves management inaccessible;
  correct the netplan and `/etc/ngfw/base-policy.env` from the VM console.

Example commands (paths and fingerprint are placeholders):

```sh
NGFW_INTEGRATION=1 deploy/image/vm/build.sh \
  --pool /path/to/signed-pool --pool-key /path/to/public-key.gpg \
  --pool-key-fpr <trusted-40-hex-fingerprint> \
  --vpp-manifest /path/to/vpp/manifest.json \
  --management-interface ens160 --profile vm --size-gib 96
```

For a long build use `nohup`, a task-owned log and the PID returned by the shell;
record that PID in task status. Inspect/poll the log rather than launching another
build. Output is `.scratch/w16-images-<profile>/output`. An existing task directory
is refused, preserving failed-build evidence. Inspect owned mounts and loop
devices before manually removing scratch for a retry. Never remove another task's
scratch. Raw intermediates are removed after successful conversion; final outputs
and evidence remain.

Each build emits qcow2, streamOptimized VMDK, OVF/OVA and VHDX. `SHA256SUMS` covers
all artifacts and `manifest.json`; the manifest records installed package versions,
VPP producer provenance, profile, layout digest and per-artifact digests. SHA-256
checksums are integrity checks, not image signatures. Inspect signature provenance
and obtain images through a trusted release channel.

The offline inspector checks fstab labels, rootB reservation, GRUB/EFI files,
installed NGFW/VPP and infrastructure packages, nftables and service units,
cloud datasource configuration, locked Unix passwords, empty machine-id, no SSH
host keys, no built-in bootstrap/API secrets and no firstboot completion state.
The builder emits the partition table and `qemu-img info` for each disk, checks
supported formats and compares every converted disk byte-for-byte to the raw
image. VHD/VHDX and unsupported VMDK structural checks are explicitly recorded as
unsupported; comparison is required rather than reporting those checks passed.

## Import into a hypervisor

Verify `sha256sum -c SHA256SUMS` before import. Initially attach only the management
NIC and use the explicit interface name supplied during the build.

| Hypervisor | Artifact and import | Management NIC |
|---|---|---|
| KVM | Create a VM using the existing `ngfw.qcow2` disk; select UEFI or BIOS, 4 vCPUs, 16 GiB RAM and serial console. Do this on an authorized hypervisor, never the shared NGFW host. | virtio-net |
| Proxmox | `qm importdisk <VMID> ngfw.qcow2 <storage>` then attach the imported disk as SCSI, choose VirtIO SCSI controller and boot order; use OVMF (UEFI) or SeaBIOS. | VirtIO |
| VMware | Import `ngfw.ova` with the vSphere deployment wizard; map the Management network and retain EFI firmware. OVF includes 4 CPUs, 16 GiB RAM, a SCSI disk and VmxNet3 NIC. | vmxnet3 |
| Hyper-V | Create a Generation 2 VM with `ngfw.vhdx`, disable Secure Boot, allocate 16 GiB RAM and attach a synthetic NIC. Generation 1/BIOS boot needs separate acceptance. | netvsc |

Cloud-init VM datasources are `NoCloud` and `ConfigDrive`. Supply a seed image or
config drive for host naming and permitted SSH public keys; no default Unix user
or password login is enabled. API admin credentials are generated separately by
P14's firstboot unit and displayed once on the console. SSH host keys and machine
identity are generated per clone. A compatibility symlink maps P14's
`firstboot.done` observation to P10's current atomically published
`firstboot-complete`, so bootstrap credentials stop being regenerated after
provisioning and the console timer can detect the first admin login.

## Cloud profiles and imports

Build each cloud profile separately; changing format alone does not change the
in-image datasource or drivers. `--profile aws` sets the `Ec2` datasource and ENA/
NVMe initramfs modules and emits a fixed VHD for VM Import. `--profile azure` sets
`Azure` and Hyper-V modules and emits a fixed VHD with a 1 MiB-aligned virtual size.
`--profile gcp` sets `GCE`, virtio modules and emits a gzip tar with `disk.raw` as
its sole member. There is no cloud account operation in the builder.

These are **documented commands, not executed acceptance evidence**. Upload only
after human/account authorization and configure the provider's import permissions,
network/security groups and serial-console access. Credentials are never image
inputs. Replace all placeholders with account-approved values.

```sh
# AWS: upload ngfw-aws.vhd to the approved S3 bucket, then use a local containers.json:
# [{"Description":"NGFW","Format":"VHD","UserBucket":{"S3Bucket":"BUCKET","S3Key":"ngfw-aws.vhd"}}]
aws ec2 import-image --description 'NGFW Ubuntu 26.04' \
  --boot-mode uefi --disk-containers file://containers.json

# Azure: upload ngfw-azure.vhd as a page blob. Create a generalized Gen2 image:
az image create --resource-group <group> --name <image> --os-type Linux \
  --hyper-v-generation V2 --source <approved-VHD-blob-URL>

# GCP: upload ngfw-gcp.tar.gz to an approved Cloud Storage bucket:
gcloud compute images create <image> --source-uri gs://<bucket>/ngfw-gcp.tar.gz \
  --guest-os-features UEFI_COMPATIBLE --architecture X86_64
```

ENA/netvsc/virtio/vmxnet3 drivers are available in the image kernel/initramfs. VPP
continues to boot with `dpdk { no-pci }` until the hardware wizard explicitly binds
NICs. Cloud drivers do not authorize DPDK or automatically bind the management NIC.
Cloud routing integration, HA route-table changes, marketplace listings, image
signing and Secure Boot remain outside this task.

## Dataplane NICs and deferred acceptance

Keep the management NIC under the guest kernel. For DPDK, add separate vmxnet3 or
virtio dataplane NICs and follow the hardware wizard's supported driver policy.
SR-IOV/PCI passthrough requires a separate authorized hypervisor, host IOMMU,
supported VF/device driver, suitable IOMMU groups and guest hugepages/memlock.
Do not enable or reconfigure SR-IOV on the shared host from this workflow.

Deferred real acceptance requires a disposable VM for each hypervisor and approved
cloud accounts. Boot with BIOS and UEFI where applicable; verify each filesystem
label/mount, rootB unmounted, management DHCP and serial console; confirm the
once-only random API admin password, successful admin login/password cleanup,
unique machine-id and SSH keys across two clones, `ngfw-firstboot` completion,
PostgreSQL/nftables/ngfw-agent/API/web health, VPP 26.06 and `dpdk { no-pci }`;
reboot and repeat state/health checks. Inspect the management firewall and verify
a separate dataplane NIC stays unbound until authorized wizard setup. For each
cloud, verify import completion, metadata/datasource, ENA/netvsc/virtio driver,
management connectivity, serial recovery and reboot persistence. Record failures
as code failures; only unavailable hardware/accounts defer execution.
