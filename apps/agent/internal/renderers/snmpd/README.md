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
`[A-Za-z0-9_.-]{8,64}` (review L1: at least 8), passphrases `[A-Za-z0-9_.,:;@%+=/~^*!?-]{8,64}` (net-snmp's USM minimum is 8). The
plaintext exists only in `snmpd.conf` (0600, `File.Secret`). The renderer's `rfkit.Redactor` masks every value it
resolved **or read back from the live file** — as whole tokens only, so a secret never blanks
unrelated words (review L1) — in errors, Validate output, Retrieve and events; a fresh agent learns
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
4. snmpd's own structured problem messages — `<file>: line N: Error|Warning: …`, a line starting `Error:` /
   `Warning:`, `Unknown token`, `Error opening …` (review L2: no loose keyword match; one benign message for a
   disabled agent) — reject the file; the
   message is redacted and the staging path shortened. Verified live: a bogus token, an OID that is not an OID and a
   7-character passphrase are rejected with snmpd's own words.

What the parse run cannot see (validated structurally instead): the listen addresses (rewritten) and the trap sinks
(removed). The typed model (`BuildModel`) validates every field with its JSON path first.

## Apply

snapshot → atomic write (0600) → then one of (review H1, D-079):

- **startup-only directive changed** (`agentaddress`, `agentXSocket`, `agentXPerms`, `master`, `exactEngineID`):
  SIGHUP re-reads the file but **keeps the old sockets** (reproduced live: after 3862→3863 + SIGHUP the agent
  answered on 3862 with the new sysLocation, 3863 refused). Apply returns `*rfkit.ActionRequired{restart}`,
  persisted in `Paths.PendingFile` (kernel boot_id + start tick, TD-1 `bootid.Reader`) and returned by every Apply
  until snmpd's main process started after the request; the file stays written. The commit engine restarts the
  unit; the next Apply (or `Converged`) clears the request.
- **snmpd not running**: nothing to reload; an enabled agent → `ActionRequired{start}`.
- **otherwise** `Reload` (SIGHUP) → **convergence** (5 s): the UDP sockets snmpd's main process actually holds
  (`/proc/<pid>/fd` → `/proc/<pid>/net/udp{,6}`, unconnected, ephemeral-range client sockets ignored) equal the
  rendered listen set exactly — a stray `0.0.0.0:161` fails it — **and** GET `sysName.0`, `sysLocation.0`,
  `sysContact.0` equal the rendered values → on any failure restore + reload (own context).

`Converged(ctx)` runs the same socket + value check on the live file (for the engine after it restarted snmpd).
The main PID comes from the controller (`systemctl show snmpd -p MainPID`, or the test's child). The credential and endpoint for that GET come from the rendered file itself (first v3 user with
auth, else a community whose source admits 127.0.0.1; endpoint 127.0.0.1 through the wildcard or an explicit
loopback listen address, else the first IPv4 listen address). When there is nothing the agent could ask (disabled,
v3 noAuth only, no local credential), only the socket check applies.

**Default listen (review M1):** without `services.snmp.listen` snmpd binds `udp:127.0.0.1:161,udp6:[::1]:161`
only, never `0.0.0.0`/`[::]`. The rendered file carries `# WARNING:` lines for that default and for any explicit
wildcard listen address; `snmpd.Warnings(files, paths)` returns them for the commit engine (they never fail
Validate).

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
