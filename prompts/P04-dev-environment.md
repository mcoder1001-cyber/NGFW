# Task P04 — Lab tooling for the shared host + veth/netns packet rig + binapi   (prepend 00-CONTEXT.md)

## Goal
Give every worker and the manager a safe, scripted way to use the **VPP 26.06 that already runs on this host**
(`docs/lab/host-vrx-a.md`), to pass real packets through it without data NICs, and to generate the GoVPP bindings.
No Docker, no libvirt, no nested KVM. Other VMware VMs will be added by the product owner later; design for them,
but everything here must work on `vrx-a` = this host today.

## Hard rules for this task
- `docs/lab/host-vrx-a.md` says `handover: pending`: **`tools/lab` never edits `/etc/vpp/*`, never installs/removes packages,
  never restarts or kills VPP, never runs `scripts/00,10,20,30,50-*.sh` on this host.** `provision vrx-a` only *verifies*.
- Everything you create on VPP or in Linux carries the caller's `VRX_TEST_PREFIX` (see `docs/lab/shared-host-rules.md`).

## Build exactly this
1. `tools/lab` (bash; Go later if it grows): inventory `test/topology/*.yml` (name, mgmt ip or `local`, role, NIC→segment map,
   PCI ids, addresses). Local mode: `vrx-a` = `local` — commands run directly, no SSH. Remote mode (future VMs) via SSH.
   Subcommands: `status` (VPP active? `show_version`, plugins loaded, sockets, handover flag, rig state), `vppctl <vm> <cmd>`,
   `provision <vm>` (local: verify-only + print the manual steps; remote: full provisioning for Ubuntu 26.04 from our source-built
   debs in `/root/vpp/build-root/*.deb` — never the FD.io repo, D-001), `restart-vpp <vm>` / `kill-vpp <vm>` (**refuse with a
   clear message while handover is pending; afterwards exclusive `flock /run/lock/vrx-vpp.lock`**), `lock shared|exclusive <cmd>`
   (wrapper around `/run/lock/vrx-lab.lock` for test harnesses), `env <slot>` (prints the slot exports from shared-host-rules.md).
2. **Packet rig** `tools/lab rig up|down|gc <prefix>` (prefix = `VRX_TEST_PREFIX`, ≤ 6 chars): namespaces `ns-<prefix>-lan` / `ns-<prefix>-wan`,
   veth pairs `<prefix>l0↔<prefix>l1`, `<prefix>w0↔<prefix>w1` (peers moved into the namespaces), VPP `create host-interface name <prefix>l0`
   and `<prefix>w0` (af_packet plugin is loaded), addresses from `10.<slot>.1.0/24` and `10.<slot>.2.0/24`, default routes in the
   namespaces via VPP. `rig down` deletes exactly its objects (VPP host-interfaces via binapi/vppctl, veths, namespaces); `rig gc` removes
   leftovers by prefix. Output always includes `path: af_packet`.
3. `tools/binapi-gen.sh`: reads `/usr/share/vpp/api/{core,plugins}/*.api.json` **on this host** (`VPP_API_DIR` overridable) and runs
   `binapi-generator` for **all** plugins present into `apps/agent/binapi/` (generating everything avoids per-factory regeneration — D-014);
   idempotent; commits nothing itself. Document that `apps/agent/binapi/` is P04-owned, then manager-owned.
4. Local services for the control plane: install `postgresql` (Ubuntu 26.04 version; require ≥ 16) and `valkey` (or `redis-server` if valkey is
   unavailable — record which in `deploy/dev/README.md`) as **localhost-bound systemd services**; `deploy/dev/pg-test.sh create|drop <name>`
   for throwaway per-slot databases; `tools/lab status` reports both.
5. Smoke test `test/integration/smoke/` (Go, `VRX_INTEGRATION=1`, shared lock): govpp connects to `/run/vpp/api.sock`, `show_version` == 26.06,
   `rig up`, ping `ns-<p>-lan → ns-<p>-wan` through VPP, rx counters on both host-interfaces increased, `rig down` leaves no prefixed objects.
   Output records `path: af_packet`.
6. `docs/lab/vmware.md`: the VM shapes the product owner should create later (vrx-b/c: 4 vCPU, 6 GB, 4× vmxnet3 on `mgmt/lan/wan/p2p`;
   host-lan/wan, peer-frr, peer-sswan: 1 vCPU, 1 GB) and how `provision` will treat them (26.04 + our debs + DPDK `dev <pci>` on vmxnet3).

## Acceptance (paste the evidence)
- [ ] `tools/lab status` shows VPP active, version 26.06, plugin count, `handover: pending`
- [ ] `tools/lab rig up w9 && ping` through VPP succeeds; `vppctl show interface` lists `host-w9l0`/`host-w9w0` with rx counters; `rig down` clean
- [ ] `tools/lab kill-vpp vrx-a` refuses while handover is pending (exit ≠ 0, clear message)
- [ ] `apps/agent/binapi/` committed; rerunning the generator yields an empty `git diff`
- [ ] `deploy/dev/pg-test.sh create w9 && drop w9` works; `psql`/`valkey-cli ping` on localhost

## Out of scope
Docker, libvirt, nested KVM, DPDK on this host (no data NICs), physical NIC drivers, performance, product `.deb`s (P10),
any change to `/etc/vpp`, packages or the vpp unit.
