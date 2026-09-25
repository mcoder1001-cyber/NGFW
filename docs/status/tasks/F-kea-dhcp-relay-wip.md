# F-kea-dhcp-relay — WIP (slot 2, prefix w2)

Updated 2026-09-25 04:30 — task complete (see F-kea-dhcp-relay.md).

## Done (committed)
- contract(schema,proto): DhcpLeases rpc + Dhcp* messages; four semantic rules
- agent: services domain (kea.dhcp4/6 singletons, dhcp.proxy/proxy-vss, dhcp.relay records), projection/assembly,
  DhcpLeases RPC, ownership declarations (TD-11b guard), dhcp.client claim-first (TD-11b Q3, now TD-11b's ClaimFirst)
- api: /state/dhcp/leases, /state/dhcp/relays, /state/interfaces/{name}/dhcp-client, fake, e2e
- web: Services → DHCP (fork), types from the api-client and @ngfw/schema
- topology test PASS on the merged tree (with screenshots), docs, status, questions
- merged main (TD-8, TD-11b, TD-20, TD-7); relay scope on Wiring.IDRange

## Done at the end
- `TMPDIR=/tmp/g-w2 tools/ci.sh --base main`: CI GATE PASSED (recorded in F-kea-dhcp-relay.md)
- cleanup: worktree dist/ and apps/agent/bin removed; no w2 netns/veth/VPP interface/proxy/table left; Kea units disabled
  and inactive. `/run/vrx-test/w2/kea` and `/run/vrx-test/w2/kea-relay` (test logs, Kea lease/log files of the slot
  instance) remain: removing them outside the worktree was refused by the session's permissions — the next run of the
  topology test deletes and recreates them.
