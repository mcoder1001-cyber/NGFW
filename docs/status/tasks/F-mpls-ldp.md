# F-mpls-ldp — LDP (RFC 5036)

Merged in PR #56 (2026-09-27). In-container slice; the FRR section + FRR→VPP label sync are F-mpls-ldp-host.

## Delivered

- **Contract**: `routing.mpls.ldp` (routerId, transportAddress, interfaces, neighbors{passwordRef?}, labelRange?) with
  semantic validation (MPLS-enabled/default-VRF interfaces, local transport address, IPv4 neighbour keys, secret-ref
  password, no static/SR label in the LDP dynamic range). Proto `MplsConfig.ldp=10` + messages, `MplsLdpState` RPC,
  `EventKind 23`; drift + buf green; regenerated YANG.
- **API**: `GET /api/v1/state/routing/mpls/ldp/{neighbors,bindings,sync}` (bindings paged; sync status), agentError on
  501; `mpls-ldp.events` topic. Fake agent stubs `MplsLdpState`. e2e green.
- **Web**: LDP tab on Routing → MPLS (neighbours, LIB, sync chip); en/fa.
- **Docs**: `docs/user/routing/mpls-ldp.md`.

## Deferred → F-mpls-ldp-host (needs lab FRR/VPP)

- FRR `mpls ldp` renderer section (RF-1), rendered to FRR's canonical `show running-config` form (probe first).
- `frrsync/ldp`: read `show mpls ldp binding/neighbor json`, translate FEC+in-use bindings to `mpls-route.ldp` (its own
  scope), install via the scheduler through seam S1 with the LCP (Linux→VPP) mapping; PHP/ECMP; hold-down + flush.
- The neighbour poller (up/down → `mpls-ldp.events`), and the live host session test.
- Table 0 only under `VRX_DF7_GLOBALS` with the globals lock. Ingress imposition and LDP IPv6 remain out of scope.
