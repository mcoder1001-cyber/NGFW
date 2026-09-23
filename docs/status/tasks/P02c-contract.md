# P02c — contract changes (schema group (c) + proto sync, D-061)

Branch `task/P02c` (commits `contract(schema): …` and `contract(proto): …` on top of main). The schema is not tagged yet
(`contracts-v1` pending), so reshapes decided by the manager (D-050…D-054) and the review (F4, F5, F7, F9) are applied now rather than as
always-PENDING changes after the tag. Field-by-field reference: `docs/contracts/schema-vpn-tunnels-services-ha.md`.

## packages/schema (`contract(schema): model vpn, tunnels, services and ha …`)

| change | kind | decision |
|---|---|---|
| `ha.vrrp` array → record keyed by name | reshape (pre-tag) | D-053, F9 |
| group-(c) sub-trees `.prefault({})` / `.default({})` (`RootConfig.parse({})` fully populated) | normalised form changes; JSON Schema additive | D-017, D-053, F5 |
| secret references `<kind>/<name>`, kinds `psk\|key\|cert\|password\|token`, each field pins its kind | pattern tightened (pre-tag) | D-051, F7 |
| `vpn.ipsec.tunnels.*.underlayVrf`, `vpn.remoteAccess.*.underlayVrf`; `vrf` = overlay | additive field + meaning pinned | F4 |
| `services.qos` (policers, shapers, maps, interfaces) | additive | D-052 |
| `services.ntp` extended (deny, port, rateLimit, orphan, rtcSync, ntsServer); only NTP model | additive; `system.ntp` removed by P02a | D-050, F3 |
| exports `ServicesDnsSchema`, `ServicesNtpSchema`, `ServicesNtpServerSchema` (were `DnsSchema`, `NtpSchema`, `NtpServerSchema`) | TS export rename (pre-tag) | D-047, F2 |
| helper primitives (`secretReference`, `transportPort`, `hostOrIpAddress`, `ipv4OrIpv6Cidr`, `wireguardKey`, `dnsName`, …) no longer exported | TS export removal (pre-tag) | D-054, F8 |
| all-numeric strings rejected where "IP or hostname" is accepted | pattern tightened | F12 |
| new semantic validators `vpn.wireguard-address-overlap`, `vpn.proposal-compatible`, `services.qos-references`, `services.qos-consistency`; changed keys of `vpn.ipsec-peer-unique` (F6), `tunnels.endpoints-unique` (F11), `tunnels.address-overlap` (F10), relay source VRF (F13) | tier (b) rules | F6, F10, F11, F13, F14 |

## packages/proto (`contract(proto): sync vpn/tunnels/services/ha messages …`)

| message / field | kind |
|---|---|
| `GreTunnel` 1–13, `VxlanTunnel` 1–16, `IpipTunnel` 1–13 (were empty) | additive |
| `DhcpService`, `DnsService`, `SnmpService`, `LldpService`, `IpfixService` filled (were empty); new `DhcpServer`, `DhcpRelay`, `DhcpSubnet`, `DhcpPool`, `DhcpOption`, `DhcpReservation`, `Dns*`, `SocketAddress` | additive |
| `ServicesConfig.ntp = 6` (`NtpService`), `ServicesConfig.qos = 7` (`QosService`, `QosPolicer`, `QosPolicerAction`, `QosShaper`, `QosMap`, `QosMapEntry`, `QosInterface`) | additive |
| `HaConfig.vrrp`: `repeated VrrpInstance = 1` → `map<string, VrrpInstance> = 2`, `reserved 1` | **reshape** (pre-tag, D-053; new number so no old bytes are misread) |
| `HaConfig.cluster = 3` (`HaCluster`), `VrrpInstance` 1–14 (was empty) | additive |
| `IpsecTunnel.underlay_vrf = 26`, `RemoteAccessProfile.underlay_vrf = 16` | additive (F4) |
| `test/fixtures/all-domains.json`: group-(c) domains populated leaf-for-leaf, secret refs in `<kind>/<name>` form, `closeAction` a valid value, 64-bit leaves as strings | fixture |
| `apps/agent/internal/contracttest/desiredstate_test.go:472`, `packages/proto/test/desired-state.test.ts:234-236` | one-line literal updates forced by the `vrrp` reshape (outside my file set; needed for a green gate) |

Proof: `TestStrictDecodeOfEveryDocument` decodes every valid `packages/schema/examples/*.json` (all 11 group-(c) documents included) and
`fixtures/all-domains.json` strictly and round-trips them key-for-key; the TS mirror test does the same (`docs/status/tasks/P02c.md`,
"Review fixes").
