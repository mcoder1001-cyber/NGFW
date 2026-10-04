# Progress

Updated 2026-10-04 from plan/tasks.yaml (estimated hours are the plan's, not actuals).

**Overall: 80.6% by hours (1272.0/1577.5 h), 82.0% by tasks (173/211)**

| state | tasks |
|---|---|
| merged | 173 |
| review | 0 |
| running | 6 |
| ready | 7 |
| parked | 10 |
| failed | 0 |
| todo | 15 |

| stage | merged h / total h | % | tasks merged/total | running | ready | parked |
|---|---|---|---|---|---|---|
| S0 | 6 / 6 | 100.0% | 1/1 | 0 | 0 | 0 |
| S1 | 73 / 73 | 100.0% | 9/9 | 0 | 0 | 0 |
| S2 | 264 / 274 | 96.4% | 28/29 | 0 | 0 | 1 |
| S3 | 16 / 19 | 84.2% | 1/2 | 0 | 0 | 1 |
| S4 | 839.0 / 1002.0 | 83.7% | 126/150 | 4 | 6 | 8 |
| S5 | 74 / 155.5 | 47.6% | 8/16 | 2 | 1 | 0 |
| S6 | 0 / 48 | 0.0% | 0/4 | 0 | 0 | 0 |

Merged measures reviewed source completion; deferred lab acceptance is not PASS. Running describes remaining implementation, not verified worker activity.

## Remaining implementation / review

- P11 — Wave B (day 10-12): strongSwan+VPP build (staging sysroot) + IPsec S2S + tunnel dashboards (running, Codex manager delegated worker; assignment unverified; remaining implementation state, not evidence of a live worker)
- F-ikev2-native — Wave B (day 10-12): VPP native IKEv2 responder path (running, unassigned; worker activity unverified)
- F-vrrp-config-sync — Wave C (day 13-15): VRRPv3 (VPP plugin + keepalived path), config sync, cluster UI (running, cloud session modest-keller; assignment unverified; remaining implementation state, not evidence of a live worker)
- P10 — Debian packaging + systemd + install (26.04, our VPP debs) (running, unassigned; assignment unverified; remaining implementation state, not evidence of a live worker)
- TD-19 — Install & lab provisioning from product artifacts (running, unassigned; assignment unverified; remaining implementation state, not evidence of a live worker)
- F-multiwan-host — Multi-WAN on the lab: two WAN netns, failover time, balance split, per-member NAT (running, dashboard_finish; assignment unverified; remaining implementation state, not evidence of a live worker)

## Parked

- LAB-vpp-per-slot — parked_on: PENDING-vpp-host-hardening
- P12-fib-proof — parked_on: PENDING-vpp-host-hardening (via LAB-vpp-per-slot)
- F-lb-host — parked_on: lab topology/host acceptance prerequisite; NOTRUN
- F-srv6-host — parked_on: lab topology/host acceptance prerequisite; NOTRUN
- F-mpls-srmpls-host — parked_on: lab topology/host acceptance prerequisite; NOTRUN
- F-rule-expiry-host — parked_on: lab topology/host acceptance prerequisite; NOTRUN
- F-global-blocking-host — parked_on: lab topology/host acceptance prerequisite; NOTRUN
- F-pppoe-client-host — parked_on: lab topology/host acceptance prerequisite; NOTRUN
- F-nat46-host — parked_on: remaining live packet/FIB/restart/API acceptance
- F-ospf-host — parked_on: remaining live packet/FIB/restart/API acceptance
