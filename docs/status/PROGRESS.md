# Progress

Updated 2026-09-27 from plan/tasks.yaml (estimated hours are the plan's, not actuals).

**Overall: 62.3% by hours (823.0/1320.5 h), 64.2% by tasks (95/148)**

| state | tasks |
|---|---|
| merged | 95 |
| review | 7 |
| running | 3 |
| ready | 20 |
| parked | 2 |
| failed | 0 |
| todo | 21 |

| stage | merged h / total h | % | tasks merged/total | running | ready | parked |
|---|---|---|---|---|---|---|
| S0 | 6 / 6 | 100.0% | 1/1 | 0 | 0 | 0 |
| S1 | 73 / 73 | 100.0% | 9/9 | 0 | 0 | 0 |
| S2 | 264 / 274 | 96.4% | 28/29 | 0 | 0 | 1 |
| S3 | 16 / 19 | 84.2% | 1/2 | 0 | 0 | 1 |
| S4 | 426.0 / 753.0 | 56.6% | 52/88 | 2 | 18 | 0 |
| S5 | 38 / 147.5 | 25.8% | 4/15 | 1 | 2 | 0 |
| S6 | 0 / 48 | 0.0% | 0/4 | 0 | 0 | 0 |

## Running / review

- F-system-identity — Wave B: system identity — hostname, timezone, login/MOTD banners, DNS client + System screen (review, cloud session modest-keller)
- F-dataplane-ui — Dataplane screen: VPP plugins, NIC queues/descriptors, workers/corelist, hugepages — startup.conf preview + gated apply (review, cloud session modest-keller)
- F-management-ui — Management screen: tabbed shell (local users, AAA, API TLS, remote syslog) + apply of management.tls (review, cloud session modest-keller)
- P10 — Debian packaging + systemd + install (26.04, our VPP debs) (running, unassigned)
- WEB-4b — Pre-built HA/VRRP + cluster screens merged UNROUTED (running, cloud session modest-keller)
- TD-17 — apply-startup product mode: installed paths, appliance approval gate (sha256 + dead-man), lab gate kept as a mode (review, cloud session modest-keller)
- TD-21 — Scheduler scale: executor.dependents and topo re-sort keys on every operation (O(n^2)); 4000 objects = 13.7 s — index dependents once per plan (review, cloud session modest-keller)
- TD-26 — Core VRF tolerant delete: a table VPP keeps locked (nat64 never releases its FIB locks) is left, recorded and warned, not a failed transaction (review, cloud session modest-keller)
- TD-27 — ifsanitize: clear inherited SPAN source state and LLDP entries on interface create (V19 family) (review, cloud session modest-keller)
- F-global-blocking — Global Blocking: IP list file (upload or download from a server URL with scheduled refresh), enforced on chosen interfaces (running, cloud session charming-johnson)

## Parked

- LAB-vpp-per-slot — parked_on: PENDING-vpp-host-hardening
- P12-fib-proof — parked_on: PENDING-vpp-host-hardening (via LAB-vpp-per-slot)
