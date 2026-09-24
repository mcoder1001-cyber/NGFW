# TD-3 WIP — V19 guard (D-095)

Slot 2 (w2). Branch task/TD-3.

| part | state |
|---|---|
| (a) create-time sanitizer `internal/vpp/ifsanitize` + wired into loopback (core), tap, af_packet, memif, bond, sub-interface, DF-6 tunnels (df6.IfDescriptor: gre, ipip, 6rd, vxlan, vxlan-gpe, gtpu, l2tp, pppoe), mpls tunnel; metric `vrx_agent_iface_sanitize_*`; IPsec SPD (manager add-on, DF-5 M3) | code + unit tests done; host test pending |
| (b) table Delete refuses while bindings exist | pending |
| (c) restart simulations delete dependents first | pending |
| (d) ci.sh pre-flight | pending |

Notes
- VPP facts (source-verified, /root/vpp): feature arcs are cleared on interface delete; per-index vectors (ip classify,
  in/out ACL, policer/flow classify, vxlan bypass bitmap, ADL config, SPD) are not. ip classify is read directly by
  ip4_add_interface_routes → classify DPO on the /32 → crash vector. policer/flow_classify_dump are broken (DF-7) → probe-unbind.
