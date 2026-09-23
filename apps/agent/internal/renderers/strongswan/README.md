# strongswan renderer (RF-2)

Desired `vpn.ipsec.{proposals,tunnels}` → `swanctl.conf` fragments + secrets + `strongswan.conf`
→ loaded into charon over **VICI** → state and events read back over VICI. The renderer does not
care which kernel plugin charon loads: `DaemonConfig.Plugins` is the load list (stock:
`DefaultPlugins()` with `kernel-netlink socket-default`; P11 passes its `kernel-vpp socket-vpp` list).
Field-by-field mapping: `docs/agent/renderers/strongswan.md`.

## Files (all paths from one injected `Paths`)

| file | product path | mode | content |
|---|---|---|---|
| strongswan.conf | `/etc/strongswan.conf` | 0640 root:root | `charon { load_modular = no; load = …; plugins.vici.socket; filelog/journal }`, `swanctl { load; socket }` |
| vrx.conf | `/etc/swanctl/conf.d/vrx.conf` | 0640 root:root | `connections { … }`, `pools { … }`, `authorities { … }` |
| vrx-secrets.conf | `/etc/swanctl/conf.d/vrx-secrets.conf` | **0600** root:root, `File.Secret` | `secrets { ike-<conn> { id-local; id-remote; secret = 0s<base64> } }` |

The packaged `/etc/swanctl/swanctl.conf` (`include conf.d/*.conf`) is not rendered; at boot the
unit's `swanctl --load-all` loads the same files the renderer loaded over VICI.
Tests: `TestPaths(prefix, instance)` → `/run/vrx-test/<prefix>/swan/<instance>/…`.

## Escaping (strongSwan settings grammar)

`settings_lexer.l`: names are any printable except `. , : { } = " #` and blanks; `include` + blank
is a statement wherever a name may start; `:` starts a section reference; unquoted values end at
newline, `#` or `}`; quoted values understand `\n \r \t \<c>`. The renderer is stricter:

- section names `[A-Za-z0-9_+-]{1,64}`, never `include` (case-insensitive). Document names may
  contain `.` (a lookup path separator): `ConnName` maps `.` → `+` (bijective — `+` is not in
  `objectName`), `TunnelName` reverses it;
- every free-text value (identities, the plugin list) is quoted with `\` and `"` escaped after
  rejecting control characters, U+2028/2029 and invalid UTF-8; everything else is a validated bare
  token (`[A-Za-z0-9_.:/%,|+@=-]`): addresses (canonical), masked prefixes, proposal keywords,
  numbers, `%any`, file names;
- identities must be an IP, FQDN, e-mail, `@#keyid`, DN (`C=CH, O=Org, CN=gw`) or `%any`;
- PSKs are never quoted: `secret = 0s<base64>` (any byte string, nothing to escape);
- descriptions become a `# comment` line (a comment ends only at a line break, which is rejected).

## Validate — there is no offline checker

`swanctl` only talks to a running charon, so `Validate` is structural: every file must round-trip
byte-for-byte through the strict parser in `settings.go` (the canonical subset: tab indentation,
`name {`, `}`, `key = token`, `key = "quoted"`, `# comment`; no includes, references, trailing
comments, other escapes), then the trees are checked: known keys per section only, quoting matches
the key type, proposal keyword grammar (IKE/ESP/AH, AEAD ⇔ no integrity, IKE AEAD needs a PRF),
masked traffic selectors, canonical addresses, identity shapes, `0s` base64 PSKs of 8–1024 bytes,
every PSK connection has its `ike-<conn>` secret whose owners are the connection's identities,
no orphan secrets, log level ≤ 1, the VICI socket is the one Apply uses.
`WithChecker(Checker{ViciSocket, Runner})` adds `swanctl --load-all --noprompt --file <staged>
--uri unix://<scratch>` against a **scratch** charon (never the live socket — `New` refuses it),
with every `start_action` rewritten to `none` in the staged copy. Only the integration test uses it.

## Apply — VICI, transactional

1. parse + check the files (same parser), build the VICI requests exactly as swanctl does
   (`load_conns.c`/`load_creds.c`/`load_pools.c`: list keys split on commas, `certs`/`cacerts`/
   `pubkeys` read from `x509/ x509ca/ pubkey/` — max 64 KiB each —, secrets decoded);
2. snapshot + atomic write (temp + fsync + rename, modes above);
3. `load-authority`, `load-shared` (id = `ike-<conn>`), `load-pool`, `load-conn`; then unload what
   charon has and the files do not (`unload-conn` + `terminate` of its SAs, `unload-shared`,
   `unload-pool`, `unload-authority`) — the renderer owns charon's VICI config like `swanctl --load-all`;
