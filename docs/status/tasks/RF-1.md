# RF-1 — FRR renderer framework — status

Branch `task/RF-1` (worktree `/root/ngfw-wt/RF-1`, slot 12, daemon-owner frr). Base `main@40ba948`. Not merged.

## What was built

`apps/agent/internal/renderers/frr` (package README has the details; mapping table in `docs/agent/renderers/frr.md`):

- **Paths** injected everywhere (`ProductPaths()` = `/etc/frr`, `/var/run/frr`, `frr:frr 0640`; `TestPaths("w12")` =
  `/run/vrx-test/w12/frr/{etc,run}`, FRR pathspace `-N w12`, owner root).
- **Escaping**: `Hostname` `[A-Za-z0-9][A-Za-z0-9.-]{0,62}`; `IfName`/`VRFName` `[A-Za-z0-9_.-]{1,15}`; `Description`
  printable ASCII 1–80, no leading `!`/`#`, no leading/trailing/double blanks; prefixes/addresses via `net/netip`
  (masked, canonical, no zones). Plus the renderers' `CheckRendered` backstop and a per-line check on every section's
  output (no line breaks, control chars, invalid UTF-8, bare `end`).
- **Section registry**: `type Section interface { Name() string; Order() int; Render(desired proto.Message) ([]string, error) }`,
  `RegisterSection` (orders 400–899 for protocols; framework owns 0 globals, 100 vrf, 200 interface, 300 static).
  The whole file is re-rendered each time; frr-reload.py computes the diff. `frr.Desired(msg)` gives any section the
  typed `*vrxv1.DesiredState`.
- **Templates** (`templates/framework.tmpl`, `text/template` via `renderers.NewTemplate`): `frr version` / `frr defaults
  traditional` / `hostname` / `log syslog informational` / `service integrated-vtysh-config` → `vrf <name>` (+ its static
  routes, FRR's canonical form) → `interface <name>` + description → `ip route` / `ipv6 route` (nexthop ip / interface /
  ip+interface / blackhole, `tag`, distance; ECMP one line per hop) → protocol sections → `end`; `vtysh.conf`.
- **Validate**: `vtysh --config_dir <staging> --vty_socket <run> -N <ns> -C -f <staged frr.conf>` (no daemon needed).
  **DryRun**: `frr-reload.py --test … <staged file>` → normalised diff. **Apply**: snapshot → atomic write → `frr-reload.py
  --reload …` → on failure restore + reload the old file. Never a restart, never `systemctl`.
- **Retrieve / State**: constant `ShowCommand`s only (`show running-config` normalised, `show version`, `show vrf`,
  `show ip route vrf all json`, `show ipv6 route vrf all json`, `show interface vrf all json`, + `show ip route json` /
  `show ipv6 route json` / `show interface json` constants) and `RegisterStateReader` (P12: `show bgp summary json`);
  `Retrieve` returns a `structpb.Struct` (D-055; Q2), `State.StaticRoutes()` / `DecodeRIB` typed helpers.
- **Events**: `Poller.Step/Watch` at 1 Hz; framework pollers `routes` (RIB count per afi/vrf) and `interfaces`
  (up/down → `EVENT_KIND_LINK_UP/DOWN`); `RegisterPoller` for protocol pollers.
- **`frrtest` harness** (importable by P12/F-*): own netns `ns-<prefix>-frr` with dummy/vrf links (or an existing
  prefixed namespace), test dirs, `/run/frr/<prefix>` symlink for mgmtd (Q6), mgmtd → zebra → staticd (+ optional
  protocol daemons) as `ip netns exec` children with `-N <prefix> -A 127.0.0.1 -P 0`, argv asserted before start,
  stop by own pidfiles after a `/proc/<pid>/cmdline` check, full cleanup (also of a killed earlier run).
