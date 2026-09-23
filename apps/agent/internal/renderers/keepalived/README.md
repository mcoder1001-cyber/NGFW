# keepalived renderer (RF-4, WBS D9.1)

`ha.vrrp.<name>` with `engine: "keepalived"` (enabled) → `keepalived.conf` → `keepalived -t` → SIGHUP →
convergence from keepalived's own JSON dump → state from the notify helper's state files (+ the dump) → 1 Hz
events. Instances with `engine: "vpp"` belong to the VPP vrrp descriptor (DF-7) and are never rendered here.
Mapping table: `docs/agent/renderers/keepalived.md`.

## Paths and files

| | product (`ProductPaths`) | tests (`TestPaths("w8", binDir, "ns-w8-a")`) |
|---|---|---|
| config | `/etc/keepalived/keepalived.conf`, `root:root 0640` (`Secret` when it carries a PASS key) | `/run/vrx-test/w8/keepalived/keepalived.conf` |
| state files | `/run/vrx/keepalived/<instance>.state` | `/run/vrx-test/w8/keepalived/state/` |
| notify helper | `/usr/libexec/vrx/vrx-keepalived-notify` (P10 packages `cmd/vrx-keepalived-notify`) | built by the test into `binDir` |
| checks | `/usr/libexec/vrx/checks/<name>` (shipped only; empty until F-vrrp) | `binDir/checks/vrx-check-ok` |
| JSON dump | `$TMPDIR/keepalived.json` = `/tmp` (see P10 note) | `/run/vrx-test/w8/keepalived/tmp` |
| netns | — | `ns-w8-a` (`keepalived -t -s ns-w8-a`) |
| control channel | `systemctl reload keepalived`, `systemctl kill --kill-whom=main --signal=<JSON> keepalived` | SIGHUP / SIGJSON to the child PID |

`/run` is mounted `noexec` on this host: executables (helper, checks) cannot live under `/run/vrx-test`; tests use
a root-owned 0755 temp dir under `/tmp` (keepalived's script security accepts it and rejects group/world-writable
paths).

## What keepalived is allowed to run

Exactly two kinds of scripts ever appear in `keepalived.conf`, with `enable_script_security` and `script_user
root`:

- `notify_master|backup|fault|stop "<helper> <state dir> INSTANCE <name> <STATE>"` — our helper, fixed argv. It
  validates every argument and writes `<state dir>/<name>.state` atomically as one JSON line
  `{"name","type","state","time"}`. That file is the state and event channel.
- `vrrp_script <name> { script "<ChecksDir>/<check>" … }` — `check` must be a name in the shipped allow-list
  (`WithChecks`), never user text or a path (`/bin/sh`, `script "…"` keys are rejected; unknown stand-in keys are
  errors).

Never rendered: `vrrp_strict`, `use_vmac`, `no_accept` (keepalived would install nftables/iptables rules on the
host — firewall ownership is F-host-acl-nftables, D-057), `include`, `$VAR` definitions, `@`/`~SEQ` lines.

## VRRP authentication (secrets)

VRRPv3 (RFC 5798) has no authentication; keepalived 2.3.4 answers `vrrp_version 3` + `authentication` with "does
not support authentication. Ignoring." (and `-t` fails). An instance with the stand-in
`ha.vrrp.<n>.keepalived.authRef` (kind `psk`, D-051) is therefore rendered as **VRRPv2**: `version 2` +
`authentication { auth_type PASS auth_pass <key> }` — IPv4 only, whole-second advertisement interval, key 1–8
characters `[A-Za-z0-9_.,:;@%+=/~^*-]` (keepalived truncates longer keys). PASS is a cleartext key on the wire; it
is only a guard against misconfiguration, not security — documented for F-vrrp.

The key is only in `keepalived.conf` (0640 root). keepalived's JSON and data dumps contain it in plaintext
(`auth_data`): Retrieve copies a whitelist of fields (`ParseDump`), never `auth_data` or the script strings, and
deletes `keepalived.json` right after reading — also when the caller's deadline expired (the wait for the dump has
its own bound, so a late dump is still read and removed). `keepalived -t` output is redacted.

## Interfaces

keepalived binds Linux interfaces. The document names VPP interfaces; `WithInterfaceMapper` translates. The
product default is `NoMapper` (every keepalived instance is rejected with a clear error) until F-vrrp passes the
linux-cp mapping — the renderer never binds a Linux interface that merely shares the VPP name (RF-1 review L4).
Tests use `PrefixMapper("w8-")`. `-t` checks that the interfaces exist in the target namespace, which is why
Validate passes `-s <ns>` (keepalived's own namespace option; the product allowlist has no `ip netns exec`
trampoline).

## Apply / Retrieve / events

Apply: state dir → snapshot → atomic write → SIGHUP (same PID) → convergence: SIGJSON until the dump lists exactly
the rendered instances with the rendered interface, VRID and base priority (10 s) → on failure restore + reload.
State files of instances that no longer exist are removed after success. With no instance at all the check is
skipped (keepalived has no VRRP child to report).

Retrieve: per rendered instance the notify state (`state`, `since`) and keepalived's view (`state`, `interface`,
`vrid`, `version`, `basePriority`, `effectivePriority`, `vipsSet`, `vips`, advert/master/auth-failure counters);
`dumpError` when the daemon is down. `Poller()` reads only the state files (never signals keepalived): key =
instance, value = MASTER/BACKUP/FAULT/STOP.

`accept_mode: false`: keepalived's default (non-strict) behaviour is `accept`; enforcing no-accept needs firewall
rules (above) — not rendered, recorded in RF-4-questions.md for F-vrrp.

## P10 packaging notes

- Ship `vrx-keepalived-notify` at `/usr/libexec/vrx/`, root-owned, not group/world-writable (script security).
- Set `TMPDIR` for keepalived.service to a private directory (e.g. `/run/keepalived-vrx`, 0700): the dump files
  (0600, containing `auth_data`) otherwise land in `/tmp` until the agent deletes them; change `DumpDir` with it.

## Integration test

`TestKeepalivedIntegration`: `ip netns add ns-w8-a`, veth `w8-a`/`w8-b` **both created inside the namespace**
(no host link is touched; VRRP adverts never reach `ens192`), keepalived child `ip netns exec ns-w8-a keepalived -n
-l -P -G -f … -p … -r … -c …` (`-G`: nothing to the host's syslog; `TMPDIR` = slot dir). Checks: `-t` accepts,
and rejects a missing interface; vi1 reaches MASTER (state file) and the VIP `10.8.240.1/24` is on `w8-a`; a
second commit (priority change, VRRPv2 instance with a PASS key, sync group) is applied with SIGHUP, same PID,
convergence from the dump; a reload that never reaches keepalived is refused and rolled back; SIGTERM writes STOP
for every instance (events) and removes the VIP; the key appears in no file but `keepalived.conf`, no dump is left.
