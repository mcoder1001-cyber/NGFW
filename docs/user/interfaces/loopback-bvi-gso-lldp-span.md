# Loopbacks as BVI, GSO, port mirroring, LLDP and the delay simulator

**Screens:** Interfaces → **Interfaces** (loopbacks, the GSO switch and the mirror sessions in the interface drawer),
Interfaces → **LLDP** (`/interfaces/lldp`), Interfaces → **Port mirroring** (`/interfaces/mirroring`), Tools →
**Delay simulator (lab)** (`/tools/nsim`). **REST:** configuration through the generic pointer routes
(`/api/v1/config/interfaces/<if>/gso|mirror`, `/api/v1/config/services/lldp`, `/api/v1/config/services/nsim`); live
state `GET /api/v1/state/lldp/neighbors?page&pageSize` and, for GSO and mirror sessions, the `config` of each item of
`GET /api/v1/state/interfaces`. **CLI:** `vrx set|merge|delete interfaces <if> gso|mirror …`,
`vrx set|merge services lldp …`, `vrx set|merge|delete services nsim …` (no `show` command for the LLDP table yet — use
the REST route).

| what | configuration | VPP |
|---|---|---|
| loopback | `interfaces.loop<N>` (any N except 16000–16383, which the agent reserves for its quarantine holders) | `create_loopback_instance` (tagged `<owner>:loop<N>`) |
| loopback as bridge BVI | `interfaces.loop<N>.l2 = {bridgeDomain, bvi: true}` ([Bridging](bridge-l2.md)) | `sw_interface_set_l2_bridge` port type BVI |
| GSO | `interfaces.<if>.gso: true` | `feature_gso_enable_disable` (software segmentation on output) |
| port mirroring (SPAN) | `interfaces.<source>.mirror[] = {destination, direction: rx\|tx\|both, level: device\|l2}` | `sw_interface_span_enable_disable` |
| ERSPAN | a mirror whose `destination` is a GRE tunnel of type `erspan` | the same, to the tunnel interface |
| LLDP | `services.lldp = {enabled, systemName, txHold, txIntervalSec, interfaces[]}` | `lldp_config`, `sw_interface_set_lldp` |
| delay simulator (lab) | `services.nsim = {delayMs, bandwidthMbps, packetSize, dropFraction, crossConnect{a,b}, outputInterfaces[]}` | `nsim_configure2`, `nsim_cross_connect_enable_disable`, `nsim_output_feature_enable_disable` |

Rules checked before anything is applied (400 problem+json with the JSON pointer of the offending field):

- a mirror destination exists (an interface, a sub-interface or a tunnel of the configuration), is not its own source
  (`/interfaces/<if>/mirror/<i>/destination`), and is not itself a mirror source (no mirror loops); one session per
  destination and level;
- `gso` exists on parent interfaces only (a sub-interface has no such key); not on `local0`;
- `loop16000`–`loop16383` are refused (`/interfaces/<key>`);
- nsim: delay 0.001–10000 ms, bandwidth 0.001–100000 Mbit/s, packet size 64–9000, loss 0–1; the scheduler wheel
  (delay × bandwidth / 8 / packet size) at most 2^20 slots (32 MiB per thread); cross-connect and output interfaces exist,
  are hardware interfaces (no sub-interfaces) and differ;
- an LLDP interface exists.

## Loopbacks and the BVI

A loopback is created by naming it: `interfaces.loop720 = {enabled: true, ipv4: ["10.7.20.1/24"]}`. Joined to a bridge
domain as its BVI, it is the routed gateway of the bridged segment:

```json
{
  "interfaces": { "loop720": { "enabled": true, "ipv4": ["10.7.20.1/24"], "l2": { "bridgeDomain": "lan", "bvi": true } } },
  "routing": { "l2": { "bridgeDomains": { "lan": { "id": 7001 } } } }
}
```

`vppctl show bridge-domain 7001 detail` lists the loopback with `BVI *`. See [Bridging](bridge-l2.md) for members,
VLAN tag rewrite and cross-connects.

## GSO

`gso: true` enables VPP's software generic segmentation on the interface's output: large TCP segments that arrive from a
GSO-capable virtual interface (tap, virtio, af_packet, the host stack) are cut into MSS-sized packets before they leave an
interface that cannot segment them itself. The switch is in the interface drawer (group *loopback-bvi-gso-lldp-span*).
Checksum/TSO offloads of DPDK NICs are start-up settings, not this switch.

```
vrx set interfaces loop720 gso true
vrx commit comment "gso"
```

`vppctl show interface features loop720` lists `gso-ip4` under `ip4-output` (and the `gso-l2-*` nodes).

## Port mirroring and ERSPAN

Every session lives on its **source** interface. `direction` picks received (`rx`), transmitted (`tx`) or both frames;
`level: device` copies every frame of the interface, `level: l2` only frames in the L2 path (a bridge member or an L2
cross-connect). The Port mirroring page lists every session of the configuration with its live status (*active* = VPP
mirrors it) and a *pending* mark for uncommitted changes.

```json
{ "interfaces": { "loop720": { "mirror": [
  { "destination": "loop721", "direction": "both", "level": "device" },
  { "destination": "gre7",    "direction": "rx",   "level": "device" }
] } } }
```