- `internal/renderers/ALLOWLIST.md`: vtysh + frr-reload.py moved to *Active*; test-only `ip`, mgmtd, zebra, staticd and
  (ahead of P12/F-*) bgpd/ospfd/ospf6d/bfdd/pimd/isisd/ripd/ldpd rows (Q5: file outside my envelope list).

## How it was verified (real output, 2026-09-24, host 172.30.126.195, FRR 10.7.1)

### CI gate
```
$ tools/ci.sh --base main
…
== contract guard: HEAD vs main ==
no contract files changed …
== forbidden patterns (+ gitleaks) ==
ok: no shell/VPP/FFI access in apps/api/src apps/web/src packages/*/src
ok: no Dockerfile/compose files
ok: no kill-by-pattern in scripts
ok: no secret-shaped strings
ok: gitleaks — scanned ~145177 bytes (145.18 KB) in 721ms no leaks found
== lint · typecheck · unit tests · build (turbo) ==
Tasks:    30 successful, 30 total …
== apps/agent: make lint test build ==
ok  ngfw/agent/internal/agent 1.137s; ok  ngfw/agent/internal/contracttest 1.381s; ok  ngfw/agent/internal/renderers 1.432s; ok  ngfw/agent/internal/renderers/frr 1.990s; ok  ngfw/agent/internal/scheduler 1.110s; ok  ngfw/agent/internal/vpp 1.079s; …
== summary (quick) ==
  mode quick · wall time 1m05s · logs /root/ngfw-wt/logs/ci/RF-1-20260924-004214-860483

CI GATE PASSED
```
(Re-run on the final commit: see the end of this file.)

### Unit tests (golden, escaping/injection, argv, restore, state, events, registry)
```
$ cd apps/agent && go test -count=1 -v ./internal/renderers/frr/...
--- PASS: TestRenderGolden (0.03s)            # testdata/{empty,empty-state,hostname,vrfs,descriptions,static,static-proto,full,hostile-description}.golden
--- PASS: TestRenderVtyshConfAndPaths (0.00s)  # testdata/vtysh.conf.golden, test paths /run/vrx-test/w12/frr/etc/w12/*
--- PASS: TestRenderDeterministic (0.07s)
--- PASS: TestHostileStringsRejectedOrEscaped (0.01s)   # 20 hostile strings × 8 fields
--- PASS: TestDescriptionRmRfIsConfinedToOneLine (0.00s)
--- PASS: TestModelErrors (0.01s)
--- PASS: TestRenderOptionsChecked (0.00s)
--- PASS: TestInterfaceMapper (0.00s)
--- PASS: TestValidateArgvAndStaging (0.01s)
--- PASS: TestValidateReportsCheckerOutput (0.00s)
--- PASS: TestFilesOwnership (0.00s)
--- PASS: TestDryRunArgvAndDiff (0.00s)
--- PASS: TestApplyWritesAtomicallyAndReloads (0.00s)
--- PASS: TestApplyRestoresOnReloadFailure (0.00s)
--- PASS: TestApplyNeedsConfigDir (0.00s)
--- PASS: TestRetrieve (0.00s)
--- PASS: TestShowCommandsAreConstants (0.00s)
--- PASS: TestNormalizeConfigDropsNoise (0.00s)
--- PASS: TestPollerEvents (0.00s)
--- PASS: TestWatchStopsWithContext (0.00s)
--- PASS: TestRegisterPoller (0.00s)
--- PASS: TestSectionOrderAndSeparators (0.00s)
--- PASS: TestSectionLineBackstop (0.00s)
--- PASS: TestRegisterSection (0.00s)
--- PASS: TestFrameworkSectionsArePublicSections (0.00s)
--- SKIP: TestFRRRendererIntegration (0.00s)
PASS
ok  	ngfw/agent/internal/renderers/frr	0.185s
```
Hostile strings covered (each in hostname, vrf key, interface key, description, static vrf, prefix, next-hop address,
next-hop interface): `"; rm -rf /`, `\nrouter bgp 65000\n`, `x\nrouter bgp 65000`, `!`, `#`, `! comment`, `# comment`,
`a\r\nb`, NUL, ESC, U+2028, invalid UTF-8, unicode `ünïcödé`, TAB, 300-char, leading/trailing/double blanks, `end`, `""`
→ rejected with `ErrInput` (wrapping `renderers.ErrUnsafe`), or — where the value is a valid token — confined to its line.

