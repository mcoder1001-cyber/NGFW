# NGFW user guide

Generated from the checked-in feature guides. Regenerate with `python3 tools/docs/generate-reference.py`; verify with `--check`.

Start with [First-boot setup](getting-started.md) and the [CLI reference](cli/reference.md). Feature pages describe configuration, API/CLI equivalents and feature-specific limits.

A guide or source file is not evidence that a feature passed live acceptance. See the [acceptance register](../status/DEFERRED-ACCEPTANCE.md) and [product boundaries](../status/have-not.md). For authenticated operations and request schemas, use the OpenAPI document supplied by your installed API.

[Source navigation](source-reference.md) identifies the current API controllers and schema sources for developers and operators auditing their installed version.

## Getting started

- [First-boot setup](getting-started.md)

## Cli

- [ngfw CLI — command reference](cli/reference.md)

## Dashboard

- [Dashboard, Prometheus export and alarms](dashboard/dashboard-prom-alarms.md)
- [Dashboard](dashboard/overview.md)

## Firewall

- [Access lists — L3/L4 ACLs, MACIP ACLs, attachments, hit counters](firewall/acl.md)
- [CGNAT (DET44), MAP-E / MAP-T / LW4o6, DS-Lite, 464XLAT and CNAT](firewall/det44-map-dslite-cnat.md)
- [Host ACL — protecting the box itself (local-in, management plane)](firewall/host-acl-nftables.md)
- [NAT44-EI, NAT64, NAT66 and NPTv6](firewall/nat44-ei-64-66-nptv6.md)
- [NAT44 (endpoint-dependent): outbound PAT, 1:1, port forwards and the session browser](firewall/nat44.md)
- [NAT46 — IPv4 clients to IPv6-only servers](firewall/nat46.md)
- [Firewall objects — addresses, groups, FQDNs, services, schedules, zones, tags](firewall/object-model.md)

## Install

- [Validate an offline Debian delivery set](install/bundle.md)

## Interfaces

- [Interfaces — basics](interfaces/basics.md)
- [Bond interfaces (link aggregation, LACP)](interfaces/bonding.md)
- [Bridging: bridge domains, cross-connects, VLAN tag rewrite, time-range MAC filter](interfaces/bridge-l2.md)
- [Default data-plane NICs](interfaces/default-dataplane-nics.md)
- [Loopbacks as BVI, GSO, port mirroring, LLDP and the delay simulator](interfaces/loopback-bvi-gso-lldp-span.md)
- [VLAN sub-interfaces: 802.1Q and QinQ (802.1ad)](interfaces/vlan-qinq.md)

## Network

- [Multi-WAN (failover and load balancing)](network/multi-wan.md)
- [PPPoE client (ISP dial-up)](network/pppoe.md)

## Routing

- [BFD and redistribution](routing/bfd-redistribution.md)
- [Routing — BGP, prefix lists, route maps, Linux pairs](routing/bgp.md)
- [Multicast (IGMP, mFIB, PIM)](routing/igmp-mfib.md)
- [IS-IS, RIPv2 and RIPng](routing/isis-rip.md)
- [MPLS LDP](routing/mpls-ldp.md)
- [MPLS and SR-MPLS](routing/mpls-srmpls.md)
- [Neighbours, router advertisements, proxy ARP/ND](routing/neighbors-ra.md)
- [OSPF routing](routing/ospf.md)
- [Anti-spoofing and policy routing: uRPF, ADL, PBR, Auto-SDL](routing/rpf-adl-pbr.md)
- [Routing — VRFs, static routes, ECMP, FIB browser, ping](routing/vrf-static-ecmp.md)

## Security

- [Auto-block (brute-force and scan protection)](security/auto-block.md)
- [Global blocking (IP block lists)](security/global-blocking.md)

## Services

- [Host stack (advanced, T3)](services/host-stack.md)
- [Flow export — IPFIX (flowprobe) and sFlow](services/ipfix-sflow.md)
- [DHCP — Kea server, relay, client](services/kea-dhcp-relay.md)
- [Load balancer (VPP lb plugin) — tier T3](services/lb.md)
- [QoS — policers, rate limits and marking (flat QoS)](services/qos-flat.md)
- [SNMP (v2c / v3) and the NGFW private MIB](services/snmp.md)
- [Services — DNS resolver, NTP and remote syslog](services/unbound-chrony-syslog.md)

## System

- [External authentication (AAA) and two-factor login](system/aaa.md)
- [Dataplane: VPP workers, cores, NIC queues, hugepages, plugins](system/dataplane.md)
- [HA session state](system/ha-state-sync.md)
- [System identity: hostname, time zone, banners, DNS client](system/identity.md)
- [Licensing](system/licensing.md)
- [Management: users, AAA, API TLS, remote syslog](system/management.md)
- [اعلان‌ها](system/notifications.fa.md)
- [Notifications](system/notifications.md)
- [RESTCONF and YANG](system/restconf-yang.md)
- [Automation: Python SDK and Terraform provider](system/sdk-terraform-ansible.md)
- [High availability and configuration sync](system/vrrp-config-sync.md)

## Tools

- [Packet capture (Tools → Packet capture)](tools/capture-trace.md)

## Vpn

- [Native route-based IKEv2](vpn/ikev2-native.md)
- [Route-based IPsec site-to-site VPN](vpn/ipsec.md)
- [LISP and LISP-GPE (advanced)](vpn/lisp.md)
- [Certificate manager](vpn/pki.md)
- [SRv6 (Segment Routing over IPv6)](vpn/srv6.md)
- [Tunnel interfaces](vpn/tunnels.md)
- [WireGuard](vpn/wireguard.md)
