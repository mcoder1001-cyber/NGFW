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