`"; rm -rf /` in a description is **accepted and written verbatim inside the `description` line** (it is printable
ASCII; FRR takes the rest of the line as one LINE token; nothing reaches a shell) — `testdata/hostile-description.golden`:
```
interface w12f0
 description "; rm -rf /
exit
!
interface w12f1
 description a ! b # c; exit; end
exit
```
In every other field (hostname, names, addresses) the same string is rejected. The integration test shows FRR stores
it as the description only: `show interface vrf all json` → `"description":"\"; rm -rf /"`.

### Integration (test-scoped FRR in ns-w12-frr; `VRX_INTEGRATION=1`, lab lock shared)
```
$ eval "$(tools/lab env 12)"; cd apps/agent && VRX_INTEGRATION=1 go test -count=1 -v -run TestFRRRendererIntegration ./internal/renderers/frr/...
=== RUN   TestFRRRendererIntegration
    harness: netns ns-w12-frr, daemons map[mgmtd:913348 staticd:913586 zebra:913389], argv zebra: ip netns exec ns-w12-frr /usr/lib/frr/zebra -d -N w12 --vty_socket /run/vrx-test/w12/frr/run/w12 -i /run/vrx-test/w12/frr/run/w12/zebra.pid -A 127.0.0.1 -P 0 --log file:/run/vrx-test/w12/frr/run/w12/zebra.log --log-level warn -z /run/vrx-test/w12/frr/run/w12/zserv.api -f /run/vrx-test/w12/frr/zebra.conf
    step1: rendered frr.conf:
        frr version 10.7.1
        frr defaults traditional
        hostname ubuntu-26.04
        log syslog informational
        service integrated-vtysh-config
        !
        vrf w12red
         ip route 10.12.210.0/24 10.12.2.1
        exit-vrf
        !
        interface w12f0
         description "; rm -rf /
        exit
        !
        ip route 10.12.200.0/24 10.12.1.1
        ip route 10.12.201.0/24 blackhole tag 100 50
        !
        end
        step1: frr-reload.py --test diff:
        Lines To Add
        ============
        log syslog informational
        service integrated-vtysh-config
        vrf w12red
        exit
        vrf w12red
         ip route 10.12.210.0/24 10.12.2.1
        exit
        interface w12f0
        exit
        interface w12f0
         description "; rm -rf /
        exit
        ip route 10.12.200.0/24 10.12.1.1
        ip route 10.12.201.0/24 blackhole tag 100 50
    step1: vtysh --command 'show ip route json' (static prefixes):
        "10.12.200.0/24": [{"protocol":"static","selected":true,"destSelected":true,"distance":1,"metric":0,"installed":true,…,"vrfName":"default",…,"prefix":"10.12.200.0/24","prefixLen":24,"table":254,…,"nexthops":[{"flags":3,"fib":true,"ip":"10.12.1.1","afi":"ipv4","interfaceIndex":2,"interfaceName":"w12f0","active":true,"weight":1}]}]
        "10.12.201.0/24": [{"protocol":"static","selected":true,"destSelected":true,"distance":50,"metric":0,"installed":true,…,"prefix":"10.12.201.0/24","prefixLen":24,"tag":100,"table":254,…,"nexthops":[{"flags":3,"fib":true,"unreachable":true,"blackhole":true,"active":true,"weight":1}]}]
    step1: ip -n ns-w12-frr route show 10.12.200.0/24: 10.12.200.0/24 nhid 13 via 10.12.1.1 dev w12f0 proto static metric 20
    step1: events [routes ipv4/default: 2 -> 4 routes ipv4/w12red: 2 -> 3]
    step2: rendered frr.conf: … ip route 10.12.200.0/24 10.12.1.9 …
        step2: frr-reload.py --test diff:
        Lines To Add
        ============
        no ip route 10.12.200.0/24 10.12.1.1
        ip route 10.12.200.0/24 10.12.1.9
    step2: vtysh --command 'show ip route json' (static prefixes):
        "10.12.200.0/24": [{"protocol":"static",…,"installed":true,…,"nexthops":[{"flags":3,"fib":true,"ip":"10.12.1.9","afi":"ipv4","interfaceIndex":2,"interfaceName":"w12f0","active":true,"weight":1}]}]
    step2: PIDs unchanged map[mgmtd:913348 staticd:913586 zebra:913389] (reload, not restart)
    step3: events after link down [interfaces w12f0: up -> down routes ipv4/default: 4 -> 1 routes ipv6/default: 1 -> -]
        step4: frr-reload.py --test diff:
        Lines To Add
        ============
        vrf w12red
         no ip route 10.12.210.0/24 10.12.2.1
        exit
        no ip route 10.12.200.0/24 10.12.1.9
        no ip route 10.12.201.0/24 blackhole tag 100 50
    step4: Retrieve runningConfig [frr defaults traditional hostname ubuntu-26.04 log syslog informational service integrated-vtysh-config vrf w12red exit-vrf interface w12f0  description "; rm -rf / exit]
    step4: PIDs unchanged map[mgmtd:913348 staticd:913586 zebra:913389]
    after Stop: no process names /run/vrx-test/w12/frr; /etc/frr unchanged:
        daemons 4127 1787745928000000000 -rw-r-----
        frr.conf 489 1787745928000000000 -rw-r-----
        support_bundle_commands.conf 10383 1787745928000000000 -rw-r-----
        vtysh.conf 32 1787745928000000000 -rw-r-----
--- PASS: TestFRRRendererIntegration (15.65s)
PASS
ok  	ngfw/agent/internal/renderers/frr	15.698s
```
After every step the test also asserts `DryRun` after `Apply` is empty (idempotent rendering), that `Validate`
(`vtysh -C`) passed, and that `AssertScoped` found only `w12`-prefixed interfaces/VRFs and no `ens192`.
Full log: `/root/ngfw-wt/logs/RF-1-evidence/integration.txt`.

