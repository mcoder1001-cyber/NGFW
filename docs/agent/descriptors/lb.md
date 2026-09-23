# lb descriptors (DF-7, WBS D7.9, tier T3 — minimal)

Package `apps/agent/internal/descriptors/lb` — VPP load-balancer plugin. Messages only from
`apps/agent/binapi/{lb,lb_types}`. DF-7 conventions: see `policer.md`.

| Object type | Key | Create / Update / Delete | Retrieve | Notes |
|---|---|---|---|---|
| `lb.conf` (**global**) | `lb.conf/global` | `lb_conf` (Update in place) / Delete restores VPP's start-up values (src 255.255.255.255 / ffff:…, 1024 buckets, 40 s) | **write-only** (no getter) | registered only by `lb.RegisterGlobals` for the globals owner (D-071); 0 = keep current buckets/timeout |
| `lb.vip` | `lb.vip/<prefix>/<any\|tcp\|udp>/<port>` | `lb_add_del_vip_v2` add; Update = ErrRecreate; `is_del` | **write-only** | encap gre4/gre6/l3dsr/nat4/nat6, dscp (l3dsr), srv type + target/node port (nat), new-flows table length (power of 2), src-ip sticky. Create is idempotent: `VALUE_EXIST` is success when `lb_vip_dump` lists the same prefix/port/encap |
| `lb.as` | `lb.as/<vip prefix>/<proto>/<port>/<as addr>` | `lb_add_del_as` add / `is_del` (+`is_flush` when `flush_on_delete`) | **write-only** | `VALUE_EXIST` = already there |
| `lb.intf-nat` | `lb.intf-nat/<if>/<ip4\|ip6>` | `lb_add_del_intf_nat4` / `_nat6` | **write-only** (feature, no dump) | the enable stacks the in2out feature → applied once per VPP boot identity (D-076); disable only if applied in this VPP lifetime |

Helpers: `lb.DumpVIPs` (read-only state: what `lb_vip_dump` reports correctly), `lb.FlushVIP` (`lb_flush_vip`; only
for an existing VIP — VPP flushes an uninitialised index when the lookup fails).

Dependencies: vip → `lb.conf/global` (optional); as → its `lb.vip`; intf-nat → `interface/<if>`.

## Why write-only (VPP 26.06 api.c, evidence in DF-7.md)

- `lb_vip_details.vip.protocol = htonl(protocol)` into a u8 and `flow_table_length = htonl(mask+1)` into a u16:
  always 0 on a little-endian host (host run: `RawProtocol:0` for a tcp and a udp VIP, `RawFlowTableLength:0`);
  `src_ip_sticky` is not reported; `encap` carries the VIP *type*; `target_port` arrives byte-swapped (80 → 20480).
- deleted VIPs/ASes stay listed ("removed") until VPP's garbage collection, which the API only runs for a VIP that
  is still in use (per-VIP GC ≥ 60 s apart) — removed VIPs are only collected by the `lb vip|as|conf` CLI.
- `lb_add_del_vip(_v2)` compares the u32 enums `encap` and `type` without `ntohl`: every value but 0 is misread
  (l3dsr → `INVALID_ADDRESS_FAMILY`). The descriptor byte-swaps both (`rawEnum`) so the wire bytes equal what VPP
  expects on a little-endian host; IPv4 VIP prefixes are sent with ip46 lengths (/32 → /128).

## FIB entries

`lb.vip` installs the VIP prefix (lb DPO) in table 0 (removed on delete); `lb.as` makes VPP track each AS address
with a recursive-resolution entry in table 0 — VPP removes it only in its garbage collection, so after a delete the
`/32` (a drop while unresolved) stays until the next GC of that VIP (never, for a deleted VIP, via the API).
Table 0 is never deleted, so these do not hit V15; they are listed in DF-7.md.
