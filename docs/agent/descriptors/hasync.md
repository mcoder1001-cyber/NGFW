# NAT44-EI HA descriptors

`nat44-ei-ha.listener/global` and `nat44-ei-ha.failover/global` use the pinned
NAT44-EI binary API setters/getters, not CLI text. Listener depends on NAT EI
plugin enable; failover also depends on listener. Typed JSON specs carry IPv4,
UDP port, path MTU and refresh seconds. Getter replies are the observable source.

Only the startup-resolved globals owner sets or resets these singleton values.
Other agents require exact existing values, do not enumerate them as owned, and
never reset them. Mutations use the D-082 `ngfw-globals.lock`; cancellation while
waiting fails without mutation. Deletes disable the native endpoint with zero
address/port. Scheduler rollback re-applies the previous typed spec; reconnect
uses existing normal reconcile, with no new background owner or plugin mode change.

HA configuration projection requires an enabled cluster, NAT EI mode, explicit
local/remote IPv4 endpoints and a dedicated interface owning the local address.
The native API has no VRF parameter: a non-default sync VRF warns and applies no
endpoints. NAT ED, ACL and native IPsec state sync emit explicit support warnings.
Native IPsec requires IKEv2 rekey after failover; no SA sequence/replay sync exists.

The resync action subscribes before sending, requests a completion event, and
matches the native PID correlation value. It has a 15-second deadline and checks
startup globals ownership before native calls. The service checks live endpoints
against running configuration. A matching completion with unacknowledged messages
is a failed action, with its observation retained. Flush only flushes queued HA
updates; it does not delete NAT sessions. Completion count is process-local and
resets after restart. Packet byte/packet counters are unavailable in this API,
so the state endpoint exposes availability explicitly rather than invented zeroes.

Retrieval assembles endpoint leaves only. It never infers enabled flags, node
membership, secret references or peer session continuity from native endpoints.
The dedicated observed RPC separates desired configured flags from native activity.
