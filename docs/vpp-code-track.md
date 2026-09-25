# VPP code track — anything that would need C code inside VPP

Rule (00-CONTEXT FAST MODE): no C code in VPP during the 21-day plan. When a task hits a wall that
only VPP code would solve, the worker appends an entry here, implements the configuration-only
fallback, and says so in `docs/status/tasks/<id>.md`. The product owner decides later whether to
fund the item with a VPP engineer.

| id | item | raised by | why VPP code seems needed | fallback implemented | est. VPP effort |
|---|---|---|---|---|---|
| V1 | linux-cp / linux-nl edge patches (IPv6 RA on pairs, bond sub-ifs, multi-VRF mapping, full-table churn) | plan | upstream gaps | document edge cases | 2–4 weeks |
| V2 | HA state sync for NAT44-ED sessions / reflexive ACL | plan | VPP has HA only for NAT44-EI (`nat44_ei_ha`) | VRRP without session preservation, or NAT44-EI where HA is required | 4–6 weeks |
| V3 | Hierarchical QoS (HQoS) | plan | DPDK HQoS scheduler removed from VPP | policer + marking + flat queues | 4–8 weeks |
| V4 | NAT ALGs (SIP, active FTP, PPTP) | plan | nat44-ed has limited ALGs | document unsupported | 2–3 weeks each |
| V5 | kernel→VPP sync of MPLS/LDP and multicast (PIM) routes | plan | linux-nl syncs unicast v4/v6 only | **agent reads FRR JSON and programs VPP via API (control plane, no VPP code)** — build this instead | 0 (fallback is the plan) |
| V6 | vpp_sswan (strongSwan kernel-vpp) patches | plan | possible incompatibility with strongSwan 6.x | pin a compatible strongSwan version | 0–2 weeks |
| V7 | VPP 26.06 `acl_stats_intf_counters_enable` replies with the `acl_del_reply` message id (acl.c) and has no getter/disable path | DF-4 | upstream bug in the ACL plugin API | agent sends the request on a raw stream and treats the mismatched reply as success; the counters flag is never disabled by the agent | 1–2 days (upstream patch + our build) |
| V8 | gtpu: `gtpu_add_del_tunnel_v2` segfaults VPP whenever the add or delete fails (e.g. deleting a missing tunnel); two crashes 2026-09-24 00:25:16 / 00:26:04 (request + backtrace in DF-6-questions.md Q1) | DF-6 | error path of the handler dereferences an invalid tunnel | host test opt-in (`VRX_DF6_GTPU_HOST=1`); descriptor dumps first and never sends a duplicate add or a delete for a missing tunnel | 1 day |
| V9 | det44: `det44_plugin_disable()` segfaults after any det44 interface was removed (`src/plugins/nat/det44/det44.c`); crashed shared VPP 2026-09-23 16:03 and 2026-09-24 00:19 | DF-3 (Q0) | upstream use-after-free/NULL in the disable path | agent never disables det44 once enabled; host test opt-in `VRX_DF3_DET44=1` | 1–2 days |
| V10 | cnat: `cnat_set_snat_policy` / `cnat_snat_policy_add_del_exclude_pfx` dereference a NULL default SNAT entry (ASSERT compiled out); `cnat_translation_update` with `n_paths=0` underflows `vec_validate` | DF-3 (Q7) | missing input validation | DF-3 descriptors guard ordering and reject empty paths | 1 day |
| V11 | pnat: `pnat_bindings_details` lacks the binding index; `pnat_flow_lookup` / `pnat_binding_detach` crash before the first attach (flow bihash not instantiated); detach disables the attachment point while other bindings remain | DF-3 (Q8) | API gap + missing lazy init | index recovered via `pnat_bindings_get` cursor; calls guarded | 1–2 days |
| V12 | ip6-nd proxy: after `ip6nd_proxy_add_del` the shared VPP aborted with an out-of-memory abort in the interface-output node (stack in DF-2-questions.md) | DF-2 (Q1) | crash on a valid-looking API call | proxy-ND descriptor opt-in only (`VRX_DF2_PROXY_ND=1`), not in default Register | 1–2 days |
| V13 | lisp-gpe: the fwd-entry path dump replies with the wrong message id, so `lisp-gpe.fwd-entry` cannot be read back | DF-6 (Q8) | upstream API bug (like V7) | descriptor is write-only (D-063) | 0.5 day |
| V14 | LISP: each enable/disable cycle leaks a `<remote-N>` locator set and leaves `lisp_gpe*` interfaces that no API can delete; `l2tpv3` has no tunnel-delete message; SR-MPLS policies have no dump | DF-6 (Q5, Q9) | API gaps / leaks | LISP host test opt-in; l2tp create skipped on host; SR-MPLS write-only | 2–4 days |
| V15 | FIB: deleting a table that still holds API drop routes leaks those entries into the next table that reuses the FIB index — including another owner's table (seen on the shared host: stray `10.10.82.1/32` from slot 10) | P05 fix round | upstream FIB cleanup bug | agent removes its own routes before deleting a table; leftovers cleaned by prefix | 1–3 days |
| V16 | ipfix classify dumps (`ipfix_classify_stream_details` / `_table_details`) sent without `REPLY_MSG_ID_BASE` (flow_api.c 335, 438) → empty dump | DF-8 (Q2.1) | upstream API bug (like V7, V13) | objects write-only (D-063) | 0.5 day (two-line patch) |
| V17 | `lcp_itf_pair_get_v2` with sw_if_index ~0 replies with the v1 reply id; `sflow_interface_details` has only hw_if_index (no hw→sw mapping in the API); `show dns servers` prints v6 list from the v4 vector (CLI only) | DF-8 (Q2.2–4) | API gaps / bugs | lcp uses v1 get; sflow learns hw→sw at create time; dns uses API state | 1 day |
| V18 | tracedump / tracenode plugins not built in our 26.06 packages; prom exporter and Trace Path have no binary API | DF-8 (Q1) | build config + API gap | object types dropped from DF-8 (D-077); F-capture-trace / F-dashboard-prom-alarms decide their path (build flag via F-vpp-debs, or startup-conf via F-startup-gen) | 0.5 day build + API work for prom |
| V19 | **CRASH VECTOR (two paths: ip classify on first packet; L2 ACL/policer feature bits when the reused index is later bridged — TD-3 review H1) (2026-09-24 04:50:27, NRestarts 5):** a reused sw_if_index inherited an ip classify binding to a deleted table → first packet → SIGSEGV in `vnet_classify_find_entry` (packet path). Per-interface state survives interface deletion: the "ip classify table" setting (and ADL feature state, DF-2) stays on the sw_if_index and is inherited by the next interface reusing it — root cause of the stray /32 drop routes seen by P05 (with V15) | DF-7 (Q), DF-2 | upstream cleanup gap on interface delete | descriptors clear per-interface settings before deleting an interface; tests reset inherited state on new interfaces | 1–2 days |
| V20 | lb plugin: "removed" VIPs are freed only by the CLI-triggered cleanup (`lb conf`), not via API; lb type fields not byte-swapped by VPP (agent swaps, little-endian only); lldp uses the sw_if_index as a hw index; broken dumps: policer_classify, igmp group-prefix, vrrp peers | DF-7 (Q) | API bugs / gaps | globals owner runs lb cleanup; write-only types (D-063); documented | 2–3 days |
| V21 | vxlan keeps a per-interface bypass flag after the interface is deleted (a recreated interface with the same index silently ignores the enable — V19 family); `lcp_default_ns_get` returns uninitialised bytes when no default netns is set | DF-6 (fix round 2), DF-8 (bug 5) | cleanup gap / uninitialised reply | vxlan bypass sends disable before enable; lcp treats an invalid name as unset | 0.5–1 day |
| V22 | Two crashes 2026-09-24: (a) `fib_table_flush` via API NULL/UAF while another client still used the table (fib_entry_contribute_ip_forwarding → ip4_fib_16_table_fwding_dpo_remove) — slot collision exposed it; (b) `ip4_options_node_fn` NULL deref on the packet path during DF-7 igmp/vrrp tests (trigger being identified) | manager (D-087), DF-7 | robustness bugs in FIB flush and the router-alert path | slot 12 reserved for CI; DF-7 igmp/vrrp host tests opt-in | 1–3 days |
| V23 | (a) `feature_is_enabled` reports `is_enabled=true` for every error of `vnet_feature_is_enabled` (the handler assigns its negative `VNET_API_ERROR_*` to a bool): an unknown arc or feature (verified on vrx-a: `feature_is_enabled(ip4-unicast, no-such-feature)` = true on a fresh loopback) and a sw_if_index beyond the arc's config vector (source `vnet_feature_is_enabled`; seen once as `device-input/adl-input` = true on a fresh loopback right after a VPP restart) all read as "enabled". Likely explains TD-1's flaky "loop… already has an output ACL (ip4-outacl)" on a fresh loopback; affects DF-2 `classify.output-acl` Create/Retrieve and `adl.interface` Retrieve. (b) Per-interface binding vectors survive interface delete beyond V19/V21: in/out ACL, policer/flow classify table indices, the l2-input/l2-output feature bits (L2 input/output ACL, L2 policer classify — not feature arcs; reset on delete only for bridged/xconnected interfaces, so a stale L2 ACL bit on a later bridged port makes `l2-input-acl` read a freed table: second crash path, TD-3 review H1) and the IPsec SPD binding (`spd_index_by_sw_if_index`, DF-5 review M3 — the next interface on the index cannot get an SPD); an add on a still-bound ip slot returns 0 without enabling anything (silent ACL bypass). An unbind naming a freed table is refused (NO_SUCH_TABLE), but the classify table pool is LIFO, so the freed index can be resurrected with a placeholder table and unbound through it. (ADL per-index config is re-initialised on interface add — not affected.) Known, not handled (no crash path found, feature arcs cleared on delete): ABF attachments, NAT64/NAT66/DET44 interface flags, cnat snat-if, flowprobe | TD-3 | (a) wrong API encoding; (b) upstream cleanup gap on sw-interface delete | (a) TD-3's sanitizer never uses `feature_is_enabled`; DF-2 should confirm a "true" with a second query of a feature that cannot be on, or treat it as unknown; (b) `ifsanitize.Acquire` on every interface create (D-095; TD-3 fix round 1): L3-mode reset, resurrect freed table indices with placeholders, unbind, else quarantine the index (`quarantine:<owner>` admin-down holder) and take a fresh one; `ifsanitize.BeforeDelete` in every interface Delete; classify table Delete refuses while bound; `vrx-vpp-preflight` in `ci.sh full` before/after the suites | 0.5 day (a), 1 day (b) |
| V24 | af_packet delete: `af_packet_delete_if` (`plugins/af_packet/af_packet.c:895-900`) closes the socket fds **before** `af_packet_rx_queue_free` → `clib_file_del_by_index` (`:827`); the epoll DEL then fails with EBADF (the "harmless" `vlib_file_update: epoll_ctl() failed … errno 9` log line) and `clib_file_del` closes the same fd number a second time (`vppinfra/file.h:117`), which can close an fd another part of VPP opened meanwhile. Suspected cause of the SIGSEGV PC 0x0 at 2026-09-24 07:27:32 (NRestarts 5→6), 10 s after slot 1 deleted two af_packet interfaces whose veths were up; no core, no backtrace (not proven) | TD-3 Q1 (manager analysis, D-101) | upstream ordering bug: file must be deleted before its fd is closed | lab rig brings the host veth down before `delete host-interface` (tools/lab, D-101); TD-5: the agent's af_packet Delete quiesces the netdev first; **P08 review I1 (14:10–14:15): the EBADF line appears on every af_packet delete even with the veth down, and fd numbers are reused across interfaces → quiescing lowers the risk but does not remove the double close; keep af_packet churn on the shared VPP minimal until patched (D-107)**; af_packet is lab-only (product = DPDK). **Agent side (TD-5, task/TD-5):** Delete and Create's rollback bring the netdev down via netlink (RTM_NEWLINK clear IFF_UP, confirmed down, 200 ms settle) before every `af_packet_delete`, fail closed otherwise (no delete; the rollback leaves a named untagged orphan); guard `TestEveryAfPacketDeleteIsQuiesced`; needs CAP_NET_ADMIN (P10). Host run 2026-09-24 14:35: `errno 9` still logged on each delete with the netdev down (as I1): the quiesce lowers the risk, the double close remains until the VPP fix | 0.5 day (move `close()` after the rx-queue free + `dont_close`) |