The second session is **ERSPAN**: `gre7` is a GRE tunnel of type `erspan` (`tunnels.gre.<name> = {instance: 7, type:
"erspan", sessionId: 1, src, dst}`), so the copies travel encapsulated to the collector at the tunnel's destination.

```
vrx merge interfaces loop720 '{"mirror":[{"destination":"loop721"},{"destination":"gre7","direction":"rx"}]}'
vrx delete interfaces loop720 mirror
```

`vppctl show interface span` shows the sessions:

```
Source                           Destination                       Device       L2
loop775                          loop776                          (  both) (  none)
                                 gre778                           (    rx) (  none)
```

A mirror session is removed before either of its interfaces (VPP keeps span state of a deleted interface — the agent
never relies on VPP's cleanup).

## LLDP

```json
{ "services": { "lldp": { "enabled": true, "systemName": "vrx-a", "txHold": 4, "txIntervalSec": 30,
  "interfaces": [ { "interface": "TenGigabitEthernet0/0/0", "portDescription": "uplink", "mgmtIpv4": "192.0.2.1" } ] } } }
```

```
vrx merge services lldp '{"enabled":true,"interfaces":[{"interface":"TenGigabitEthernet0/0/0","portDescription":"uplink"}]}'
vrx commit comment "lldp"
```

- `systemName`, `txHold`, `txIntervalSec` are VPP-wide: only the product agent that owns the VPP-global settings applies
  them; on a lab slot they are reported as not applied and VPP keeps its values. An unset `systemName` keeps VPP's
  current name (VPP starts without one).
- LLDP has no getter in VPP: the configuration is applied write-only and is never read back; the **neighbour table**
  (`GET /api/v1/state/lldp/neighbors`, the LLDP page) shows every LLDP interface with the peer heard on it — chassis id,
  port id, TTL and when it was last heard (`vppctl show lldp` shows the same).
- **Use LLDP only on interfaces created at start-up (the data NICs).** VPP 26.06 enables LLDP by the *hardware*
  interface index but is given the *software* index. On interfaces created at start-up both are equal. On an interface
  created later (loopbacks, tunnels, anything after sub-interfaces were made) they can differ, and then **VPP enables
  LLDP on another hardware interface** — possibly another port of the box. The agent detects it right after the enable
  and reports `ErrIndexMismatch` (the commit fails), but it cannot undo it: no API message addresses that entry, so LLDP
  keeps running on the other interface until VPP restarts. The binary API exposes no hardware index, so the agent
  cannot check before the enable.

The table refreshes every 30 s (a walk of the VPP table holds its workers briefly); **Refresh** asks at once.

## The delay simulator (lab tool)

VPP's `nsim` adds delay, a bandwidth limit and random loss between two cross-connected interfaces or on the output of
interfaces — for lab tests of timeouts, retransmission and VPN behaviour, **not a product feature**, and **off by
default**:

- It is VPP-wide (one model, one cross-connect pair), so only the agent that owns the VPP-global settings applies it,
  and only with the lab switch **`VRX_NSIM=lab`** in the environment of the agent and of the API. Without it the API
  refuses a commit (or a rollback) that carries `services.nsim` with **409** (pointer `/services/nsim`) and records
  the attempt in the audit log (failure, 409, reason `nsim-disabled`), and an agent reports it as not applied.
- **Configuring it keeps VPP's main thread polling (one busy core) until VPP restarts** — VPP sets the simulator's
  input node to polling and never back. VPP cannot read the model back and cannot unconfigure it: removing
  `services.nsim` only detaches the interfaces.
- On a VPP with worker threads, VPP gives the main thread no scheduler wheel unless startup.conf carries
  `nsim { poll-main-thread }`, and a frame the main thread sends through the simulator (a ping from the API, other
  control traffic) would then crash VPP. The agent refuses nsim on a VPP with workers unless the operator confirms that
  setting with `VRX_NSIM_POLL_MAIN_THREAD=1`.
- The model is bounded (at most 2^20 scheduler slots, 32 MiB per thread); VPP would crash on a failed allocation, so
  both the API and the agent refuse more.

```json
{ "services": { "nsim": { "delayMs": 50, "bandwidthMbps": 100, "dropFraction": 0.001,
  "crossConnect": { "a": "TenGigabitEthernet0/0/3", "b": "TenGigabitEthernet0/0/4" } } } }
```

```
vrx merge services nsim '{"delayMs":50,"bandwidthMbps":100,"crossConnect":{"a":"TenGigabitEthernet0/0/3","b":"TenGigabitEthernet0/0/4"}}'
vrx delete services nsim
```

`vppctl show nsim` shows the model and the cross-connect. Run it in a lab, ideally shortly before a planned VPP restart.

## After a restart

The agent restores loopbacks, the BVI membership, mirror sessions, GSO and LLDP by itself: in the host check the agent
was stopped, every one of them deleted from VPP behind its back, and everything was back 0.61 s after the agent
started (reconcile 0.428 s in the agent log) with no API call. GSO and nsim are applied once per VPP instance (VPP would otherwise stack them).
