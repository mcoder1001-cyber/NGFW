# RF-3: review (independent reviewer, 2026-09-24)

Branch `task/RF-3` @ `30be7d6` (base `main@179676a`). I reviewed it on the host, slot 6, and wrote none of this code.
`git merge-tree` against the current `main` (RF-1 merged) is clean. I also extracted the merged tree and ran
`go test ./internal/renderers/...` on it: renderers, frr, frrtest, kea, unbound and chrony all pass. So the
ALLOWLIST.md edit (Q7) merges without conflict and `TestAllowlistDocumented` passes with both sections present.

## What I ran

| check | result |
|---|---|
| `tools/ci.sh --base main` (my run, log `/root/ngfw-wt/logs/ci/RF-3-20260924-013205-1399558`) | `CI GATE PASSED`, wall time 0m55s. Contract guard: "no contract files changed in the 11 commit(s)". gitleaks: "no leaks found". kea, unbound and chrony all `ok`. Matches the run pasted in `RF-3.md` |
| `VRX_INTEGRATION=1 go test -run Integration` for kea, unbound and chrony (slot 6) | all PASS (kea 1.4 s, unbound 7.7 s, chrony 1.8 s). Same steps as pasted: config-get diff = 0 before, after the change and after rollback. `list_forwards` shows `corp.example.test.`. `net.Resolver` resolves the local-data name. Client `tracking` shows stratum 11. Both chronyd logs say "Disabled control of system clock" |
| My throw-away probes (`zz_review_probe_test.go` in unbound/ and kea/, deleted afterwards, never committed) | findings H1 and M1 below |
| After all runs | `stat` of every file under `/etc/kea`, `/etc/unbound` and `/etc/chrony` (22 entries) is identical before and after. `systemctl is-active`: kea-dhcp4-server, kea-dhcp6-server, kea-ctrl-agent and unbound are `inactive`; chrony is `active` (the host timesync, PIDs 1034/1153, the same before and after). No kea, unbound or test chronyd process is left. No `ns-w6-*`. Worktree clean |

Checklist:
1. Contract: no hits.
2. Real verification: yes. The tests run real daemons as children and assert on the daemons' own output (`config-get`, `lease4-get-all`, `list_forwards`, `list_local_data`, a real DNS query, and `chronyc -c sources/tracking/serverstats`).
3. Restart safety: not applicable (no VPP objects). The files are the daemons' startup configs.
4. binapi: not used.
5. Shared host: children are killed by PID in `t.Cleanup`. Ports are in the slot range (3653 / 3623 / 3680). Kea runs in its own `ns-w6-a`. chronyd runs with `-x`, and the argv and the log are both asserted. See L4 for hygiene.
6. Security: see M3 and L1.
7. Transactions: H1, M2 and L2.
8. and 10. No UI.
9. Scope: nothing beyond the prompt. Views, the events pollers and `DecodeText` all serve the requested features.
11. My CI run matches.

---

## Findings, ranked by severity

### H1: Unbound `Apply` reports success for a listen-address or port change that Unbound never applies (no convergence check)
`apps/agent/internal/renderers/unbound/renderer.go:243-246` (`reload_keep_cache` ok → `return nil`).

`reload_keep_cache` (and `reload`) re-read `unbound.conf`, but Unbound does **not** reopen its listening sockets. I proved this live with a probe against a real `unbound -d` on slot 6:
- Base config: `interface: 127.0.0.1@3653`. I applied the same resolver with a second listen address, 127.0.0.1:3655 (free).
- `Validate` returned nil and `Apply` returned nil. The file on disk has both `interface:` lines.
- `ss -lnup` still shows only `127.0.0.1:3653` for unbound. The new local-data *was* applied, so the reload itself did happen.
- The same thing happens with an occupied port (3654): `Apply` returns nil, and Unbound keeps running on the old socket set without logging anything.

Failure scenario: an operator adds a LAN listen address to the resolver, or changes its port. The commit succeeds and `Retrieve` shows `running: true`, but clients on the new address get no answer until someone restarts Unbound. Nothing in the system knows that a restart is due. This is the RF-1 H2 pattern (success without convergence), specialised to Unbound.

