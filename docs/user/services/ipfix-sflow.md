# Flow export — IPFIX (flowprobe) and sFlow

**Screen:** *Services → Flow export* (`/services?tab=flow-export`), with three sections: **Exporters**, **Flowprobe**
and **sFlow**. **REST:** `GET /api/v1/state/ipfix` (live) and the generic configuration routes under
`/api/v1/config/services` (`services.ipfix`). **CLI:** `vrx show configuration services ipfix`, `vrx merge services ipfix …`,
`vrx commit` (`docs/user/cli/reference.md`).

Two independent mechanisms of the data plane (VPP) are configured here:

- **IPFIX flow records** (VPP `flowprobe` plugin + the IPFIX exporter): per-flow records (addresses, ports, protocol,
  byte/packet counts) of the traffic on the monitored interfaces, sent to an IPFIX collector (UDP, default port 4739).
- **sFlow packet sampling** (VPP `sflow` plugin): 1-in-N random packet samples on the selected interfaces.

> **sFlow export is not available yet.** VPP samples the packets, but sending the samples to sFlow collectors is the
> job of the `hsflowd` daemon, which this release does not ship. The screen shows a banner about it; the collectors,
> agent address and VRF you enter are kept in the configuration for when `hsflowd` is packaged. IPFIX export works
> without it.

## Exporters

Each exporter names a **collector** (address and UDP port), the **source address** the records are sent from (an
address configured on an interface of the exporter's VRF), the **VRF**, the path MTU and the template interval.

The first **enabled** exporter with an **IPv4** collector (in name order) becomes VPP's *exporter 0* — the only exporter
flowprobe records are sent through (the screen marks it **Exporter 0**). Further exporters are created as additional
VPP exporters for other record producers (for example NAT logging). VPP identifies exporters by collector address, so
two enabled exporters cannot use the same collector address. A disabled exporter stays in the configuration and is not
applied. The status column shows whether the data plane has the exporter (**Active**) and, for additional exporters,
their stats index when known.

## Flowprobe (IPFIX flow records)

- **Timers:** active timer (a long-lived flow is exported every *N* s) and passive timer (a flow is closed after *N* s
  without packets). The passive timer must not be shorter than the active timer.
- **Record fields:** L2 (MACs, EtherType), L3 (addresses, protocol), L4 (ports, TCP flags).
- **Monitored interfaces:** pick an interface, the **variant** and the **direction** (receive, transmit, both). VPP
  records **one variant per interface** — IPv4, IPv6 or L2.
  **Under the configuration default (IPv4 and IPv6 both on) IPv6 flows are NOT recorded:** the entry is applied as
  IPv4 only and the agent reports a warning. To record IPv6 on an interface, turn IPv4 off for it (D-146).

Monitored interfaces need an enabled exporter with an IPv4 collector: a commit without one is refused with
*"flowprobe records are sent through IPFIX exporter 0 only: enable an exporter with an IPv4 collector"*
(400, pointer `/services/ipfix/flowprobe/interfaces`).

## sFlow

> **No sFlow collector receives anything in this release.** VPP only samples; export to collectors needs `hsflowd`,
> which is not packaged yet (P10 follow-up). The collectors, agent address and VRF are stored for that.

Enable sFlow, set the **sampling rate** (1 in *N* packets), the counter **polling interval** and the **sampled header
size** (64–256 bytes in steps of 32; VPP would silently round any other value), and pick the **sampled interfaces**.
The lower table shows the interfaces VPP samples on and the sFlow node counters (packets processed, sampled, dropped)
from the statistics segment.

## Example: IPFIX for the LAN, sFlow 1:1000

LAN interface `lan0` (10.1.1.1/24), collector 10.1.1.9 (IPFIX on port 4739, sFlow on 6343):

```
vrx merge services ipfix '{
  "exporters": {"lan": {"collector": {"address": "10.1.1.9", "port": 4739}, "sourceAddress": "10.1.1.1", "vrf": "default"}},
  "flowprobe": {"activeTimerSec": 15, "passiveTimerSec": 120,
                "interfaces": [{"interface": "lan0", "direction": "both", "ip4": true, "ip6": false}]},
  "sflow": {"enabled": true, "samplingN": 1000, "headerBytes": 128,
            "collectors": [{"address": "10.1.1.9", "port": 6343}], "interfaces": ["lan0"]}}'
vrx commit confirm 120 comment "flow export for the LAN"
vrx confirm
vrx show configuration services ipfix
```

On the data plane (for support): `vppctl show flowprobe params`, `vppctl show flowprobe interface`, `vppctl show sflow`.
`GET /api/v1/state/ipfix` returns the same as the screen: exporters (exporter 0 first), flowprobe parameters and
interfaces, sFlow parameters, interfaces and counters.

## Notes

- **VPP-wide settings.** Exporter 0, the flowprobe parameters and the sFlow parameters are global to the data plane.
  On a box they are set by the product agent. An agent that is not the owner of these settings (a lab/test agent) only
  checks that the data plane already has the configured values and never changes them; the screen then shows a warning.
- **Changing flowprobe parameters** takes the monitored interfaces down briefly: VPP accepts new parameters only while no
  interface has flowprobe enabled, so the agent disables and re-enables them around the change.
- **After an agent restart** the agent re-enables sFlow once on each sampled interface to re-learn VPP's interface
  index mapping (VPP's sFlow API reports only hardware indexes; `docs/vpp-code-track.md` V17). Sampling continues.
- Classify-based IPFIX reports and NAT logging enables are not configured on this screen.
