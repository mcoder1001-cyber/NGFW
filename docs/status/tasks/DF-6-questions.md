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
- **Exact crashing request** (the salvaged test's `t.Cleanup` re-deleting a tunnel the test body had already deleted):
  `gtpu_add_del_tunnel_v2 {is_add: false, src_address: 10.11.4.1, dst_address: 10.11.4.2, mcast_sw_if_index: 4294967295,
  encap_vrf_id: 0, decap_next_index: 1 (L2), teid: 11300, tteid: 11401, pdu_extension: false, qfi: 0}` →
  `vnet_gtpu_add_mod_del_tunnel` returns NO_SUCH_ENTRY (key (dst, teid) not found), `sw_if_index` stays `~0`, then
  `get_combined_counters (~0, …)` indexes the interface counter vector with 0xffffffff × 16 bytes. Our encoding was
  valid; the bug is VPP's (no `rv == 0` check). A duplicate add (same dst + teid) takes the same path (TUNNEL_EXIST).
- Backtrace (`journalctl -u vpp --since 00:25`, identical for 00:19:53, 00:25:16 and 00:26:0x):
  ```
  received signal SIGSEGV, PC 0x722d577de514, faulting address 0x723d53fa92b0   # fault = base + 0x10_0000_0000 ≈ ~0 × 16
  #0  0x0000722d577de514            (gtpu_plugin.so, vl_api_gtpu_add_del_tunnel_v2_t_handler → get_combined_counters)
  #1  0x0000722d577e1862            (gtpu_plugin.so)
  #2  0x0000722d9b309965 vl_msg_api_socket_handler + 0x245
  #3  0x0000722d9b32386d vl_socket_process_api_msg + 0x1d
  ```
  The 00:26:0x crash is most likely govpp re-sending after reconnect from the same hung test process (it was bound to
  the old VPP; nothing of DF-6 was started at that time). NRestarts=2 since then; no crash after the guard (the one
  guarded host run at ~00:30 left VPP up, `ActiveEnterTimestamp` unchanged).
- Per the manager's rule the gtpu host test is now **opt-in** (`VRX_DF6_GTPU_HOST=1`, default skip); unit tests on
  the fake model the crash (`crashes` counter) and assert the descriptor never sends such a request.
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

## Q5 — SR-MPLS has no dump; mpls_route_dump cannot replace it (partial / write-only)
binapi `sr_mpls` has add/mod/del/steering/assign-endpoint-color and **no dump**. Deriving policies from
`mpls_route_dump` (task fallback) was tried on the host: the BSID entry's paths come back with `via_label = 0`
(`fib_path` encode never sets `frp_local_label` for recursive MPLS paths), i.e. the first segment of every list is
lost, and the second list added by `sr_mpls_policy_mod` was not visible as a separate path. So `sr-mpls.policy`,
`sr-mpls.steering` and `sr-mpls.endpoint-color` are write-only (Retrieve → `ErrRetrieveUnsupported`, never cached
state); Create/Delete use presence probes (BSID EOS entry in MPLS table 0; steering prefix in the IP table).
Endpoint-color has no un-assign message (cleared only with the policy). Ask: an `sr_mpls_policy_dump` upstream (VPP
code track) if SR-MPLS restart-safety matters; color-based automated steering is not modelled.

## Q6 — SRv6 plugin behaviours / srv6-mobile
`binapi/srv6_ad|am|as` are **not generated** (only `sr`, `sr_types`, `sr_mpls`, `sr_pt`, `sr_mobile`, `sr_mobile_types`),
so End.AD/AM/AS proxy SIDs cannot be built (manager: regenerate if wanted). `sr_mobile` is generated but not built
(decision, T3): `sr_mobile_policy_add` has no delete (a mobile policy is an SR policy deleted by `sr_policy_del`, and it
would appear in `sr_policies_v2_dump` where `sr.policy` claims it), and mobile local SIDs appear in
`sr_localsids_dump` only as an opaque plugin behaviour number (no prefix/behaviour name) — no faithful Retrieve.
uSID behaviours (`sr_localsid_add_del_v2` locator lengths) are not modelled either (the dump has no locator lengths).

## Q7 (INCIDENT, fixed) — deleting an IP table that still holds SR-sourced routes leaks them
During the first SR-MPLS host run my steering-presence probe filtered `ip_route_v2_dump` with `src = 0xff` (matches
nothing), so the steering delete was skipped and the fixture deleted table 11012 underneath it. VPP kept the SR route
(fib index 1) and an MPLS recursive-resolution entry for label 11600 plus MPLS table 0. Repaired on the host without a
crash (recreated table 11012 — it reused fib index 1 — then `sr_mpls_steering_add_del` del, then table delete); probe
fixed (`src = 0`) and the host test now asserts the route is gone. Lesson for P05: never delete a vrf before its
dependents (the scheduler's reverse topological delete does this).

## Q8 — V9 candidate: gpe_fwd_entry_path_details sent without the plugin message-id base
`plugins/lisp/lisp-gpe/lisp_gpe_api.c:110` sets `rmp->_vl_msg_id = htons(VL_API_GPE_FWD_ENTRY_PATH_DETAILS)` (no
`gpe_base_msg_id`), so clients receive another message (`memclnt.GetFirstMsgIDReply` in govpp). GPE forwarding-entry
locator pairs cannot be read → `lisp-gpe.fwd-entry` is write-only. Ask: record as V9 (one-line upstream fix).

## Q9 — LISP leftovers on the shared host (VPP behaviour, not DF-6 objects)
- **V10 candidate**: deleting a remote mapping leaves VPP's auto-created remote locator set: `show lisp locator-set`
  now lists `<remote-1>` … `<remote-5>` (rloc 10.11.14.2, one per opt-in LISP host run). No API can delete them (they
  have no name in the by-name hash); they disappear on the next VPP restart.
- LISP-GPE created `lisp_gpe0` and `lisp_gpe11100` interfaces (down); VPP keeps them for reuse after the EID-table map
  is removed and LISP disabled. Harmless; gone after a VPP restart. LISP itself was restored to `disabled`.
- One earlier failed run (EID decoded from `deid` instead of `seid`) orphaned a local EID; cleaned up via the API.

## Q10 — P05/P08 wiring notes (updated in the review fix round)
- Every Register is now `Register(r, client, owner, opts ...df6.Option)` (gre and ipip: `(r, client, owner)`).
  Options: `df6.WithGlobalsOwner(true)` on the globals owner only (D-071); `df6.WithClaims(store)` (default DF-1's
  `iface.Claims(owner)` — install a persisted store with `iface.SetClaimStore`, e.g. `df6.OpenFileClaimStore`; without
  it, claims and per-boot records are lost on agent restart and SR/LISP objects become invisible / toggles re-enable).
- Write-only descriptors return `df6.ErrRetrieveUnsupported`; alias it to `scheduler.ErrRetrieveUnsupported` when P05
  merges (D-073c, review L5). Globals / require variants implement `DeleteOnAbsence() bool` (P05 AbsenceDeleter).
- `lisp.enable` implicitly enables LISP-GPE in VPP; desired state with `lisp.enable` should include `lisp-gpe.enable`.
- The `df6` helpers still duplicate DF-2's; fold both into P05 core helpers when that lands (review L5).
