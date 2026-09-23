# DF-7 — WIP (slot 10, prefix w10)

State 2026-09-24: **complete** — see `DF-7.md` (evidence, decisions) and `DF-7-questions.md`.

| plugin | descriptors | unit | host | doc |
|---|---|---|---|---|
| policer | policer (R), interface (W), bind (W), classify (W) | done | done (bind: no workers) | done |
| qos | record, store, egress-map, mark | done | done | done |
| lb | conf (W, global), vip, as, intf-nat (W) | done | done (conf: globals opt-in) | done |
| span | mirror | done | done | done |
| lldp | global (W, global), interface (W) | done | done (global: opt-in; interface needs aligned loopback) | done |
| bfd | auth-key, udp-session, echo-source (global) + events | done | done (echo: opt-in) | done |
| vrrp | vr, peers, track-if, state + events | done | done | done |
| igmp | interface (W), listen, group-prefix (W, global), proxy-device (W), proxy-downstream (W) + events | done | done (prefix: opt-in) | done |
| mpls | table, interface, route, ip-bind (W), tunnel | done | done (interface/ip-bind need table 0: opt-in) | done |

Main merged in (D-069 resolver, D-071 globals/claims/index re-verify, D-076 boot identity) and applied everywhere.
VPP NRestarts 2 before the first and after the last host run of every plugin.
