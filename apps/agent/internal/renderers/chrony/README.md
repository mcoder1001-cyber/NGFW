# chrony — NTP client/server renderer (RF-3, WBS D7.4, D-050 `services.ntp`)

Installed: **chrony 4.8** (`chronyd -p` prints the parsed config and exits; `-x` disables clock
control).

| step | how |
|---|---|
| Render | `services.ntp` → `templates/{chrony.conf,vrx.sources,chrony.keys}.tmpl`. `chrony.conf` holds the instance directives and `sourcedir <conf>/sources.d`; `sources.d/vrx.sources` the `server`/`pool` lines; `chrony.keys` the symmetric keys (`Secret: true`, 0600 `_chrony:_chrony`). |
| Validate | `chronyd -p -f <staged chrony.conf>` and `chronyd -p -f <staged vrx.sources>` (server/pool lines are valid chrony.conf directives). `chrony.keys` has no checker: structural check (`<id> SHA256 HEX:<hex>`), and its content never reaches an error message. |
| Apply | snapshot → atomic write → chronyd not running: disabled → nothing, enabled → `*ActionRequired{Action: start}`; chrony.conf changed → `*ActionRequired{Action: restart}` (chrony reloads nothing else at run time), persisted in `Paths.PendingFile` and returned by every later Apply until chronyd's process (pidfile → `/proc/<pid>/stat` start time, or a new pid in the same tick) started after the request (D-079, review M2); otherwise `chronyc -h <sock> rekey` (keys changed) and `chronyc -h <sock> reload sources` (sources changed). chronyc failure → restore + repeat with the old files. |
| Retrieve | `chronyc -h <sock> -c tracking | sources | sourcestats | serverstats` (CSV → typed `Tracking`, `Source`, `SourceStats`, serverstats map) → `structpb.Struct`. |
| Events | poll `tracking` + `sources` at 1 Hz: running, stratum, leap, reference, per-source state. |

## Secrets (00-CONTEXT rule 10, D-051)

`servers[].keyRef` (`key/<name>`) is resolved through `WithSecrets(SecretResolver)`; key ids are
FNV-32a of the reference (1..2³²−1; stable when other keys are added — review L6; a collision is refused); the value is written hex-encoded (`HEX:`), so no escaping issue
exists. Errors name the reference only (a resolver error is not wrapped, it could quote the
value); `Files.Redacted()` hides the keys file; tests assert that no encoding of a key appears in
`chrony.conf` or `vrx.sources`. chrony.keys is not stored as a golden file.

## Mapping decisions

- `listen` → `bindaddress` (chrony takes one per family: two IPv4 addresses are rejected);
  `port` 0 = client only. `allow`/`deny` → prefixes via `network`; `rateLimit` →
  `ratelimit interval/burst/leak`; `localStratum` (+ `orphan`) → `local stratum N [orphan]`;
  `makestep`; `rtcSync` → `rtcsync` only when `Paths.ClockControl` (tests: false).
- `ntsServer` (NTS-KE server certificate/key) is refused (`ErrInvalid`): certificates are F-ntp.
  NTS *client* (`servers[].nts`) renders `nts` and `ntsdumpdir`.
- The document has no per-server port; `Paths.SourcePort` (tests only) appends `port N`.
- Hosts are canonical IPs or lower-case RFC 1123 names; all-numeric names are rejected.
- Paths are rendered unquoted (chrony syntax) and must match `^/[A-Za-z0-9_./-]*$`.
- chronyd refuses a command-socket directory it does not own (UID of `_chrony`, not
  world-accessible) and chronyc drops to `_chrony` before connecting, so the product layout
  (`/run/chrony` owned by `_chrony`) is also the test layout: the test instance directory is
  `_chrony:_chrony 0750`.

## Tests

Goldens for chrony.conf / vrx.sources (client with keys + NTS + pool, server mode with
allow/deny/ratelimit/orphan, product paths, local-only, test source port), hostile strings in
every field, secret-leak tests, argv, Apply decisions with a recording runner. Integration
(`VRX_INTEGRATION=1`): two `chronyd -f <cfg> -n -x` children (server: local stratum 10 on
127.0.0.1:3<slot>23; client: port 0) — client selects the server (stratum 11), reload sources +
rekey without restart, conf change → restart request → child restarted, rollback.
