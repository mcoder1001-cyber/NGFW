# HA session state

| Kind | Support | Failover behavior |
|---|---|---|
| NAT44-EI | Native UDP HA endpoints and resync | Session preservation requires matching two-node config and tested delivery |
| NAT44-ED | Unsupported (V2) | Existing sessions are lost |
| Reflexive ACL | Unsupported (V2) | Stateful entries are not replicated |
| IPsec | No sequence/replay-window sync API | Native IKEv2 must re-key on failover |

Set `ha.cluster.enabled`, select a dedicated sync interface and request
`stateSync.nat`. With `nat.mode: ei`, provide `stateSync.natListener` with
`address`, `port` (8750 default), `pathMtu` (1500 default), and `natFailover`
with remote `address`, `port`, `sessionRefreshSec` (10 default). The listener
address must exist on the selected interface; the peer must be a different
unicast IPv4 address. Use the default VRF: the native API has no VRF binding.
Existing configuration sync and VRRP remain separate controls.

Native NAT HA is **unauthenticated UDP**. Isolate it on a trusted link and restrict
reachability with existing policy. The cluster `secretRef` authenticates config
sync only; it cannot authenticate native NAT HA packets. Membership configuration
is not proof of remote sessions.

GET `/api/v1/state/ha/sync` returns configured flags separately from observed
listener/peer activity. Active means both observed endpoints exactly match the
running config; it does not prove peer reachability or session continuity. ED,
ACL and IPsec always show explicit unsupported badges. No packet counters are
available. Last completion, missed HA-message count and completion count reflect
only this agent process, and are unknown/zero after a restart.

An administrator on the globals-owner appliance can POST
`/api/v1/actions/ha/sync/resync`. The response waits for a matching native completion;
a timeout or unacknowledged messages fails. The operation is audited without keys.
A read-only user or slot agent cannot issue it. The HA cluster tab includes a
localized state-sync panel with the same restrictions.

No production convergence numbers are claimed: two-node NAT session continuity,
packet capture, VRRP convergence, native IPsec rekey and restart timing are deferred
until the isolated laboratory is provisioned. `test/topology/ha-state-sync/acceptance.py`
checks the exact native EI tuple, VRRP roles and one persistent TCP connection
through owned priority changes, then verifies restoration. Follow
`test/topology/ha-state-sync/README.md` for the leased candidate and optional
owned-VM fault mode. This procedure operates only on the provisioned private rig.
