# snmpd renderer (RF-4, WBS D7.5)

`services.snmp` → `snmpd.conf` → daemon parse run → SIGHUP → SNMP GET of the system group → 1 Hz events.
Mapping table: `docs/agent/renderers/snmpd.md`. Shared pieces (controller, secrets, apply, poller): `../rfkit`.

## Paths and files

| | product (`ProductPaths`) | tests (`TestPaths("w8")`) |
|---|---|---|
| config | `/etc/snmp/snmpd.conf`, `root:root 0600`, `Secret` | `/run/vrx-test/w8/snmpd/snmpd.conf`, 0600 |
| AgentX master socket | `/run/vrx/snmpd/agentx.sock` (F-snmp subagent) | `/run/vrx-test/w8/snmpd/agentx.sock` |
| control channel | `systemctl reload snmpd` (SIGHUP) — `rfkit.SystemdController` | SIGHUP to the child PID — `rfkit.ProcessController` |

One file only. `createUser` lines live in the main `snmpd.conf`, **not** in the persistent store
(`/var/lib/snmp/snmpd.conf`): experiment 2026-09-24 — with `-C` snmpd never reads the persistent store, and
snmpd rewrites that file itself (so a rendered copy would drift). `createUser` in `snmpd.conf` is re-read on every
SIGHUP; a changed passphrase takes effect on the reload (verified). A removed user keeps its USM entry in memory
until the next restart but loses its `rouser`/`rwuser` line on the reload, i.e. all access (VACM) at once.

## Secrets

Communities (`secretRef`, kind `password`) and USM passphrases (`authRef`/`privRef`, kind `password`) are D-051
references resolved at Render through `WithSecretResolver`. Values must be one word: communities
`[A-Za-z0-9_.-]{1,64}`, passphrases `[A-Za-z0-9_.,:;@%+=/~^*!?-]{8,64}` (net-snmp's USM minimum is 8). The
plaintext exists only in `snmpd.conf` (0600, `File.Secret`). The renderer's `rfkit.Redactor` masks every value it
resolved **or read back from the live file** in errors, Validate output, Retrieve and events; a fresh agent learns
them from the file before it reports anything. Retrieve names the credential it used (`"v3 user u1"`,
`"v2c community"`), never the value. SNMP queries run in-process through gosnmp (BSD-2): with `snmpget` the
community/passphrase would be in a process argv (`/proc/<pid>/cmdline`).

## Validate — there is no offline checker

net-snmp has no `-t`/check mode (`-H` only lists directive names). Validate therefore does a **parse run**:

1. `CheckCopy`: a staged copy in which `agentaddress` → `unix:<staging>/agent.sock`, `agentXSocket` →
   `unix:<staging>/agentx.sock`, trap/inform sinks → a comment (no packet leaves; line numbers unchanged), plus
   `[snmp] persistentDir <staging>/persist` appended.
2. `snmpd -C -c <copy> -Lf <log> -p <pid> -m "" -M <empty dir>` — daemonises after reading the config.
3. The instance is stopped by its pidfile PID (verified `/proc/<pid>/exe` = snmpd and cmdline contains the staging
   dir; SIGTERM, SIGKILL after 3 s). The staging dir is removed.
4. Any `Warning`/`Error`/`Unknown token`/`line N:` in the log (minus a fixed benign list) rejects the file; the
   message is redacted and the staging path shortened. Verified live: a bogus token, an OID that is not an OID and a
   7-character passphrase are rejected with snmpd's own words.

What the parse run cannot see (validated structurally instead): the listen addresses (rewritten) and the trap sinks
(removed). The typed model (`BuildModel`) validates every field with its JSON path first.

## Apply

`rfkit.ApplyFiles`: snapshot → atomic write (0600) → `Reload` → **convergence**: GET `sysName.0`,
`sysLocation.0`, `sysContact.0` until they equal the rendered values (5 s) → on any failure restore + reload
(own context). The credential and endpoint for that GET come from the rendered file itself (first v3 user with
auth, else a community whose source admits 127.0.0.1; endpoint 127.0.0.1 through the wildcard or an explicit
loopback listen address, else the first IPv4 listen address). When there is nothing the agent could ask (disabled,
v3 noAuth only, no local credential), the check is skipped — documented limitation.

`enabled: false` renders a config that listens only on `unix:<agentx>.disabled` and grants no access (the unit's
start/stop/enable is P10/F-snmp, never this renderer). `vrf` other than `default` is rejected until F-snmp binds
snmpd to a VRF.

## Retrieve / events

`State`: `configured`, `reachable`, `endpoint`, `credential`, `sysName`, `sysDescr`, `sysLocation`, `sysContact`,
`sysUpTime`, `error` (redacted). An unreachable agent is a state, not an error. `Poller()` (1 Hz): `reachable`,
`restarts` (sysUpTime going backwards), `sysName`/`sysLocation`/`sysContact` changes.

## Integration test

`VRX_INTEGRATION=1 go test -run TestSnmpdIntegration ./internal/renderers/snmpd/` (slot from `VRX_TEST_PREFIX`/
`VRX_SLOT`): child `snmpd -f -Lf <dir>/snmpd.log -C -c <dir>/snmpd.conf -p <dir>/snmpd.pid -m "" -M <dir>/mibs`
with `SNMP_PERSISTENT_DIR=<dir>/persist SNMPCONFPATH=<dir>`, bound only to `udp:127.0.0.1:3<N>61` (asserted on the
rendered file before start). No endpoint on argv: with `agentaddress` in the file that would bind twice ("Error
opening specified endpoint"). Checks: parse run accepts the full file and rejects a broken copy; v3 + v2c GETs; a
wrong community is not answered; SIGHUP applies a `sysLocation` change with the same PID; a reload that never
reaches the daemon is refused and rolled back; a poller event; secrets only in the 0600 file. Exclusive
`/run/vrx-test/<prefix>/snmpd.lock` per slot (RF-1 M4); the child is killed by its PID in `t.Cleanup`.