### V-new (F-nat44-ed-sessions)
**NAT44-ED ignores partial checksums from af_packet (lab path).** A TCP/UDP segment that a Linux netns sends through a
veth with tx checksum offload on arrives in VPP with a *partial* checksum: the af_packet input marks it
`VNET_BUFFER_OFFLOAD_F_TCP_CKSUM` (`plugins/af_packet/node.c` `fill_cksum_offload`, when the interface has cksum/gso
enabled). `nat44-ed-in2out` / `-out2in` update the L4 checksum incrementally without looking at the offload flags
(`plugins/nat/nat44-ed/nat44_ed_in2out.c` has no `oflags` check), so the translated segment leaves with a wrong
checksum and the far host drops it silently; ICMP (checksummed in software) passes. Seen 2026-09-24 19:03 on slot 4:
SYNs through a PAT pool and a port forward reached the far netns and were never answered; with `ethtool -K <veth> tx off`
in the namespaces the same connections succeed. Product path (DPDK NICs) delivers complete checksums; af_packet is
lab-only. Fallback implemented: the F-nat44-ed-sessions topology test turns tx checksum offload off on the rig's netns
veths (test side; `tools/lab rig up` could do the same for every slot — manager's call). VPP fix: resolve the partial
checksum before NAT (or make NAT offload-aware) — est. 0.5–1 day.

### V-new (F-nat44-ed-sessions, review L2)
**`nat44_user_session_v3_dump` ignores the user's VRF and reads one worker.** `vl_api_nat44_user_session_v3_dump_t_handler`
(`plugins/nat/nat44-ed/nat44_ed_api.c` ~1659-1690) computes `ukey.fib_index` but then matches sessions by
`s->in2out.addr` only, and its details carry no FIB; it also walks only the in2out worker of the address, while
`nat44_user_dump` reports one row per (worker, user). Effects: the same inside address in two VRFs (overlapping tenant
address space, the main multi-VRF NAT case) lists both VRFs' sessions under each user, and the VRF of a row cannot be
told; on a multi-worker VPP, sessions of load-balanced mappings on other workers are counted but never listed. Every
call also walks the worker's whole session pool under the barrier (the plugin's API handlers are not mp-safe), which
is why the agent caps per-user dumps per call (review H1). Fallback implemented (agent): users merged by (VRF,
address) with summed counts; users that share an address partition that address's dump (no session listed twice); one
dump per address in scans; per-call caps; the user page documents the possible VRF mislabel. VPP fix: also match
`s->in2out.fib_index == ukey.fib_index`, walk every worker, and add the FIB to the details (or add a paged,
cursor-based session dump) — est. 0.5–1 day.

