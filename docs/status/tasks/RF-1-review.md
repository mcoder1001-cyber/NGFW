# RF-1 — review (independent reviewer, 2026-09-24)

Branch `task/RF-1` @ `7d941e2` (base `main@40ba948`; `git merge-tree` against current `main` is clean).
Reviewed on the host, slot 12. I wrote none of this code.

## What I ran

| check | result |
|---|---|
| `tools/ci.sh --base main` (my run, log `/root/ngfw-wt/logs/ci/RF-1-20260924-004919-964424`) | `CI GATE PASSED`, wall time 0m52s, `ok ngfw/agent/internal/renderers/frr 2.004s`, gitleaks "no leaks found", contract guard "no contract files changed in the 5 commit(s)". Same as the pasted run in `RF-1.md` |
| `VRX_INTEGRATION=1 go test -run TestFRRRendererIntegration` (slot 12) | PASS (16.04 s). Same steps and output shape as pasted: both static routes in `show ip route json`, frr-reload diff, PIDs unchanged across reloads, LINK_DOWN event |
| My own throw-away probe (`zz_review_probe_test.go`, real mgmtd/zebra/staticd in `ns-w12-frr` via `frrtest`; deleted afterwards, never committed) | results in findings H1, H2 and M1 below |
| After all runs | `systemctl is-active frr` = `inactive`. `/etc/frr` `stat` (name, mtime, size) is the same before and after. No `/usr/lib/frr/*` process is running. `/run/frr` is empty (no `w12` symlink). No `ns-w12-frr`. `/run/vrx-test/w12` is empty. Worktree clean |

Checklist: (1) contract: no hits. (2) real verification: yes, real FRR daemons, and the test asserts on `show ip route json`, the kernel route and `show interface json`. (3) restart safety: not applicable (no VPP objects). The integrated `frr.conf` is the daemons' startup config. (4) binapi: not used. (5) shared host: prefixed, pidfile kill, t.Cleanup, `/etc/frr` untouched (but see M4 and L3). (6) security: findings H1, M1, M2 and L1. (7) transaction: H2 and L2. (8) and (10): no UI. (9) scope: I1. (11) CI matches.

---

## Findings, ranked by severity

### H1: a `|` in a description reaches FRR's CLI pipe handler. The description is silently truncated or dropped, and the diff never converges
`apps/agent/internal/renderers/frr/escape.go:58-79` (`Description` allows every printable ASCII byte, including `|`).

FRR's `cmd_execute` runs every config line through the pipe hook (`handle_pipe_action`), both inside the daemons and inside vtysh. When a line contains `"| "`, the hook cuts the command at that point. `| include X` becomes an output filter, and any other word after `| ` is an error. I proved it live (probe, real daemons):

