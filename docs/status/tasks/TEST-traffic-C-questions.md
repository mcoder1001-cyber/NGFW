# TEST-traffic-C questions and actual constraints

1. The prompt references `test/topology/vrrp/host.sh`, absent at base d314f0728.
   The in-tree C driver implements its own prefix-scoped keepalived bridge peer.
2. `StaticRouteSchema` has no outLabels. The product-only alternative implemented
   here is SR-MPLS steering to a labelled next-hop MPLS route. No schema change.
3. Existing table0 cannot be adopted by this globals-owner slot agent:
   `apps/agent/internal/descriptors/mpls/mpls.go` `foreign()` rejects another
   owner; `NewTableFor(..., globalsOwner=false)` only requires an existing table0,
   but that same non-global role reports `agent.unsupported-field` for IPFIX
   exporter0/flowprobe parameters. The combined strict driver therefore refuses
   preexisting table0 before host mutations. A manager private VPP with no table0
   can execute it as written. Supporting a preexisting shared table0 while keeping
   all strict API checks would require manager-owned stack role transitions or a
   separate product change; the driver never deletes or renames foreign table0.
   This is an explicit constraint, not passing shared-host evidence.
4. `tools/ci-slot.sh` is absent at this base; unchanged gate is `tools/ci.sh`.

No actual product execution failure has been observed: live execution is NOT RUN.
Do not invent a green host result or close these acceptance constraints from
unit fixtures. Manager owns `DEFERRED-ACCEPTANCE.md` updates.
