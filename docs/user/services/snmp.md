# SNMP (v2c / v3) and the VRX private MIB

VRX runs net-snmp's `snmpd` as its SNMP agent. The agent renders `/etc/snmp/snmpd.conf` from
`services.snmp`, checks it with the daemon itself before anything is applied, and reloads snmpd. VPP
interface counters and agent health are served by the VRX-MIB subagent (AgentX) of `vrx-agent`.

- **UI**: Services → SNMP (general, communities, SNMPv3 users, trap receivers, live state).
- **API**: the configuration is `services.snmp` (generic pointer routes, e.g.
  `PATCH /api/v1/config/services/snmp`, then `POST /api/v1/config/commit`); the state is
  `GET /api/v1/state/snmp`.
- **Secrets**: community strings and SNMPv3 passphrases are never part of the configuration. Store them
  with `POST /api/v1/secrets` (kind `password`, admin only) and reference them as `password/<name>`. They
  are never returned by any GET and never logged.

> Current limitation: the API→agent secret channel is not built yet
> (docs/decisions/PENDING-secret-channel.md). Until it is, the agent resolves `password/…` references only
> from a slot-local test fixture; on an appliance, communities and users are refused with a clear error.

> By default snmpd listens on **loopback only** (`127.0.0.1:161`, `[::1]:161`). Set `listen` to reach it
> from the management network. Only the `default` VRF is supported: binding snmpd to another VRF needs a
> Linux VRF (linux-cp), which this build does not create.

## A v2c read-only community restricted to a management prefix

```sh
# 1. the community string (admin only; the value never comes back)
curl -sS -X POST https://vrx/api/v1/secrets -H "authorization: Bearer $TOKEN" \
  -H 'content-type: application/json' \
  -d '{"kind":"password","name":"snmp-noc","value":"<community, 8-64 of A-Z a-z 0-9 _ . ->"}'

# 2. the agent
curl -sS -X PATCH https://vrx/api/v1/config/services/snmp -H "authorization: Bearer $TOKEN" \
  -H 'content-type: application/json' -d '{
    "enabled": true,
    "listen": [{"address": "192.0.2.10"}],
    "sysName": "vrx-a", "sysLocation": "rack 12", "sysContact": "noc@example.net",
    "communities": {"noc": {"secretRef": "password/snmp-noc", "access": "ro", "sources": ["192.0.2.0/24"]}}
  }'
curl -sS -X POST 'https://vrx/api/v1/config/commit?comment=snmp' -H "authorization: Bearer $TOKEN"
```

`192.0.2.10` must be an address of an interface in the VRF (validated at commit). Requests from outside
`192.0.2.0/24` are not answered.

Restrict what a community sees with a view:

```json
"views": {"mgmt": {"include": ["system", "interfaces", ".1.3.6.1.4.1.8072.9999.9999.7853"]}},
"communities": {"noc": {"secretRef": "password/snmp-noc", "sources": ["192.0.2.0/24"], "view": "mgmt"}}
```

## An SNMPv3 authPriv user

```json
"v3Users": {
  "monitor": {
    "securityLevel": "authPriv",
    "authProtocol": "sha256", "authRef": "password/snmp-monitor-auth",
    "privProtocol": "aes",    "privRef": "password/snmp-monitor-priv",
    "access": "ro"
  }
}
```

Passphrases: 8–64 characters. `aes256` privacy is not available in the installed net-snmp build.

## A trap receiver

```json
"trapReceivers": [
  {"address": "192.0.2.50", "version": "v3", "user": "monitor"},
  {"address": "nms.example.net", "port": 162, "version": "v2c", "community": "noc", "inform": true}
]
```

A receiver must name an existing community (v2c) or user (v3); otherwise the edit is refused with
`400 application/problem+json` and the pointer `/services/snmp/trapReceivers/<i>/community`.
snmpd sends a `coldStart` trap when it starts.

## Walking the agent (from your management station)

`snmpwalk` is **not** installed on the appliance; run it on your own station:

```sh
# v2c
snmpwalk -v2c -c "$COMMUNITY" 192.0.2.10 system
# v3 authPriv
snmpwalk -v3 -l authPriv -u monitor -a SHA-256 -A "$AUTH" -x AES -X "$PRIV" 192.0.2.10 system
# the VRX-MIB interface table (numeric, or with the MIB file below: VRX-MIB::vrxIfTable)
snmpwalk -v3 -l authPriv -u monitor -a SHA-256 -A "$AUTH" -x AES -X "$PRIV" 192.0.2.10 .1.3.6.1.4.1.8072.9999.9999.7853
```

## The private MIB (VRX-MIB)

File: `deploy/snmp/VRX-MIB.txt` in the source tree (copy it to your NMS's MIB directory; it imports
`NET-SNMP-MIB`). Placeholder OID `1.3.6.1.4.1.8072.9999.9999.7853` (net-snmp's `netSnmpPlaypen`) until an
enterprise number is registered — expect it to change once.

| object | OID suffix | meaning |
|---|---|---|
| `vrxAgentVersion.0` | `.1.1.0` | agent version |
| `vrxVppConnected.0` | `.1.2.0` | true(1) while the agent is connected to VPP |
| `vrxRunningRevision.0` | `.1.3.0` | last applied transaction id |
| `vrxCommitCounter.0` | `.1.4.0` | applied transactions observed |
| `vrxIfTable` | `.2.1.<col>.<sw_if_index+1>` | VPP interfaces: name (2), admin (3), oper (4), in/out octets (5/6), in/out packets (7/8), in/out errors (9/10) — Counter64 |

The standard IF-MIB (`.1.3.6.1.2.1.2`) is net-snmp's own and shows the **Linux** interfaces (taps), not
VPP's; use `vrxIfTable` for the data plane. Switch the subagent off with `"subagent": {"enabled": false}`.

## State

`GET /api/v1/state/snmp` shows whether the configuration is applied, whether snmpd answers (read back over
SNMP by the agent, credential named only), `pendingAction` — e.g. a changed `listen` address or engine id is
applied by snmpd only at a restart; the agent writes the file and reports the needed restart instead of
restarting snmpd itself — and whether the VRX-MIB subagent is registered.
