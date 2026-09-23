# VMware lab — VM shapes for the product owner and how `tools/lab provision` treats them (P04)

Lab = VMware VMs, VPP with DPDK on vmxnet3; no Docker, no libvirt, no nested KVM (D-002). Today only **`vrx-a`** exists:
it is the dev/CI host itself (`docs/lab/host-vrx-a.md`), driven in *local mode* (`mgmt: local`, no SSH). The machines
below are **planned**; their inventory files already exist in `test/topology/*.yml` with `state: planned` and
`mgmt: tbd` so that `tools/lab` knows their shape and refuses to touch them until they are real.

## VM shapes to create (vSphere)

| VM | role | vCPU | RAM | disk | NICs (all **vmxnet3**) → port group / segment | notes |
|---|---|---|---|---|---|---|
| `vrx-b` | VRX under test (VPP + our stack) | 4 | 6 GB | 40 GB | `ens192`→**mgmt**, `ens224`→**lan**, `ens256`→**wan**, `ens161`→**p2p** | DPDK on lan/wan/p2p |
| `vrx-c` | second VRX (HA/VRRP, BGP/OSPF peer, IPsec peer) | 4 | 6 GB | 40 GB | same 4 NICs as vrx-b | p2p links vrx-b↔vrx-c |
| `host-lan` | traffic endpoint on the LAN side | 1 | 1 GB | 10 GB | `ens192`→mgmt, `ens224`→**lan** | Ubuntu 26.04, iperf3, ping, scapy |
| `host-wan` | traffic endpoint on the WAN side | 1 | 1 GB | 10 GB | `ens192`→mgmt, `ens224`→**wan** | same |
| `peer-frr` | external BGP/OSPF router (FRR) | 1 | 1 GB | 10 GB | `ens192`→mgmt, `ens224`→**wan** | FRR from the Ubuntu archive |
| `peer-sswan` | external IPsec responder (strongSwan) | 1 | 1 GB | 10 GB | `ens192`→mgmt, `ens224`→**wan** | stock strongSwan (kernel path) |

Port groups: `mgmt` (routed, 172.30.126.0/24 — the same network as vrx-a), `lan`, `wan`, `p2p` — the last three **isolated**
(no uplink, promiscuous mode + forged transmits + MAC changes **accepted** on the vSwitch/port group, otherwise VRRP,
bridging and MAC-learning tests fail). Guest OS: **Ubuntu 26.04 LTS server**, root SSH key of the dev host installed,
`cloud-init` may be used to set the hostname and the mgmt address. Hardware: `EFI`, `IOMMU` exposed to the guest if
the ESXi version allows it (then `vfio-pci`; otherwise `uio_pci_generic` for DPDK on vmxnet3), 2 MB hugepages are set
by `provision` (`vm.nr_hugepages=1024`; the 6 GB VMs keep 2 GB for VPP).

Proposed addressing (edit `test/topology/*.yml` if the owner chooses differently):

| segment | vrx-b | vrx-c | hosts/peers |
|---|---|---|---|
| lan | 10.101.1.1/24 | 10.102.1.1/24 | host-lan 10.101.1.100 |
| wan | 10.101.2.1/24 | 10.102.2.1/24 | host-wan 10.101.2.100 · peer-frr 10.101.2.200 · peer-sswan 10.101.2.201 |
| p2p | 10.100.9.1/30 | 10.100.9.2/30 | — |

## vrx-a today (the host we already have)

7× vmxnet3 are present on 2026-09-23: `ens192` (0000:0b:00.0, mgmt, kernel) plus **six unconfigured, DOWN** NICs
(`ens161` 04:00.0, `ens193` 0c:00.0, `ens224` 13:00.0, `ens225` 14:00.0, `ens256` 1b:00.0, `ens257` 1c:00.0) — recorded in
`test/topology/vrx-a.yml` as `segment: unassigned`. Binding any of them to DPDK needs `dpdk { dev <pci> }` in
`/etc/vpp/startup.conf` → only after `handover: done` (D-012), by the manager. Until then the data path on vrx-a is the
af_packet veth/netns rig (`tools/lab rig`, D-010) and every test records `path: af_packet`.

## How `tools/lab provision <vm>` treats each machine

- **`vrx-a` (`mgmt: local`)** — *verify only*: OS, our `vpp*` 26.06 packages, `vpp.service`, sockets, `show version`,
  plugin count, af_packet, hugepages, 143 `.api.json`, `binapi-generator`, Go, tools, PostgreSQL ≥ 16, Valkey. It prints
  PASS/FAIL per item and the manual steps; it never edits `/etc/vpp/*`, never installs/removes packages, never restarts VPP.
- **`vrx-b`, `vrx-c` (`role: vrx`, remote)** — full provisioning for Ubuntu 26.04 from **our source-built debs**
  `/root/vpp/build-root/*.deb` (D-001, never the FD.io repo): `scp` the debs, `apt-get install ./*.deb`,
  `vm.nr_hugepages=1024`, render `/etc/vpp/startup.conf` from the inventory (`unix/api-segment/socksvr` like vrx-a plus
  `dpdk { dev <pci> { name <segment> } … blacklist <mgmt pci> }` for every NIC with `driver: dpdk`), `driverctl
  set-override <pci> vfio-pci`, `systemctl enable --now vpp`, then `show version` / `show hardware-interfaces`.
  `provision <vm>` without `--apply` prints the plan and the rendered startup.conf (dry run); with `state: planned` it
  refuses (exit 3) until `mgmt`/`pci` are filled in. **The remote path is written but untested — no remote VM exists yet.**
  Three guards stand in front of `--apply` (review F4): the `mgmt` address must not be one of **this** host's addresses
  (`127.x`, `::1`, anything in `ip addr`/`hostname -I` — ssh-to-self would rewrite `/etc/vpp/startup.conf` here), the
  inventory `role` must be `vrx`, and the caller must export `VRX_LAB_REMOTE_APPLY=1` to confirm that `/etc/vpp`,
  `/etc/sysctl.d/80-vpp.conf` and the package set of `root@<mgmt>` will be changed. Without all three it refuses (exit 1).
- **`host-*`, `peer-*` (remote, no VPP)** — out of P04's scope; `provision` prints a one-line plan and **refuses `--apply`**
  (only `role: vrx` machines ever get VPP from this tool). Their software (iperf3, FRR, strongSwan) will be installed by
  the tasks that need them (S6 `tri` topology).

Remote commands run as `root@<mgmt>` over SSH (`BatchMode`, 5 s connect timeout) and never against an address of the
machine they run on. `tools/lab status <vm>` / `vppctl <vm> …` work the same way once a VM is `state: active`. Data path
on these VMs: **DPDK on vmxnet3** (`vpp-plugin-dpdk`), tests then record `path: dpdk`. `restart-vpp`/`kill-vpp <vm>` read
the handover flag from `docs/lab/host-<vm>.md` in the canonical main checkout **and** the calling worktree (both must say
`done`; no inventory or environment override) — for a new VM that file is written in checklist step 4 below.

## When the VMs exist — checklist

1. Fill `mgmt` and each NIC's `pci` (`lspci -nn | grep -i vmxnet`) in `test/topology/<vm>.yml`; set `state: active`.
2. `tools/lab provision <vm>` (dry run) → review the rendered startup.conf → `tools/lab provision <vm> --apply`.
3. `tools/lab status <vm>`, `tools/lab vppctl <vm> show hardware-interfaces` — vmxnet3 ports must appear as `VMXNET3` DPDK devices.
4. Record the result in `docs/lab/host-<vm>.md` (like `host-vrx-a.md`) and in `docs/decisions/LOG.md`.
