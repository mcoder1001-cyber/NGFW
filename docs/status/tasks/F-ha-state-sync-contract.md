# Additive HA state-sync contract
No existing shape removed/renamed. HaCluster.StateSync fields4 nat_listener and5
nat_failover; ActionRequest13 ha_sync; new HaSyncState RPC and messages at allocated
F-ha-state-sync section. No new EventKind: native resync completion is consumed
inside the bounded action. Endpoint IPv4/UDP/path-MTU and refresh validation;
cluster-disabled state sync rejected with exact pointer. NAT44-ED/ACL/IPsec flags
remain accepted and are reported as unsupported warnings by the agent.
NAT HA UDP is unauthenticated; the configuration-sync cluster secret cannot secure
it. Native API has no explicit VRF/interface binding field: use a dedicated sync
interface and isolated network/ACL; do not claim encryption or VRF enforcement.
State telemetry distinguishes configured/observed matching endpoints from true
remote session continuity. Packet counters explicitly unavailable; process-local
completed-action count and last unacknowledged HA-message count are accurately named.
