# OSPF routing

Routing → OSPF offers OSPFv2 (IPv4) and OSPFv3 (IPv6) tabs. Save changes to candidate and commit
from the pending change bar. Areas must be explicitly declared; `0` and `0.0.0.0` name the same
backbone. The backbone cannot be stub/NSSA. The selected interface must have an address of the
correct family and belong to the process VRF. A router ID is required if that VRF has no IPv4 address.
Linux-cp interface mappings are required so FRR uses the Linux side of the configured VPP interface.

A single-area v2 example (replace loop0 with your addressed LCP interface):

```sh
ngfw configure merge routing '{"ospf":{"routerId":"192.0.2.1","areas":{"0":{}},"interfaces":{"loop0":{"area":"0","passive":true}}}}'
ngfw commit
```

For a totally stubby non-backbone area, add `"1":{"type":"stub","noSummary":true}` to `areas`
and assign the relevant interface to area `1`. NSSA is also supported. Both ends of each adjacency
must agree on area, timers, network type and authentication.

For IPv6, use the OSPFv3 tab or the following equivalent:

```sh
ngfw configure merge routing '{"ospf6":{"routerId":"192.0.2.1","areas":{"0":{}},"interfaces":{"loop0":{"area":"0","networkType":"point-to-point"}},"redistribute":{"connected":{}}}}'
ngfw commit
```

The live neighbor table polls observed FRR state; it never treats candidate interfaces as adjacencies.
Full neighbors have a green state chip. Unavailable/invalid readers, partial observations and truncation
are explicit. API reads are authenticated and limited to 100 neighbors. The interfaces and database
routes expose bounded public scalar fields; database pagination uses `offset` and `limit` (1–100).
Reader output larger than 1 MiB is rejected rather than returned unbounded or silently treated as empty.

- `GET /api/v1/state/ospf?version=2` (or `version=3`)
- `GET /api/v1/state/routing/ospf/neighbors?version=3`
- `GET /api/v1/state/routing/ospf/interfaces?version=2`
- `GET /api/v1/state/routing/ospf/database?version=2&offset=0&limit=100`
- `/api/v1/state/routes?proto=ospf` shows the FIB independently of the adjacency table.

OSPFv2 MD5 accepts `auth:{"type":"md5","keyId":7,"keyRef":"password/ospf-link"}` under
an interface. The reference is resolved through the existing FRR secret resolver, never inline in
configuration. Keys must fit 16 bytes; longer keys fail instead of being truncated. Renderer output,
diffs, retrieve and diagnostics redact the key. **The production routing-password channel remains
unavailable until PENDING-secret-channel is approved.** Without an approved resolver, commit fails
at the auth reference; it never silently runs unauthenticated. Resolver fixtures verify source behavior.
`auth:{"type":"none"}` explicitly disables v2 authentication. OSPFv3 has no v2 MD5/BFD or NBMA
controls in this model; unsupported fields are rejected rather than silently ignored.

The existing FRR singleton re-renders the whole desired configuration on startup/reconciliation;
rollback removes the router and interface protocol lines. Neighbor changes publish EventKind 20 for
both families. No action on this screen restarts FRR or VPP. Packet/FIB convergence, route withdrawal,
restart recovery and deployed browser acceptance require the shared laboratory and are not claimed
by the source-only tests.

Command references: [FRR OSPFv2](https://docs.frrouting.org/en/latest/ospfd.html) and
[FRR OSPFv3](https://docs.frrouting.org/en/latest/ospf6d.html). Some older FRR builds print multiple
separate JSON documents for `vrf all` v3 reads; these are reported unavailable/invalid, not fabricated
healthy state. Verify the appliance's FRR version before accepting multi-VRF observation.
