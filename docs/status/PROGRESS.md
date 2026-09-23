# Progress

Updated 2026-09-24 from plan/tasks.yaml (estimated hours are the plan's, not actuals).

**Overall: 11.2% by hours (98/878 h), 13.4% by tasks (11/82)**

| state | tasks |
|---|---|
| merged | 11 |
| review | 0 |
| running | 12 |
| ready | 4 |
| parked | 0 |
| failed | 0 |
| todo | 55 |

| stage | merged h / total h | % | tasks merged/total | running | ready | parked |
|---|---|---|---|---|---|---|
| S0 | 6 / 6 | 100.0% | 1/1 | 0 | 0 | 0 |
| S1 | 68 / 73 | 93.2% | 8/9 | 1 | 0 | 0 |
| S2 | 24 / 223 | 10.8% | 2/17 | 11 | 3 | 0 |
| S3 | 0 / 16 | 0.0% | 0/1 | 0 | 0 | 0 |
| S4 | 0 / 384 | 0.0% | 0/37 | 0 | 0 | 0 |
| S5 | 0 / 128 | 0.0% | 0/13 | 0 | 1 | 0 |
| S6 | 0 / 48 | 0.0% | 0/4 | 0 | 0 | 0 |

## Running / review

- P03b — Proto sync after schema groups merge: mirror final P02a/b/c field names into DesiredState, buf breaking check, regen, drift-guard test (running, host-agent slot2)
- P05 — Agent core: govpp, reconciler, ownership scoping, gRPC server, resync, confirm timer (running, host-agent slot7)
- P06 — API core: datastore, commit engine, auth/RBAC/audit, telemetry relay (running, host-agent slot1)
- P07a — UI shell: theme/RTL, i18n, SchemaForm, ServerDataGrid, WS hook, frame skeleton (running, host-agent slot8)
- DF-2 — Descriptors: ip_neighbor, ip6_nd (RA, DAD), urpf, abf, classify (ip tables/routes are P05 core) (running, host-agent slot3)
- DF-3 — Descriptors: nat44_ed (nat), nat44_ei, nat64, nat66, det44, map, cnat, pnat (running, host-agent slot9)
- DF-5 — Descriptors: ipsec, ikev2, wireguard (running, host-agent slot4)
- DF-6 — Descriptors: gre, ipip, vxlan, vxlan_gpe, gtpu, l2tp, pppoe, sr (srv6 + mpls), lisp (running, host-agent slot11)
- DF-7 — Descriptors: policer, qos, lb, span, lldp, bfd, vrrp, igmp, mpls (running, host-agent slot10)
- DF-8 — Descriptors: dhcp, dns, flowprobe, sflow, prom, pcap/tracenode, lcp [skip-unless-loaded] (running, host-agent slot5)
- RF-1 — Renderers: frr (renderer framework: files, vtysh -C, frr-reload.py, JSON state; protocol semantics in P12/F-*) (running, host-agent slot12)
- RF-3 — Renderers: kea-dhcp4/6 + ctrl-agent, unbound, chrony (running, host-agent slot6)

## Parked

- none
