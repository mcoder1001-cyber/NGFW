# Progress

Updated 2026-09-24 from plan/tasks.yaml (estimated hours are the plan's, not actuals).

**Overall: 24.8% by hours (226/910 h), 25.9% by tasks (22/85)**

| state | tasks |
|---|---|
| merged | 22 |
| review | 1 |
| running | 5 |
| ready | 0 |
| parked | 0 |
| failed | 0 |
| todo | 57 |

| stage | merged h / total h | % | tasks merged/total | running | ready | parked |
|---|---|---|---|---|---|---|
| S0 | 6 / 6 | 100.0% | 1/1 | 0 | 0 | 0 |
| S1 | 73 / 73 | 100.0% | 9/9 | 0 | 0 | 0 |
| S2 | 147 / 233 | 63.1% | 12/19 | 5 | 0 | 0 |
| S3 | 0 / 16 | 0.0% | 0/1 | 0 | 0 | 0 |
| S4 | 0 / 406 | 0.0% | 0/38 | 0 | 0 | 0 |
| S5 | 0 / 128 | 0.0% | 0/13 | 0 | 0 | 0 |
| S6 | 0 / 48 | 0.0% | 0/4 | 0 | 0 | 0 |

## Running / review

- P06 — API core: datastore, commit engine, auth/RBAC/audit, telemetry relay (running, host-agent slot1)
- DF-5 — Descriptors: ipsec, ikev2, wireguard (running, host-agent slot4)
- DF-7 — Descriptors: policer, qos, lb, span, lldp, bfd, vrrp, igmp, mpls (running, host-agent slot10)
- RF-4 — Renderers: snmpd, keepalived, rsyslog (running, host-agent slot8)
- F-startup-gen — startup.conf generator: hugepages, workers, RSS, NUMA, isolcpus, dpdk dev/name mapping, plugin enable list (running, host-agent slot12)
- F-vpp-debs — VPP package build pipeline: pinned 26.06 source, patch series, reproducible .deb build script (review, host-agent slot2)

## Parked

- none
