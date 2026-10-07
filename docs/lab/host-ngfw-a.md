# Host `ngfw-a` = dev/CI host 172.30.126.195 — verified facts

Verified read-only on 2026-10-03 17:02 +0330. Re-verify before relying on anything marked (volatile).

| Item | Fact |
|---|---|
| OS / kernel | Ubuntu 26.04.1 LTS, **7.0.0-31-generic**, VMware VM (VMXNET3), **32 vCPU, 62.7 GiB guest RAM, 1 NUMA node**, 197 GB root. Latest guest reading: 43.8 GiB available. VMware balloon state is not visible from the guest and was not re-verified. |
| VPP | **v26.06-release, built from source** in `/root/vpp` (tag v26.06, c3200b88d) by the VPP bring-up agent; packages from `/root/vpp/build-root/*.deb` |
| Installed packages | vpp, vpp-plugin-core, vpp-plugin-dpdk, vpp-drivers, vpp-crypto-engines, libvppinfra, python3-vpp-api (all 26.06-release). Also built but not installed: vpp-dev, libvppinfra-dev, vpp-dbg, vpp-plugin-devtools |
| Service | `vpp.service` active + enabled, `/usr/bin/vpp -c /etc/vpp/startup.conf` (volatile) |
| startup.conf | `unix { nodaemon, log /var/log/vpp/vpp.log, cli-listen /run/vpp/cli.sock, gid vpp }`, `api-trace on`, `api-segment { gid vpp }`, `socksvr { default }`, `cpu { }` (main core only, no workers), `dpdk { blacklist 0000:0b:00.0, no-pci }` |
| Sockets | `/run/vpp/api.sock`, `/run/vpp/cli.sock`, `/run/vpp/stats.sock` — owner root:vpp, mode 775 → **ngfw-agent runs as root** (decided: P05 step 4; the `ngfw` socket group is created by packaging, never by tests) |
| API definitions | `/usr/share/vpp/api/{core,plugins}` — 143 `*.api.json` files → `binapi-generator` runs **locally**, no copying from another VM |
| Hugepages | `/etc/sysctl.d/80-vpp.conf` requests **4096 × 2 MB**; verified live on 2026-10-03: `vm.nr_hugepages=4096`, `HugePages_Total=4096`, `HugePages_Free=4045` (~8 GiB reserved, ~7.9 GiB free), `vm.max_map_count=1048576`. The old report of 555/1024 pages is stale. |
| Netlink socket buffers | verified live 2026-10-03: `net.core.rmem_max` and `net.core.wmem_max` are both 268435456 (256 MiB), matching `/etc/sysctl.d/60-ngfw-netlink.conf`. |
| Plugins | 94 on disk. **Enabled 2026-09-24 (D-060, PENDING-handover option 2):** `linux_cp_plugin.so`, `linux_nl_plugin.so` (P12/FRR), `npt66_plugin.so` (NPTv6). **Still not loaded:** `ip6_dad_autoremove`, `idpf`, `fateshare`, unittest plugins. Enabling them = a `plugins { plugin X { enable } }` block in startup.conf → requires the handover below |
| Data-plane NICs | **Six vmxnet3 NICs added by the product owner (seen 2026-09-23 13:17):** `ens161` 0000:04:00.0, `ens193` 0000:0c:00.0, `ens224` 0000:13:00.0, `ens225` 0000:14:00.0, `ens256` 0000:1b:00.0, `ens257` 0000:1c:00.0 — all DOWN, kernel `vmxnet3` driver, **port-group mapping unknown** (ask the product owner). Management stays `ens192` 0000:0b:00.0 (blacklisted from DPDK). VPP lists all seven in `show pci`; none is bound to DPDK yet — that needs `dpdk { dev 0000:xx:00.0 }` in startup.conf → handover-gated (D-012). Inventory: `test/topology/ngfw-a.yml` (P04) |
| Known log noise → crash suspect | `vlib_file_update: epoll_ctl() failed ... host-<p>l0 queue 0 errno 9` on every `delete host-interface` — NOT harmless: fd closed before the epoll DEL, double close (V24, D-101). Bring the host veth down before deleting an af_packet interface |
| libvirt | installed by mistake earlier (`virbr0` 192.168.122.1) — unused, harmless; may be purged |
| Service / recent health | `vpp.service` was active at 2026-10-03 17:02 +0330, started 12:48:02, `NRestarts=0`, process RSS about 626 MiB. This is a point-in-time guest observation, not a test result. |
| Crash forensics | systemd-coredump installed 2026-09-24 12:40 (+ drop-in LimitCORE=infinity); before that cores went to tmpfs and were lost. `/var/log/vpp/` exists since 12:53 (VPP's own log never worked before). vpp-dbg built, not installed |
| Source provenance | `/root/vpp` = c3200b88 = v26.06 = head of upstream stable/2606 (0 commits after the tag, verified 2026-09-24); installed debs byte-identical to build-root |
| af_packet rings | VPP default TX ring = 66 KiB × 1024 = 66 MiB zeroed on the main thread; old stalls of 40 s–5.5 min were observed in September. Rig and agent use `tx-size 2048 tx-per-block 256` (D-108). Retest latency on the current host after the memory increase. |

## Consequences for the plan
1. **Packet tests:** until handover, use `tools/lab rig` — `create host-interface` (af_packet) on veth pairs with peers in network namespaces (real VPP forwarding, `path: af_packet`). After handover, the six data NICs are bound to DPDK via the startup.conf generator (`dpdk { dev … }`) and tests can also run on the DPDK path (`path: dpdk`); the port-group mapping (which NIC is lan/wan/dmz/p2p) must come from the product owner first.
2. **P12 (FRR/linux-cp) and NPTv6 need startup.conf changes** → they are gated on the handover flag below or on an explicit PENDING decision.
3. **Workers:** no `cpu { corelist-workers }` yet — fine for functional work; irrelevant for FAST MODE (no performance work).

## Per-slot test VPPs (LAB-vpp-per-slot, 2026-10-06)
No host change: `tools/lab vpp up <N>` starts extra VPP processes (`ngfw-vpp-w<N>`, `/run/ngfw-test/w<N>/vpp`, shm prefix `w<N>`) without
hugepages, without DPDK and without touching `/etc/vpp`, `vpp.service` or `/run/vpp`; see `shared-host-rules.md` §13. Verified 2026-10-06:
slot 20 up/down twice, `vpp.service` MainPID 1014 / NRestarts 0 before and after, HugePages_Free unchanged by the instance.

## Handover
`handover: pending` — owner of `/root/vpp`, `/etc/vpp/startup.conf`, packages and `vpp.service` is the VPP bring-up agent.
Until `handover: done`: everyone may **use** VPP (API, vppctl, create *prefixed* interfaces/tables/routes per `shared-host-rules.md`); **nobody restarts or kills the VPP process** and nobody changes startup.conf, packages or the unit (D-012). Restart-safety is proven by the agent-restart simulation in FAST MODE DoD (3).
**Shared instance:** integration tests run under `flock -s /run/lock/ngfw-lab.lock`; the manager's `tools/ci.sh full` and (after handover) any VPP restart take `flock -x`; nobody touches `local0` or objects without their prefix.
When the product owner flips this to `done`, the manager agent owns it; startup.conf changes then go through the generator (D0.6) or an explicit task, and are recorded in `docs/decisions/LOG.md`.
