# Renderers — how to write one (RF-*, P11, P12)

A renderer turns desired state for one GPL daemon into config files, validates them with the
daemon's own checker, applies them through the daemon's control channel and reads the daemon's
actual state back. The daemon stays a separate process (00-CONTEXT rule 7); nothing here links
its code, and no user input ever reaches a shell (rule 9). `renderer.go` is the frozen contract;
this file answers *where do I put things, how do I stay safe, how do I test on the shared host*.
Interface changes go through a `contract/<id>` branch, never a direct edit.

## Where files go

```
apps/agent/internal/renderers/<daemon>/
  renderer.go                  type Renderer struct{ runner renderers.Runner; paths Paths; ... }
                               func New(runner renderers.Runner, opts ...Option) *Renderer
  templates/*.tmpl             text/template sources, embedded with //go:embed
  testdata/*.golden            expected output per template path (go test -update to refresh)
  <daemon>_test.go             unit: golden files + hostile strings, RecordingRunner
  <daemon>_integration_test.go integration: VRX_INTEGRATION=1, child daemon, slot port
  README.md                    daemon specifics: paths, checker, reload channel, limitations
docs/agent/renderers/<daemon>.md   desired state ↔ rendered directives table
internal/renderers/ALLOWLIST.md    one row per binary you invoke (test-enforced)
```

`Name()` is the daemon name in lower-case (`frr`, `strongswan`, `kea-dhcp4`, `unbound`,
`chrony`, `keepalived`, `snmpd`, `rsyslog`). The live paths (`/etc/frr/frr.conf`, ...) are
constructor options with production defaults so tests can point the renderer at a temp dir.

## The four methods

**Render(ctx, desired) → Files** — pure. No I/O, no clock, no randomness; the same input gives
byte-identical output (golden tests depend on it). Build every file with
`renderers.NewTemplate` + `renderers.Execute`; every user-controlled string goes through exactly
one helper in the template:

| helper | use for | behaviour |
|---|---|---|
| `{{ident .Name}}` | tokens the daemon parses: interface, VRF, peer, tunnel names | rejects anything but `[A-Za-z0-9_.:/@-]` |
| `{{quoted .Desc}}` | quoted strings with C-like escapes (FRR, swanctl, Unbound, chrony) | escapes `"` and `\`, rejects control chars |
| `{{line .Text}}` | free text to end of line where the syntax has no quotes | rejects control chars and line separators |
| `{{json .Any}}` | JSON configs (Kea) | always representable, escapes everything |
| `{{addr .IP}}` `{{prefix .P}}` `{{network .P}}` | addresses | `net/netip` canonical form or error |

`missingkey=error` is on: a typo in a field name fails the render. `Execute` also runs
`CheckRendered` (no CR/NUL/ESC/U+2028, valid UTF-8) as a backstop for a forgotten helper — but
a raw `{{.Field}}` still lets an LF through, so the review rule is *no raw user strings in
templates*. Numbers and enums from proto fields are fine raw. Mark PSK/key/password files with
`Secret: true` and a mode that is not world-readable (`Files.Validate` enforces it); log with
`files.Redacted()`.

**Validate(ctx, files) → error** — the daemon's dry run against a *staged copy*:

```go
st, err := renderers.Stage(files)        // /etc/frr/frr.conf → <tmp>/etc/frr/frr.conf
defer st.Close()
_, err = r.runner.Run(ctx, renderers.Command{Path: vtysh, Args: []string{"-C", "-f", st.Path(r.paths.Conf)}})
```

Checkers: `vtysh -C -f`, `kea-dhcp4 -t`, `kea-dhcp6 -t`, `unbound-checkconf`, `keepalived -t -f`,
`swanctl --load-all` on a staged `--file`; chrony/snmpd/rsyslog have no reliable offline checker:
validate structurally and say so in your README. Validate must never touch the live paths or
signal the running daemon — the commit engine calls it for every renderer before applying any.

**Apply(ctx, files) → error** — atomic and reversible (AD-4):

```go
snap, err := renderers.TakeSnapshot(files.Paths()...)   // content+mode+owner or "absent"
if err := renderers.WriteFiles(files); err != nil { _ = snap.Restore(); return err }
if _, err := r.runner.Run(ctx, reloadCmd); err != nil {
    restoreErr := snap.Restore()
    _, _ = r.runner.Run(ctx, reloadCmd)                   // bring the daemon back to the old config
    return errors.Join(err, restoreErr)
}
```

Reload channels: FRR `frr-reload.py --reload`, Kea control-agent HTTP `config-set` (no process
at all), Unbound `unbound-control reload`, chrony `chronyc reload sources`, strongSwan VICI
socket (`swanctl` binary only as fallback), keepalived/snmpd/rsyslog `systemctl reload|restart
<unit>` — only for the unit your envelope says you own. Apply must be idempotent: the commit
engine calls `Apply(previousFiles)` to roll back.

**Retrieve(ctx) → proto.Message** — actual state as structured data, never by parsing
human-readable text when JSON exists: `vtysh -c "show bgp summary json"`, Kea `config-get` /
`lease4-get-all`, `unbound-control stats_noreset`, `chronyc -c sources`, `swanctl --list-sas`
via VICI. Return the domain's state message; the API's `/api/v1/state/**` reads it.

## Running processes

Only through `renderers.Runner`. Production wiring uses `renderers.NewSystemRunner(binaries)`
where `binaries` is your package's `renderers.NewAllowlist(...)` of absolute paths; unit tests
inject `renderers.NewRecordingRunner()` and assert the recorded argv. The runner executes a fixed
argv with a minimal environment, a timeout and bounded output; there is no shell, so `"; rm -rf /"`
in an argument is just an argument. Rendered content travels through files or `Stdin`, never
argv. **List every binary in `ALLOWLIST.md`** — `TestAllowlistDocumented` fails otherwise, and
also fails on `exec.Command` outside `helpers_exec.go` or any `sh -c`/`bash -c`.

## Tests

Unit (`make test`, always on):

- Golden files for every template path: `testdata/<case>.golden`, compared byte for byte.
- Hostile strings in every user field — at least `"; rm -rf /`, `a\nb`, `a\r\nb`, NUL, ESC,
  U+2028, invalid UTF-8 — must be rejected or escaped (copy the `hostile` table from
  `helpers_template_test.go`). RENDERER-FACTORY-TEMPLATE acceptance requires the `"; rm -rf /`
  case explicitly.
- `RecordingRunner`: Validate/Apply issue exactly the expected argv; Apply restores the snapshot
  when the reload fails.

Integration (`VRX_INTEGRATION=1`, `vpptest.SkipUnlessIntegration(t)`, `vpptest.LockLab(t)`), on
the shared host, following `docs/lab/shared-host-rules.md` §3 and §5:

- **Never** the system unit, never `/etc/<daemon>` paths, never `systemctl` on a unit you do not
  own. Start the daemon as a **child process** with a test-scoped config dir and pidfile
  (`zebra -N <ns> -f <cfg>`, `kea-dhcp4 -c <cfg>`, `unbound -c <cfg>`, `chronyd -f <cfg> -x`,
  `snmpd -f -c <cfg> -p <pid> 127.0.0.1:<port>`, `keepalived -f <cfg>` inside a rig namespace).
- Bind only to `127.0.0.1:<your slot port>` (envelope: `VRX_HTTP_PORT`-family ports, `w<N>`
  prefix) or to rig veths inside `ns-<prefix>-*`; assert the rendered listen list before starting;
  never `ens192`.
- The task envelope names the daemon you own (`daemon-owner:`); everybody else mocks or skips
  with `t.Skip`. Kill by the PID you spawned (`cmd.Process.Kill()` in `t.Cleanup`), never
  `pkill`. Leave the system unit stopped and disabled. `pgrep -f <your cfg dir>` must be empty
  after the run.
- Redact secrets in fixtures: the literal `VRX_TEST_PSK_<id>`, never a real PSK.