### Acceptance checks
```
$ grep -rn "sh -c\|bash -c" apps/agent/internal/renderers/frr ; echo rc=$?
rc=1
$ pgrep -af '[/]run/vrx-test/w12/frr' ; echo rc=$?
rc=1
$ systemctl is-active frr ; systemctl is-enabled frr
inactive
disabled
$ stat -c '%n %s %Y %U:%G %a' /etc/frr /etc/frr/*      # before (= session start) and after the runs: identical
/etc/frr 4096 1790149582 frr:root 750
/etc/frr/daemons 4127 1787745928 frr:root 640
/etc/frr/frr.conf 489 1787745928 frr:root 640
/etc/frr/support_bundle_commands.conf 10383 1787745928 frr:root 640
/etc/frr/vtysh.conf 32 1787745928 frr:root 640
$ ls -la /run/frr ; ip netns list | grep -c w12-frr
drwxr-xr-x  2 frr  frr   40 … .          # no w12 symlink left
0
```
zebra/staticd PIDs identical before and after the config change (step2) and after removing everything (step4): see
the integration output above.

## Out of scope / left undone

- `router bgp/ospf/isis/bfd/pim` sections, VRF↔netns/kernel VRF mapping, linux-cp interface-name mapping (P12 passes
  `WithInterfaceMapper`), FIB verification in VPP, `/etc/frr/daemons` and unit enablement (P10), API/UI.
