# Progress

Updated 2026-10-08 from plan/tasks.yaml (estimated hours are the plan's, not actuals).

**Overall: 93.6% by hours (1477.0/1578.5 h), 93.4% by tasks (198/212)**

| state | tasks |
|---|---|
| merged | 198 |
| review | 2 |
| running | 7 |
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
| S4 | 934.0 / 1003.0 | 93.1% | 141/151 | 7 | 0 | 3 |
| S5 | 142 / 155.5 | 91.3% | 14/16 | 0 | 0 | 1 |
| S6 | 48 / 48 | 100.0% | 4/4 | 0 | 0 | 0 |

Merged measures reviewed source completion; deferred lab acceptance is not PASS. Running describes remaining implementation, not verified worker activity.

## Remaining implementation / review

- P08 — Vertical slice: interfaces end to end (af_packet rig) (review, ngfw-46 slot1; IP unnumbered source675cd9ea independently R4-approved and staged in PR214 (remote036ca641). Final cumulative CI/merge and native VPP acceptance pending.)
- F-wireguard — Wave B (day 10-12): WireGuard peers/keys (running, unassigned; Production sealed-generation delivery and WG lifecycle tests implemented in secrets-complete; shared API key-delivery security review found a CA-key boundary defect in syslog selection. Correction and combined integration required; historical fixture-only secret path is not completion.)
- P12 — Wave B (day 10-12): FRR + linux-cp framework, BGP (running, unassigned; BGP MD5 production sealed credentials and historical-generation rollback source followup in secret-consumers. Independent final review/integration/CI pending; native mgmtd/FIB proof remains separate.)
- F-unbound-chrony-syslog — Wave B (day 10-12): Unbound DNS, chrony NTP, syslog export + log explorer (running, unassigned; Version-pinned NTP and TLS syslog consumers implemented in secret-consumers; independent review and central startup/projection wiring pending. TLS client-key delivery needs paired leaf-certificate validation before decryption.)
- F-host-stack — Wave C (day 13-15): expose host-stack config: session layer, TCP/UDP tuning, TLS, QUIC, HTTP static, HTTP/3 proxy (running, unassigned; Namespace secret references now have source binding to existing sealed generations; focused lifecycle checks in progress. Central integration/review/CI pending.)
- F-snmp — Wave C (day 13-15): SNMP v2c/v3 via snmpd renderer + private MIB (running, unassigned; Production sealed-generation v2c/v3 bindings and rotation/recovery tests implemented on secrets-complete; final shared selector review, integration and CI pending.)
- TD-19 — Install & lab provisioning from product artifacts (review, unassigned; Reviewed complete Python release pins/provenance and offline wheel materialization in PR212; staged in completion PR214. Final aggregate CI/integration pending; real Ubuntu26.04 install/boot remains deferred. No live developer implied.)
- F-pppoe-client-host — PPPoE client on the lab: pppd vs an accel-ppp/rp-pppoe server, VPP FIB mirror, reconnect, MSS clamp, screenshot (running, unassigned; Kernel-carrier, exclusive raw-parent transport, distinct logical transit identity, PD LAN application and WAN readiness integration actively being implemented on published isolated checkpoints. PR210 lifecycle source staged in PR214; not lab-only and not complete.)
- F-multiwan-host — Multi-WAN on the lab: two WAN netns, failover time, balance split, per-member NAT (running, dashboard_finish; DHCP learned gateway source independently approved and staged. Worker wan_complete resumed to connect verified PPP carrier gateway and bounded namespace probe; source is not yet lab-only.)

## Parked

- F-ra-vpn — parked_on: native supplier/session/identity and EAP TLS acceptance; source integrated, operational negative receipts require diagnosis
- P12-fib-proof — parked_on: PR180 HOLD: current mgmtd startup failed at unchanged 30s deadline before 200-route proof
- F-global-blocking-host — parked_on: lab topology/host acceptance prerequisite; NOTRUN
- F-nat46-host — parked_on: remaining live packet/FIB/restart/API acceptance
- F-ospf-host — parked_on: remaining live packet/FIB/restart/API acceptance
