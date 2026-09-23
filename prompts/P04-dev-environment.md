# Task P04 — Developer environment: VMs with real VPP + DPDK   (prepend 00-CONTEXT.md)

## Goal
`tools/lab up` gives every agent and every CI job real **VPP 26.06 running with DPDK on
virtio NICs inside QEMU/KVM VMs** — the same code path as physical hardware. No Docker.
Moving to hardware later means changing the PCI whitelist and driver, nothing else.

## Build exactly this
1. `tools/lab` (bash or Go, your call — state why): subcommands
   `up [topology]`, `down`, `snapshot <name>`, `restore <name>`, `restart-vpp <vm>`,
   `kill-vpp <vm>` (SIGKILL, then systemd restarts it), `ssh <vm> [cmd]`, `vppctl <vm> <cmd>`,
   `status`. Uses libvirt (`virsh`, `virt-install`) with isolated networks acting as L2
   segments: `mgmt`, `lan`, `wan`, `dmz`, `p2p-ab`, `p2p-bc`.
2. Base image: Ubuntu 24.04 cloud image + cloud-init nocloud. Provisioning runs
   `scripts/00-add-repos.sh` (VPP_REPO=2606) and `scripts/10-install-runtime.sh`, then
   `scripts/30-tune-dataplane.sh APPLY=yes HUGEPAGES_1G=2 ISOLATED=1`. Build once into
   `images/vrx-base.qcow2`; VMs are copy-on-write overlays. Cache the image in CI.
3. Router VMs `vrx-a`, `vrx-b`, `vrx-c` (4 vCPU, 6 GB, virtio NICs on mgmt + 3 segments):
   VPP `startup.conf` generated per VM — `dpdk { dev <pci> ... uio-driver uio_pci_generic }`
   for every non-mgmt NIC, `cpu { main-core 0 corelist-workers 1 }`, `socksvr`, `statseg`,
   plugins as in docs/09. Management NIC stays with the kernel (netplan). FRR, Kea, Unbound,
   chrony installed but **disabled** (renderers enable them).
4. Peer VMs: `host-lan`, `host-wan` (scapy, iperf3, tcpdump), `peer-frr` (FRR 10, kernel
   networking), `peer-sswan` (stock strongSwan, kernel XFRM). 1 vCPU, 1 GB each.
5. Topologies as YAML in `test/topology/*.yml` (which VMs, which NICs on which segment,
   addresses). `tools/lab up single` = one vrx + host-lan + host-wan; `tools/lab up tri` = full set.
6. `tools/binapi-gen.sh`: copies `/usr/share/vpp/api/**/*.api.json` out of `vrx-a` and runs
   `binapi-generator` for the plugins we use (interface, ip, l2, ip_neighbor, vpe, bond, vxlan,
   gre, ipip, gtpu, l2tp, pppoe, sr, lisp, acl, nat44_ed, nat44_ei, nat64, nat66, det44, map,
   cnat, ipsec, ikev2, wireguard, lcp, abf, urpf, policer, qos, lb, span, lldp, bfd, vrrp,
   igmp, mpls, dhcp, dns, flowprobe, sflow, prom, pcap/tracenode) into `apps/agent/binapi/`.
   Commit the output. **Only legal source of VPP API names in the repo.**
7. Smoke test `test/integration/smoke/`: connects with govpp to `vrx-a` (socket forwarded
   over ssh or agent gRPC), `show_version` == 26.06, both DPDK interfaces present and in
   `dpdk` driver (`vppctl show hardware`), set IPs, ping from host-lan through VPP to host-wan
   (host-wan default route via vrx-a), rx counters increased.
8. CI: `pr.yml` uses `tools/lab up single` on a KVM-capable runner (nested virt or a physical
   CI box); `nightly.yml` uses `tri`. Document runner requirements.

## Acceptance
- [ ] `tools/lab up single` from a clean host in < 6 min (image cached), `status` shows VPP active
- [ ] `tools/lab vppctl vrx-a show hardware` lists virtio NICs under DPDK, not af_packet
- [ ] `tools/lab kill-vpp vrx-a` → VPP back in < 20 s (systemd), agent (later) reconciles
- [ ] `apps/agent/binapi/` committed; rerunning the generator yields an empty `git diff`

## Out of scope
Docker/compose of any kind. Physical NIC drivers. Performance. Product `.deb`s (P10) — the
lab installs from the FD.io/FRR repos plus our packages when present.