Fix:
- In `Apply`, compare the old and the new file on the directives that only take effect at startup: `interface`, `port`, `interface-view`, `username`, `chroot`, `directory`, `pidfile`, the `remote-control:` block and `do-daemonize`.
- When any of them changed, write the file and return `*ActionRequired{Action: "restart"}` (as chrony does) instead of reloading.
- Also add a cheap post-reload check that the rendered forwards and local zones are present in `list_forwards` / `list_local_zones`. Return `ErrDaemon` on a mismatch and take the existing restore path.
- Add a unit test with a `RecordingRunner`, and extend the integration test to change the listen port.

### M1: the Kea subnet-id collision probing depends on the other subnets, so adding a subnet can renumber an existing one and move its leases
`apps/agent/internal/renderers/kea/build.go:266-278` (`subnetID`, linear probing in sorted build order).

On a collision, the name that sorts first gets the hash and the other gets hash+1. Whether an existing subnet keeps its id therefore depends on subnets added later. I found a real collision by brute force: `lan/vlan957918` and `lan/vlan1340126` both hash to 2851699104. A probe against the renderer (unit, no daemon) gave:
- before: `10.6.1.0/24` (`vlan957918`) has id **2851699104**
- after adding `vlan1340126` (10.6.2.0/24): `10.6.2.0/24` has id **2851699104**, and `10.6.1.0/24` has **2851699105**

Kea keys memfile leases and statistics by subnet-id. After `config-set`, the existing 10.6.1.x leases point at the id of the *other* subnet. The default `lease-checks: warn` keeps them, so they end up misattributed: wrong pool utilisation, and renew/rebind or reclamation follow the wrong subnet. The probability is small (about n²/2³³ per family), but the failure is silent and hard to diagnose. Renaming a server or a subnet also renumbers it (D-RF3-3 should say so).

Fix: make the id a pure function of its own name.
- Either reject a collision with `ErrInvalid` ("subnet X collides with Y; rename one"), which is deterministic and needs no state,
- or re-hash with a salt (`server/subnet#1`, `#2` …), so that an existing subnet's id never depends on which other subnets exist. With a salt, the existing subnet keeps its first-choice id unless *it* is the one that is new, which needs the previous id map as input, so rejecting is the simpler option.

Add the colliding pair above as a unit test.

### M2: chrony `ActionRequired{restart}` is lost when the caller does not act on it, and the next `Apply` returns nil while chronyd runs the old config
`apps/agent/internal/renderers/chrony/renderer.go:262-283`.

`changed` compares the new files with the files **on disk**. `Apply` #1 writes the new `chrony.conf` and returns `restart`. If the commit engine does not restart (today nothing acts on it, Q3), or the restart fails, then `Apply` #2 with the same document sees `changed[conf] == false` and returns `nil`. The daemon keeps running the previous `allow`, `local stratum` or `bindaddress` indefinitely, and `Retrieve` cannot show it. Unbound's H1 fix would have the same hole.

Fix: decide "needs restart" against what the daemon runs, not against the file.
- Cheapest: compare the conf file's mtime with the daemon's start time (`/proc/<pid from pidfile>/stat` starttime). If the file is newer than the process, return `ActionRequired{restart}` again.
- Alternative: keep a `<conf>.applied` hash that is written only after the daemon (re)started.

Do the same in unbound (H1) and in Kea's `start` case.

### M3: kea-ctrl-agent is rendered for the product with no authentication. It is an unauthenticated local `config-set` endpoint, on a component that Kea 3.0 deprecates
`apps/agent/internal/renderers/kea/build.go:566-580` (`buildCtrlAgent`), product `ProductPaths().CtrlAgentPort = 8000` (`paths.go:96-110`).

The DHCP servers' unix sockets are protected by `/run/kea` 0750 `_kea`. The rendered ctrl-agent, however, accepts anything on `127.0.0.1:8000`, with no `authentication` block. Any local process under any UID can then POST `{"command":"config-set","service":["dhcp4"],…}`: the Node API, snmpd, unbound's user, or a compromised web process. That process can make the DHCP server hand out a rogue router or DNS server to the whole LAN (a LAN-wide MITM), or load any hook from the hooks directory.

Kea 3.0.3 itself logs `CTRL_AGENT_IS_DEPRECATED … Its function has been moved to Kea servers` (seen in `/run/vrx-test/w6/kea/log/kea-ctrl-agent.log`). The renderer's own apply and retrieve paths use the unix sockets only. The ctrl-agent is used only to reload itself and, in the test, to prove the round trip. The prompt says the ctrl-agent is needed "only where the product needs the remote path", and no product consumer exists yet. The version decision ("decide per installed version, record it") is also missing from the decision table.

