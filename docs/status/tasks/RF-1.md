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
