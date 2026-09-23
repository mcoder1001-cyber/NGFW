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
| V8 | gtpu API handler SIGSEGV (two crashes 2026-09-24 00:25:16 / 00:26:04, backtrace in `gtpu_plugin.so` via `vl_msg_api_socket_handler`) | DF-6 test, manager | crash on an API message — exact message/fields pending in DF-6-questions.md | host test opt-in only (D-064); descriptor validates inputs before sending | 1–3 days (input validation) |
| V9 | det44: `det44_plugin_disable()` segfaults after any det44 interface was removed (`src/plugins/nat/det44/det44.c`); crashed shared VPP 2026-09-23 16:03 and 2026-09-24 00:19 | DF-3 (Q0) | upstream use-after-free/NULL in the disable path | agent never disables det44 once enabled; host test opt-in `VRX_DF3_DET44=1` | 1–2 days |
| V10 | cnat: `cnat_set_snat_policy` / `cnat_snat_policy_add_del_exclude_pfx` dereference a NULL default SNAT entry (ASSERT compiled out); `cnat_translation_update` with `n_paths=0` underflows `vec_validate` | DF-3 (Q7) | missing input validation | DF-3 descriptors guard ordering and reject empty paths | 1 day |
| V11 | pnat: `pnat_bindings_details` lacks the binding index; `pnat_flow_lookup` / `pnat_binding_detach` crash before the first attach (flow bihash not instantiated); detach disables the attachment point while other bindings remain | DF-3 (Q8) | API gap + missing lazy init | index recovered via `pnat_bindings_get` cursor; calls guarded | 1–2 days |
| V12 | ip6-nd proxy: after `ip6nd_proxy_add_del` the shared VPP aborted with an out-of-memory abort in the interface-output node (stack in DF-2-questions.md) | DF-2 (Q1) | crash on a valid-looking API call | proxy-ND descriptor opt-in only (`VRX_DF2_PROXY_ND=1`), not in default Register | 1–2 days |
