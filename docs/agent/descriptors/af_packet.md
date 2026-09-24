# Descriptors — af_packet plugin (DF-1)

Package `apps/agent/internal/descriptors/af_packet` (Go package `afpacket`), model `afpacket_model.proto`
(agent-internal stand-in, D-055). `afpacket.Register(r, client, owner, opts...)` (options: `WithLinks`, `WithSettle` — tests only).

| Object type (descriptor) | Key | Depends on | VPP messages | Update | Notes / limitations |
|---|---|---|---|---|---|
| `af-packet.host-interface` | `af-packet.host-interface/<name>` | nothing in VPP (the Linux netdev is a precondition) | `af_packet_create_v3`, `af_packet_delete` (by host_if_name), `sw_interface_tag_add_del`; Retrieve `af_packet_dump` (+ `sw_interface_dump` for tag and mode) | ErrRecreate | VPP names it `host-<host_if_name>`. mode ethernet / ip (decoded from the presence of an L2 address: ip-mode interfaces use the ip hw class). **Not modelled**: flags (qdisc-bypass, cksum-gso, version-2), frame sizes/counts, queue counts — `af_packet_details` reports only sw_if_index and host_if_name, so they could never round-trip; VPP defaults apply, except the ring sizes (fixed, below). af-packet queues start in **interrupt** mode (see `interface.rx-mode`). If tagging fails the host-interface is deleted again (review M3). Open (review L3): any Linux netdev can be attached, incl. the management NIC — a deny-list belongs to schema / F-* validation (P08's `vethOnly` wraps Create); the delete-side quiesce already refuses to take down a netdev that is not a veth (below). |

## Delete ordering (D-101/V24)

`af_packet_delete_if` in VPP 26.06 closes the socket fds before `clib_file_del` (`plugins/af_packet/af_packet.c:895-900`,
`docs/vpp-code-track.md` V24): the epoll DEL fails with EBADF and the same fd number is closed a second time. Deleting while the
Linux netdev is up is the suspected cause of the shared-VPP SIGSEGV at 2026-09-24 07:27:32 (D-101). The agent therefore
**quiesces** the host netdev before every `af_packet_delete` it sends:

- **Delete**: quiesce → `ifsanitize.BeforeDelete` → `af_packet_delete` (no frame crosses the interface while its bindings are
  cleared). **Create's rollback** (an untagged or unclean interface, `iface.AcquireAndTag` / `ifsanitize.Acquire`): quiesce →
  `af_packet_delete`. `Update` is ErrRecreate, so it goes through Delete.
- **Quiesce** (`quiesce.go`, netlink in `quiesce_linux.go`, `golang.org/x/sys/unix`; no `ip` exec — `host_if_name` is config
  input): `RTM_GETLINK` by name → ifindex, `IFF_UP` and the link kind (`IFLA_LINKINFO`/`IFLA_INFO_KIND`); if up, `RTM_NEWLINK`
  (`ifi_change = IFF_UP`, `ifi_flags = 0`, acked); read again → must be down; then a **200 ms settle** (the same as `tools/lab
  rig_side_down`; chosen, not tuned). A netdev that is gone (ENODEV) or already down needs nothing: the delete goes ahead. Any
  other failure (EPERM, a 2 s netlink timeout, still up) is `afpacket.ErrQuiesce` naming the netdev and D-101/V24, and **no
  `af_packet_delete` is sent** (fail closed). In the rollback that leaves an **untagged** `host-<dev>` in VPP; the Create error
  names that orphan (`untagged orphan host-<dev> (sw_if_index N) left in VPP`). Retrieve does not see it (untagged), so an
  operator removes it after bringing the netdev down.
- **veth only (D-105, TD-5 review L3):** the quiesce brings down only a netdev of kind `veth`. An **up** netdev of any other
  kind — a physical NIC such as the management NIC `ens192` (no link kind), a bond, a vlan — is refused with `ErrQuiesce`
  wrapping `afpacket.ErrNotVeth` (the error names D-105): nothing is sent to VPP and the netdev stays up. af_packet is lab-only
  on veths; an af_packet interface on another netdev is removed by an operator. A netdev that is already down is not brought
  down, so its delete goes ahead whatever its kind.
- **A failed delete (review L1):** when the quiesce took the netdev down and a later step fails with the interface provably
  still in VPP — `ifsanitize.BeforeDelete` failed, or the quiesce failed after its link-down (confirm, settle) — the netdev is
  brought **up** again (`RTM_NEWLINK ifi_flags = IFF_UP`, best effort, WARN logged): the scheduler keeps the object in running
  config, and its data path must not stay dead with nothing saying why. A failed `af_packet_delete` (its outcome in VPP is
  unknown) leaves the netdev down with a WARN (`netdev left down`).
- **Create's rollback context (review L2):** the rollback runs on `context.WithoutCancel(ctx)` bounded by
  `afpacket.RollbackTimeout` (30 s), so a Create that failed because its own context expired (an I6 API stall) still removes the
  untagged interface instead of leaving an orphan and a netdev down.
- VPP brings the netdev back up itself at the next create (`af_packet_create_if` sets `IFF_UP`, `af_packet.c:683-692`); apart
  from the failed-delete case above nothing else restores it. VPP logs one `af_packet_fd_error: Network is down` per queue when
  the link goes down (the handler only reads and logs the socket error, `af_packet.c:165-183`), during the settle, while the
  file is still registered.
- **This lowers the V24 risk; it does not remove the double close.** VPP still logs `vlib_file_update: epoll_ctl() failed …
  errno 9` on every delete with the netdev down (P08 review I1, D-107; TD-5's host run too), and fd numbers are reused across
  interfaces. Only the VPP fix (V24: move `close()` after the rx-queue free) removes it.
- The product agent needs **`CAP_NET_ADMIN`** for the link-down (the vrx-agent systemd unit, P10); without it every af_packet
  Delete fails closed with EPERM. af_packet is lab-only (the product data path is DPDK).
- The agent must run in **VPP's network namespace** (review N3): VPP resolves `host_if_name` in its own netns
  (`af_packet.c:663`), the quiesce in the agent's. In another netns the quiesce would miss the device (ENODEV → "gone, go
  ahead") or act on a same-named one. The lab agent and the product unit both run in the host netns, like VPP.
- `TestEveryAfPacketDeleteIsQuiesced` (`af_packet/guard_test.go`) scans every non-test Go file under `apps/agent/` (test-helper
  packages included, generated `binapi/` excluded) and fails on any use of `AfPacketDelete` outside
  `(*HostInterfaceDescriptor).quiescedDelete`, an `AfPacketDelete` that no **checked** `quiesce` precedes (`if err :=
  d.quiesce(…); err != nil { return … }`, or `…, err :=` followed by that if — `_ = d.quiesce(…)` does not count, review N2), or a
  string literal matching `(?i)\bdel\w*\s+host-int` (`delete host-interface` and the prefixes VPP's CLI accepts, review N1).

**Rings (D-108, amended by D-113)**: every create asks for `tx_frame_size = 67584` (2048 × 33, VPP's default),
`tx_frames_per_block = 16` (1 block of 1,081,344 B = 264 pages, ≈ 1 MiB) and `rx_frame_size = 2048`, `rx_frames_per_block = 8`
(160 blocks, ≈ 2.5 MiB): ≈ 3.5 MiB instead of VPP's defaults of ≈ 76 MiB (TX 66 KiB frames × 1024, RX 2048 × 32 × 160).
Allocating those stalled VPP's only thread, and so every API client, for 40 s to 5.5 min on the lab VM (I6, ESXi memory
reclaim). The rings shrink by frame **count** only. The TX frame **size must not shrink** (TD-5 review H1): VPP's TX path
copies every buffer of a packet into its frame slot with **no length check** against the frame size
(`af_packet/device.c:561-573`, the v2 path the same), behind a 48-byte header, and GSO/jumbo frames do reach it (the lab veths
have TSO/GSO on, the ethernet-mode interface has an L3 MTU of 9000, L2 paths never check the MTU). With 2048-byte slots such a
frame overruns into the next slot, and past the last one out of the ring's mmap — a crash of the shared VPP. The values match
the `tools/lab` rig (`tx-size 67584 tx-per-block 16 rx-size 2048 rx-per-block 8`).

Tests create the veth pair `w<N>-afXX` / `w<N>-afXXp` with a fixed-argv `ip link` rig helper
(test-only, marked `ALLOW:`), set `disable_ipv6=1` on both ends before `up` (no packets), delete it in Cleanup after the
quiesced descriptor Delete — but keep it when a descriptor Delete failed, since an af_packet interface may still be attached
(review N4; `tools/lab rig gc w<slot>` removes it) — and never assign addresses. The host test deletes through the descriptor
and asserts the netdev reads down afterwards.
