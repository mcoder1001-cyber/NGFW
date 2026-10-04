# IS-IS, RIPv2 and RIPng

Configure Routing → IS-IS and RIP, choose the protocol tab, save to candidate, then commit through the pending changes bar. Interfaces must exist and have an LCP mapping; the agent maps their VPP names to Linux interfaces. The interface table summarizes candidate configuration, while the live grid reads independent observed FRR state every five seconds. An unavailable reader is displayed explicitly and never treated as an empty successful table. Peer observations currently cover the default VRF; protocol-filtered routes and IS-IS observations cover all VRFs supported by FRR's JSON command.

## Level-2 IS-IS

```json
{"routing":{"isis":{"net":"49.0001.1921.6800.1001.00","level":"level-2","interfaces":{"loop0":{"ipv4":true,"ipv6":true,"circuitType":"level-2","networkType":"point-to-point","metric":20}}}}}
```

Equivalent rendered commands, when loop0 maps to eth0:

```text
interface eth0
 ip router isis ngfw
 ipv6 router isis ngfw
 isis circuit-type level-2-only
 isis network point-to-point
 isis metric 20
exit
router isis ngfw
 is-type level-2-only
 net 49.0001.1921.6800.1001.00
exit
```

Omitted family switches enable both IPv4 and IPv6. Both cannot be disabled. A level-1 instance rejects a level-2 circuit at `/routing/isis/interfaces/<name>/circuitType`; the converse also rejects unsupported level-1 circuits. The model contains one process with one NET, so duplicate local NET system IDs cannot be represented. Choose the last three groups (the system ID) uniquely across your routing domain; the configuration validator cannot discover another box's ID.

Area/domain authentication uses `areaPasswordRef` and `domainPasswordRef`, each a `password/<name>` secret reference. The renderer emits `area-password md5 <resolved-key>` / `domain-password md5 <resolved-key>` through its redacting resolver. Production password delivery is pending the approved routing secret channel: configured authentication fails closed when the resolver is unavailable, with a precise reference pointer. Fixture tests do not establish live password delivery.

IS-IS operates over raw OSI/CLNS Layer 2. Linux IP routes alone do not establish that frames reach FRR. The `lcp.osi-proto` descriptor can enable protocol 0x83 through VPP's generated OSI API only for the designated globals owner with `NGFW_ISIS_OSI_ENABLE=1`. The default is disabled with a configuration warning. The operator must approve an exclusive globals maintenance window before enabling it on a shared box. No enable was executed for this implementation.

**VPP 26.06 has no OSI disable API.** Configuration removal or rollback removes FRR sections but cannot revoke the global punt. The descriptor's Delete is an explicit no-op and readback uses `lcp_osi_proto_get`; only a separately authorized VPP restart clears the switch. This opt-in has no packet/FIB acceptance claim.

## RIPv2

```json
{"routing":{"rip":{"version":2,"networks":["192.0.2.0/24"],"interfaces":{"loop0":{"passive":false}},"defaultMetric":1,"redistribute":{"static":{"metric":2}}}}}
```

```text
router rip
 version 2
 default-metric 1
 network 192.0.2.0/24
 network eth0
 redistribute static metric 2
exit
```

Only version 2 is accepted. Networks must be IPv4 canonical prefixes, with host bits clear and no duplicates. Passive interfaces still advertise their connected networks but send no periodic updates.

## RIPng

```json
{"routing":{"ripng":{"networks":["2001:db8::/64"],"interfaces":{"loop0":{"passive":true}},"redistribute":{"static":{"metric":2}}}}}
```

```text
router ripng
 default-metric 1
 network 2001:db8::/64
 network eth0
 passive-interface eth0
 redistribute static metric 2
exit
```

RIPng has a separate IPv6-only configuration and no `version 2` command. The schema uses the existing `ospf` source leaf for IPv6 redistribution, which renders as FRR `ospf6`. Redistributing RIPng into other protocols remains outside scope.

Read-only endpoints:

- `/api/v1/state/routing/isis/adjacencies`
- `/api/v1/state/routing/isis/database?offset=0&limit=100`
- `/api/v1/state/routing/rip/peers?version=2` or `version=ng`
- `/api/v1/state/routing/rip/routes?version=ng&offset=0&limit=100`

Readers have fixed commands and output limits; API responses project bounded public fields and page at most 100 rows. RIP routes here are FRR RIB observations. Use the existing VPP route browser (`/state/routes?proto=isis|rip`) for forwarding observations; FRR observations alone do not prove packet forwarding. Live 20-prefix FIB, withdrawal, restart, rollback and real-endpoint browser acceptance are deferred to an isolated authorized laboratory run.

CLI grammar references: [FRR IS-IS](https://docs.frrouting.org/en/latest/isisd.html), [FRR RIP](https://docs.frrouting.org/en/stable-10.2/ripd.html), [FRR RIPng](https://docs.frrouting.org/en/stable-10.2/ripngd.html). Peer parser formatting is based on FRR 10.2 `rip_peer_display` / `ripng_peer_display`; unsupported format versions are reported unavailable rather than guessed.

## RIPv2 MD5 authentication

Set `routing.rip.interfaces.<name>.auth` to
`{"type":"md5","keyId":7,"keyRef":"password/rip-peer"}` through the schema-driven
IS-IS/RIP form or the ordinary `/api/v1/config/routing` candidate endpoint. The key ID
must be 1–255 and reference a password secret. The resolved key must be 1–16 bytes,
a single printable CLI token; secret values never appear in candidate GET responses,
renderer previews or logs. The FRR CLI equivalent is an owned `key chain` with
`key <id>` / `key-string <secret>` plus interface commands
`ip rip authentication mode md5` and `ip rip authentication key-chain <chain>`.
RIPv1 stays disabled (`version 2`); plaintext authentication is unsupported.
Set `auth` to `{"type":"none"}` or remove it to remove both interface authentication
commands and the owned key chain from the next complete desired configuration.
RIPng has no RIPv2 authentication and rejects this setting.

OSPF and RIPv2 MD5 references use the same transaction-selected, socket-only
sealed password cache already wired for IS-IS. Only configured MD5 authentication
leaves are delivered; RIPng and unauthenticated leaves never select a secret.
An unavailable password fails closed. Secret selection, rendering and redaction
are tested; live authenticated routing, packet exchange and daemon rollback
require lab acceptance. The historical pending decision does not constitute
evidence that these source paths were tested on live peers.
CLI syntax reference: https://docs.frrouting.org/en/stable-10.6/ripd.html#rip-authentication.
