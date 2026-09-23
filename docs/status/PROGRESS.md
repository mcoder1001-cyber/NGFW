# Progress

Updated 2026-09-23 from plan/tasks.yaml (estimated hours are the plan's, not actuals).

**Overall: 5.0% by hours (42/848 h), 7.6% by tasks (6/79)**

| state | tasks |
|---|---|
| merged | 6 |
| review | 2 |
| running | 9 |
| ready | 6 |
| parked | 1 |
| failed | 0 |
| todo | 55 |

| stage | merged h / total h | % | tasks merged/total | running | ready | parked |
|---|---|---|---|---|---|---|
| S0 | 6 / 6 | 100.0% | 1/1 | 0 | 0 | 0 |
| S1 | 36 / 71 | 50.7% | 5/9 | 3 | 0 | 0 |
| S2 | 0 / 215 | 0.0% | 0/16 | 6 | 6 | 0 |
| S3 | 0 / 16 | 0.0% | 0/1 | 0 | 0 | 0 |
| S4 | 0 / 372 | 0.0% | 0/36 | 0 | 0 | 1 |
| S5 | 0 / 120 | 0.0% | 0/12 | 0 | 0 | 0 |
| S6 | 0 / 48 | 0.0% | 0/4 | 0 | 0 | 0 |

## Running / review

- P02a — Schema group (a): system, dataplane, interfaces, vrfs, routing, management + primitives/diff/merge-patch/gen (running, desktop-agent slot1)
- P02b — Schema group (b): nat, objects, acl (running, desktop-agent slot5)
- P02c — Schema group (c): vpn, tunnels, services, ha (running, desktop-agent slot6)
- P05 — Agent core: govpp, reconciler, ownership scoping, gRPC server, resync, confirm timer (running, desktop-agent slot7)
- P07a — UI shell: theme/RTL, i18n, SchemaForm, ServerDataGrid, WS hook, frame skeleton (review, desktop-agent slot8)
- DF-1 — Descriptors: bond, l2 (bridge, xconnect), memif, tap, host-interface/af_packet, subinterface, admin-state, mtu, rx-mode (running, desktop-agent slot2)
- DF-2 — Descriptors: ip_neighbor, ip6_nd (RA, DAD), urpf, abf, classify (ip tables/routes are P05 core) (running, desktop-agent slot3)
- DF-3 — Descriptors: nat44_ed (nat), nat44_ei, nat64, nat66, det44, map, cnat, pnat (running, desktop-agent slot9)
- DF-4 — Descriptors: acl (incl. macip), acl stats (review, desktop-agent slot10)
- DF-5 — Descriptors: ipsec, ikev2, wireguard (running, desktop-agent slot4)
- DF-6 — Descriptors: gre, ipip, vxlan, vxlan_gpe, gtpu, l2tp, pppoe, sr (srv6 + mpls), lisp (running, desktop-agent slot11)

## Parked

- P12 — parked_on: handover
