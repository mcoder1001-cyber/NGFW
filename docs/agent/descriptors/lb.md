# lb descriptors (DF-7, WBS D7.9, tier T3 — minimal)

Package `apps/agent/internal/descriptors/lb` — VPP load-balancer plugin. Messages only from
`apps/agent/binapi/{lb,lb_types}`. DF-7 conventions: see `policer.md`.

| Object type | Key | Create / Update / Delete | Retrieve | Notes |
|---|---|---|---|---|
| `lb.conf` (**global**) | `lb.conf/global` | `lb_conf` (Update in place) / Delete restores VPP's start-up values (src 255.255.255.255 / ffff:…, 1024 buckets, 40 s) | **write-only** (no getter) | registered only by `lb.RegisterGlobals` for the globals owner (D-071); 0 = keep current buckets/timeout |
| `lb.vip` | `lb.vip/<prefix>/<any\|tcp\|udp>/<port>` | `lb_add_del_vip_v2` add; Update = ErrRecreate; `is_del` | **write-only** | encap gre4/gre6/l3dsr/nat4/nat6, dscp (l3dsr), srv type + target/node port (nat), new-flows table length (power of 2), src-ip sticky. Create: `VALUE_EXIST` is success only for a VIP this owner recorded (D-080 boot record, written after its own successful add); an unrecorded existing VIP is `dfkit.ErrNotOurs`, never adopted. Delete only of a recorded VIP; `NO_SUCH_ENTRY` = success (review M4) |
| `lb.as` | `lb.as/<vip prefix>/<proto>/<port>/<as addr>` | `lb_add_del_as` add / `is_del` (+`is_flush` when `flush_on_delete`) | **write-only** | same record rule as the VIP: `VALUE_EXIST` without our record = `ErrNotOurs`; Delete of an absent AS/VIP = success |
| `lb.intf-nat` | `lb.intf-nat/<if>/<ip4\|ip6>` | `lb_add_del_intf_nat4` / `_nat6` | **write-only** (feature, no dump) | the enable stacks the in2out feature → applied once per D-080 boot identity (kernel boot_id, VPP PID, start time) and sw_if_index/name (D-076, review H1); disable only with a matching record |

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

## Runtime enum check (review M2)

The byte swap is not trusted blindly: after every add with a non-zero encap, `lb_vip_dump` (which reports the VIP
type correctly) must show the requested encapsulation for that prefix/port. If VPP rejected the type
(`INVALID_ADDRESS_FAMILY`) or created another one, the wrong VIP is removed, the byte order is switched
(`enumNative`, i.e. a VPP with the V20 `ntohl` fix) and the add retried once; if neither order works the Create
fails with `lb.ErrEnumOrder` (loud, nothing left behind). Unit-tested against a fake that does and one that does not
convert (`TestVIPEnumOrder`).

## Leak per update / delete (review M3)

Every VIP delete — and so every VIP change, which is ErrRecreate = delete + add — leaves one "removed" VIP plus the
recursive-resolution `/32` of each of its ASes in VPP until a global GC (`lb vip|as|conf` CLI) or a VPP restart.
The API cannot collect them. Product answer: the globals owner runs the GC (DF-7-questions Q1). The host test is
opt-in (`VRX_DF7_LB=1`) so CI runs stop adding leftovers.