4. convergence: `list-conns` per rendered connection must show the rendered version, addresses,
   children, modes and traffic selectors; `get-shared`/`get-pools` exactly the rendered sets;
   nothing else loaded (RF-1 review H2);
5. any failure: restore the snapshot and load the previous files the same way (own context and
   timeout); with no valid previous files everything is unloaded.

Reloading an unchanged connection is a no-op in charon (established SAs survive — integration
test), so Apply is idempotent. `strongswan.conf` changes need a charon restart (not done by the
renderer). Not safe for concurrent calls (the commit engine serialises commits).

## Retrieve, events, actions

`State`/`Retrieve` (`*structpb.Struct`, D-055 stand-in until P11 adds an IPsec state message):
`version`, `stats`, `get-conns`, then **per connection** `list-conns{ike}` and `list-sas{ike}`
(≤ 64 SAs each), `get-shared` (ids only), `get-pools`; `unlistedSas` = SAs in `stats` not
belonging to a VICI-loaded connection. `Watch` subscribes to `ike-updown`, `child-updown`,
`ike-rekey`, `child-rekey`; when the event session breaks it emits a `daemon` event, polls
`list-sas` at 1 Hz (state changes → `poll` events) and re-subscribes with backoff 1 s … 30 s.
`Event.ToProto` → `EVENT_KIND_UNSPECIFIED` + attributes (`EVENT_KIND_ERROR` for daemon down).
`Initiate`/`Terminate` for the Actions API and tests.

## govici (MIT) and its limits

`github.com/strongswan/govici` v0.8.2 — structured VICI instead of parsing `swanctl` text (the
prompt's reason for a dependency outside the stack). Hardening around it:
- it allocates whatever a packet header announces → `boundedConn` fails the session above 1 MiB;
- its streaming drops events silently once 128 packets are buffered → never an unbounded
  listing; per-connection streams with caps; `ErrTooLarge` instead of a truncated answer;
- `CallStreaming` releases the session lock before iterating → one session per operation.

## Secrets

Resolved through `WithSecretResolver` (D-051 `psk/<name>` only). The plaintext exists in memory,
in the `load-shared` request and as base64 in the 0600 file. The redactor masks every resolved
value (plaintext, base64, hex) and every `secret = …` in errors, logs, Retrieve and events;
malformed references are not echoed (they may be pasted secrets); parse errors never quote lines;
charon's log level is capped at 1 (higher levels can log key material).
`TestPlantedSecretNeverLeaks` (unit) and the integration test prove it.

## Tests

- unit: goldens (`go test ./internal/renderers/strongswan -run Golden -update`; the secrets
  golden stores `secret = <redacted>` and the decoded values are asserted separately), hostile
  strings in every field, strict parser, tampered files, fake-charon Apply/rollback/convergence,
  planted secret, Retrieve bounds, events and Watch reconnect;
- integration (`VRX_INTEGRATION=1`, lab lock, slot prefix): `swantest` starts charon-systemd in
  `ns-<prefix>-a|b|v` (veth `<prefix>-a`↔`<prefix>-b`, `10.<N>.250.1/2`), IKEv2 PSK tunnel
  end to end. Without an installed strongSwan, extract the stock packages (no install, nothing
  under `/etc`):

  ```
  D=/run/vrx-test/<prefix>/swan-stock; mkdir -p $D/debs $D/root; cd $D/debs
  apt-get download strongswan-charon strongswan-swanctl strongswan-libcharon libstrongswan charon-systemd
  for f in *.deb; do dpkg -x $f ../root; done
  ```

  The harness then runs every daemon/swanctl on a thread in a private mount namespace with
  read-only overlays of the extracted `usr/lib` and `usr/sbin` (the stock binaries hard-code
  `/usr/lib/ipsec/plugins`; `/run` is noexec). `charon-systemd` instead of `/usr/lib/ipsec/charon`
  because charon's pid file is the compile-time `/var/run/charon.pid` (no strongswan.conf setting).
  Daemons are stopped by SIGTERM to the PID the harness spawned (verified argv0 + parent);
  a slot lock keeps two packages from sharing one slot's daemons.

Known: a SIGKILLed charon (kernel-netlink) leaves its xfrm states in the kernel; the kernel-vpp
equivalent (SAs left in VPP after a charon crash) is P11/DF-5's to reconcile.
