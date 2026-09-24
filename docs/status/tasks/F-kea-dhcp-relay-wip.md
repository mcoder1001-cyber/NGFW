# F-kea-dhcp-relay — WIP (slot 2, prefix w2)

Updated 2026-09-25 00:00.

## Done (committed)
- contract(schema,proto): DhcpLeases rpc + Dhcp* messages; semantic rules (reservation outside pools, one Kea VRF per
  family, one relay source per client VRF/family, DHCP client excludes static IPv4)
- agent: services domain (kea.dhcp4/kea.dhcp6 singletons over RF-3's renderer, dhcp.proxy/proxy-vss, dhcp.relay
  records), projection/assembly, DhcpLeases RPC, unit tests
- fix: dhcp.client claim-first (TD-11b Q3) + regression test
- api: /state/dhcp/leases, /state/dhcp/relays, /state/interfaces/{name}/dhcp-client, fake, e2e; api-client + CLI regen

## In progress
- UI (Services → DHCP tab with sub-tabs), topology test on the host VPP (Kea in ns-w2-wan, relay in table 2001,
  dhclient in ns-w2-lan, VPP DHCP client), docs, status file, CI

## Left
- screenshot against the real endpoint, restart-safety evidence, CI run
