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
Tests: `TestPaths(prefix, instance)` → `/run/vrx-test/<prefix>/swan/<instance>/…`. `Paths` is
**required** (`WithPaths`; no implicit product default — review M2). `Paths.BootRecord`
(product `/var/lib/vrx/agent/strongswan-charon-boot`) persists the acknowledged charon start time.

`WithOwnerPrefix(p)`: every rendered connection/pool/authority name must start with `p`, and
Apply/State only touch objects with that prefix (shared secrets `ike-<p>…`), so several owners —
test slots on one shared charon (P11) — never unload each other's objects. The product sets none
and owns charon's VICI configuration like `swanctl --load-all`.

## Escaping (strongSwan settings grammar)

`settings_lexer.l`: names are any printable except `. , : { } = " #` and blanks; `include` + blank
is a statement wherever a name may start; `:` starts a section reference; unquoted values end at
newline, `#` or `}`; quoted values understand `\n \r \t \<c>`. The renderer is stricter:

- section names `[A-Za-z0-9_+-]{1,64}`, never `include` (case-insensitive); tunnel names at most
  60 characters so the derived secret name `ike-<name>` fits (review L1). Document names may
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

## Apply — VICI, transactional, converged down to the live SAs

1. parse + check the files (same parser), build the VICI requests exactly as swanctl does
   (`load_conns.c`/`load_creds.c`/`load_pools.c`: list keys split on commas, `certs`/`cacerts`/
   `pubkeys` read from `x509/ x509ca/ pubkey/` — max 64 KiB each —, secrets decoded);
2. snapshot + atomic write (temp + fsync + rename, modes above);
3. `load-authority`, `load-shared` (id = `ike-<conn>`), `load-pool`, `load-conn`; then unload what
   charon has and the files do not (`unload-conn` + terminate its SAs, `unload-shared`,
   `unload-pool`, `unload-authority`), owned objects only;
4. config convergence: `list-conns` per rendered connection must show the rendered version,
   addresses, children, modes and traffic selectors; `get-shared`/`get-pools` exactly the owned
   sets (RF-1 review H2);
5. **SA convergence (review H1).** charon rekeys an established SA from the config it was
   negotiated with, so a new config alone changes nothing on the wire. Apply therefore
   fingerprints every connection (IKE: version, addresses, proposals, auth rounds, identities,
   PSK + owners; per child: selectors, ESP/AH proposals, mode, if_ids, replay window) and
   compares with the previous files: changed IKE part → its IKE_SAs are terminated by unique id,
   changed child → those CHILD_SAs (graceful DELETE to the peer, forced after 3 s). Independently
   every live SA that visibly contradicts the loaded config (selector outside the configured
   prefixes, identity/version mismatch, child no longer configured) is terminated. Success needs
   `list-sas` to show none of them any more. `Impact(files)` tells the commit engine/UI up front:
   `reestablish` / `update` / `add` / `remove` per connection;
6. **start actions are the renderer's (review M1).** charon undoes and re-runs start actions
   whenever a connection is *replaced* (any peer-config change, e.g. DPD) — tearing the tunnel
   down — and runs a new `start` next to an existing CHILD_SA (duplicate). So `start_action = start`
   is withheld from the loaded config (the files keep it for the boot path) and the renderer
   initiates such a child itself, non-blocking, only when it has no CHILD_SA and no IKE_SA is
   connecting. `trap` stays with charon (policy only). Soft edits never disturb an SA; they take
   effect at the next negotiation;
7. any failure: restore the snapshot and load the previous files the same way (own context and
   timeout; SAs are only re-evaluated against the failed plan if its reconciliation ran); with no
   valid previous files every owned object is unloaded;
8. `*ActionRequired{restart}` (not a failure) when charon's loaded plugins (`stats`) lack a
   plugin of the rendered `load` list: strongswan.conf needs a charon restart (review L5, D-079
   shape; re-derived on every Apply, so it persists until charon runs the new set).

Reloading an unchanged connection is a no-op, so Apply is idempotent (integration test: the
IKE_SA keeps its unique id). Not safe for concurrent calls (the commit engine serialises commits).

## Retrieve, events, actions

`State`/`Retrieve` (`*structpb.Struct`, D-055 stand-in until P11 adds an IPsec state message):
`version`, `stats`, `get-conns`, then **per connection** `list-conns{ike}` and `list-sas{ike}`
(≤ 64 SAs each), `get-shared` (ids only), `get-pools`; `unlistedSas` = SAs in `stats` not
belonging to an owned connection; `truncated` = connections with more than 64 IKE_SAs (listed
partially, never a failure — review L3); `staleSas` = live SAs contradicting the loaded config;
`daemonStartedAt`/`ackedStartedAt`/`restarted` (review M3): charon's `stats.uptime.since` against
the acknowledged value in `Paths.BootRecord` (first observation = baseline). After a restart the
previous charon's kernel/VPP SAs may be orphaned (a SIGKILLed charon leaves its xfrm states — see
the integration test); P11/DF-5 reconcile them and call `AckRestart`. `Watch` subscribes to `ike-updown`, `child-updown`,
`ike-rekey`, `child-rekey`; when the event session breaks it emits a `daemon` event, polls
`list-sas` at 1 Hz (state changes → `poll` events) and re-subscribes with backoff 1 s … 30 s; on
(re-)subscription it emits `daemon/restarted` (attrs `started`, `acked`) while a restart is
unacknowledged. While subscribed it re-lists SAs every 30 s and immediately when its event buffer
is full (govici drops events silently then — review M4: a `resync` event, then `poll` diffs), so
a lost event is repaired within 30 s at most.
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
in the `load-shared` request (its `String` is redacted) and as base64 in the 0600 file. Retrieve,
State and events carry no key material by construction (VICI never returns it), so they are not
rewritten: redaction is by field, not by replacing the secret's text everywhere (review L2 — that
corrupted state equal to a PSK and gave GET readers an oracle). Errors and tool output mask
`secret = …` assignments; parse errors never quote lines; malformed secret references are not
echoed; resolver errors are the secret store's own text. charon's log level is capped at 1.
The Validate checker stages files under `TMPDIR` and loads candidates into the scratch charon's
memory (test-only; the integration test points `TMPDIR` at the slot's tmpfs — review L4).
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

`swantest.Crash` simulates a charon crash (SIGKILL): its xfrm states stay in the namespace
(`FlushXfrm` cleans them; `Stop` flushes after a SIGKILL fallback). A `go test -timeout` panic
skips `t.Cleanup` (review L6): the next run refuses the slot while a daemon answers; clean up by
hand with `pgrep -af /run/vrx-test/<prefix>/swan` → `kill <pid>`, `ip netns delete ns-<prefix>-{a,b,v}`.
