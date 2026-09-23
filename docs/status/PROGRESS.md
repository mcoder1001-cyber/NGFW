# Progress

Updated 2026-09-24 from plan/tasks.yaml (estimated hours are the plan's, not actuals).

**Overall: 4.8% by hours (42/876 h), 7.3% by tasks (6/82)**

| state | tasks |
|---|---|
| merged | 6 |
| review | 2 |
| running | 9 |
| ready | 7 |
| parked | 1 |
| failed | 0 |
| todo | 57 |

| stage | merged h / total h | % | tasks merged/total | running | ready | parked |
|---|---|---|---|---|---|---|
| S0 | 6 / 6 | 100.0% | 1/1 | 0 | 0 | 0 |
| S1 | 36 / 71 | 50.7% | 5/9 | 3 | 0 | 0 |
| S2 | 0 / 223 | 0.0% | 0/17 | 6 | 6 | 0 |
| S3 | 0 / 16 | 0.0% | 0/1 | 0 | 0 | 0 |
| S4 | 0 / 384 | 0.0% | 0/37 | 0 | 0 | 1 |
| S5 | 0 / 128 | 0.0% | 0/13 | 0 | 1 | 0 |
| S6 | 0 / 48 | 0.0% | 0/4 | 0 | 0 | 0 |

## Running / review

- P02a — Schema group (a): system, dataplane, interfaces, vrfs, routing, management + primitives/diff/merge-patch/gen (running, host-agent slot1)
- P02b — Schema group (b): nat, objects, acl (running, host-agent slot5)
- P02c — Schema group (c): vpn, tunnels, services, ha (running, host-agent slot6)
- P05 — Agent core: govpp, reconciler, ownership scoping, gRPC server, resync, confirm timer (running, host-agent slot7)
- P07a — UI shell: theme/RTL, i18n, SchemaForm, ServerDataGrid, WS hook, frame skeleton (review, host-agent slot8)
- DF-1 — Descriptors: bond, l2 (bridge, xconnect), memif, tap, host-interface/af_packet, subinterface, admin-state, mtu, rx-mode (running, host-agent slot2)
- DF-2 — Descriptors: ip_neighbor, ip6_nd (RA, DAD), urpf, abf, classify (ip tables/routes are P05 core) (running, host-agent slot3)
- DF-3 — Descriptors: nat44_ed (nat), nat44_ei, nat64, nat66, det44, map, cnat, pnat (running, host-agent slot9)
- DF-4 — Descriptors: acl (incl. macip), acl stats (review, host-agent slot10)
- DF-5 — Descriptors: ipsec, ikev2, wireguard (running, host-agent slot4)
- DF-6 — Descriptors: gre, ipip, vxlan, vxlan_gpe, gtpu, l2tp, pppoe, sr (srv6 + mpls), lisp (running, host-agent slot11)

## Parked

- P12 — parked_on: handover
