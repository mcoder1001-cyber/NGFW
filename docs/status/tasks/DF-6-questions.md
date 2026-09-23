# DF-6 — questions / incidents for the manager

## Q1 (INCIDENT, V8 candidate) — gtpu_add_del_tunnel_v2 segfaults VPP 26.06 on any failed add/del
- `plugins/gtpu/gtpu_api.c` `vl_api_gtpu_add_del_tunnel_v2_t_handler` calls `get_combined_counters (sw_if_index, …)`
  right after `vnet_gtpu_add_mod_del_tunnel`, also when it failed and `sw_if_index` is still `~0` (duplicate add
  → TUNNEL_EXIST, delete of a missing tunnel → NO_SUCH_ENTRY). Indexing the counter vector with `~0` segfaults VPP.
- **It happened on the shared host**: the salvaged gtpu integration test deleted its tunnels in the body and again in
  `t.Cleanup`; the second delete crashed VPP on 2026-09-24 at ~00:19:53 and 00:25:16 (+0330), systemd restarted it
  (journal: `received signal SIGSEGV … faulting address 0x7b89d050ea70`, handler frame in the gtpu plugin). Other
  workers' VPP state was lost at those times. Apologies — the 00:19:48 crash (stack `vlib_log → unformat_vnet_sw_interface`)
  is **not** from DF-6 as far as I can tell (none of the DF-6 plugins log from API handlers) — please check other slots.
- Fix in DF-6 (configuration-only fallback): `gtpu.tunnel` pre-checks with `gtpu_tunnel_v2_dump` and never sends an add
  whose (dst, teid) key exists (typed `gtpu.ErrTunnelExists`) nor a delete of a missing tunnel (treated as done);
  validation also rejects src == dst and decap_next > 3 before sending. The fake models "would crash" and a unit test
  (`TestV8Guard`) asserts zero such requests. Residual risk: a race with another client creating the same key between
  dump and add (none on this host).
- Ask: record as **V8** in `docs/vpp-code-track.md` (I do not own that file): upstream patch = only read counters when
  `rv == 0`.

## Q2 — l2tpv3 tunnels have no delete message
binapi `l2tp` has `l2tpv3_create_tunnel`, `l2tpv3_set_tunnel_cookies`, `l2tpv3_interface_enable_disable`,
`l2tpv3_set_lookup_key`, `sw_if_l2tpv3_tunnel_dump` — no delete (VPP source confirms). `l2tp.tunnel` Delete returns
`df6.ErrNoDelete`; the host integration test creates a tunnel only with `VRX_DF6_L2TP_CREATE=1` (it would outlive the
test). The lookup key is a write-only global (no getter): Retrieve → `ErrRetrieveUnsupported`, Delete = no-op.

## Q3 — pppoe.session cannot be created on the host without PPPoE discovery traffic
VPP creates a session only for a client MAC that `pppoe-input` has learned (link table) from PADI/PADR packets.
Without traffic `pppoe_add_del_session` returns INVALID_SW_IF_INDEX, mapped to typed `pppoe.ErrClientNotLearned`.
The host test verifies that path and skips the create/retrieve part; full verification needs the packet rig (not DF-6).

## Q4 — write-only object types (no dump in VPP)
vxlan.bypass, vxlan-gpe.bypass, gtpu.bypass, l2tp.interface-enable, pppoe.cp, l2tp.lookup-key: Retrieve returns
`df6.ErrRetrieveUnsupported`. The scheduler needs a policy for such descriptors (trust the last applied state?).
