# DF-7 — WIP (slot 10, prefix w10)

Started 2026-09-24. Shared helpers in `apps/agent/internal/descriptors/df7` (codec, interface snapshot +
ownership, fib paths, keys, options) and `df7/df7test` (fake with interface table, plan diff, host helpers).

| plugin | descriptors | unit | host | doc |
|---|---|---|---|---|
| policer | policer (R), interface (W), bind (W), classify (W) | done | done | todo |
| qos | record, store, egress-map, mark | todo | todo | todo |
| lb | conf, vip, as, intf-nat (all W — dump bugs) | todo | todo | todo |
| span | mirror | todo | todo | todo |
| lldp | global (W), interface (W) | todo | todo | todo |
| bfd | auth-key, udp-session, echo-source + events | todo | todo | todo |
| vrrp | vr, peers, track-if, state + events | todo | todo | todo |
| igmp | interface (W), listen, group-prefix (W), proxy-device (W), proxy-downstream (W) + events | todo | todo | todo |
| mpls | table, interface, route, ip-bind (W), tunnel | todo | todo | todo |

R = Retrieve from a dump; W = write-only (D-063 ErrRetrieveUnsupported).
VPP NRestarts baseline 2 (checked before the first policer host run and after: 2).
