# F-kea-dhcp-relay — WIP (slot 2, prefix w2)

Updated 2026-09-25 04:15.

## Done (committed)
- contract(schema,proto): DhcpLeases rpc + Dhcp* messages; four semantic rules
- agent: services domain (kea.dhcp4/6 singletons, dhcp.proxy/proxy-vss, dhcp.relay records), projection/assembly,
  DhcpLeases RPC, ownership declarations (TD-11b guard), dhcp.client claim-first (TD-11b Q3, now TD-11b's ClaimFirst)
- api: /state/dhcp/leases, /state/dhcp/relays, /state/interfaces/{name}/dhcp-client, fake, e2e
- web: Services → DHCP (fork), types from the api-client and @ngfw/schema
- topology test PASS on the merged tree (with screenshots), docs, status, questions
- merged main (TD-8, TD-11b, TD-20, TD-7); relay scope on Wiring.IDRange

## In progress
- `TMPDIR=/tmp/g-w2 tools/ci.sh --base main`

## Left
- CI result into F-kea-dhcp-relay.md, final cleanup (dist, bin, /run/vrx-test/w2/kea*)
