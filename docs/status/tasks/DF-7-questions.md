# DF-7 — questions for the manager (worker keeps going; nothing here blocks the branch)

Each item: what I found, what I did, options. VPP findings are candidates for `docs/vpp-code-track.md` (not my
file — please copy the ones you accept).

## Q1 — lb plugin API bugs (VPP 26.06 `src/plugins/lb/api.c`) → all lb objects write-only

- `lb_vip_details`: `vip.protocol = htonl(protocol)` into a u8 and `flow_table_length = htonl(mask+1)` into a u16 are
  always 0 on x86 (host: `RawProtocol:0` for tcp/80 and udp/8080 VIPs, `RawFlowTableLength:0`); `encap` carries the
  VIP type; `target_port` arrives byte-swapped (80 → 20480); `src_ip_sticky` not reported.
- `lb_add_del_vip(_v2)` compares the u32 enums `encap` and `type` without `ntohl` → only gre4/clusterip (value 0)
  work from any correct client; l3dsr failed with `INVALID_ADDRESS_FAMILY`. **Workaround in the descriptor**: both
  enums are byte-swapped before sending (`lb.rawEnum`, documented, little-endian VPP only).
- Deleted VIPs/ASes are only marked "removed"; the API runs garbage collection only for a VIP still in use (≥ 60 s
  apart), so removed VIPs and the ASes' recursive-resolution FIB entries stay until the `lb vip|as|conf` CLI runs a
  global GC or VPP restarts. **Leftovers of my slot (corrected, review M3)**: before the 02:23 VPP restart the
  reviewer counted 26 "removed" VIPs of slot 10 (`show lb vips`: `#vips: 27 #ass: 24`) and table 0 `10.10.31.1/32`
  with `refs:8` (plus `10.10.31.2/32`, `10.10.31.3/32`, RR-sourced drop entries of AS addresses — not API routes,
  not removable via the API). Every host run added three removed VIPs; each VIP delete or change (ErrRecreate)
  leaks one. The VPP crash-restart of 2026-09-24 02:23 (D-087, Q9) wiped them: after it `show lb vips` is empty and
  `show ip fib` has no `10.10.*` entry. **Now**: the lb host test is opt-in (`VRX_DF7_LB=1`) so CI adds no new
  leftovers; the per-update leak is documented in `lb.md`. Table 0 is never deleted, so V15 does not apply.
  Options: (a) the globals owner runs `vppctl lb conf` once after lb churn (global GC, keeps the conf values) — I do
  not, it is a global CLI action (D-071); (b) leave until the next VPP restart; (c) VPP patch (V-item: ntohl in the
  two handlers + details encoding + GC on delete). Recommendation (a) as the product answer, (c) as a V-item.

## Q2 — classify table key

The DF-7 prompt names DF-2's key `classify-table/…`; task/DF-2 builds `classify.table/<name>`
(`classify.TableKey`). I depend on `classify.table/<name>` (`df7.ClassifyTableKey`) and resolve names through an
injected function (`df7.WithClassifyTables`, P05 wires DF-2's classify Store). Please confirm or tell me the final
key; it is one constant.

## Q3 — stale per-sw_if_index "ip classify table" = source of P05's stray `10.10.82.1/32` (V15 follow-up)

`ip4_add_interface_routes` adds a classify-sourced `/32` for an interface address when
`lookup_main.classify_table_index_by_sw_if_index[sw_if_index]` is set — and VPP never resets that slot when the
interface is deleted. A new interface that reuses the index (another worker's loopback) inherits the binding: every
address added to it gets a classify `/32` that survives the address removal. That is how `10.10.82.1/32` (my igmp
loopback, table 10080) and `10.10.70.1/32` (vrrp loopback, table 0) appeared; with V15 the first then leaked into
other owners' tables. Fixes: my host-test helpers now clear the ip classify binding (repeatedly — earlier runs had
stacked references) before removing an address, unbind from the table, and assert with `ip_route_dump` that
nothing of `10.10.0.0/16` is left before the table is deleted; `10.10.70.1/32` was cleaned that way, table 10080 is
verified empty (`ip_route_dump table 10080: nothing of 10.10.0.0/16 left`). Options for the product: (a) DF-2's
`classify.interface-ip-table` Delete and DF-1's interface delete clear the binding; (b) VPP patch (reset the slot in
the sw-interface delete callback). Recommend both.

