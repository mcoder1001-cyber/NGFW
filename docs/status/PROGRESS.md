# Progress

Updated 2026-10-11 from plan/tasks.yaml (estimated hours are the plan's, not actuals).

**Overall: 99.2% by hours (1566.5/1578.5 h), 99.1% by tasks (210/212)**

| state | tasks |
|---|---|
| merged | 210 |
| review | 0 |
| running | 2 |
| ready | 0 |
| parked | 0 |
| failed | 0 |
| todo | 0 |

| stage | merged h / total h | % | tasks merged/total | running | ready | parked |
|---|---|---|---|---|---|---|
| S0 | 6 / 6 | 100.0% | 1/1 | 0 | 0 | 0 |
| S1 | 73 / 73 | 100.0% | 9/9 | 0 | 0 | 0 |
| S2 | 274 / 274 | 100.0% | 29/29 | 0 | 0 | 0 |
| S3 | 19 / 19 | 100.0% | 2/2 | 0 | 0 | 0 |
| S4 | 1001.0 / 1003.0 | 99.8% | 150/151 | 1 | 0 | 0 |
| S5 | 145.5 / 155.5 | 93.6% | 15/16 | 1 | 0 | 0 |
| S6 | 48 / 48 | 100.0% | 4/4 | 0 | 0 | 0 |

Merged measures reviewed source completion; deferred lab acceptance is not PASS. Running describes remaining implementation, not verified worker activity.

## Remaining implementation / review

- F-ra-vpn — remote-access VPN IKEv2+EAP (running, routing_acceptance; full canonical appliance acceptance remains open; bounded transport repair is reviewed and tested)
- F-pppoe-client-host — PPPoE client on the lab: pppd vs an accel-ppp/rp-pppoe server, VPP FIB mirror, reconnect, MSS clamp, screenshot (running, wan_acceptance; full acceptance remains open: native NAT transit-address selection and .195 original11-second peer recovery failure)

## Parked

- none
