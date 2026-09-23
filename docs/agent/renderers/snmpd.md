# snmpd renderer — desired state ↔ rendered directives (RF-4)

Code: `apps/agent/internal/renderers/snmpd` (README there: parse-run Validate, secrets, convergence, test harness).
File: `/etc/snmp/snmpd.conf`, `root:root 0600` (holds communities and USM passphrases), written atomically; applied
with `systemctl reload snmpd` (SIGHUP, same PID); validated by a daemon parse run of a staged check copy; state
read over SNMP (gosnmp, in-process).

Stand-in fields (D-055) are read from a `*structpb.Struct` input at the JSON path the schema would use; they are
not in `packages/schema` yet (RF-4-questions.md Q2).

| desired state (JSON path) | rendered | validation |
|---|---|---|
| `services.snmp.enabled` false/unset | `agentaddress unix:<agentx>.disabled`, nothing else (no access, no network) | — |
| `services.snmp.vrf` | — | only `default` (F-snmp binds VRFs) |
| `services.snmp.listen[i].{address,port}` | `agentaddress udp:<ip>:<port>[,udp6:[<ip6>]:<port>…]` (empty: `udp:0.0.0.0:161,udp6:[::]:161`) | `netip`, no zone, port 1–65535 (default 161), no duplicates |
| — | `dontLogTCPWrappersConnects yes` | fixed |
| `services.snmp.engineId` | `exactEngineID 0x<hex>` | 5–32 hex bytes |
| `services.snmp.sysName` | `sysName <h>` | hostname, ≤ 253 |
| `services.snmp.sysLocation`, `.sysContact` | `sysLocation <text>` / `sysContact <text>` — rest of the line, written verbatim (snmpd keeps quotes literally) | printable ASCII 1–255, no leading/trailing blank; `"; rm -rf /` is legal text and stays on its line (golden `hostile-location`) |
| `services.snmp.sysServices` *(stand-in)* | `sysServices <n>` | 0–127 |
| `services.snmp.views.<name>.{include,exclude}[]` *(stand-in)* | `view <name> included\|excluded <numeric OID>` | name `[A-Za-z0-9_.-]{1,64}` ≠ `vrx_all`; OID numeric or from the fixed symbolic list (`system`, `interfaces`, `ifMIB`, `mib-2`, …) → numeric; ≤ 32 each |
| — | `view vrx_all included .1` (default view) | fixed |
| `services.snmp.communities.<name>` `secretRef` (password/…) | `rocommunity[6] <community> <source> -V <view>` (`rw…` for `access: rw`), one line per source; no sources → `default` for IPv4 and IPv6 | D-051 ref of kind `password`; value `[A-Za-z0-9_.-]{1,64}`; sources CIDR (masked) |
| `….communities.<name>.view` *(stand-in)* | `-V <view>` | must be a defined view |
| `services.snmp.v3Users.<name>` | `createUser <name> <AUTH> "<auth>" [<PRIV> "<priv>"]` + `rouser\|rwuser <name> noauth\|auth\|priv -V <view>` | name token; `securityLevel` noAuthNoPriv/authNoPriv/authPriv with matching refs; auth md5/sha/sha256/sha512 → MD5/SHA/SHA-256/SHA-512; priv aes/des → AES/DES (**aes256 rejected**: not in this net-snmp build); passphrases kind `password`, `[A-Za-z0-9_.,:;@%+=/~^*!?-]{8,64}` |
| `….v3Users.<name>.view` *(stand-in)* | `-V <view>` | defined view |
| `services.snmp.trapReceivers[i]` v2c | `trap2sink\|informsink <host> <community> <port>` | host IP (IPv6 bracketed) or hostname; `community` names an entry of `communities` (its resolved string is rendered) |
| `services.snmp.trapReceivers[i]` v3 | `trapsess [-Ci] -v 3 -u <user> -l <level> [-a <A> -A "<auth>"] [-x <P> -X "<priv>"] udp:<host>:<port>` | `user` names an entry of `v3Users` |
| — | `master agentx` / `agentXSocket unix:<path>` / `agentXPerms 0600 0700` | path from `Paths` (F-snmp subagent) |
| `services.snmp.monitors.disks[i].{path,minPercent}` *(stand-in)* | `disk <path> <n>%` | absolute clean path `[A-Za-z0-9_./-]`, 1–99 (default 10), ≤ 16 |
| `services.snmp.monitors.load.{max1,max5,max15}` *(stand-in)* | `load <1> <5> <15>` | 1–1000, all three required |
| `services.snmp.description` | not rendered | — |

Unknown keys inside stand-in objects are errors. Retrieve: `configured`, `reachable`, `endpoint`, `credential`
(named, never the value), `sysName`, `sysDescr`, `sysLocation`, `sysContact`, `sysUpTime`, `error`.