## Q4 — interface-ip dependencies for BFD / VRRP not declared

The prompt asks for an optional `interface-ip` dependency (P05 key `interface-ip/<if>/<addr>/<len>`). A BFD session
has only the local address (no prefix length) and VPP 26.06 does not require it on the interface; a VR's addresses
are virtual. Declaring the key would need the prefix length in the Value, which Retrieve could only get from
`ip_address_dump`. Options: (a) as built — no such dependency (ordering by registration order); (b) add an optional
`local_prefix_len` to the BFD session, filled by Retrieve from `ip_address_dump`. I chose (a); (b) is small if P08
wants it.

## Q5 — MPLS table 0 is VPP-global

VPP creates no MPLS table. `sw_interface_set_mpls_enable` needs table 0 (`NO_SUCH_FIB` otherwise — host run) and
locks it; `mpls_ip_bind_unbind` creates/locks it. The host has no table 0, so `mpls-interface` / `mpls-ip-bind` are
only exercised on the host with `VRX_DF7_GLOBALS=1` (unit tests cover them). Proposal: the globals owner's desired
state declares `mpls-table/0` when any MPLS feature is on; `mpls-interface` depends on it optionally.

## Q6 — other VPP 26.06 findings (V-item candidates)

| Plugin | Finding | DF-7 handling |
|---|---|---|
| lldp | `sw_interface_set_lldp` passes sw_if_index to `lldp_cfg_intf_set` as a **hw_if_index** (disable path too) | Create verifies with `lldp_dump`, `ErrIndexMismatch`; host test searches an aligned loopback (test-only `cli_inband show hardware-interfaces`) or skips |
| policer | `policer_classify_dump`: ~0 returns nothing; one index = out-of-bounds read (`vec_len` on an element pointer) | never called; `policer.classify` write-only |
| policer | `policer_input(apply=0)` writes the per-interface slot without `vec_validate` (OOB on an interface never applied) | un-apply only when applied in this VPP lifetime (boot identity) |
| policer | `policer_input_v2`/`_output_v2` reply with the v1 reply id | v1 by-name messages used |
| igmp | `igmp_group_prefix_dump` sends `igmp_details` message ids | write-only (probe logged `unexpected message: *igmp.IgmpDetails` earlier; today the list was empty) |
| vrrp | `vrrp_vr_peer_dump` for all VRs sends `vrrp_vr_details`; `vrrp_vr_add_del` validates priority/interval before `is_add` | per-VR peer dump; delete sends the values |
| mpls | per-interface enable counter is a u8 decremented without a check (disable of a disabled interface wraps to 255 and breaks the next enable) | disable only while `mpls_interface_dump` lists the interface |
| qos | record/store enables are reference counted | Delete disables until VPP answers VALUE_EXIST |

No VPP crash in any DF-7 run: `systemctl show vpp -p NRestarts` = 2 before the first run of every plugin and after
the last run (D-064).

## Q7 — for P05 / P08 wiring

