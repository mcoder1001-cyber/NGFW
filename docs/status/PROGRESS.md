# Progress

Updated 2026-09-24 from plan/tasks.yaml (estimated hours are the plan's, not actuals).

**Overall: 16.1% by hours (141/878 h), 18.3% by tasks (15/82)**

| state | tasks |
|---|---|
| merged | 15 |
| review | 2 |
| running | 10 |
| ready | 0 |
| parked | 0 |
| failed | 0 |
| todo | 55 |

| stage | merged h / total h | % | tasks merged/total | running | ready | parked |
|---|---|---|---|---|---|---|
| S0 | 6 / 6 | 100.0% | 1/1 | 0 | 0 | 0 |
| S1 | 73 / 73 | 100.0% | 9/9 | 0 | 0 | 0 |
| S2 | 62 / 223 | 27.8% | 5/17 | 9 | 0 | 0 |
| S3 | 0 / 16 | 0.0% | 0/1 | 0 | 0 | 0 |
| S4 | 0 / 384 | 0.0% | 0/37 | 0 | 0 | 0 |
| S5 | 0 / 128 | 0.0% | 0/13 | 1 | 0 | 0 |
| S6 | 0 / 48 | 0.0% | 0/4 | 0 | 0 | 0 |

## Running / review

- P05 — Agent core: govpp, reconciler, ownership scoping, gRPC server, resync, confirm timer (review, host-agent slot7)
- P06 — API core: datastore, commit engine, auth/RBAC/audit, telemetry relay (running, host-agent slot1)
- DF-3 — Descriptors: nat44_ed (nat), nat44_ei, nat64, nat66, det44, map, cnat, pnat (running, host-agent slot9)
- DF-5 — Descriptors: ipsec, ikev2, wireguard (running, host-agent slot4)
- DF-6 — Descriptors: gre, ipip, vxlan, vxlan_gpe, gtpu, l2tp, pppoe, sr (srv6 + mpls), lisp (running, host-agent slot11)
- DF-7 — Descriptors: policer, qos, lb, span, lldp, bfd, vrrp, igmp, mpls (running, host-agent slot10)
- DF-8 — Descriptors: dhcp, dns, flowprobe, sflow, prom, pcap/tracenode, lcp [skip-unless-loaded] (running, host-agent slot5)
- RF-2 — Renderers: strongswan (swanctl/VICI path; vrx build lands in P11) (running, host-agent slot3)
- RF-3 — Renderers: kea-dhcp4/6 + ctrl-agent, unbound, chrony (running, host-agent slot6)
- RF-4 — Renderers: snmpd, keepalived, rsyslog (running, host-agent slot8)
- F-startup-gen — startup.conf generator: hugepages, workers, RSS, NUMA, isolcpus, dpdk dev/name mapping, plugin enable list (review, host-agent slot12)
- F-vpp-debs — VPP package build pipeline: pinned 26.06 source, patch series, reproducible .deb build script (running, host-agent slot2)

## Parked

- none
