# Progress

Updated 2026-10-08 from plan/tasks.yaml (estimated hours are the plan's, not actuals).

**Overall: 92.9% by hours (1467.0/1578.5 h), 92.9% by tasks (197/212)**

| state | tasks |
|---|---|
| merged | 197 |
| review | 7 |
| running | 3 |
| ready | 0 |
| parked | 5 |
| failed | 0 |
| todo | 0 |

| stage | merged h / total h | % | tasks merged/total | running | ready | parked |
|---|---|---|---|---|---|---|
| S0 | 6 / 6 | 100.0% | 1/1 | 0 | 0 | 0 |
| S1 | 73 / 73 | 100.0% | 9/9 | 0 | 0 | 0 |
| S2 | 274 / 274 | 100.0% | 29/29 | 0 | 0 | 0 |
| S3 | 0 / 19 | 0.0% | 0/2 | 0 | 0 | 1 |
| S4 | 924.0 / 1003.0 | 92.1% | 140/151 | 3 | 0 | 3 |
| S5 | 142 / 155.5 | 91.3% | 14/16 | 0 | 0 | 1 |
| S6 | 48 / 48 | 100.0% | 4/4 | 0 | 0 | 0 |

Merged measures reviewed source completion; deferred lab acceptance is not PASS. Running describes remaining implementation, not verified worker activity.

## Remaining implementation / review

- P08 — Vertical slice: interfaces end to end (af_packet rig) (review, ngfw-46 slot1; IP unnumbered source675cd9ea independently R4-approved and staged in PR214 (remote036ca641). Final cumulative CI/merge and native VPP acceptance pending.)
- F-wireguard — Wave B (day 10-12): WireGuard peers/keys (review, unassigned; Combined sealed credential source9d61005d independently R2-approved; R4 CA-key isolation correction approved. Eight focused race packages and36 API tests PASS. Staged in PR214; final cumulative generation/CI/merge and native consumer acceptance pending.)
- P12 — Wave B (day 10-12): FRR + linux-cp framework, BGP (review, unassigned; Combined sealed credential source9d61005d independently R2-approved; R4 CA-key isolation correction approved. Eight focused race packages and36 API tests PASS. Staged in PR214; final cumulative generation/CI/merge and native consumer acceptance pending.)
- F-unbound-chrony-syslog — Wave B (day 10-12): Unbound DNS, chrony NTP, syslog export + log explorer (review, unassigned; Combined sealed credential source9d61005d independently R2-approved; R4 CA-key isolation correction approved. Eight focused race packages and36 API tests PASS. Staged in PR214; final cumulative generation/CI/merge and native consumer acceptance pending.)
- F-host-stack — Wave C (day 13-15): expose host-stack config: session layer, TCP/UDP tuning, TLS, QUIC, HTTP static, HTTP/3 proxy (review, unassigned; Combined sealed credential source9d61005d independently R2-approved; R4 CA-key isolation correction approved. Eight focused race packages and36 API tests PASS. Staged in PR214; final cumulative generation/CI/merge and native consumer acceptance pending.)
- F-snmp — Wave C (day 13-15): SNMP v2c/v3 via snmpd renderer + private MIB (review, unassigned; Combined sealed credential source9d61005d independently R2-approved; R4 CA-key isolation correction approved. Eight focused race packages and36 API tests PASS. Staged in PR214; final cumulative generation/CI/merge and native consumer acceptance pending.)
- TD-19 — Install & lab provisioning from product artifacts (review, unassigned; Reviewed complete Python release pins/provenance and offline wheel materialization in PR212; staged in completion PR214. Final aggregate CI/integration pending; real Ubuntu26.04 install/boot remains deferred. No live developer implied.)
- F-setup-wizard — First-boot setup wizard: language/time, admin password, WAN (DHCP/static/PPPoE), LAN + DHCP, safe defaults, one commit (running, Codex completion campaign 2026-10-04; Accepted wizard prompt includes PPPoE when client source is merged. Reopened to connect additive PPPoE setup input/UI to distinct logical WAN carrier, sealed credential references and safe rerun/collision behavior; source worker assigned. Existing DHCP/static proofs retained; final source review and CI pending.)
- F-pppoe-client-host — PPPoE client on the lab: pppd vs an accel-ppp/rp-pppoe server, VPP FIB mirror, reconnect, MSS clamp, screenshot (running, unassigned; Kernel-carrier, exclusive raw-parent transport, distinct logical transit identity, PD LAN application and WAN readiness integration actively being implemented on published isolated checkpoints. PR210 lifecycle source staged in PR214; not lab-only and not complete.)
- F-multiwan-host — Multi-WAN on the lab: two WAN netns, failover time, balance split, per-member NAT (running, dashboard_finish; DHCP learned gateway source independently approved and staged. Worker wan_complete resumed to connect verified PPP carrier gateway and bounded namespace probe; source is not yet lab-only.)

## Parked

- F-ra-vpn — parked_on: native supplier/session/identity and EAP TLS acceptance; source integrated, operational negative receipts require diagnosis
- P12-fib-proof — parked_on: PR180 HOLD: current mgmtd startup failed at unchanged 30s deadline before 200-route proof
- F-global-blocking-host — parked_on: lab topology/host acceptance prerequisite; NOTRUN
- F-nat46-host — parked_on: remaining live packet/FIB/restart/API acceptance
- F-ospf-host — parked_on: remaining live packet/FIB/restart/API acceptance
