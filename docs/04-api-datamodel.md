# API and data model

## Datastore schema (PostgreSQL, essentials)

```sql
config_revision(id, created_at, author_id, comment, parent_id, payload jsonb, hash)
config_candidate(id, owner_id, locked_at, payload jsonb, base_revision_id)
audit_log(id, ts, user_id, source_ip, action, resource, before jsonb, after jsonb, result)
app_user(id, username, password_hash, role, mfa_secret, disabled, last_login)
api_key(id, user_id, name, hash, scopes, expires_at, last_used)
secret(id, kind, ref, ciphertext, created_at)          -- PSKs, private keys
system_event(id, ts, severity, subsystem, code, message, data jsonb)
```

`payload` is the **whole config document** as one JSON object validated by one root
JSON Schema. Storing whole snapshots (not per-row config) makes diff, rollback, export
and import trivial, and 900k-route BGP tables never live here anyway.

## Config document shape

```jsonc
{
  "system":     { "hostname": "...", "timezone": "...", "ntp": {...}, "dns": {...} },
  "dataplane":  { "workers": 8, "rxQueues": 4, "hugepages": "16G", "pciWhitelist": [...] },
  "interfaces": { "TenGigabitEthernet0/0/0": { "enabled": true, "mtu": 9000,
                    "ipv4": ["10.0.0.1/24"], "vrf": "default", "subinterfaces": {...} } },
  "vrfs":       { "default": {...}, "customer-a": { "id": 10 } },
  "routing":    { "static": [...], "bgp": {...}, "ospf": {...}, "isis": {...}, "bfd": {...} },
  "nat":        { "mode": "endpoint-dependent", "inside": [...], "outside": [...],
                  "static": [...], "portForwards": [...], "cgnat": {...} },
  "objects":    { "addresses": {...}, "addressGroups": {...}, "services": {...}, "schedules": {...} },
  "acl":        { "lists": {...}, "attachments": [...] },
  "vpn":        { "ipsec": { "tunnels": {...}, "proposals": {...} }, "wireguard": {...} },
  "tunnels":    { "gre": {...}, "vxlan": {...}, "ipip": {...} },
  "services":   { "dhcp": {...}, "dns": {...}, "snmp": {...}, "lldp": {...}, "ipfix": {...} },
  "ha":         { "vrrp": [...] },
  "management": { "users": [...], "aaa": {...}, "tls": {...}, "syslog": [...] }
}
```

Each top-level key owns a Zod schema in `packages/schema`, which generates:
TypeScript types → OpenAPI components → JSON Schema for the UI form renderer. **One
definition, three consumers.** This is the highest-leverage decision in the codebase.

## Endpoint families

```
GET    /api/v1/config                      whole running config
GET    /api/v1/config/candidate            whole candidate
PATCH  /api/v1/config/{path}               edit candidate (RFC 7386 merge-patch)
PUT    /api/v1/config/{path}               replace node in candidate
DELETE /api/v1/config/{path}
GET    /api/v1/config/diff                 structured candidate↔running diff
POST   /api/v1/config/validate             3-tier validation, no apply
POST   /api/v1/config/commit?confirm=120   apply (optionally with auto-revert timer)
POST   /api/v1/config/commit/confirm       cancel the auto-revert
POST   /api/v1/config/discard
GET    /api/v1/config/revisions            history
POST   /api/v1/config/rollback/{rev}
GET    /api/v1/config/export  |  POST /api/v1/config/import

GET    /api/v1/state/interfaces            live, read-only
GET    /api/v1/state/routes?vrf=&proto=&prefix=   server-side paged
GET    /api/v1/state/bgp/neighbors
GET    /api/v1/state/ipsec/sas
GET    /api/v1/state/sessions              NAT/flow table, paged+filtered
GET    /api/v1/state/system                CPU per worker, memory, hugepages, temps

POST   /api/v1/actions/ping                { target, vrf, count } → streamed result
POST   /api/v1/actions/traceroute
POST   /api/v1/actions/capture             BPF filter → pcap download
POST   /api/v1/actions/clear-counters
POST   /api/v1/actions/reboot | shutdown
POST   /api/v1/actions/upgrade             signed bundle, staged, auto-rollback

WSS    /api/v1/stream                      { subscribe: ["iface.counters", "bgp.events", ...] }
```

## gRPC agent surface (protobuf sketch)

```proto
service Dataplane {
  rpc Apply(ApplyRequest) returns (ApplyResponse);       // desired state, whole or per-subsystem
  rpc Retrieve(RetrieveRequest) returns (DesiredState);  // actual state dumped from VPP+daemons
  rpc DryRun(ApplyRequest) returns (ValidationReport);
  rpc StreamStats(StatsRequest) returns (stream StatsBatch);
  rpc StreamEvents(EventRequest) returns (stream Event); // link up/down, SA rekey, BGP transitions
  rpc Action(ActionRequest) returns (stream ActionOutput); // ping, capture, vppctl passthrough (admin only)
}
```

`Apply` carries a transaction id and a `confirmTimeoutSec`; the agent keeps the previous
desired state in memory so it can self-revert without talking to the API.
