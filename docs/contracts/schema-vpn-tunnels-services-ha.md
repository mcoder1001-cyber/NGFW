# Schema contract — `vpn`, `tunnels`, `services`, `ha` (group (c), P02c)

Part of the `packages/schema` contract (index: `docs/contracts/schema.md`, D-024). Source of truth:
`packages/schema/src/domains/{vpn,tunnels,services,ha}.ts`; cross-object rules: `packages/schema/src/semantic/{vpn,tunnels,services,ha}.ts`.
**Changing this package requires a `contract/<id>` branch with a `contract(schema): …` commit** (enforced by `tools/ci.sh --base main`);
renaming or reshaping an existing field is always a PENDING decision (`docs/decisions/decision-policy.md` #1).

## Conventions shared by the four domains

- **Populated domain roots (D-017, D-053).** Every sub-tree is `.prefault({})` / `.default({})`: `RootConfig.parse({}).vpn` is
  `{ ipsec: { settings: {…}, proposals: {}, tunnels: {} }, wireguard: { interfaces: {} }, pki: { cas: {}, certificates: {} }, remoteAccess: {} }`,
  `tunnels` is `{ gre: {}, vxlan: {}, ipip: {} }`, `services` fills `dhcp, dns, snmp, lldp, ipfix, ntp, qos` (disabled defaults), `ha` is
  `{ vrrp: {} }`. Only sub-trees with required fields stay optional: `pki.hsm`, `dns.vppCache`, `ipfix.sflow`, `ha.cluster`. Consumers read
  `config.vpn.ipsec.tunnels` directly.
- **Every object is `strictObject`.** Unknown keys are rejected, which is how inline `psk`, `password`, `privateKey`, `community` … fields fail.
- **Secrets are references (D-051).** Fields ending in `Ref` (`secretRef`, `privateKeyRef`, `presharedKeyRef`, `certificateRef`, `authRef`,
  `privRef`, `keyRef`, `passwordRef`, `pinRef`) hold exactly `<kind>/<name>` with `kind ∈ psk | key | cert | password | token` and
  `name = [A-Za-z0-9][A-Za-z0-9_.-]{0,62}`; each field pins its kind (IPsec/RADIUS/WireGuard PSKs `psk/`, private and symmetric keys `key/`,
  certificates `cert/`, EAP/SNMP/PIN passphrases and community strings `password/`). A pasted secret (`MyS3cretPSK2026`, a hex or base64
  key, a PEM block) is therefore a **schema error**. The schema checks the *format*; **the API (`POST /api/v1/secrets`, P06) checks that the
  reference exists** — that, not the schema, is the enforcement of rule #10. `vpn.no-inline-secret-material` is an extra guard for free-text
  fields: any PEM armour (`BEGIN <label>` between five-dash fences) anywhere, and hex (≥ 16 bytes) / padded base64 (≥ 16 bytes) blobs outside
  `publicKey`, `engineId`, `data` and `*Ref` fields, in all four domains.
- **VRF is first-class** (vdom.md #1): `vrf` (default `'default'`) on every tunnel, IPsec tunnel, WireGuard interface, remote-access
  profile, service instance and virtual router. **`vrf` is always the overlay** (the FIB of the tunnel interface / the protected traffic /
  the client pools) and **`underlayVrf` the FIB of the outer packets** (tunnel `src`/`dst`, IPsec `localAddr`/`remoteAddr`, WireGuard
  listen address) on every tunnel kind, IPsec tunnel, remote-access profile and WireGuard interface (F4). `default` always exists; any
  other name must be a key of `vrfs`.
- **Names are unique within their domain only** (vdom.md #2): record keys are `objectName` (`^[A-Za-z0-9][A-Za-z0-9_.-]*$`, ≤ 63); the
  `tunnels` domain enforces uniqueness across `gre`/`vxlan`/`ipip`.
- **Interface references** (`interface`, `interfaces[]`, `mcastInterface`, `track[].interface`) are VPP names (`vppInterfaceName`) resolved
  against `interfaces` (+ sub-interfaces `<parent>.<key>` / `<parent>.<vlanId>`), tunnels with an explicit `instance` (`gre<n>`, `ipip<n>`,
  `vxlan_tunnel<n>`) and WireGuard interfaces (`wg<instance>`). A tunnel without `instance` cannot be referenced.
- **Helper primitives are private (D-054).** `secretRefOf`/`secretReference`, `transportPort`, `hostOrIpAddress`, `resolvableHostname`,
  `ipv4OrIpv6Cidr`, `wireguardKey`, `dnsName`, `descriptionField` (≤ 255, no control characters, D-049), `enabledFlag`, `vrfRef`,
  `underlayVrfRef`, `u32Int`, `mtuField`, `httpsUrl`, `ikeIdentity` live in `domains/_shared/primitives.ts`, which `index.ts` does not
  re-export; only schemas, types and constants of the four domain files are package API. The later dedupe with P02a's `primitives.ts` is an
  internal refactor.
- **Hostnames vs mistyped addresses (F12).** Wherever a field accepts "IP or hostname" (`remoteAddr`, WireGuard endpoints, RADIUS/SNMP trap
  receivers, NTP servers/pools), an all-digits-and-dots string (`203.0.113.999`) is rejected instead of being resolved at apply time.

## `vpn`

| path | type / constraints | notes |
|---|---|---|
| `ipsec.settings.cryptoEngine` | `auto\|native\|ipsecmb\|openssl` (auto) | VPP crypto engine (D6.1) |
| `ipsec.settings.asyncCrypto` | boolean (false) | crypto workers / QAT |
| `ipsec.proposals{name}` | `{ description?, ike, esp }` | referenced by tunnels and remote-access profiles |
| `…ike` | `{ encr: IKE cipher, integ?: sha1\|sha256\|sha384\|sha512\|md5\|aesxcbc\|aescmac, prf?, dh: modp768…modp8192\|ecp192…ecp521\|modp*s*\|curve25519\|curve448 }` | `integ` required for classic ciphers, forbidden for AEAD (`aes*gcm*`, `chacha20poly1305`); strongSwan keywords |
| `…esp` | `{ encr: ESP cipher (adds `null`), integ?, dh? (PFS) }` | same AEAD rule |
| `ipsec.tunnels{name}` | see below | P11 shape: `localAddr, remoteAddr, localId, remoteId, auth{psk(secretRef)\|cert}, proposal, localTs[], remoteTs[], ikeVersion, dpd, natT, mode, rekey, startAction, vrf, routeBased{ipipInterface}` |
| `…enabled` / `description` | boolean (true) / ≤ 255 | |
| `…engine` | `strongswan\|vpp-ikev2` (strongswan) | `vpp-ikev2` requires `ikeVersion: 2` |
| `…ikeVersion` / `mode` / `protocol` | `1\|2` (2) / `tunnel\|transport` (tunnel) / `esp\|ah` (esp) | |
| `…localAddr` | ipAddress | must be configured on an interface in `underlayVrf` (semantic) |
| `…remoteAddr` | ipAddress \| hostname (not all-numeric) \| `%any` | `%any` = responder-only, `startAction` must not be `start`; same family as `localAddr`; ≠ `localAddr` |
| `…localId` / `remoteId` | printable ASCII ≤ 255, no `"` | FQDN, e-mail, IP, `@keyid`, DN |
| `…auth` | `{ method: 'psk', secretRef: psk/… }` \| `{ method: 'cert', certificate, remoteCa? }` | cert names → `pki.certificates` / `pki.cas` |
| `…proposal` | objectName | must exist in `ipsec.proposals` |
| `…localTs[]` / `remoteTs[]` | CIDR ≤ 64 each ([]) | policy-based: both non-empty; route-based: defaults to any |
| `…dpd` | `{ enabled (true), delaySec 1–86400 (30), timeoutSec (150), action clear\|trap\|restart (restart) }` | |
| `…natT` / `mobike?` / `fragmentation?` | boolean (true) / boolean / `yes\|no\|force\|accept` | MOBIKE needs IKEv2 |
| `…rekey` | `{ ikeSec 60–604800 (14400), espSec (3600), espBytes?, espPackets?, reauth (false) }` | |
| `…startAction` / `closeAction` | `none\|start\|trap` (start / none) | |
| `…vrf` | objectName (default) | **overlay**: FIB the traffic selectors apply to / the protected IPIP interface belongs to |
| `…underlayVrf` | objectName (default) | FIB IKE and ESP run in; `localAddr` is configured in it (F4) |
| `…routeBased?` | `{ ipipInterface: objectName }` | name in `tunnels.ipip`; tunnel mode only; semantic: IPIP `src` = `localAddr`, IPIP `underlayVrf` = `underlayVrf`, IPIP `vrf` = `vrf`, mode/dst agree, one IPsec tunnel per IPIP |
| `…esn` / `antiReplay` | boolean (false / true) | |
| `wireguard.interfaces{name}` | `{ enabled, description?, instance u32, vrf, underlayVrf, listenAddress, listenPort (51820), privateKeyRef: key/…, address[] ≤ 32 (no overlap with other interfaces of `vrf`), mtu (1420), peers{} }` | VPP interface `wg<instance>`; `listenAddress` must be configured in `underlayVrf` |
| `…peers{name}` | `{ description?, publicKey: wireguardKey, presharedKeyRef?: psk/…, endpoint? {address: host\|ip, port}, allowedIps[] 1–256, persistentKeepaliveSec 0–65535 (0) }` | public key unique per interface; an allowed prefix belongs to one peer |
| `pki.cas{name}` | `{ description?, certificateRef: cert/…, crl? {url?, refreshIntervalSec 300–2592000 (86400)}, ocspUrl? }` | D6.4 |
| `pki.certificates{name}` | `{ description?, certificateRef?: cert/…, privateKeyRef: key/…, ca?, acme? {directoryUrl, domains[] 1–100, email?, challenge http-01\|dns-01}, expiryAlertDays 1–365 (30) }` | `certificateRef` or `acme` required |
| `pki.hsm?` | `{ enabled, module (abs. path), tokenLabel?, pinRef?: password/… }` | PKCS#11 |
| `remoteAccess{name}` | `{ enabled, description?, localAddr (in `underlayVrf`), localId?, vrf (overlay: client pools), underlayVrf, auth eap-mschapv2\|eap-tls\|eap-radius\|pubkey, certificate, clientCa?, proposal, pools[] 1–16 {name, prefix, dns[] ≤ 4}, splitTunnel[], users[] {username, passwordRef: password/…}, radius? {servers[] {address, port (1812), secretRef: psk/…}}, dpd, rekey }` | D6.9; eap-radius needs `radius`, eap-mschapv2 needs users or radius, eap-tls/pubkey need `clientCa`; pools must not overlap across profiles |

Exports: `VpnSchema`/`VpnConfig`, `IpsecProposalSchema`, `IkeProposalSchema`, `EspProposalSchema`, `IpsecTunnelSchema`, `IpsecAuthSchema`,
`IpsecDpdSchema`, `IpsecRekeySchema`, `IpsecSettingsSchema`, `IpsecSchema`, `WireguardInterfaceSchema`, `WireguardPeerSchema`, `WireguardSchema`,
`PkiCaSchema`, `PkiCertificateSchema`, `PkiSchema`, `RemoteAccessProfileSchema`, `RemoteAccessPoolSchema`, `RemoteAccessUserSchema`,
`IPSEC_*` algorithm lists, `IPSEC_ANY_PEER`.

## `tunnels`

Common fields of every tunnel: `enabled` (true), `description?`, `instance?` (u32, fixes the VPP name), `src` (ipAddress; configured on an
interface in `underlayVrf`, not unspecified/multicast), `underlayVrf` (default), `vrf` (default; the tunnel interface's FIB), `mtu?`,
`ipv4[]`/`ipv6[]` (≤ 16 each, unique; not on L2 tunnels), `bridgeDomain?` (u32, L2 tunnels only).

| path | extra fields / constraints |
|---|---|
| `gre{name}` | `dst` (same family, ≠ src, not multicast), `type l3\|teb\|erspan` (l3), `sessionId 0–1023` (required iff erspan); `teb`/`erspan` are L2; endpoints unique per `(underlayVrf, src, dst, type, sessionId)` (F11) |
| `vxlan{name}` | `dst` (unicast peer or multicast group), `vni 0–16777215`, `srcPort`/`dstPort` (4789), `mcastInterface` (required iff `dst` is multicast), `decap l2\|ip4\|ip6` (l2; `l2` is L2) |
| `ipip{name}` | `mode p2p\|p2mp` (p2p), `dst?` (required for p2p, forbidden for p2mp), `dscp? 0–63` |

Exports: `TunnelsSchema`/`TunnelsConfig`, `GreTunnelSchema`, `VxlanTunnelSchema`, `IpipTunnelSchema`, `GRE_TUNNEL_TYPES`, `VXLAN_VNI_MAX`,
`TUNNEL_KINDS` (kind → VPP name prefix). VXLAN-GPE, GTP-U, L2TPv3, PPPoE (D6.6 "schedule last") are additive future keys.

## `services`

Kea servers and Unbound resolvers are **records of instances**, each with its own `vrf` and bind set (vdom.md #5).

| path | type / constraints |
|---|---|
| `dhcp.servers{name}` | `{ enabled, description?, family ipv4\|ipv6 (ipv4), vrf, interfaces[] 1–64 (unique, exist, in `vrf`), leaseTimeSec 60–2592000 (3600), renewTimerSec? < rebindTimerSec? < leaseTimeSec, authoritative (true), subnets{}, options[] }` — subnets match `family`, do not overlap within the server nor across servers of the same VRF, and lie within a prefix of one of `interfaces` |
| `…subnets{name}` | `{ description?, subnet CIDR, pools[] 1–32 {start, end} (inside subnet, ordered, non-overlapping), gateway? (v4 only, inside), dnsServers[]/ntpServers[] ≤ 4 (same family, unique), domainName?, domainSearch[] ≤ 8, leaseTimeSec?, options[] ≤ 64, reservations{} }` |
| `…reservations{name}` | `{ mac? \| duid? (exactly one; duid v6 only), ip (inside subnet, unique), hostname?, options[] }` — clients unique (case/separator-insensitive) |
| `…options[]` | `{ code 1–65535 (v4: ≤ 254), data printable ≤ 1024, alwaysSend (false) }`, codes unique per list |
| `dhcp.relays{name}` | `{ enabled, description?, family, vrf (client side), serverVrf? (= vrf), interfaces[] 1–64, servers[] 1–8 (family, unique), sourceAddress (family; configured in `serverVrf ?? vrf` — VPP `dhcp_proxy_config` uses it as the source towards the server in `server_vrf_id`, F13) }` |
| `dns.resolvers{name}` | `{ enabled, description?, vrf, listen[] 1–16 {address, port (53)} (unique; configured in `vrf` or wildcard; one resolver per socket per VRF), accessControl[] {prefix, action allow\|deny\|refuse\|allow_snoop}, forwarders[] ≤ 8 (upstream; not a listen socket), forwardZones[] ≤ 256 {zone, forwarders[] 1–8, forwardFirst}, localZones[] ≤ 256 {zone, type static\|transparent\|redirect\|refuse\|deny\|nodefault, records[] ≤ 4096}, dnssec {enabled (true), trustAnchorAuto (true)}, cache {minTtlSec ≤ maxTtlSec, prefetch, msgCacheMb, rrsetCacheMb}, threads 1–64, qnameMinimisation, hideIdentity, hideVersion, logQueries }` |
| `…upstream` | `{ address, port (53), tls (false), tlsServerName? (tls only) }` |
| `…records[]` | `{ name: dnsName, type A\|AAAA\|CNAME\|MX\|NS\|PTR\|SRV\|TXT, ttlSec 0–604800 (3600), data }` — `data` typed per record type |
| `dns.vppCache?` | `{ enabled (false), upstreams[] 1–8 }` (VPP dns plugin) |
| `snmp` | `{ enabled (false), description?, vrf, listen[] ≤ 8 {address, port (161)}, engineId? (5–32 hex bytes), sysName?, sysLocation?, sysContact?, communities{ name → {secretRef: password/…, access ro\|rw, sources[] CIDR} }, v3Users{ name → {securityLevel (authPriv), authProtocol sha\|sha256\|sha512\|md5, authRef?: password/…, privProtocol aes\|aes256\|des, privRef?: password/…, access} }, trapReceivers[] ≤ 16 {address, port (162), version v2c\|v3, community?/user? (matching version, defined), inform} }` — community strings and USM keys are secrets |
| `lldp` | `{ enabled (false), systemName?, txHold 1–10 (4), txIntervalSec 1–3600 (30), interfaces[] ≤ 256 {interface (exists, unique), portDescription?, mgmtIpv4?, mgmtIpv6?, mgmtOid?} }` |
| `ipfix.exporters{name}` | `{ enabled, description?, collector {address, port (4739)}, sourceAddress (same family, ≠ collector, configured in `vrf`), vrf, pathMtu 68–1450 (512), templateIntervalSec 1–3600 (20), udpChecksum (false) }` |
| `ipfix.flowprobe` | `{ activeTimerSec (15) ≤ passiveTimerSec (120), recordL2/L3/L4 (false/true/true), interfaces[] ≤ 256 {interface (exists, unique), direction rx\|tx\|both, l2, ip4, ip6 (≥ 1)} }` |
| `ipfix.sflow?` | `{ enabled (false), samplingN 1–2^31-1 (10000), pollingIntervalSec (20), headerBytes 64–256 (128), collectors[] 1–4 {address, port (6343)}, agentAddress? (configured in `vrf`), vrf, interfaces[] }` |
| `ntp` | `{ enabled (false), vrf, servers[] ≤ 16 {address host\|ip (unique), iburst (true), prefer, minPoll?/maxPoll? -6–24, nts (false), keyRef?: key/… (exclusive with nts)}, pools[] ≤ 8, allow[] CIDR (server mode), deny[] CIDR, listen[] (configured in `vrf`), port 0–65535 (123; 0 = client only), rateLimit? {interval, burst, leak}, localStratum? 1–15, orphan (false; needs localStratum), rtcSync (true), ntsServer? {certificateRef: cert/…, keyRef: key/…, port (4460)}, makestep {thresholdSec (1), limit (3)} }` — enabled needs a server, pool or local stratum. **The only NTP model** (D-050: chrony is a daemon rendered by RF-3; P02a removes `system.ntp`). One instance, bound to `vrf` (F15, v1 decision) |
| `qos.policers{name}` | `{ description?, type 1r2c\|1r3c-rfc2697\|2r3c-rfc2698\|2r3c-rfc4115\|2r3c-mef5cf1 (1r2c), rateUnit kbps\|pps (kbps), cir ≥ 1, eir? (two-rate only, required there; RFC 2698 eir ≥ cir), cb, eb? (three-colour only, required there), round closest\|up\|down, colorAware (three-colour only), conformAction/exceedAction/violateAction {action transmit\|drop\|mark-and-transmit, dscp 0–63 iff mark} }` — VPP policer plugin (D-052) |
| `qos.shapers{name}` | `{ description?, rateKbps ≥ 1, burstBytes? ≥ 64 }` — rendered as an egress single-rate token bucket (VPP 26.06 has no queueing shaper outside the excluded HQoS, V3) |
| `qos.maps{name}` | `{ description?, id? u32 (unique), rows {ext?, vlan?, mpls?, ip?: [{from, to}]} }` — `from` within the source range (ext 0–255, vlan/mpls 0–7, ip 0–63), unique per row, ≥ 1 entry; VPP qos egress map |
| `qos.interfaces{vppInterfaceName}` | `{ description?, policer? {input?, output?}, shaper?, record? ext\|vlan\|mpls\|ip, store? {source, value}, mark? {map, output} }` — ≥ 1 action; shaper ⊕ policer.output; record ≠ store.source; semantic: interface/policer/shaper/map exist, marked values fit the output header |

Exports: `ServicesSchema`/`ServicesConfig`, `Dhcp{Option,Pool,Reservation,Subnet,Server,Relay}Schema`, `DhcpSchema`, `DNS_RECORD_TYPES`,
`Dns{Upstream,ForwardZone,Record,LocalZone,AccessControl,Resolver,VppCache}Schema`, `ServicesDnsSchema`,
`Snmp{Community,V3User,TrapReceiver}Schema`, `SnmpSchema`, `LldpInterfaceSchema`, `LldpSchema`,
`Ipfix{Exporter,FlowprobeInterface,Flowprobe}Schema`, `SflowSchema`, `IpfixSchema`, `ServicesNtpServerSchema`, `ServicesNtpSchema`,
`QOS_POLICER_TYPES`, `QOS_SOURCES`, `QOS_SOURCE_MAX`, `Qos{PolicerAction,Policer,Shaper,MapEntry,Map,Interface}Schema`, `QosSchema`.
DNS/NTP names carry the `Services` prefix so they cannot collide with P02a's `System*` exports under `export *` (D-047, F2).
`system.dns` (P02a: the management host's resolver client) and `services.dns.resolvers` (Unbound instances serving clients) are
intentionally different objects (F3).

## `ha`

| path | type / constraints |
|---|---|
| `vrrp{name}` | record keyed by `objectName` (D-053, F9: pointer-addressable `/ha/vrrp/<name>`, merge-patchable); value `{ enabled, description?, interface (exists; in `vrf`), vrId 1–255, addressFamily ipv4\|ipv6 (ipv4), priority 1–255 (100; 255 = owner), advertisementIntervalMs 10–40950 step 10 (1000), preempt (true), acceptMode (false), unicast? {peers[] 1–16 (family, unique)}, addresses[] 1–32 (family, unique; within an interface prefix; claimed once per VRF), vrf, engine vpp\|keepalived (vpp), track[] ≤ 16 {interface (exists, ≠ own, unique), priorityDecrement 1–253 (10)} }` — identity `(interface, addressFamily, vrId)` is unique (RFC 5798 / VPP `vrrp_vr_add_del` semantics) |
| `cluster?` | `{ enabled (false), nodeName: hostname, peers[] 1–8 {name (≠ nodeName, unique), address (unique)}, port (4370), secretRef: key/…, interface? (exists), vrf, configSync (true), stateSync {nat, ipsec, acl (false)} }` (D9.2/D9.3) |

Exports: `HaSchema`/`HaConfig`, `VrrpInstanceSchema`, `VrrpTrackSchema`, `HaClusterSchema`.

## Semantic validators (tier (b); `{ pointer, message }[]`, pointers via `jsonPointer()`)

| name | domains read | rule |
|---|---|---|
| `vpn.proposal-exists` | vpn | tunnel / remote-access `proposal` exists in `ipsec.proposals` |
| `vpn.pki-reference-exists` | vpn | cert auth `certificate`/`remoteCa`, remote-access `certificate`/`clientCa`, certificate `ca` exist in `pki` |
| `vpn.vrf-exists` | vpn, vrfs | `vrf` and `underlayVrf` of IPsec tunnels, remote-access profiles and WireGuard interfaces exist |
| `vpn.local-address-configured` | vpn, interfaces, tunnels | `localAddr` (IPsec, remote access) and WireGuard `listenAddress` are configured on an interface in their `underlayVrf` |
| `vpn.route-based-ipip` | vpn, tunnels | `routeBased.ipipInterface` exists; IPIP `src` = `localAddr`, IPIP `underlayVrf` = `underlayVrf`, IPIP `vrf` = `vrf`; `%any` ↔ p2mp, fixed peer ↔ p2p with matching `dst`; one IPsec tunnel per IPIP |
| `vpn.ipsec-peer-unique` | vpn | enabled tunnels: `(underlayVrf, localAddr, remoteAddr)` unique for fixed peers; `%any` responders clash only when `(underlayVrf, localAddr, localId, remoteId, auth.method)` is identical (F6) |
| `vpn.proposal-compatible` | vpn | AH needs `esp.encr: null`; IKEv1 rejects AEAD IKE ciphers and `curve25519`/`curve448` (F14) |
| `vpn.wireguard-unique` | vpn | `instance` unique; `(underlayVrf, listenAddress, listenPort)` unique; per interface: peer `publicKey` unique, an allowed prefix routed to one peer |
| `vpn.wireguard-address-overlap` | vpn, interfaces | WireGuard `address[]` does not overlap physical / sub-interface / other WireGuard prefixes of its `vrf` (F10) |
| `vpn.remote-access-pools-no-overlap` | vpn | client pools do not overlap across profiles |
| `vpn.no-inline-secret-material` | vpn, tunnels, services, ha | no PEM armour anywhere; no hex / padded-base64 key blob (≥ 16 bytes) outside `publicKey`/`engineId`/`data`/`*Ref` (F7) |
| `tunnels.name-unique` | tunnels | a tunnel name is used by one kind only |
| `tunnels.vrf-exists` | tunnels, vrfs | `vrf` and `underlayVrf` exist |
| `tunnels.source-address-configured` | tunnels, interfaces, vpn | `src` is configured on an interface in `underlayVrf` |
| `tunnels.endpoints-unique` | tunnels | per kind: `(underlayVrf, src, dst[, vni][, GRE type, sessionId])` unique (F11) |
| `tunnels.instance-unique` | tunnels | `instance` unique per kind |
| `tunnels.mcast-interface-exists` | tunnels, interfaces | VXLAN `mcastInterface` exists |
| `tunnels.address-overlap` | tunnels, interfaces, vpn | tunnel interface prefixes do not overlap each other or any physical / sub-interface / WireGuard prefix within a VRF (F10) |
| `services.vrf-exists` | services, vrfs | every instance VRF (incl. relay `serverVrf`) exists |
| `services.interface-references` | services, interfaces, tunnels, vpn | DHCP server/relay, LLDP, flowprobe, sFlow interfaces exist; DHCP interfaces are in the instance VRF |
| `services.dhcp-subnet-within-interface-prefix` | services, interfaces, tunnels, vpn | each subnet lies within a prefix of one of the server's (known) interfaces |
| `services.dhcp-subnets-unique` | services | subnets of different servers in one VRF do not overlap |
| `services.bind-address-configured` | services, interfaces, tunnels, vpn | relay `sourceAddress` (in `serverVrf ?? vrf`, F13), DNS/SNMP/NTP bind addresses, IPFIX `sourceAddress`, sFlow `agentAddress` are configured in their VRF (wildcards pass) |
| `services.dns-listen-unique` | services | one resolver per `(vrf, address, port)`; a wildcard covers the specific addresses of its family |
| `services.qos-references` | services, interfaces, tunnels, vpn | QoS attachments reference an existing interface, policers, shaper and map |
| `services.qos-consistency` | services | map ids unique; values a map writes fit the header marked with it |
| `ha.vrrp-vrid-unique` | ha | `(interface, addressFamily, vrId)` unique |
| `ha.interface-exists` | ha, interfaces, tunnels, vpn | VR interface exists and is in the VR's `vrf`; tracked and cluster interfaces exist |
| `ha.vrrp-virtual-addresses` | ha, interfaces, tunnels, vpn | a virtual address is claimed once per VRF and lies within a prefix of the interface (skipped for unnumbered) |
| `ha.vrf-exists` | ha, vrfs | VR and cluster `vrf` exist |

Shared helpers (not part of the package surface): `semantic/tunnels-common.ts` — IPv4/IPv6 math (`parseIp`, `parseCidr`, `cidrContains`,
`cidrsOverlap`, `isMulticast`, …) and duck-typed lookups (`knownVrfs`, `interfaceIndex`, `addressConfiguredInVrf`) that read only the
documented field names of `interfaces`/`vrfs`, so they work with the P02s placeholders and with P02a's full models. `domains/tunnels.ts` and `domains/services.ts` import the IP math from there (a layering
smell only; it moves to `src/ip.ts` when P02a's lands).

## Fixtures (`packages/schema/examples/`)

- Valid (schema + semantics): `vpn-site-to-site.json` (AEAD + legacy proposals, route-based PSK tunnel over `tunnels.ipip`, `%any` hub with a
  p2mp IPIP, IKEv1 policy-based), `vpn-wireguard.json`, `vpn-remote-access.json` (PKI + EAP), `tunnels-gre-vxlan-ipip.json`,
  `services-dhcp-dns.json`, `services-snmp-lldp-ipfix-ntp.json`, `ha-vrrp.json`.
- Schema-invalid (`invalid-*`, rejected by `RootConfig`): `invalid-vpn-inline-psk.json`, `invalid-vpn-wireguard-key.json`,
  `invalid-ha-vrid.json` (`ha.vrrp` as a record), `invalid-services-snmp-inline-community.json`, `invalid-tunnels-vxlan-vni.json`.
- Semantic-invalid (`<domain>-semantic-<rule>.json`, schema-valid, exactly one finding): `vpn-semantic-missing-proposal.json`,
  `tunnels-semantic-source-not-configured.json`, `services-semantic-dhcp-subnet-outside-interface.json`, `ha-semantic-duplicate-vrid.json`.

## Out of scope / additive later (recorded so P06/P07/P11 do not assume them)

- `ha.vrrp` is name-addressable; every other group-(c) collection is a record too, except ordered lists (`pools[]`, `listen[]`, `options[]`,
  `trapReceivers[]`, `lldp.interfaces[]`, `peers[]`) which are diff leaves (D-021).
- `services.snmp`, `services.ntp`, `services.lldp`, `services.ipfix.flowprobe`, `services.ipfix.sflow` are **one instance, bound to `vrf`**
  (v1 decision, F15); LLDP/flowprobe/sFlow are global in VPP anyway.
- Additive keys not modelled yet: VXLAN-GPE, GTP-U, L2TPv3, PPPoE (`tunnels.*`), manually-keyed SAs (`vpn.ipsec.manualSas`), Unbound views,
  HQoS (excluded, V3), `services.lb` / `services.hostStack` (T3), SRv6/LISP (home to be decided with P02a). DHCP client lives on the interface
  (`interfaces.<name>.dhcpClient`, D-050, P02a).

## Decisions recorded by P02c (see `docs/status/tasks/P02c.md`)

Populated domain roots (D-017/D-053) · private helper primitives in `domains/_shared` (D-054) · `<kind>/<name>` secret references (D-051) ·
`vrf` = overlay / `underlayVrf` = outer packets everywhere (F4) · `ha.vrrp` record (D-053) · `services.ntp` is the only NTP (D-050) ·
`services.qos` (D-052) · `Services*` export names (D-047) · shared IP math in `semantic/tunnels-common.ts` · VRID unique per (interface,
address family) · `<domain>-semantic-*` fixture naming · interface references by VPP name (tunnels need `instance`).
