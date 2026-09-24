# FRR renderer (RF-1) — framework

Desired state → `frr.conf` + `vtysh.conf` → `vtysh -C` → `frr-reload.py --reload` (diff, the daemons keep
running) → state from `vtysh --command "show … json"` → 1 Hz change events. Protocol sections (P12 BGP, F-ospf,
F-isis-rip, F-bfd-redistribution, F-mpls-srmpls, F-igmp-mfib) plug in from their own packages; they never edit
the files here. Mapping table desired state ↔ directives: `docs/agent/renderers/frr.md`.

## Files

| file | what |
|---|---|
| `paths.go` | `Paths` (ConfDir, RunDir, Namespace = FRR pathspace `-N`, BinDir, ReloadLog, FileOwner, FileMode); `ProductPaths()` = `/etc/frr`, `/var/run/frr`, `frr:frr 0640`; `TestPaths("w12")` = `/run/vrx-test/w12/frr/{etc,run}`, pathspace `w12`, owner root |
| `escape.go` | strict validators `Hostname`, `IfName`, `VRFName`, `Description` (+ template funcs `hostname ifname vrfname desc`) |
| `model.go` | `Desired(msg)` input normalisation (`*vrxv1.DesiredState` or D-055 `*structpb.Struct` document + stand-in fields), `BuildModel` (validated typed model), D-072 static ownership (`StaticOwnedByFRR`, `RegisterStaticSelector`), `NoMapper`/`IdentityMapper` |
| `section.go` | `Section` interface, `RegisterSection`, order constants, framework sections, `assemble` (+ per-line backstop) |
| `secrets.go` | `RenderContext` (typed state, `MapInterface`, `Secret(ref)`), `SecretResolver`, redaction (`RegisterRedaction`, built-in FRR secret patterns) |
| `templates/framework.tmpl` | `globals`, `vrfs`, `interfaces`, `static`, `route`, `vtysh.conf` |
| `renderer.go` | `New`, options, `Render`, `Validate`, `DryRun`, `Apply`, `NormalizeDiff` |
| `state.go` | `ShowCommand` constants, `Show`/`ShowJSON`, `StateReader` + `RegisterStateReader`, `State`, `Retrieve`, `DecodeRIB`, `NormalizeConfig` |
| `events.go` | `PollFunc` + `RegisterPoller`, framework pollers `routes` / `interfaces`, `Poller.Step/Watch`, `Event.ToProto` |
| `frrtest/harness.go` | test-scoped daemon harness for this package and every consumer (see below) |

## Adding a protocol (P12 and F-*)

```go
package bgp // e.g. internal/renderers/frr/bgp — your own directory, your own files

func init() {
	frr.RegisterSection(section{})                                               // order 400..899
	frr.RegisterStateReader(frr.StateReader{Key: "bgpSummary", Command: "show bgp summary json"})
	frr.RegisterPoller("bgp-neighbors", pollNeighbors)                           // key = peer, value = state
	// only for secret-bearing commands the built-in patterns do not cover:
	frr.RegisterRedaction(regexp.MustCompile(`some-cmd secret (\S+)`))
}

type section struct{}
func (section) Name() string { return "bgp" }
func (section) Order() int   { return 500 }
func (section) Render(rc *frr.RenderContext) ([]string, error) {
	bgp := rc.Desired.GetRouting().GetBgp()                     // typed state (rc.Input = original message)
	pw, err := rc.Secret(n.GetPasswordRef())                    // "password/<name>" (D-051) → plaintext
	src, ok := rc.MapInterface(n.GetUpdateSource())             // VPP name → Linux (linux-cp) name
	...
	return []string{"router bgp 65000", " neighbor 10.0.0.1 password " + pw, "exit"}, nil
}
```

Rules: return complete lines including the block's `exit`; every user string through a validator
(`frr.IfName`, `frr.RouteIfName`, `frr.VRFName`, `frr.Description`, `renderers.Ident/Addr/Network/Prefix`); numbers
typed. **No `|` in any LINE-type token** (descriptions): FRR's CLI pipe hook cuts every config line at `"| "` in the
daemons and in vtysh, silently (review H1). The framework rejects any line with a line break, control character,
invalid UTF-8, `"| "` or a bare `end`, rejects duplicate section names, and re-renders the whole file each time
(frr-reload.py computes the diff). Render the form FRR prints in `show running-config` (keyword order, defaults
omitted): `Apply` runs a convergence check after every reload and fails (and rolls back) when FRR holds something
else than the rendered file (review H2).

