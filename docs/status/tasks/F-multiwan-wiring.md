# F-multiwan-wiring recovery closeout

Source reconciliation: main already integrates the implementation in PR154 (9994929bb): durable health probes, owned failover and weighted routes, per-member ED NAT interfaces/pools, bounded dead-link session cleanup, ABF group expansion and drift recovery. This row was stale rather than a missing entire implementation.

This task's new change fixes ABF group expansion when IPv4 and IPv6 groups share WAN interfaces: policy paths select the route's VRF/address family identity. Previously both groups could choose the IPv6 route solely because the interface matched. Regression uses separate active members for the two families.

Branch codex/F-multiwan-wiring-20261004. PR159. Initial published SHA5ecb349d5ed26fa02cbcb71d7ba046119df346a2 matches localceb753880 tree37bb76170fd81bf6cbf60edcfac360ce28c586eb. CLI push403; authorized GitHub connector publication verified. Subsequent documentation checkpoint SHA is the branch head shown in GitHub.

Actual validation: `go test -race ./internal/multiwan -count=1` PASS `ok ngfw/agent/internal/multiwan 2.090s`. Root independent code review APPROVE: route identity matches VRF/default family and regression covers shared-member dual-stack selection. Hosted quick gate initially failed two baseline schema/UI fixture regressions; root is fixing those on prerequisite PR162 before revalidation. No skipped or changed quick gate is claimed.

Laboratory acceptance remains explicitly deferred in docs/status/DEFERRED-ACCEPTANCE.md: real timed failover/restore, 1000-flow weighted distribution and affinity, NAT/ABF packets and appliance restart/rollback. Static gateways and default namespace/default VRF probes are the current supported modes; DHCP/PPPoE gateway handoff remains a separate functionality item. Native ECMP sticky membership changes are unproven; documented configuration fallback is failover mode.

Manager next action: merge prerequisite quick-fixture correction, rebase PR159, run unchanged complete hosted quick gate on exact integration tree and merge sequentially with expected-head checks. Update the board after actual merge; no worker board edits.