Fix, either:
- (a) Do not render or ship `kea-ctrl-agent.conf` in `ProductPaths()` (keep it for tests, or behind an explicit option). Record a D-RF3 decision "Kea 3.0: ctrl-agent deprecated, unix sockets only".
- or (b) Render `authentication: {type: basic, realm, directory, clients: [{user, password-file}]}` with the password file as a `Secret` file (0600 `_kea`), resolved through a secret resolver like chrony's. Never inline the password: Kea echoes `config-get`.

### M4: the product Unbound config points its control socket and pidfile into `/run/unbound`, which nothing creates. The packaged `unbound.service` has no `RuntimeDirectory`
`apps/agent/internal/renderers/unbound/paths.go:340-351` (`RunDir: "/run/unbound"`), template `control-interface`.

On this host `ls /run/unbound` fails with "No such file or directory". `systemctl cat unbound` shows `ExecStart=/usr/sbin/unbound -d -p $DAEMON_OPTS` with no `RuntimeDirectory=`. Debian's stock config uses `/run/unbound.ctl` directly in `/run`. With the rendered product config, Unbound cannot bind `remote-control` and fails at startup. After that, every `Apply` / `Retrieve` sees `ErrNotRunning`. The tests cannot catch this, because `TestPaths` puts everything in one existing directory.

Fix: product `ControlSocket` = `/run/unbound.ctl` (the Debian default; no pidfile, since the unit passes `-p`). Alternatively, document a drop-in `RuntimeDirectory=unbound` as a packaging requirement (P-packaging owner) and add a unit test pinning the product paths. Check Kea (`/run/kea` via `RuntimeDirectory=kea`, fine) and chrony (`/run/chrony` exists, fine) the same way. They are OK.

### L1: the RF-1 review patterns checked for kea, unbound and chrony. No hit except those listed above
- **Config-line injection through the daemon's own parser.**
  - *Unbound*: the only free text (resolver description) lands in a `#` comment line, `renderers.Line` rejects CR, LF, NEL, U+2028 and U+2029, and Unbound's lexer ends a comment only at `\n`.
  - Unbound's quoted strings do not unescape `\"` (`DQANY = [^"\n\r\\]|\\.`). This is harmless here, because no user string reaches a quoted value except DNS names (regex: no quote, colon or space) and view names (object-name regex).
  - TXT data is `\DDD`-escaped, including the `i` of `include`, and it round-trips live. `forward-addr …#name` is a hostname regex. There is no `server:`/`include:`/`include-toplevel:` path.
  - *chrony*: hosts, prefixes and numbers are all typed. `#`, `!`, `;` and `%` cannot appear. There is no free text.
  - *Kea*: `encoding/json` puts every user string inside a JSON string literal, so Kea's `<?include ?>`, `#` and `//` preprocessing only applies outside strings. Option data is printable ASCII (it can still add CSV fields to the user's own option, which is only semantic).
- **Secrets.**
  - `chrony.keys` is `Secret`, 0600 `_chrony` (temp file 0600 from `CreateTemp`, restored with the original mode and owner).
  - Staging for `Validate` sits in a 0700 temp dir.
  - `chronyd -p` prints the `keyfile` path, not the keys (checked).
  - `Retrieve` holds no key material, resolver errors are not wrapped, and `TestSecretErrorsDoNotLeak` covers it.
  - Kea and Unbound have no secrets in RF-3 scope (DDNS/TSIG are out of scope). M3 is the one secret-adjacent gap.
- **Unbounded reads.** Kea answers are capped at 64 MiB. Leases are read in pages of 1000, up to `MaxLeases` 100 000, with `leasesTruncated` set (`kea/state.go:19,72`). But see L3. `unbound-control list_local_data` goes through `SystemRunner`'s silent 4 MiB cut (RF-1 M3). With F-dns blocklists (hundreds of thousands of local-data lines), `Retrieve` would return a quietly truncated list. Surface `limitedBuffer.truncated`, or drop `list_local_data` from `Retrieve` in favour of counts.
- **Harness cross-kill.** Each test kills only the PID it spawned. Kea refuses to reuse an existing `ns-<prefix>-a` instead of deleting it. The directories are per-daemon. No `pkill`.
- **Stale control sockets.** Unbound (`net.Dial` unix) and chrony (`unixgram` dial) probe the socket rather than `stat` it. Kea `stat`s and then dials, so a dead Kea returns `ErrNotRunning`. OK.

