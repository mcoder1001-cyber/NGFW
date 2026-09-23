# Host `vrx-a` = dev/CI host 172.30.126.195 — verified facts

Verified read-only on 2026-09-23 12:16 +0330. Re-verify before relying on anything marked (volatile).

| Item | Fact |
|---|---|
| OS / kernel | Ubuntu 26.04.1 LTS, 6.19.0-6-generic, VMware VM (VMXNET3), 30 vCPU, 39 GB RAM, 197 GB root |
| VPP | **v26.06-release, built from source** in `/root/vpp` (tag v26.06, c3200b88d) by the VPP bring-up agent; packages from `/root/vpp/build-root/*.deb` |
| Installed packages | vpp, vpp-plugin-core, vpp-plugin-dpdk, vpp-drivers, vpp-crypto-engines, libvppinfra, python3-vpp-api (all 26.06-release). Also built but not installed: vpp-dev, libvppinfra-dev, vpp-dbg, vpp-plugin-devtools |
| Service | `vpp.service` active + enabled, `/usr/bin/vpp -c /etc/vpp/startup.conf` (volatile) |
| startup.conf | `unix { nodaemon, log /var/log/vpp/vpp.log, cli-listen /run/vpp/cli.sock, gid vpp }`, `api-trace on`, `api-segment { gid vpp }`, `socksvr { default }`, `cpu { }` (main core only, no workers), `dpdk { blacklist 0000:0b:00.0, no-pci }` |
| Sockets | `/run/vpp/api.sock`, `/run/vpp/cli.sock`, `/run/vpp/stats.sock` — owner root:vpp, mode 775 → **vrx-agent runs as root** (decided: P05 step 4; the `vrx` socket group is created by packaging, never by tests) |
| API definitions | `/usr/share/vpp/api/{core,plugins}` — 143 `*.api.json` files → `binapi-generator` runs **locally**, no copying from another VM |
| Hugepages | 1024 × 2 MB = 2 GB via `/etc/sysctl.d/80-vpp.conf` (no kernel-cmdline hugepages, no isolcpus) |
| Plugins | 94 on disk, 84 loaded. **Not loaded (disabled by default):** `linux_cp_plugin.so`, `linux_nl_plugin.so` (needed by P12/FRR), `npt66_plugin.so` (needed for NPTv6, D4.3), `ip6_dad_autoremove`, `idpf`, `fateshare`, unittest plugins. Enabling them = a `plugins { plugin X { enable } }` block in startup.conf → requires the handover below |
| Data-plane NICs | **Six vmxnet3 NICs added by the product owner (seen 2026-09-23 13:17):** `ens161` 0000:04:00.0, `ens193` 0000:0c:00.0, `ens224` 0000:13:00.0, `ens225` 0000:14:00.0, `ens256` 0000:1b:00.0, `ens257` 0000:1c:00.0 — all DOWN, kernel `vmxnet3` driver, **port-group mapping unknown** (ask the product owner). Management stays `ens192` 0000:0b:00.0 (blacklisted from DPDK). VPP lists all seven in `show pci`; none is bound to DPDK yet — that needs `dpdk { dev 0000:xx:00.0 }` in startup.conf → handover-gated (D-012). Inventory: `test/topology/vrx-a.yml` (P04) |
| Known log noise |  on every  — af_packet quirk of this VPP build, harmless (found by the P04 review) |
| libvirt | installed by mistake earlier (`virbr0` 192.168.122.1) — unused, harmless; may be purged |

## Consequences for the plan
1. **Packet tests:** until handover, use `tools/lab rig` — `create host-interface` (af_packet) on veth pairs with peers in network namespaces (real VPP forwarding, `path: af_packet`). After handover, the six data NICs are bound to DPDK via the startup.conf generator (`dpdk { dev … }`) and tests can also run on the DPDK path (`path: dpdk`); the port-group mapping (which NIC is lan/wan/dmz/p2p) must come from the product owner first.
2. **P12 (FRR/linux-cp) and NPTv6 need startup.conf changes** → they are gated on the handover flag below or on an explicit PENDING decision.
3. **Workers:** no `cpu { corelist-workers }` yet — fine for functional work; irrelevant for FAST MODE (no performance work).

## Handover
`handover: pending` — owner of `/root/vpp`, `/etc/vpp/startup.conf`, packages and `vpp.service` is the VPP bring-up agent.
Until `handover: done`: everyone may **use** VPP (API, vppctl, create *prefixed* interfaces/tables/routes per `shared-host-rules.md`); **nobody restarts or kills the VPP process** and nobody changes startup.conf, packages or the unit (D-012). Restart-safety is proven by the agent-restart simulation in FAST MODE DoD (3).
**Shared instance:** integration tests run under `flock -s /run/lock/vrx-lab.lock`; the manager's `tools/ci.sh full` and (after handover) any VPP restart take `flock -x`; nobody touches `local0` or objects without their prefix.
When the product owner flips this to `done`, the manager agent owns it; startup.conf changes then go through the generator (D0.6) or an explicit task, and are recorded in `docs/decisions/LOG.md`.