- Wiring the renderer into the agent's commit engine (no engine integration point for daemon renderers exists on main yet).
- Static-route `weight`/`description` (no staticd equivalent). `line vty` is not rendered while it has no content.
- Proto fields for tag/blackhole, a routing state message and routing event kinds (Q1–Q3; stand-ins used).

## Open questions — `docs/status/tasks/RF-1-questions.md`
Q1 proto static `tag` + blackhole next hop (stand-in via structpb) · Q2 no FRR state message (Retrieve returns
structpb) · Q3 no routing event kind · Q4 static routes owned by VPP descriptor and/or staticd (double programming
with linux-nl) · Q5 ALLOWLIST.md edited though outside my file list · Q6 `/run/frr/<prefix>` symlink for mgmtd ·
Q7 harness namespace `ns-<prefix>-frr` instead of the rig's.

## Decisions taken (for the LOG)

| decision | options | chosen / why |
|---|---|---|
| Input type for Render | (a) `*vrxv1.DesiredState` only (b) + `*structpb.Struct` document with stand-in fields | (b) D-055: tag/blackhole are not in the proto; sections get the typed state via `frr.Desired` |
| `"; rm -rf /` in a description | (a) reject (b) write verbatim | (b) printable ASCII is a legitimate description; FRR's `description LINE...` takes it as one token and no shell exists; every *other* field rejects it. Golden + integration prove confinement |
| Descriptions: double blanks / leading-trailing blanks | (a) normalise (b) reject | (b) FRR re-joins tokens with single blanks → permanent frr-reload delta; never rewrite silently |
| Static routes in a VRF | (a) `ip route … vrf X` at top level (b) inside `vrf X … exit-vrf` | (b) FRR's canonical running-config form → empty diff after apply |
| `line vty` | (a) always render (b) only with content | (b) empty `line vty` never shows in running-config → re-applied on every commit |
| `hostname` | (a) always (b) only when set | (b) FRR 10.7 daemons report the system hostname anyway |
| Harness daemon start | (a) foreground child via exec.Command (b) `-d` through the allow-listed runner, pidfile | (b) keeps `exec.Command` inside `helpers_exec.go`; the runner's `ErrWaitDelay` with exit 0 is treated as started |
| mgmtd socket location | (a) symlink `/run/frr/<prefix>` → test dir (b) mount namespace | (a) no shell/new helper needed; pathspace-scoped; refused if the path is anything else (Q6) |
| Harness namespace | (a) rig `ns-<prefix>-lan` (b) own `ns-<prefix>-frr` | (b) FRR tests need no VPP; `Options.NetNS` reuses the rig for P12 (Q7) |
| Branch history | (a) keep (b) recreate | (b) a WIP commit held a test literal (an `Event` composite literal with a `Key` field) and a status draft held pasted `%+v` event output that gitleaks' generic-api-key rule flags (false positives, no secret); `docs/contributing.md`: flagged history is recreated, not fixed forward — the RF-1 commits after the envelope were recreated (2 commits); `Event.String()` now prints `poller key: old -> new` |

## CI on the final tree
```
$ tools/ci.sh --base main          # on 460be45 (code identical to the final commit, which only adds this section)
ok: gitleaks — scanned ~162175 bytes (162.18 KB) in 784ms no leaks found
ok  	ngfw/agent/internal/agent	1.142s; ok  	ngfw/agent/internal/contracttest	1.350s; ok  	ngfw/agent/internal/renderers	1.399s; ok  	ngfw/agent/internal/renderers/frr	1.929s; …
  mode quick · wall time 0m57s · logs /root/ngfw-wt/logs/ci/RF-1-20260924-004643-919257
CI GATE PASSED
```
Evidence files: `/root/ngfw-wt/logs/RF-1-evidence/{unit,integration}.txt`, `etc-frr-{before,after}.txt`.

## Review fixes (fix round after `docs/status/tasks/RF-1-review.md`, APPROVE WITH CHANGES)

