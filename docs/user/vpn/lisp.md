# LISP and LISP-GPE (advanced)

VRX exposes a minimal LISP / LISP-GPE set: the global switches, local locator sets, local EIDs, EID-table maps
(VNI → VRF or bridge domain), static remote mappings, adjacencies, static LISP-GPE forwarding entries, map-resolvers,
map-servers and the proxy-ITR locator set. Configuration lives at `tunnels.lisp`; the UI is **VPN → LISP (advanced)**
with the sub-tabs Locators, EIDs, Mappings and Resolvers, each showing whether VPP has the configured objects.
Live state: `GET /api/v1/state/lisp`.

## Example: one static mapping

```json
{
  "vrfs": { "overlay": { "id": 1100 } },
  "interfaces": {
    "host-w11-eth0": { "enabled": true, "ipv4": ["10.11.1.1/24"] },
    "loop1100": { "enabled": true, "vrf": "overlay", "ipv4": ["10.11.100.1/24"] }
  },
  "tunnels": {
    "lisp": {
      "enabled": true,
      "gpe": true,
      "locatorSets": { "w11-rloc": { "locators": [{ "interface": "host-w11-eth0", "priority": 1, "weight": 1 }] } },
      "localEids": [{ "vni": 1100, "eid": "10.11.100.0/24", "locatorSet": "w11-rloc" }],
      "eidTables": { "1100": { "vrf": "overlay" } },
      "remoteMappings": [{ "vni": 1100, "eid": "10.11.200.0/24", "rlocs": [{ "address": "10.11.1.2" }] }],
      "adjacencies": [{ "vni": 1100, "reid": "10.11.200.0/24", "leid": "10.11.100.0/24" }]
    }
  }
}
```

Through the API: `PUT /api/v1/config/tunnels/lisp` with the `lisp` object, then `POST /api/v1/config/commit` (the CLI's
configuration mode edits the same candidate). The VPP CLI equivalent is roughly:

```
lisp enable
lisp locator-set add w11-rloc iface host-w11-eth0 p 1 w 1
lisp eid-table map vni 1100 vrf 1100
lisp eid-table add eid 10.11.100.0/24 locator-set w11-rloc vni 1100
lisp remote-mapping add vni 1100 eid 10.11.200.0/24 rloc 10.11.1.2 p 1 w 1
lisp adjacency add reid 10.11.200.0/24 leid 10.11.100.0/24 vni 1100
```
Check with `vppctl show lisp locator-set`, `show lisp eid-table`, `show lisp map-cache`.

## Rules

- EIDs are masked IP prefixes or MAC addresses in canonical form, unique per VNI (local and remote share the space).
- Every VNI other than 0 used by a local EID or remote mapping needs an `eidTables` entry: a VRF for IP EIDs, a bridge
  domain (by id) for MAC EIDs.
- `gpe` and every LISP object require `enabled`; GPE entries require `gpe`. VPP switches LISP-GPE on together with LISP.
- The switches and the proxy-ITR are VPP-global: only the globals-owner agent sets them; any other agent only checks
  them and fails with a clear "managed by the globals owner only" error when LISP is off. Removing `enabled` does not
  switch LISP off.

## Known limits (VPP 26.06)

- **V13 — GPE entries cannot be read back.** VPP answers the path dump with the wrong message id, so the agent cannot
  see a GPE entry's locator pairs. It applies the entry once per VPP boot and re-applies it after a VPP restart;
  Retrieve does not list GPE entries, and the UI shows them as "write-only" (only their VNI is visible).
- **V14 — leaks.** Deleting a remote mapping leaves VPP's internal `<remote-N>` locator set behind, and every LISP
  enable/disable leaves `lisp_gpe*` interfaces that no API deletes. They are harmless but only a VPP restart removes
  them. Avoid toggling LISP.
- Not modelled: map-server authentication keys, NSH EIDs, the `one` API, LISP-GPE over IPsec.