Secrets (D-051/D-072, review M2): resolve references only through `rc.Secret`; the value is validated as one FRR WORD
(no blanks, quotes, backslash, `|`), frr.conf is marked `Secret`, and the renderer masks the value (plus every match of
the built-in FRR secret patterns: `password`, `neighbor … password`, `ip ospf authentication-key`, `message-digest-key`,
`key-string`, `isis/area/domain-password`, `ip rip authentication string`) with `<redacted>` in `Show`/`State`/
`Retrieve`, `DryRun` and every error. frr-reload.py runs with `--log-level critical` so its log never holds config lines.

Static routes (D-072): one programmer per route. `frr.StaticOwnedByFRR(i, route, ext)` decides; default
`FlaggedStatic` = only routes with the explicit flag (stand-in `routing.static[i].frr: true`). The FRR renderer renders
exactly those; **the P05 static-route descriptor must skip exactly those** (call the same function). When the real
schema/proto flag lands, its owner calls `frr.RegisterStaticSelector` once to read it.

Import the protocol package from the agent's wiring (blank import) so its `init()` runs; tests that must not see
registrations use `frr.WithSections(...)` / `frr.WithStateReaders(...)`.

## Commands (fixed argv, no shell; `internal/renderers/ALLOWLIST.md`)

| step | argv |
|---|---|
| Validate | `/usr/bin/vtysh --config_dir <staging>/<ConfDir> --vty_socket <RunDir> [-N <ns>] -C -f <staging>/<ConfFile>` |
| DryRun | `/usr/lib/frr/frr-reload.py --test --log-level critical --logfile <ReloadLog> --bindir /usr/bin --confdir <ConfDir> --rundir <SocketDir> --vty_socket <RunDir> [--pathspace <ns>] <staged frr.conf>` |
| Apply | same with `--reload` and the live `<ConfFile>` |
| Show | `/usr/bin/vtysh --config_dir <ConfDir> --vty_socket <RunDir> [-N <ns>] -c "<ShowCommand constant>"` |

Production wiring: `frr.New(frr.NewSystemRunner(), frr.WithSecretResolver(…), frr.WithInterfaceMapper(<linux-cp mapper>))`.
`NewSystemRunner` carries `MaxShowOutput` = 64 MiB per vtysh answer: `show ip route vrf all static json` measured
5 346 734 bytes for 10 240 routes (~520 B/route) → the bound holds ≥ 90 000 FRR static routes; output reaching the
bound is `ErrTruncated`, never parsed. State reads only scoped commands (`… vrf all static json`, `… vrf all summary
json`) and stream-decodes the RIB (`StreamRIB`); the poller reads only the summaries (review M3). Default mapper is
`NoMapper` (review L4): no interface blocks and no interface next hops until P12 injects the linux-cp mapping.

## FRR 10.7 facts this code depends on (probed on the host, 2026-09-24)

- `-N <ns>` appends `/<ns>` to vtysh's and frr-reload's `--config_dir`/`--vty_socket`: the integrated config is
  `<ConfDir>/<ns>/frr.conf`, sockets `<RunDir>/<ns>/*.vty`. The daemons take `--vty_socket <RunDir>/<ns>` literally.