`git merge main` first (`5ec4f24`, brings D-072 and the proto with `StaticRoute.blackhole`). Then:

| finding | fix | commit |
|---|---|---|
| H1 `\|` in descriptions | `Description` rejects `\|`; `checkLine` rejects `"\| "` / trailing `\|` in any section line; `Section` doc: no `\|` in LINE tokens | `827f1be` (code, unit), `a37879d` (live test) |
| H2 Apply success without convergence | after `--reload` exit 0, `Apply` runs the `--test` diff on the rendered (staged) files; non-empty → `ErrDaemon "not converged"` → restore + reload | `827f1be`, live `a37879d` |
| M1 keyword/IP-shaped next-hop interfaces, bad gateways | `RouteIfName` (IP/prefix-shaped names; equal to or an abbreviation of `blackhole reject null0 tag label table vrf nexthop-vrf onlink color segments bfd track weight`, case-insensitive); `IfName` rejects IP-shaped names; gateways: no unspecified/multicast/loopback, link-local only with an interface | `827f1be` |
| M2 secrets | `Section.Render(*RenderContext)` with `rc.Secret("<kind>/<name>")` (D-051; `WithSecretResolver`), frr.conf marked `Secret`; redactor (resolved values + built-in FRR secret patterns + `RegisterRedaction`) on `Show`/`State`/`Retrieve`, `DryRun`, every tool error, render errors; frr-reload.py `--log-level critical` | `827f1be`, live `a37879d` |
| M3 whole-RIB reads | State reads `show ip[v6] route vrf all static json` (stream-decoded, `StreamRIB`) + `… summary json`; poller reads summaries only; `frr.NewSystemRunner()` bound `MaxShowOutput` = 64 MiB (documented sizing), output at the bound = `ErrTruncated` | `827f1be`, live 10 240 routes `a37879d` |
| M4 harness cross-kill | exclusive `flock /run/vrx-test/<prefix>/frr.lock` for the harness lifetime; `ours()` = `/proc/<pid>/exe` is the daemon binary **and** argv has `-i <own pidfile>` | `827f1be`, live `a37879d` |
| D-072 | static routes rendered only when `frr.StaticOwnedByFRR` (default `FlaggedStatic`: stand-in `routing.static[i].frr: true`); `RegisterStaticSelector` hook for the real flag; P05 descriptor must skip exactly those (README, questions Q4) | `827f1be` |
| L1 `ip` trampoline | `TestProductAllowlistHasNoTrampoline`; ALLOWLIST row "test-only; never in a production allowlist" | `827f1be`, `a37879d` |
| L2 rollback | rollback reload on `context.WithoutCancel` + own timeout (test); "no previous file" and "no concurrent Apply" documented | `827f1be` |
| L4 IdentityMapper default | product default `NoMapper`; tests/harness pass `IdentityMapper` | `827f1be` |
| I1 / I2 | ALLOWLIST rows "unused until <task>"; README note on `vtysh write` in tests | `a37879d` |
| Interface mapper for sections | `rc.MapInterface` | `827f1be` |
| (found while testing) | FRR 10.7 ignores `hostname`: `NormalizeDiff` drops `hostname` lines (else every Apply without `system.hostname` = the system hostname would fail the convergence check); diff output with only "Lines To Add" is parsed | `a37879d` |

