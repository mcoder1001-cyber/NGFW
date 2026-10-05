# Progress

Updated 2026-10-05 from plan/tasks.yaml (estimated hours are the plan's, not actuals).

**Overall: 96.9% by hours (1529.0/1578.5 h), 94.8% by tasks (201/212)**

| state | tasks |
|---|---|
| merged | 201 |
| review | 0 |
| running | 2 |
| ready | 0 |
| parked | 9 |
| failed | 0 |
| todo | 0 |

| stage | merged h / total h | % | tasks merged/total | running | ready | parked |
|---|---|---|---|---|---|---|
| S0 | 6 / 6 | 100.0% | 1/1 | 0 | 0 | 0 |
| S1 | 73 / 73 | 100.0% | 9/9 | 0 | 0 | 0 |
| S2 | 264 / 274 | 96.4% | 28/29 | 0 | 0 | 1 |
| S3 | 16 / 19 | 84.2% | 1/2 | 0 | 0 | 1 |
| S4 | 980.0 / 1003.0 | 97.7% | 144/151 | 1 | 0 | 6 |
| S5 | 142 / 155.5 | 91.3% | 14/16 | 1 | 0 | 1 |
| S6 | 48 / 48 | 100.0% | 4/4 | 0 | 0 | 0 |

Merged measures reviewed source completion; deferred lab acceptance is not PASS. Running describes remaining implementation, not verified worker activity.

## Remaining implementation / review

- F-ra-vpn — remote-access VPN IKEv2+EAP (running, unassigned; Owner-approved independent strongSwan engine; active author upgrade; guarded lifecycle and actual EAP acceptance implementation ongoing.)
- TEST-traffic-B — Wave-B traffic scenario (IPsec, WireGuard, GRE/VXLAN, BGP/OSPF→FIB, DHCP relay) with FRR/strongSwan peers in netns (running, /root/traffic_b; verified live developer in current chat)

## Parked

- TD-19 — parked_on: PENDING-TD19-repository-trust
- LAB-vpp-per-slot — parked_on: PENDING-vpp-host-hardening
- P12-fib-proof — parked_on: PR180 HOLD: current mgmtd startup failed at unchanged 30s deadline before 200-route proof
- F-srv6-host — parked_on: lab topology/host acceptance prerequisite; NOTRUN
- F-mpls-srmpls-host — parked_on: lab topology/host acceptance prerequisite; NOTRUN
- F-global-blocking-host — parked_on: lab topology/host acceptance prerequisite; NOTRUN
- F-pppoe-client-host — parked_on: lab topology/host acceptance prerequisite; NOTRUN
- F-nat46-host — parked_on: remaining live packet/FIB/restart/API acceptance
- F-ospf-host — parked_on: remaining live packet/FIB/restart/API acceptance