- `vtysh -C -f` parses against vtysh's full command tree (bgp/ospf/… compiled in) without any daemon running. It
  cannot see daemon-side semantics (e.g. a VRF the kernel does not have: accepted; staticd reports "not
  installed"). It prints errors as `line N: % Unknown command…` on stdout with exit 2.
- `frr-reload.py` writes `vtysh write` (FRR's own rendition of frr.conf, plus `frr.conf.sav`) when the file it is
  given is not `<confdir>/frr.conf` — always true with a pathspace, never in the product (no pathspace). So in tests
  the file on disk after an Apply (and the snapshot a later rollback restores) is FRR's text, not ours (review I2);
  the convergence check always compares against the *rendered* files (staged copy).
- `vtysh -f` (used by frr-reload.py) continues after a line a daemon rejects and exits 0 → the convergence check.
- The CLI pipe hook cuts every config line at `"| "` → `|` rejected.
- `ip route` keywords accept unique abbreviations (`bl` = blackhole) and interface names are read as keywords or
  addresses (`Null0`, `reject`, `10.0.0.1`) → `RouteIfName`.
- `frr-reload.py`'s default `--logfile` is `/var/log/frr/frr-reload.log`: always passed explicitly.
- Canonical forms: `ip route P NH [IF] tag T D` (tag before distance, distance 1 omitted); VRF static routes live
  inside `vrf X … exit-vrf`; an empty `line vty` never appears in `show running-config` (so the framework does
  not render it — it would be a permanent frr-reload delta); `frr version` / `frr defaults` / `service
  integrated-vtysh-config` are never deleted by frr-reload.
- `hostname` is not forwarded to the daemons in 10.7: they always report the system hostname. It is rendered only
  when `system.hostname` is set, and `NormalizeDiff` drops `hostname` lines (a delta frr-reload can never apply).
- There is no `show vrf json` in 10.7 (`show vrf` text is parsed: `vrf NAME id N table T` / `vrf NAME inactive`).
- `show interface … json` omits `operationalStatus` for an administratively down interface.
- No push channel without linking FRR (Ubuntu builds `--disable-grpc`): events poll at 1 Hz.

## Test harness (`frrtest`) — for this package and every consumer

`frrtest.Start(t, frrtest.Options{Prefix: "w12", Links: …, Daemons: …})` (integration only):

- takes an exclusive `flock` on `/run/vrx-test/<prefix>/frr.lock` for its lifetime (review M4): a second harness on
  the same slot (another package in the same `go test ./...`) waits instead of killing the first one's daemons;
  daemons are identified by `/proc/<pid>/exe` = the daemon binary **and** argv `-i <this harness's pidfile>`;
- creates namespace `ns-<prefix>-frr` (or reuses `Options.NetNS`, e.g. the rig's `ns-<prefix>-lan`, not deleted)
  with the requested dummy/vrf links (names must carry the prefix); refuses a namespace containing `ens192`;
- dirs `/run/vrx-test/<prefix>/frr/{etc/<prefix>,run/<prefix>}`; the socket dir belongs to `frr` (the daemons drop
  privileges and refuse `-u root`);
- **symlink `/run/frr/<prefix>` → `/run/vrx-test/<prefix>/frr/run/<prefix>`**: mgmtd binds `mgmtd_fe.sock` /
  `mgmtd_be.sock` in `/var/run/frr/<pathspace>` regardless of `--vty_socket`. Only a missing path or the harness's
  own symlink is accepted; removed on Stop;
- starts `ip netns exec <ns> /usr/lib/frr/<d> -d -N <prefix> --vty_socket <sock> -i <sock>/<d>.pid -A 127.0.0.1 -P 0
  --log file:<sock>/<d>.log --log-level warn [-z <sock>/zserv.api] [-f <base>/zebra.conf]` for mgmtd, zebra,
  staticd (+ `Options.Daemons`, e.g. `bgpd`), asserting the argv first (127.0.0.1, port 0, no ens192, no /etc/frr,
  paths under the base, not the root namespace);
- Stop (t.Cleanup): SIGTERM → SIGKILL by the PIDs from the harness's own pidfiles, after checking
  `/proc/<pid>/cmdline` names the harness socket dir; removes the symlink, the namespace and the base dir. Leftovers
  of a killed run are cleaned the same way at the next Start.
- mgmtd still *tries to read* legacy `/etc/frr/<daemon>.conf` files at start (none exist; nothing under `/etc/frr` is
  written — the integration test fingerprints `/etc/frr` before/after).

Run: `eval "$(tools/lab env 12)"; cd apps/agent && VRX_INTEGRATION=1 go test -count=1 -v -run Integration ./internal/renderers/frr/`

## Limitations / not here

- Static-route `weight` and `description` have no staticd equivalent and are not rendered.
- Interface names: the product default `NoMapper` maps nothing; `IdentityMapper` (tests/harness) renders only valid
  Linux names. VPP names need the linux-cp mapping P12 passes with `WithInterfaceMapper`. A static next hop on an
  unmapped interface is an error; a description on one is skipped.
- Apply is not safe for concurrent calls on one set of paths (the commit engine serialises commits). With no previous
  frr.conf (never in the product) a failed Apply removes the file and the daemons keep the partial state until the
  next successful Apply.
- VRF ↔ kernel VRF device/netns mapping is P12's policy; the framework renders `vrf <name>` blocks only.