- Install persisted stores: `iface.SetClaimStore(owner, …)` (claims of objects on untagged NICs, DF-1's store) and
  `df7.SetBootStore(owner, dfkit.NewFileBootStore(…))` (D-080 boot records: applied-once records of
  `policer.interface` and `lb.intf-nat` — value `<sw_if_index>/<name>` — and ownership records of lb VIPs/ASes and
  of MPLS label routes in table 0). With the default in-memory stores an agent restart without a VPP restart would
  re-add the two feature enables once and would no longer recognise its own VIPs/ASes/table-0 labels (Create →
  `ErrNotOurs`, Delete → no-op), i.e. they need the persisted store.
- `registry.Register(r, client, registry.Config{Owner, GlobalsOwner, BFDSecrets, Options})` registers all 28 (32
  with globals) DF-7 descriptors; `BFDSecrets` is the encrypted-store lookup by conf-key id.
- StreamEvents: `bfd.WatchEvents`, `vrrp.WatchEvents`, `igmp.WatchEvents` return channels of typed events keyed by
  the object key.
- Values are `*structpb.Struct` of the typed specs (D-055); P03b swaps them for leaf messages.

## Q8 — LLDP sw/hw index mismatch cannot be undone through the API (review M6)

`lldp_api.c` passes the API's sw_if_index to `lldp_cfg_intf_set(hw_if_index=…)`. Enable keys the new entry by hw
index X; `lldp_dump` reports it as `hw(X)->sw_if_index` = Y; disable looks the entry up by `hw(arg)->sw_if_index`
(`lldp_cli.c`, the `else` branch). A disable with X therefore removes the entry keyed Y (another interface's LLDP,
if any) and never the stray one; the argument that reaches key X is the hw index of our interface, which no API
message exposes (`sw_interface_details` has none). **Done**: Create detects the mismatch (dump diff), sends no
disable, claims nothing and fails loudly (`ErrIndexMismatch … NOT undone`); unit test asserts no disable is sent.
The stray entry stays until a VPP restart. Options: (a) accept + V-item (`lldp_cfg_intf_set` should map sw→hw with
`vnet_get_sup_hw_interface`) — recommended; (b) allow a test-only / operator CLI with the right hw name; (c) disable
`lldp.interface` on hosts where hw and sw indexes diverge (only NICs created at start-up are safe).

## Q9 — VPP crash 2026-09-24 02:23:03 during my parallel host run (D-064, D-087)

What happened: I ran the DF-7 host tests with one `go test` over nine packages, which runs packages in parallel;
`TestVRRPOnHost` (VRs 10/11 on loop1070, sw_if_index 5) and `TestIGMPOnHost` (IGMP host/router mode on
loop1080-1082, a host-mode listen on loop1082) ran at the same time. One second later VPP died (NRestarts 3 → 4);
every slot's objects were wiped. Journal (`journalctl -u vpp --since 02:22:55 --until 02:23:05`):

```
vrrp_vr_transition:386: VR [1] sw_if_index 5 VR ID 11 IPv4 transitioning to Backup
vrrp_vr_transition_vmac:226: Deleting virtual MAC address 00:00:5e:00:01:0b on hardware interface 5
vrrp_vr_start_stop:1025: 2 VRs configured, 2 VRs running
received signal SIGSEGV, PC 0x792cf7a0c3f8, faulting address 0x0
Code:  89 08 89 d3 48 39 dd 75 2a 49 83 c6 04 49 ff cd 49 8d 58 ff
#0  0x0000792cf7a0c3f8 ip4_options_node_fn + 0x158   (libvnet.so.26.06)
#1  0x0000792cf767d68e                               (libvlib.so.26.06)
#2  0x0000792cf767bb1e vlib_main + 0x1f3e
systemd: vpp.service: Main process exited, code=killed, status=6/ABRT; restart counter is at 4
```

Trigger (analysis, read-only `/root/vpp`; not reproduced — reproducing needs a crash): both tests make VPP send IPv4
packets with the Router Alert option on loopbacks. VRRP: a VR entering Backup calls `vrrp_vr_multicast_group_join`
→ `vrrp_igmp_pkt_build` (IGMPv3 report for 224.0.0.18, header length 6, option 0x94 04 00 00, sent via
`ip4-rewrite-mcast` on the VR interface, `sw_if_index[VLIB_RX] = 0`). IGMP: host-mode listens and router-mode
queries also carry Router Alert. A loopback echoes its TX into its own `ethernet-input`, so these packets re-enter
`ip4-input` → `ip4-options` (header length > 5). In `ip4_options_node_fn` a Router-Alert IGMP packet takes
`ip_lookup_set_buffer_fib_index(ip4_main.fib_index_by_sw_if_index, b)` and moves from the punt to the local
next; the faulting instruction is a 32-bit store (`mov %ecx,(%rax)`) through a NULL pointer, which fits the node's
enqueue/next-frame path or that write, not a read of the packet. The same pair of tests ran in parallel in phase 1
without a crash, so it is timing- or state-dependent (e.g. both a VRRP join and an IGMP packet in one frame). The
exact line needs the debug build / core (no core on the host).

Done: (1) host tests only one package at a time (`go test -p 1`, one package per run, NRestarts before/after each;
evidence in DF-7.md); (2) `TestVRRPOnHost` and `TestIGMPOnHost` are opt-in (`VRX_DF7_VRRP_HOST=1`,
`VRX_DF7_IGMP_HOST=1`, `df7test.CrashOptIn`) — run them alone in a manager window; unit tests stay on the fake.
For `docs/vpp-code-track.md`: V-item candidate "ip4-options SIGSEGV on looped-back Router-Alert IGMP (VRRP join /
IGMP host) on loopbacks, VPP 26.06".