### Unit (`go test -count=1 -v ./internal/renderers/frr/...`, 38 PASS, integration SKIP)
```
--- PASS: TestHostileStringsRejectedOrEscaped (0.01s)   # + "a | b", "a | include b", "uplink|ISP-A", Null0, null0, blackhole, bl, reject, tag, 10.12.1.1, 2001:db8::1, 10.0.0.0/8
--- PASS: TestModelErrors (0.00s)                       # + gateway 0.0.0.0 / :: / 224.0.0.5 / 127.0.0.1, fe80::1 without interface, blackhole with next hops
--- PASS: TestSectionLineBackstop (0.00s)               # + "| " / "| include" / trailing "|"
--- PASS: TestApplyFailsWhenNotConverged (0.00s)        # reload, --test (non-empty), rollback reload; frr.conf restored
--- PASS: TestApplyConvergedIsSuccess (0.00s)
--- PASS: TestRollbackIgnoresCallerCancellation (0.00s)
--- PASS: TestSecretNeverReturned (0.00s)               # planted secret absent from Validate error, DryRun, Apply error, Retrieve, Show, argv; frr.conf Secret
--- PASS: TestSecretPatternsWithoutResolvedValue (0.00s)
--- PASS: TestSecretResolutionErrors (0.00s)
--- PASS: TestShowTruncationIsAnError (0.00s)
--- PASS: TestStreamRIBShapesAndScale (0.21s)           # 12 001 entries streamed
--- PASS: TestStateUsesScopedCommandsOnly (0.00s)       # no full-RIB command in Retrieve or a poll step
--- PASS: TestProductAllowlistHasNoTrampoline (0.00s)
--- PASS: TestStaticOwnership (0.00s)                   # D-072 default + RegisterStaticSelector once
ok  	ngfw/agent/internal/renderers/frr	…
```

### Integration against test-scoped FRR 10.7.1 (`VRX_INTEGRATION=1 go test -count=1 -v -timeout 25m ./internal/renderers/frr/...`)
```
--- PASS: TestFRRRendererIntegration (27.03s)
    step1: events [routes ipv4/default/static: - -> 2 routes ipv4/w12red/static: - -> 1]
    step2: PIDs unchanged map[mgmtd:1206834 staticd:1207234 zebra:1207084] (reload, not restart)
    step3: events after link down [interfaces w12f0: up -> down routes ipv4/default/connected: 1 -> - routes ipv4/default/local: 1 -> - routes ipv4/default/static: 2 -> 1 routes ipv6/default/connected: 1 -> -]
    step4: PIDs unchanged map[mgmtd:1206834 staticd:1207234 zebra:1207084]
    H1: description "a | include b" rejected: frr: invalid desired state: interfaces.w12f0.description: renderers: unsafe value: description "a | include b" contains '|' (FRR's CLI pipe)
    H1: description "a | b" rejected: … contains '|' (FRR's CLI pipe)
    H1: description "uplink|ISP-A" rejected: … contains '|' (FRR's CLI pipe)
    H1: description "a ! b # c; exit; end" applied, stored verbatim "a ! b # c; exit; end", converged
    H1: description "x ? y" applied, stored verbatim "x ? y", converged
    H1: description "exit-vrf" applied, stored verbatim "exit-vrf", converged
    H1: description "end" applied, stored verbatim "end", converged
    H1: description "a\\b" applied, stored verbatim "a\\b", converged
    H1: description "$(reboot) `id`" applied, stored verbatim "$(reboot) `id`", converged
    H1: description "\"; rm -rf /" applied, stored verbatim "\"; rm -rf /", converged
--- PASS: TestReviewH1DescriptionsLive (25.28s)
    H2: Apply returned: frr: daemon error: not converged after frr-reload.py --reload (FRR did not take these lines):
        Lines To Delete
        ===============
        Lines To Add
        ============
        no ip route 10.12.230.0/24 blackhole
        ip route 10.12.230.0/24 bl
    H2: after rollback no 10.12.230.0/24 in RIB or running-config; previous config converged
--- PASS: TestReviewH2NotConvergedLive (11.75s)
    M2: DryRun: Lines To Delete ⏎ =============== ⏎ Lines To Add ⏎ ============ ⏎ log syslog informational ⏎ service integrated-vtysh-config ⏎ password <redacted> ⏎
    M2: Show: Building configuration... ⏎ … ⏎ ! ⏎ password <redacted> ⏎ ! ⏎ end ⏎
    M2: Validate error: frr: daemon error: vtysh -C rejected frr.conf: line 6: % Unknown command[4]: password <redacted> bogus-extra Configuration file[<staging>/run/vrx-test/w12/frr/etc/w12/frr.conf] processing failure: 2
    M2: frr-reload.log:                                   # empty (log level critical)
--- PASS: TestReviewM2SecretLive (9.94s)                 # raw vtysh (ground truth) shows the password; Retrieve/Show/DryRun/errors/log do not
    M3: 10240 FRR static routes: Apply 59.485s (incl. convergence check), State 4.032s (10240 routes, static json 5346734 bytes, bound 67108864), poll step 245ms (summary json 479 bytes), summary ipv4/default/static=10240
--- PASS: TestReviewM3ManyRoutesLive (103.11s)
ok  	ngfw/agent/internal/renderers/frr	177.481s
--- PASS: TestHarnessArgvScoped (0.00s)
--- PASS: TestOursNeedsExeAndPidfileArgv (0.00s)
    M4: second Start blocked ≥3s on /run/vrx-test/w12/frr.lock; first harness daemons map[mgmtd:1206055 staticd:1206385 zebra:1206320] alive
    M4: second harness started after release with map[mgmtd:1209477 staticd:1210662 zebra:1209535] and stopped
--- PASS: TestHarnessSlotLockSerialises (33.77s)
ok  	ngfw/agent/internal/renderers/frr/frrtest	33.807s
```
Note on M3: the 10 240-route static JSON is 5.3 MB, above the old 4 MiB runner default — it would have been cut
silently before; now the renderer's runner bound is 64 MiB and anything reaching it is `ErrTruncated`. Applying 10k
lines through frr-reload.py takes ~60 s (reload timeout 120 s) — large FRR-owned static sets are not the D-072 default.

