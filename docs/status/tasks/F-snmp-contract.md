# F-snmp — contract change (D-086): RF-4's SNMP stand-ins into `services.snmp`

On branch `task/F-snmp` (envelope: no own contract branch). Additive only; no field renamed, renumbered or reshaped.
Numbers from `docs/status/wave-BC-numbers.md` § F-snmp.

| JSON path (schema) | proto | shape / bounds (same limits as `renderers/snmpd/model.go`) |
|---|---|---|
| `services.snmp.sysServices` | `SnmpService.sys_services = 12` (`optional uint32`) | 0–127, optional |
| `services.snmp.views` | `SnmpService.views = 13` (`map<string, SnmpView>`) | record, default `{}`; key `[A-Za-z0-9_.-]{1,64}`, not `vrx_all`; `SnmpView{repeated include = 1; repeated exclude = 2}`, include 1–32, exclude ≤ 32, numeric OID or the renderer's symbolic allow-list |
| `services.snmp.monitors` | `SnmpService.monitors = 14` (`SnmpMonitors{repeated SnmpMonitorDisk disks = 1; SnmpMonitorLoad load = 2}`) | optional; disks ≤ 16 `{path (absolute clean), minPercent 1–99 default 10}`; load `{max1, max5, max15}` 1–1000, all required |
| `services.snmp.subagent` | `SnmpService.subagent = 15` (`SnmpSubagent{optional bool enabled = 1}`) | optional; absent = enabled; `enabled` default `true` — the UI switch for the VRX-MIB AgentX subagent |
| `services.snmp.communities.<n>.view` | `SnmpService.Community.view = 4` | optional view name |
| `services.snmp.v3Users.<n>.view` | `SnmpService.V3User.view = 7` | optional view name |

New messages in the `// ----- F-snmp -----` section: `SnmpView`, `SnmpMonitors`, `SnmpMonitorDisk`, `SnmpMonitorLoad`,
`SnmpSubagent`. No new RPC (`SnmpState` not needed: the renderer's Retrieve state is carried by the API's
`GET /api/v1/state/snmp` through the existing Retrieve of `services`), no `ActionRequest` member, no `EventKind`.

Schema: sub-schemas in `packages/schema/src/domains/ext/snmp.ts`; key lines in `domains/services.ts`
(`SnmpSchema`, `SnmpCommunitySchema`, `SnmpV3UserSchema`) — **unanchored** (no `wave-BC: F-snmp` anchor exists in
those blocks; appended at the end of each block, marked `// F-snmp (unanchored)`); export in `src/index.ts`
(unanchored, after the anchor list).

Verification: `packages/proto` vitest 70/70 (incl. the new round-trip fixture `test/fixtures/snmp-full.json`),
`buf lint` clean, `apps/agent/internal/contracttest` ok, schema vitest 1219/1219.
