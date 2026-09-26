# Progress

Updated 2026-09-26 from plan/tasks.yaml (estimated hours are the plan's, not actuals).

**Overall: 61.6% by hours (745.0/1208.5 h), 62.7% by tasks (84/134)**

| state | tasks |
|---|---|
| merged | 84 |
| review | 2 |
| running | 4 |
| ready | 16 |
| parked | 2 |
| failed | 0 |
| todo | 26 |

| stage | merged h / total h | % | tasks merged/total | running | ready | parked |
|---|---|---|---|---|---|---|
| S0 | 6 / 6 | 100.0% | 1/1 | 0 | 0 | 0 |
| S1 | 73 / 73 | 100.0% | 9/9 | 0 | 0 | 0 |
| S2 | 264 / 274 | 96.4% | 28/29 | 0 | 0 | 1 |
| S3 | 16 / 19 | 84.2% | 1/2 | 0 | 0 | 1 |
| S4 | 348.0 / 641.0 | 54.3% | 41/74 | 3 | 15 | 0 |
| S5 | 38 / 147.5 | 25.8% | 4/15 | 1 | 1 | 0 |
| S6 | 0 / 48 | 0.0% | 0/4 | 0 | 0 | 0 |

## Running / review

- F-unbound-chrony-syslog — Wave B (day 10-12): Unbound DNS, chrony NTP, syslog export + log explorer (review, unassigned)
- F-mpls-srmpls — Wave C (day 13-15): static MPLS + SR-MPLS (LDP split to F-mpls-ldp, D-085/D-109) (running, unassigned)
- F-srv6 — Wave C (day 13-15): SRv6 policies, network programming, service chaining proxies, SRv6-mobile (running, unassigned)
- F-lb — Wave C (day 13-15): Load Balancer plugin (GRE/NAT/L3DSR/maglev) (running, unassigned)
- P10 — Debian packaging + systemd + install (26.04, our VPP debs) (running, unassigned)
- WEB-2 — Config screen kit (generic list+drawer+live-status over any candidate path) + data widgets + Secrets page (review, ngfw-46 slot1)

## Parked

- LAB-vpp-per-slot — parked_on: PENDING-vpp-host-hardening
- P12-fib-proof — parked_on: PENDING-vpp-host-hardening (via LAB-vpp-per-slot)