### Host state after all runs
```
$ grep -rn "sh -c\|bash -c" apps/agent/internal/renderers/frr ; echo $?
1
$ pgrep -af '[/]usr/lib/frr/' ; echo $?
1
$ systemctl is-active frr
inactive
$ stat -c '%n %s %Y %U:%G %a' /etc/frr /etc/frr/*        # identical before/after this round and to the first run
/etc/frr 4096 1790149582 frr:root 750
/etc/frr/daemons 4127 1787745928 frr:root 640
/etc/frr/frr.conf 489 1787745928 frr:root 640
/etc/frr/support_bundle_commands.conf 10383 1787745928 frr:root 640
/etc/frr/vtysh.conf 32 1787745928 frr:root 640
$ ls -A /run/frr ; ip netns list | grep -c w12-frr ; ls -A /run/vrx-test/w12
0
frr.lock                      # the empty slot lock file stays (deleting it would break the flock for a waiter)
```

### CI
```
$ tools/ci.sh --base main
no contract files changed in the 9 commit(s) of HEAD since main (69ed862)
ok: gitleaks — scanned ~256460 bytes (256.46 KB) in 877ms no leaks found
ok  ngfw/agent/internal/renderers 1.551s; ok  ngfw/agent/internal/renderers/frr 3.463s; ok  ngfw/agent/internal/renderers/frr/frrtest 1.121s; …
  mode quick · wall time 1m28s · logs /root/ngfw-wt/logs/ci/RF-1-20260924-011826-1237934
CI GATE PASSED
```
Evidence logs: `/root/ngfw-wt/logs/RF-1-evidence/{unit-fix,integration-fix}.txt`, `etc-frr-{before,after}-fix.txt`.

Decisions this round (for the LOG): `Section.Render` takes `*RenderContext` (options: keep `proto.Message` + global
resolver / pass a context — chosen: context, carries secrets + mapper without globals); frr-reload.py log level
`critical` (options: `warning` — still logs failed commands with their lines, rejected); hostname lines ignored in the
diff (FRR 10.7 cannot apply them); lock file kept on disk (removing it breaks flock mutual exclusion).
