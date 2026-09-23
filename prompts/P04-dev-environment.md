# Task P04 — Lab environment: VMware VMs with real VPP + DPDK on vmxnet3   (prepend 00-CONTEXT.md)

## Goal
`tools/lab up <topology>` gives every agent and every CI job real **VPP 26.06 with the DPDK
plugin driving vmxnet3 NICs inside VMware VMs**. No Docker, no nested KVM, no libvirt.
Moving to physical hardware later means changing the PCI whitelist and driver in the
generated `startup.conf`, nothing else.

## Facts about the environment (verified)
- The dev/CI host `172.30.126.195` is itself a VMware VM (Ubuntu 26.04, 30 vCPU, 39 GB,
  197 GB disk, vmxnet3) **without** nested virtualization. It builds, runs CI, and drives
  the lab over SSH. It does not host VMs.
- Router VMs are separate VMware VMs. DPDK's `vmxnet3` PMD works there; VPP also ships a
  native `vmxnet3` plugin — we use the **DPDK** path because it is the hardware path.
- The project lives in `/root/ngfw` on the dev host; work there as root.

## Build exactly this
1. `tools/lab` (Go or bash — state why): `up <topology>`, `down`, `status`,
   `ssh <vm> [cmd]`, `vppctl <vm> <cmd>`, `restart-vpp <vm>`, `kill-vpp <vm>`,
   `snapshot <vm> <name>` / `restore <vm> <name>` (via `govc` when vSphere credentials are
   present in `~/.config/ngfw/lab.env`; otherwise print the manual step and continue),
   `provision <vm>` (idempotent SSH provisioning). Inventory in `test/topology/*.yml`:
   VM name, management IP, role (router|host|peer-frr|peer-sswan), NIC→segment mapping,
   PCI addresses of the data NICs, addresses to configure.
2. **Router VM provisioning** (`tools/lab provision vrx-a`): Ubuntu 24.04 (or the OS the
   team fixes — read `docs/decisions/os.md`), `scripts/00-add-repos.sh` with `VPP_REPO=2606`
   (or the source-built packages from `deploy/vpp/` if the OS has no upstream build),
   `scripts/10-install-runtime.sh`, `scripts/30-tune-dataplane.sh APPLY=yes HUGEPAGES_1G=2
   ISOLATED=1`, then a generated `startup.conf`: `dpdk { dev <pci> ... }` for every non-mgmt
   vmxnet3, `uio-driver vfio-pci` with `vfio.enable_unsafe_noiommu_mode=1` (fallback
   `uio_pci_generic`), `cpu { main-core 0 corelist-workers 1 }`, `socksvr`, `statseg`,
   plugins per docs/09. Management NIC stays with the kernel (netplan). FRR/Kea/Unbound/chrony
   installed but disabled — renderers own them.
3. **Peer/host VM provisioning**: `host-lan`, `host-wan` (scapy, iperf3, tcpdump; default
   route via the router), `peer-frr` (FRR 10, kernel networking), `peer-sswan` (stock strongSwan,
   kernel XFRM). Minimal specs (1 vCPU, 1 GB).
4. Topologies: `single` = vrx-a + host-lan + host-wan; `tri` = vrx-a/b/c + hosts + peers.
   Segments are vSphere port groups (`mgmt`, `lan`, `wan`, `dmz`, `p2p-ab`, `p2p-bc`); document
   the required VM shapes in `docs/lab/vmware.md` so a human can create them in minutes.
5. `tools/binapi-gen.sh`: copies `/usr/share/vpp/api/**/*.api.json` from `vrx-a` and runs
   `binapi-generator` for the plugins we use (interface, ip, l2, ip_neighbor, vpe, bond, vxlan,
   gre, ipip, gtpu, l2tp, pppoe, sr, lisp, acl, nat44_ed, nat44_ei, nat64, nat66, det44, map,
   cnat, ipsec, ikev2, wireguard, lcp, abf, urpf, policer, qos, lb, span, lldp, bfd, vrrp,
   igmp, mpls, dhcp, dns, flowprobe, sflow, prom, pcap/tracenode) into `apps/agent/binapi/`.
   Commit the output. **Only legal source of VPP API names in the repo.**
6. Smoke test `test/integration/smoke/`: over SSH/agent socket to `vrx-a`: `show_version`
   == 26.06; data NICs present under the `dpdk` driver (`vppctl show hardware` shows
   `VMXNET3`); set IPs; ping host-lan → host-wan through VPP; rx counters increased.
7. CI: `pr.yml` runs `tools/lab up single` against the always-on lab VMs (serialised with a
   lock — one PR at a time on `vrx-a`, or per-PR VMs when govc is available); `nightly.yml`
   uses `tri`. Every run restores the `clean` snapshot first when snapshots are available.

## Acceptance
- [ ] `tools/lab up single` from the dev host → `status` shows VPP active on `vrx-a` in < 3 min (VMs pre-created)
- [ ] `tools/lab vppctl vrx-a show hardware` lists the vmxnet3 NICs under DPDK
- [ ] `tools/lab kill-vpp vrx-a` → VPP back in < 20 s (systemd); agent (later) reconciles
- [ ] `apps/agent/binapi/` committed; rerunning the generator yields an empty `git diff`

## Out of scope
Docker, libvirt, nested KVM. Physical NIC drivers. Performance. Product `.deb`s (P10).