### L2: rollback uses the caller's context; no Apply mutex (same as RF-1 L2)
The rollback runs on the caller's context in `unbound/renderer.go:254`, `kea/renderer.go:293,299,314` and `chrony/renderer.go:292-296`. If the forward step failed because `ctx` expired, the rollback `config-set` / `reload` fails immediately and leaves the daemon on the new (possibly half-applied) config while the files are restored. Use `context.WithoutCancel(ctx)` with its own timeout. There is no mutex around `Apply`: document that the commit engine serialises commits.

### L3: Kea `Retrieve` returns up to 100 000 leases on every call
`kea/state.go:72`. That is 100 control round trips, and tens of MB of `structpb`, per `Retrieve`. `Retrieve` is the drift/reconcile read, and leases are not desired state. The lease browser is F-dhcp. Make leases opt-in (`State(ctx, WithLeases)`), and report only counts from `statistic-get` in `Retrieve`.

### L4: shared-host hygiene: the worker's manual experiment directories are left in the slot
`/run/vrx-test/w6/c1/` and `/run/vrx-test/w6/x/` (hand-written chrony.conf, ub.conf, k4.json, a `chronyd.pid` for a dead PID) and a stale `/run/vrx-test/w6/unbound/unbound.ctl` are all still there. Both chronyd runs in them logged "Disabled control of system clock", so there was no clock risk. `ns-w6-a` is also unknown to `tools/lab rig gc`: after a SIGKILLed run, the next Kea test fails fatally ("already exists") until someone deletes it by hand. Remove the leftovers, and add `ns-<P>-a` to `rig gc` (P04/P09 follow-up, like RF-1 L3).

### L5: an idle Unbound config binds `127.0.0.1@53` even in tests
`unbound/build.go:277` (`Port: 53`) and `templates/unbound.conf.tmpl:24`. With no enabled resolver, the rendered file listens on `127.0.0.1:53`. That is outside the slot's `36xx` range and could collide with a host resolver if a test ever starts or restarts Unbound on an idle document. Use the slot port in `TestPaths` (for example `Paths.IdlePort`), or render `interface: 127.0.0.1@<last port>`.

### L6: chrony key IDs are reassigned when a key reference is added
`chrony/build.go:296-305`. Ids 1..n follow the sorted reference order, so adding `key/a` before `key/b` moves `key/b` from id 1 to 2. `rekey` and then `reload sources` handle this, but every authenticated source is replaced (it loses its sync state), and between the two chronyc calls the old source lines reference remapped ids. Derive the id from the reference (for example FNV-32 of the ref, which can never collide within 16 servers, with the collision rejected) so it stays stable.

### L7: `IdentityMapper` is the Kea product default (same as RF-1 L4)
`kea/renderer.go:95`. `loop0`/`tap0`-style VPP names pass `ifNameRe`, and Kea binds non-existent or wrong Linux interfaces. `service-sockets-require-all` defaults to false, so `config-set` still returns 0. Make the product default "no mapping → ErrInvalid" until the linux-cp mapper is injected.

### I1: host chrony.service (Q1)
chrony.service is the host's own timesync (active and enabled since boot, PIDs unchanged across my runs, currently stratum 0 because it has no upstream). The test instances are fully separate:
- own `pidfile`, `bindcmdaddress` socket, `driftfile` and `logdir` under `/run/vrx-test/w6`
- `cmdport 0`, `-x`
- server on 127.0.0.1:3623, client with `port 0`
- no `rtcsync`/`rtcfile`

The host chronyd does not use the test server, and the test does not touch the host socket (`/run/chrony/chronyd.sock`) or the host's 127.0.0.1:323. I found no interaction. Accept "chrony.service untouched" as the acceptance wording for this host.

### I2: Q3 (duck-typed `ActionRequired` in three packages)
This is acceptable for now. P05 should hoist it into `renderers` (one type, one `errors.As`) before the commit engine consumes it. M2 also belongs in that design.

---

## Required before merge
H1, M1, M2, M4. Each is a local change in `unbound/renderer.go`, `kea/build.go`, `chrony/renderer.go` or `unbound/paths.go`, plus unit tests. After the fixes, re-run the integration tests, including a listen-port change for unbound.

## Required before any product use of Kea (may be a follow-up, but must be on the board)
M3 (drop the product ctrl-agent or add basic auth with a secret password file). Then L1 (truncation surfacing for `list_local_data`), L3 and L7.

**APPROVE WITH CHANGES**
