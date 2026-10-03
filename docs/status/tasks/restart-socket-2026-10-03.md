# Restart/socket fixture repair — 2026-10-03

Legacy topology fixtures constructed clean agent environments but dropped the required VPP ownership range. Bonding, host ACL, object model, VLAN/QinQ, bridge L2, NAT44 ED/EI and loopback/BVI now pass their parsed slot's `VRX_VPP_TABLE_BASE`; neighbors/RA already passed it. The product's explicit-range guard remains enabled.

After a restart, a socket file alone did not establish readiness of the API's persistent gRPC connection. Its expected reconnect backoff retained an earlier ENOENT error even after the new agent reconciled VPP. Nine fixtures now require a connectable Unix socket and, on restart, a successful read-only API RPC before mutations. Commits are never retried; agent downtime remains observable as 503. No API production connection semantics changed.

The LBGS fixture predated the tunnels desired-state family: it tagged ERSPAN as agent-owned but only named its generic interface, so resync correctly removed an undesired owned tunnel. It now declares the tunnel under `tunnels.gre` alone, uses its slot's valid tunnel instance range, supplies an independent configured underlay source and explicitly opts its test API into the NSIM lab gate. `VRX_GLOBALS_OWNER=0` stays in force, so NSIM globals are not changed. Fixture cleanup handles failures before LLDP adoption and refuses foreign owner tags. No GRE/IPIP descriptor ownership defect was found; owned IPIP fixtures must likewise declare their tunnel desired state.

Neighbors/RA's exact prefix lifetime assertion now bounds the remaining lifetime by actual time elapsed since apply plus integer-second quantization. Lifetimes above the configured target or decaying faster than elapsed time still fail. Its strict drift assertion remains enabled. The descriptor now reconstructs configured lifetimes only when both observed expiration deadlines match the persisted setter timing window; the final topology verifies this fix across restart and rollback.

## Validation

Each topology ran with `go test -json -race -count=1 -timeout=8m ./...`, real API and agent, its own throwaway database, slot 6 and a fresh disposable VPP. Follow-through runs used a freshly built agent and rebuilt API including drift canonicalization. The shared system VPP remained PID 1014, NRestarts 0; no own network namespaces remained after cleanup. `git diff --check` passed.

| Topology | Latest result | Detail |
|---|---|---|
| Host ACL/nftables | PASS (20.69 s) | Restart, rollback, nftables restoration |
| Object model | PASS (74.60 s) | 8 passing test/subtest results; persistence/resolver/rollback |
| VLAN/QinQ | PASS (21.00 s) | 6 passing results; restart/rollback |
| Bonding | PASS (15.83 s) | Restart, empty drift, rollback after API drift fix |
| Bridge L2 | PASS (19.84 s) | 6 passing results |
| NAT44 ED | PASS (30.24 s) | 7 passing results; dataplane, restart/rollback |
| NAT44 EI/NAT64/NAT66/NPTv6 | PASS (28.48 s) | 8 passing results; restart/rollback |
| Loopback/BVI/GSO/LLDP/SPAN | PASS (17.95 s) | Owned ERSPAN adoption, restart, both rollbacks, clean leftovers |
| Neighbors/RA | PASS (15.03 s) | Strict empty drift, bounded lifetime assertion, restart/recreate and rollback |

Screenshot and explicit packet evidence opt-ins remain skipped where indicated in JSON counts; those skips are not converted into passes. Earlier legacy failures remain in the evidence directory. A fresh production restart regression (`secret channel: snapshot identity mismatch` for nil/empty secret maps) was discovered and reported to the root agent; its cache canonicalization fix was validated by the subsequent successful restart runs. Those failed attempts are preserved under `secret-cache-regression/`.

All nine topology scopes passed. Latest JSON evidence uses the `final-` prefix. The final RA run includes the descriptor apply-to-dump timing-window hardening; its rebuilt snapshot hashes are recorded in `ra-fixed-builds.sha256`. Earlier countdown failures and the first successful pre-hardening run remain separately preserved. Machine counts: [summary](restart-socket-2026-10-03-evidence/summary.json); snapshot binary hashes: [builds](restart-socket-2026-10-03-evidence/final-builds.sha256). No secrets were found by the focused token/private-key log scan.
