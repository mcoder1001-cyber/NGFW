# DF-6 WIP (slot 11, prefix w11, tables/VNIs/TEIDs/labels 11000–11999, addrs 10.11.0.0/16 + fd11::/16)

| plugin | status |
|---|---|
| gre | done (unit + host) |
| ipip (tunnel, 6rd) | done (unit + host) |
| vxlan (tunnel, bypass) | done (unit + host) |
| vxlan_gpe (tunnel, bypass) | done (unit + host) |
| gtpu (tunnel+tteid update, forward, bypass) | done; V8 crash guard added (see DF-6-questions Q1) |
| l2tp (tunnel+cookies update, interface-enable, lookup-key) | done; tunnel create opt-in on host (no delete) |
| pppoe (session, cp) | done; session create skips on host (Q3) |
| sr (localsid, policy, steering, encap source/hop-limit) | todo |
| sr_mpls (policy, steering, endpoint color) | todo |
| lisp | todo |
| docs/agent/descriptors/*.md | todo |
