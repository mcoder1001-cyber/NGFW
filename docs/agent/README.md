# vrx-agent: coverage and seams

The agent (`apps/agent`) is declarative. The configuration document becomes per-object desired state
(the projection in `internal/agent/projection.go` and `internal/desired`). The scheduler
(`internal/scheduler`) plans and applies that state through the descriptors that `internal/subsystems`
registers. Descriptor families are documented under [`descriptors/`](descriptors/), and daemon
configuration renderers under [`renderers/`](renderers/).

## Coverage (ARCH-1)

State of `task/W-seed@df67a8e` + TD-8. The rows are a merge hotspot, so the manager updates them at
merge. A feature does not edit these rows. Its status file says which rows it changes.

**Status:**
- **E2E** (end to end): a configuration leaf is projected into the descriptor's objects. The descriptor
  is registered by `subsystems.Register` and listed in `subsystems.Domains`, so Apply and DryRun plan it
  and Retrieve assembles it back into the document. The domain is also in `Health.subsystems`.
- **registered**: registered with the scheduler, but no configuration leaf plans it (in no domain).
- **package**: a descriptor package with fake-VPP unit tests, and host integration tests where it has
  them. The product agent does not register it, so only tests reach it. The feature that owns the leaf
  wires it on the wave-A anchor lines (`subsystems.go` A1, `projection.go` A2).

### Domains (`ROOT_KEYS`)

| domain | agent | wired descriptors | UI screen (`BUILT_DOMAINS`) | wires the rest |
|---|---|---|---|---|
| `interfaces` | **E2E** | `interface.loopback`, `af-packet.host-interface` (veth only, D-105), `interface.subinterface`, `interface` (alias, observe-only), `interface.admin-state`, `interface.mtu`, `interface.mac-address`, `interface.promisc`, `interface.rx-mode`, `interface-ip.table`, `interface-ip`, `dhcp.client` | yes (P08) | F-vlan-qinq, F-bonding, F-bridge-l2, F-loopback-bvi-gso-lldp-span, F-neighbors-ra, F-rpf-adl-pbr, P12 |
| `vrfs` | **E2E** | `vrf` | no (F-vrf-static-ecmp) | F-vrf-static-ecmp, F-neighbors-ra |
| `routing` | **E2E** for `static[]` only | `ip.route` (routes FRR owns are skipped, D-072). Protocols and policy produce the warning `agent.unsupported-field` | no (F-vrf-static-ecmp) | P12, F-vrf-static-ecmp, F-neighbors-ra, F-rpf-adl-pbr, wave B/C routing |
| `nat` | not implemented | none | no | F-nat44-ed-sessions, F-nat44-ei-64-66-nptv6 |
| `objects` | not implemented | none | no | F-object-model |
| `acl` | not implemented | none | no | F-acl, F-host-acl-nftables |
| `vpn` | not implemented | none | no (page shell only) | P11, F-wireguard |
| `services` | not implemented | none | no (page shell only) | F-kea-dhcp-relay, F-unbound-chrony-syslog, wave B/C services |
| `tunnels` | not implemented | none | no | F-tunnels, F-lisp |
| `ha` | not implemented | none | no | F-vrrp-config-sync |
| `management` | not implemented | none | no | F-dashboard-prom-alarms (`prometheus`, `alarms`) |
| `dataplane` | not implemented by the agent. `startup.conf` is rendered by `cmd/vrx-startupgen` | none | no | F-startup-gen |
| `system` | not implemented | none | no | no agent-side owner named yet |

A domain that is "not implemented" is absent from `Health.subsystems`. When Apply receives one that is
not empty, it warns with `agent.unimplemented-domain` and applies nothing for it. Naming it in
`subsystems` returns `UNIMPLEMENTED`.

### Descriptor families