| description | vtysh -C | Apply returns | stored by FRR (`show interface json`) | DryRun after Apply |
|---|---|---|---|---|
| `a \| include b` | ok | `nil` | `"description":"a"` | not empty (re-applies forever) |
| `a \| b` | ok | `nil` | *no description at all* | not empty |
| `a ! b # c; exit; end`, `x ? y`, `exit-vrf`, `end`, `a\b`, `$(reboot) `` `id` `` | ok | `nil` | verbatim | empty |

Failure scenario: an operator sets `interfaces.X.description = "uplink | ISP-A"`. The commit reports success, but FRR holds a different config (or none). Every later commit shows a phantom diff, and `Retrieve` drift never clears. P12 will reuse `frr.Description` for BGP neighbor and route-map descriptions, which are also LINE tokens, so the same hole spreads there.

Fix: reject `|` in `Description` (the simplest option; `"| "` is the trigger, but rejecting the character is safer). Add `a | b` and `a | include b` to `TestHostileStringsRejectedOrEscaped`. Document in `Section` (section.go:31) that every LINE-type token must not contain `|`, and make `checkLine` (section.go:169) reject `"| "` in any rendered line as a backstop.

### H2: `Apply` returns success when FRR did not apply a line. There is no convergence check after the reload
`apps/agent/internal/renderers/frr/renderer.go:220-222`.

`frr-reload.py --reload` hands the added lines to `vtysh -f`. vtysh keeps going after a failing line and exits 0. In the probe, the `a | b` case returned `Apply err: <nil>` although the line was never applied. So the commit engine records a revision the data plane does not have. The integration test checks `DryRun == ""` after every Apply, but that check lives only in the test, not in `Apply`.

Fix: after a successful `--reload`, run the same `--test` diff (`DryRun` on the written file). Treat a non-empty result as `ErrDaemon` ("not converged: <diff>"), then take the existing restore-and-reload path. Add a unit test with a `RecordingRunner` whose `--test` answer is non-empty after the reload.

What does work: when frr-reload *does* exit non-zero, the rollback restores the previous state. In the probe, a next hop named `tag` passed `vtysh -C`, failed in zebra, and `Apply` returned the error. The running config and the file on disk were both back at the previous route.

### M1: next-hop interface names that are CLI keywords or IP-shaped change the route's meaning
`model.go:314-324`, `templates/framework.tmpl:21` (`{{with .Interface}} {{ifname .}}`).

`IfName` (`[A-Za-z0-9_.-]{1,15}`) accepts names that FRR's `ip route` grammar reads as something else. All of these were verified live:

- `Null0` and `blackhole` become a blackhole route. `reject` becomes a reject route.
- `bl` is a keyword abbreviation: FRR stores `ip route … blackhole`, and the diff never converges.
- `10.12.1.1` as an *interface* is installed as a *gateway* (`ip route 10.12.234.0/24 10.12.1.1`).
- `tag` passes Validate, then fails at Apply.

This is semantic injection, not code execution. The operator still gets a route that differs from what the document says: an interface next hop becomes a silent drop. For the same reason, IP-shaped interface names in `interface <name>` blocks are also questionable.

Fix: in `buildRoutes`, reject next-hop interface names that parse as an IP address (`netip.ParseAddr`). Also reject names that equal, case-insensitively, or are a prefix of any keyword of the `ip route` / `ipv6 route` grammar (`blackhole reject null0 tag label table vrf nexthop-vrf onlink color segments bfd track`). Unit-test each case.

Same file: the gateway check accepts `0.0.0.0`/`::`, multicast, loopback, and IPv6 link-local without an interface. The probe applied `ip route 0.0.0.0/0 0.0.0.0` and `ipv6 route 2001:db8:12::/64 fe80::1` without error. Reject unspecified, multicast and loopback gateways, and require an interface with a link-local gateway.

### M2: no secrets path. P12 cannot add `neighbor … password` without editing framework files, and today the password would leak in four places
- `Section.Render(desired proto.Message)` (section.go:37) has no context or secret resolver. P12's `password(secretRef)` must be resolved to plaintext somewhere, and the only way is a framework change.
- `Retrieve`/`State` returns the whole `show running-config` (state.go:167-171) inside a `structpb`, which is what a `/state` GET will serve. FRR prints `neighbor X password <plaintext>` there. This breaks 00-CONTEXT rule 10 ("never returned by GET").
- `DryRun` returns frr-reload's "Lines To Add", which the commit engine shows to the UI and which includes the password line.
- `toolMessage` (renderer.go:236-255) puts vtysh output such as `line N: … line: <offending line>` into the error. That goes to problem+json and to logs.
- `reloadArgs` passes `--log-level info` (paths.go:114). frr-reload.py then writes every added line to `--logfile` (`/usr/lib/frr/frr-reload.py:2552` `log.info(f"{filename} content\n{pformat(lines_to_configure)}")`, and `:2525` for executed deletions). That means `/var/log/frr/frr-reload.log` would keep plaintext BGP/OSPF keys permanently.

Fix, in RF-1 or as the first commit of P12, but decided now:
- Add a redaction hook: `RegisterRedactor(re)` or `Section.SecretPatterns()`, applied in `NormalizeConfig`, `NormalizeDiff`, `toolMessage` and to `State.RunningConfig`. Mask with `<redacted>`.
- Pass a resolver into rendering, e.g. `Render(ctx, desired, Secrets)`, or a `WithSecretResolver` option that `frr.Desired` exposes.
- Use `--log-level warning` for `--reload`.

frr.conf mode 0640 `frr:frr` is already right.

### M3: `Retrieve` and the 1 Hz route poller dump the whole RIB. Both fail once it is larger than a few thousand routes
`state.go:180`, `events.go:105-107`.

Every `Retrieve` and every poll tick runs `show ip route vrf all json` and `show ipv6 route vrf all json`. That is about 1 KB of pretty-printed JSON per route. `SystemRunner` silently truncates output at `DefaultMaxOutput` = 4 MiB (`helpers_exec.go:206-226`), so `json.Valid` fails and the whole of `State()`/`Retrieve()` returns an error. With roughly 4-5k routes (a modest BGP feed, and P12's own test announces 200), Retrieve stops working and the poller logs an error every second. It also burns CPU on zebra.

Fix:
- Poller: use `show ip route vrf all summary json` (counts per protocol and VRF).
- Retrieve: read only what the framework owns (`show ip route vrf all static json` / `show ipv6 route vrf all static json`).
- Let protocol readers ask for their own filtered commands.
- Surface truncation: expose `limitedBuffer.truncated` as an error instead of returning cut output.

### M4: two consumers of `frrtest` on the same slot kill each other's daemons
`frrtest/harness.go:163` (`killStale`), `:399-402` (`ours`), `:164` (`RemoveAll(Base)`).

`tools/ci.sh full` runs `go test -race ./...` with slot 12 for *every* package, and Go runs packages in parallel. `vpptest.LockLab` takes only a *shared* lock. Once P12 and F-ospf add their own `frrtest.Start(Prefix: "w12")`, the second package's `killStale` will find the first package's daemons. They pass `ours()`, because the cmdline contains the same socket dir. It SIGTERMs them and `RemoveAll`s the other test's directory. The result is flaky, cross-killing integration runs. This is the realistic way the pidfile kill can hit a "foreign" PID: foreign to the test, though still one of our own daemons.

Fix:
- Take an exclusive `flock` on `/run/vrx-test/<prefix>/frr.lock` for the harness lifetime, so a second Start waits. Alternatively, allow a sub-prefix per package (`w12b`), which `prefixRe` already accepts.
- Tighten `ours()`: check that `/proc/<pid>/exe` is the daemon binary and that argv holds `-i <this pidfile>`. The current check is a substring match on the socket dir. A plain `tail -f /run/vrx-test/w12/frr/run/w12/zebra.log` also matches (my own `pgrep -af` shell matched the same way). Low risk, but cheap to close.

### L1: `/usr/bin/ip` in an allowlist is an exec trampoline
`ALLOWLIST.md` row `ip netns exec <ns> <daemon>`, `frrtest.Binaries()` (harness.go:63).

Any runner whose allowlist contains `/usr/bin/ip` can run *any* binary through `ip netns exec`. Today only the test harness has one. But `frrtest` is a non-`_test` package and `h.Runner` is what `h.Renderer()` uses.

Fix: add a unit test asserting that `frr.Binaries()` never contains `/usr/bin/ip`. Also consider a guard in `helpers_exec.go`: when `Path == /usr/bin/ip`, the argument after `netns exec <ns>` must itself be in the allowlist. Mark the row "test-only; never in a production allowlist".

### L2: rollback gaps
`renderer.go:224-231`.

1. The rollback reload uses the caller's `ctx`. If the first reload failed because the context expired (the 120 s `reloadTimeout` or the engine's deadline), the rollback fails immediately and leaves partial state: frr-reload applies deletes and adds as separate vtysh runs. Use `context.WithoutCancel(ctx)` with its own timeout for the rollback.
2. When no previous file existed (`!existed`), `Restore` deletes the file and no reload runs, so the partially applied config stays. Product always has `/etc/frr/frr.conf`, so this is low, but document it or reload an empty config.
3. `Apply` has no mutex. Two concurrent Applies would interleave. The commit engine probably serialises commits; say so in the doc comment.

### L3: `/run/frr/<prefix>` symlink (Q6)
`harness.go:216-226`. It is acceptable, with conditions. The link is pathspace-scoped, it is refused unless it is missing or already our own link, it is removed on Stop, and a stale one is cleaned on the next Start.

Residual issues:
- `/run/frr` is owned by `frr`, so the Lstat→Symlink step races with that user. That is not a real threat on this host.
- A SIGKILLed test leaves a dangling link, and so does `ns-<prefix>-frr` (Q7). Neither is known to `tools/lab rig gc` or to the nightly prefix sweep. Add both to `tools/lab rig gc` (a P04/P09 follow-up, not RF-1).

### L4: IdentityMapper is the product default
`renderer.go:73`, `model.go:104`. Until P12 injects the linux-cp mapper, VPP interface names that happen to be valid Linux names (`loop0`, `tap0`, `ipip0`) get `interface loop0 / description …` blocks in FRR, and static routes point at non-existent Linux interfaces. Make the product default "no mapping" (every interface `ok=false`). Keep `IdentityMapper` for tests and the harness.

### I1: scope
ALLOWLIST rows for bgpd, ospfd, ospf6d, bfdd, pimd, isisd, ripd and ldpd were added ahead of P12/F-*, as was `frrtest.Options.Daemons` support for them. Both are small and serve the declared consumers. I accept them, but the rows should say "unused until <task>".

### I2: the rendered file differs between tests and product
With `-N`, `frr-reload.py --confdir <ConfDir>` receives `<ConfDir>/<ns>/frr.conf`, and `filename != target` (`frr-reload.py:2567-2568`). So frr-reload runs `vtysh write` and FRR rewrites the file after every test Apply: I saw a `hostname` line added on disk. In the product the paths are equal and no write happens. That is harmless, but the snapshot restored in tests is FRR's text, not ours. Worth one line in the README.

---

## The questions I was asked to rule on

- **`"; rm -rf /` written verbatim (decision table).** I tried it and the variants live. frr-reload.py uses `subprocess.Popen` with a list and has no `shell=True` (grep). Additions reach FRR through `vtysh -f <file>`, and description deletions are reduced to `no description` (`frr-reload.py:1314-1323`). FRR stores `"; rm -rf /`, `$(reboot) `` `id` ``, `a ! b # c; exit; end`, `exit-vrf`, `end` and `a\b` verbatim, and the diff converges. So writing printable ASCII verbatim is safe with respect to shells and to line structure, and I accept that decision. **Except for `|`** (H1).
- **Q4: VPP descriptor and FRR staticd both programming `routing.static` once linux-nl is active.** This is a real risk, not only noise:
  - linux-nl would mirror staticd's kernel route into the VPP FIB as a second source (lcp) next to the descriptor's API source. Deleting it through one owner leaves the prefix forwarding through the other.
  - Next hops are resolved differently: Linux tap names versus VPP interfaces, and kernel table IDs versus VPP table IDs.
  - The VPP route descriptor's `Retrieve` sees lcp-sourced entries it did not create, which produces a drift or delete loop unless it filters by FIB source.

  **Proposed rule (for the LOG, owner P12):** each static route has exactly one owner.
  - By default, `routing.static` is programmed into VPP by the descriptor only, and FRR does not render it.
  - A route is given to FRR only when it must take part in routing (redistribution, next-hop tracking, BFD). An explicit per-route flag marks it (additive schema field, e.g. `routing.static[].frr: true`), and then the descriptor skips it and linux-nl installs it.
  - Never both. The VPP descriptor's `Retrieve` ignores FIB entries not sourced by the agent.
  - Until P12 lands that flag, the framework should render static routes only when a renderer option (`WithStaticRoutes`) is set. The P12 integration then picks the owner explicitly instead of inheriting double programming.

  Not blocking RF-1.