### V-new (F-nat44-ei-64-66-nptv6)
**(a) npt66 has no dump (`npt66_binding_dump`).** VPP 26.06's npt66 plugin has one message, `npt66_binding_add_del`;
bindings can be listed only with the CLI `show npt66 bindings` (without the interface). The agent's `npt66.binding`
descriptor is therefore write-only (D-063): it re-applies every desired binding on each resync (safe: VPP's add
overwrites the interface's binding and enables the features only for a new one, so no D-076 record is needed), but it
cannot see drift, cannot delete a binding that left the configuration while the agent was down, and Retrieve never
reports NPTv6. Proposed: `npt66_binding_dump` → `npt66_binding_details {sw_if_index, internal, external}` (+ an
interface-delete hook that frees the binding: today a binding survives its interface, and a later interface reusing the
index silently gets no npt66 feature on its first add) — est. 0.5 day.
**(b) `nat64_st_details` report the wrong ports.** `nat64_api_st_walk` (`plugins/nat/nat64/nat64_api.c` ~316-320) sets
`il_port` twice — the second time to `ste->r_port` — and never sets `r_port`: every NAT64 session row carries the remote
port as the inside port and 0 as the remote port (seen on slot 4, 2026-09-25 00:46: CLI `fd00:4:1::2 46001 … 10.4.2.2
8000`, API il_port 8000, r_port 0). Fallback implemented: the agent's NAT64 pager takes the remote port from `il_port`
when `r_port` is 0 and the inside port from the BIB (`nat64_bib_dump`, by the outside endpoint). Fix: one line
(`rmp->r_port = ste->r_port;`) — est. 0.1 day.
**(c) nat64 leaks FIB locks on tenant VRFs.** `nat64_add_del_prefix` locks the VRF's IPv6 table on add and never unlocks
it on delete ("TODO: missing fib_table_unlock"), and `nat64_add_del_static_bib_entry` calls
`fib_table_find_or_create_and_lock` on every add AND delete without an unlock; disabling the plugin does not release them
either. A VRF that carried a NAT64 prefix or static BIB can therefore not be deleted until VPP restarts (its IPv6 table
stays with `nat64-hi` locks; the agent's VRF delete then fails its verify → DEGRADED). Seen on slot 4 (`show ip6 fib
table 4064`: `locks:[nat64-hi:24]` after a few runs). Fallback: documented (user page: keep NAT64 in the default VRF
on a box, or keep the tenant VRF); the topology test keeps its slot VRF in the configuration. Fix: unlock on prefix
delete, lock only on BIB add and unlock on BIB delete — est. 0.5 day.
**(d) (not a bug, noted for users) nat44-ei port forwards need a pool address.** `nat44_ei_add_static_mapping` reserves
the external port on a pool address (`nat44_ei_reserve_port`) unless static-mapping-only is on, and answers
NO_SUCH_ENTRY for an address outside the pool (unlike nat44-ed). The builder refuses such a mapping with a pointer
(`nat.ei-port-forward-pool`).