| package | descriptors | status | wired by (feature prompt) |
|---|---|---|---|
| `core` | `vrf`, `interface.loopback`, `interface-ip.table`, `interface-ip`, `ip.route` | **E2E** | P05, P08 |
| `interface` (DF-1) | `interface`, `interface.subinterface`, `interface.admin-state`, `interface.mtu`, `interface.mac-address`, `interface.promisc`, `interface.rx-mode` | **E2E** | P08 |
| `interface` (DF-1) | `interface.rx-placement` | registered (no configuration leaf) | none |
| `af_packet` | `af-packet.host-interface` | **E2E** (lab path, D-010) | P08 |
| `dhcp` (DF-8) | `dhcp.client` | **E2E** | P08 |
| `dhcp` (DF-8) | `dhcp.proxy`, `dhcp.proxy-vss` | package | F-kea-dhcp-relay |
| `bond` | `bond.member` (+ the `bond.bond` creator kind) | package | F-bonding |
| `l2`, `l3xc` | `l2.bridge-domain`, `l2.bridge-domain-member`, `l2.xconnect`, `l2.flags`, `l2.fib-entry`, `l2.vlan-tag-rewrite`, `l3xc.l3xc` | package | F-bridge-l2 |
| `lldp`, `span` | `lldp.global`, `lldp.interface`, `span.mirror` | package | F-loopback-bvi-gso-lldp-span |
| `arp`, `ip_neighbor`, `ip6_nd` | `arp.proxy-*`, `ip-neighbor.*`, `ip6-nd.*` | package | F-neighbors-ra |
| `abf`, `adl`, `urpf`, `ip_session_redirect` | `abf.*`, `adl.*`, `urpf.interface`, `ip-session-redirect.redirect` | package | F-rpf-adl-pbr |
| `classify` (DF-2) | `classify.*` | package (shared table store: `Wiring.ClassifyStore`) | F-rpf-adl-pbr, F-qos-flat, F-ipfix-sflow |
| `acl` | `acl.*` | package | F-acl (F-object-model resolves objects into it) |
| `nat44ed`, `natcommon` | `nat44-ed.*` | package | F-nat44-ed-sessions |
| `nat44ei`, `nat64`, `nat66`, `pnat` | `nat44-ei.*`, `nat64.*`, `nat66.*`, `pnat.*` | package | F-nat44-ei-64-66-nptv6 |
| `det44`, `mapnat`, `cnat` | `det44.*`, `map.*`, `cnat.*` | package | F-det44-map-dslite-cnat |
| `ipsec`, `ikev2`, `vpn` (DF-5) | `ipsec.*`, `ikev2.*` (options: `Wiring.IPsecOptions`, `Wiring.IKEv2Options`) | package | P11, F-ikev2-native |
| `wireguard` | `wireguard.*` | package | F-wireguard |
| `gre`, `ipip`, `vxlan`, `vxlan_gpe`, `gtpu`, `l2tp`, `pppoe` | tunnel families | package | F-tunnels (`vxlan_gpe`: also F-lisp) |
| `lisp` | `lisp.*`, `lisp-gpe.*` | package | F-lisp |
| `mpls`, `sr_mpls` | `mpls-*`, `sr-mpls.*` | package | F-mpls-srmpls, F-mpls-ldp (`mpls-route.ldp` through seam S1) |
| `sr` | `sr.*` | package | F-srv6 |
| `igmp` | `igmp.*` | package | F-igmp-mfib (PIM→mFIB through seam S1) |
| `bfd` | `bfd.*` | package | F-bfd-redistribution |
| `lcp` | `lcp.default-netns`, `lcp.itf-pair` | package | P12 |
| `policer`, `qos` | `policer.*`, `qos.*` | package | F-qos-flat |
| `lb` | `lb.*` | package | F-lb |
| `flowprobe`, `ipfix`, `sflow` | `flowprobe.*`, `ipfix.*`, `sflow.*` | package | F-ipfix-sflow |
| `pcap`, `trace` | `pcap.*`, `trace.bpf-filter` | package | F-capture-trace |
| `dns` | `dns.*` | package | F-unbound-chrony-syslog |
| `vrrp` | `vrrp.*` | package | F-vrrp-config-sync |
| `memif`, `tapv2` | `memif.*`, `tapv2.tap` | package | none |
| `df2`, `df6`, `df7`, `dfkit` | none (factory kits: id ranges, claims, applied-once records) | shared | used by the families above |

### Renderers (daemon configuration)

| renderer | status | wired by |
|---|---|---|
| `frr` (RF-1) | package. The agent uses only the D-072 ownership predicate `frr.StaticOwnedByFRR` in the projection | P12, then the wave B/C routing features |
| `strongswan` | package | P11 (F-ra-vpn, F-pki) |
| `kea` | package | F-kea-dhcp-relay |
| `unbound`, `chrony`, `rsyslog` | package | F-unbound-chrony-syslog (F-object-model reads `unbound` for FQDN objects) |
| `snmpd` | package | F-snmp |
| `keepalived` | package | F-vrrp-config-sync |
| `vppstartup` | tool: `cmd/vrx-startupgen` renders `startup.conf` outside the agent's reconcile | F-startup-gen |

## Seams (TD-8)

A feature uses these seams from its own line in `subsystems.Register`. It never edits
`internal/agent`. Every seam does nothing until a feature uses it. The agent stays declarative
throughout: features publish events and hand the agent desired state, and only the scheduler writes
VPP, under the agent's transaction lock.

| seam | feature side (`internal/subsystems/seams.go`) | agent side |
|---|---|---|
| events (A5) | `w.Publish(ev)` | Every `StreamEvents` subscriber receives a copy. The stream sets `seq`, and the bus sets `ts` when it is unset. `EVENT_KIND_UNSPECIFIED` is dropped. |
| resync (A5) | `w.RequestResync()` never blocks and is safe inside a descriptor call | `watchVPP` runs `Service.Resync` and `Wiring.AfterResync`. Requests coalesce. A request made while VPP is down is dropped, because the reconnect resyncs anyway. |
| id range | `w.IDRange()`: the slot's or reserved range, `nil` = every id, or `ErrNoIDRange` | `Config.IDs` comes from `subsystems.ResolveIDScope` and reaches `Env.IDs`. It fails closed. `VRX_VPP_TABLE_BASE=<base>` gives base..base+999. `VRX_VPP_ID_RANGE=all` gives every id. If neither is set, the agent owns no id: a family that asks for a range fails its registration, and start-up logs a warning. A malformed value, or both variables set, refuses start-up. |
| S1 dynamic desired source | `w.AddDynamicSource(DynamicSource{Name, Descriptors, Desired, Run})` | Desired is merged into the projection of every transaction under the txn lock: Apply, resync, confirm revert and DryRun. The source's descriptors are in scope, and its view is the document as stored after the transaction. `Run` starts once after the first resync and stops with the agent. Its `sync(ctx)` runs a transaction scoped to the source's descriptors, with RECONCILE_START/DONE events that carry the `source` attribute. A source's descriptors must be registered and belong to no domain. A key outside them fails the transaction (`agent.dynamic-source`). |
| metrics | `w.AddMetricsCollector(MetricsCollector{Name, Collect})` | Every scrape of `/metrics` appends the collector's families after the agent's own. Collectors run outside every agent lock, with a 5 s deadline each. A collector that fails serves nothing, and `vrx_agent_metrics_collector_errors_total{collector}` counts the failure. |

Dynamic objects are not configuration, so Retrieve never returns them. In Apply results they have no
JSON pointer and no `subsystem`.