- **Q5: ALLOWLIST.md edited outside the envelope.** Acceptable. `TestAllowlistDocumented` forces it, and the P05a rules say "add the row in the same commit that adds the call". The diff is rows only: two Planned rows moved to Active, plus new rows, and no rule text changed. Conflicts with other RF-* branches are append-only. Add the L1 wording.
- **Q6: `/run/frr/<prefix>` symlink.** Acceptable, see L3.
- **Q7: `ns-<prefix>-frr`.** Acceptable, since FRR tests should not need VPP. It needs a `rig gc` entry (L3).
- **Q10: task-branch history rewrite.** Noted only. The reflog shows the reset to `5afa26f` and 4 WIP commits recreated as `fde86a5`. The manager's envelope commits (`5f31b35`, `5afa26f`) are untouched. I re-ran gitleaks on the dropped range: 3 hits, all `generic-api-key` on the strings `ipv4/default`/`ipv6/default` in `RF-1.md`, which are false positives and not secrets. The dropped objects stay in the local object store until gc. The rewrite also erased the 45-minute WIP trail. That is acceptable under `docs/contributing.md`.
- **P12 plug-in without touching framework files.** It works for `router bgp` / `route-map` / `prefix-list` sections (`RegisterSection`, orders 400-899, `frr.Desired`), for `show bgp summary json` (`RegisterStateReader`) and for neighbour polling (`RegisterPoller`). I checked frr-reload.py's `save_contexts`: a second `interface X` block from F-ospf/F-pim is merged with the framework's block, so per-interface protocol lines also work. It does **not** work for:
  - the BGP password (M2),
  - an interface-name mapping inside a protocol section: `neighbor … update-source <if>` and interface-level lines need the InterfaceMapper, which lives on the Renderer and is not passed to `Section.Render`. Expose it, e.g. `frr.Mapper(desired)` or a `RenderContext`,
  - neighbour event kinds (Q3): `Event.ToProto` maps only the `interfaces` poller.

---

## Required before merge
H1, H2, M1. Each is a small change in `escape.go`, `renderer.go` or `model.go` plus unit tests. Re-run the integration test.

## Required before P12/F-* start (may land as a follow-up task, but must be on the board)
M2 (secret resolver and redaction, reload log level), M3 (RIB-size-safe Retrieve and poller), M4 (harness lock), and the InterfaceMapper exposure.

**APPROVE WITH CHANGES**
