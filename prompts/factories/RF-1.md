# Task: RF-1 — Renderers for daemons: frr (renderer framework: files, vtysh -C, frr-reload.py, JSON state; protocol semantics in P12/F-*)   (prepend 00-CONTEXT.md)

## Goal
Write the agent-side **renderer** for FRR (WBS D3.2): desired state → validated `frr.conf` → applied through FRR's own control channel
(`frr-reload.py`, diff-based, never a restart) → `Retrieve` from `vtysh … json` output → events. This task builds the **framework**: file
layout, section registry, escaping, validate/apply/retrieve runners, the test-scoped daemon harness. `router bgp` / `ospf` / `bfd` sections
are plugged in later by P12 and F-* through the registry you publish — you render only the framework sections (globals, vrf, interface
description, static routes). Same pattern for every GPL daemon; P11 (strongSwan) is the reference — copy its shape, not its content.

## Inputs to read first
- `apps/agent/internal/renderers/renderer.go` + `README.md` + `ALLOWLIST.md` (P05a) — the interface and helpers you implement
  (`Name / Render / Validate / Apply / Retrieve`, `Files{Mode, Owner, Content}`, atomic write, fixed-argv runner, strict escaping)
- Daemon docs: FRR 10 user guide (`frr-reload.py`, integrated config, `-N` pathspace, `mgmtd`), `man vtysh`, `man zebra`, `frr-reload.py --help` on the
  host. Installed on this host (disabled): frr + frr-pythontools (`/usr/lib/frr/*`, `/usr/lib/frr/frr-reload.py`), strongswan (stock; vrx build
  comes from P11), kea-dhcp4/6 + kea-ctrl-agent, unbound, chrony, snmpd, keepalived, rsyslog. Check `vtysh -c "show version"` — FRR ≥ 10 needs
  `mgmtd` running for staticd/vtysh config paths.
- `packages/proto` messages for the domain (P03) — the input type (`Routing` globals, vrf list, static routes, interface descriptions); if a field is
  missing, `contract/<id>` branch first, questions file, continue
- `prompts/P12-frr-linuxcp.md` — the first consumer; its `router bgp` section must slot into your registry without touching framework files
- `docs/lab/shared-host-rules.md` — you are `daemon-owner: frr` for this task; others mock or skip; slot prefix `w<N>`, addresses `10.<N>.0.0/16`

## Scope — build exactly this, per daemon
Daemon set: **zebra, mgmtd, staticd** (framework); bgpd/ospfd/ospf6d/bfdd/pimd/isisd/ripd are started by P12/F-* tests, not yours. All paths come
from one injected `Paths` struct (product: `/etc/frr/frr.conf`, `/etc/frr/vtysh.conf`, `/var/run/frr`; tests: `/run/vrx-test/w<N>/frr/…`).
1. Templates in `internal/renderers/frr/templates/*.tmpl` rendered with `text/template` and **strict escaping helpers** — no user string reaches the file
   unescaped. Framework sections in fixed order: `frr version` / `frr defaults traditional` / `hostname` / `log syslog informational` /
   `service integrated-vtysh-config` → `vrf <name>` blocks → `interface <name>` (description only) → `ip route` / `ipv6 route` (staticd; per vrf,
   nexthop ip / interface / blackhole / distance / tag) → `line vty` → `end`. Also `vtysh.conf` (`service integrated-vtysh-config`).
   Publish `type Section interface { Name() string; Order() int; Render(desired proto.Message) ([]string, error) }` + `RegisterSection` so protocol
   tasks add `router …` blocks; the framework re-renders the **whole** file every time (frr-reload diffs). Escaping: hostname `[A-Za-z0-9][A-Za-z0-9.-]{0,62}`;
   description printable ASCII ≤ 80, no newline, no leading `!`/`#`; interface/vrf names `[A-Za-z0-9_.-]{1,15}`; addresses/prefixes typed, never raw.
2. `Validate()` — `vtysh -N <ns> --config_dir <dir> -C -f <frr.conf>` (confirm flags with `vtysh --help` on the host; document when a line cannot be
   dry-run-checked). Also run `frr-reload.py --test` when daemons are up and return the diff — the commit engine shows it in `DryRun`.
3. `Apply()` — write `frr.conf` + `vtysh.conf` atomically (temp + rename, owner `frr:frr` 0640 in product, root in tests), then apply via the control
   channel: `frr-reload.py --reload --confdir <dir> --rundir <dir> --vty_socket <dir> -N <ns> <frr.conf>` (bindir `/usr/lib/frr`; exact flags from
   `--help`). **Fixed argv, no shell**; list every binary you invoke in `internal/renderers/ALLOWLIST.md`: `/usr/bin/vtysh`, `/usr/lib/frr/frr-reload.py`,
   test-only `/usr/lib/frr/{zebra,mgmtd,staticd}`, `/usr/bin/ip` (netns exec). Never `systemctl restart frr` on a config change.
4. `Retrieve()` — daemon state as structured data through one `ShowJSON(cmd)` runner whose commands are **constants** (never user input):
   `vtysh -N <ns> --vty_socket <dir> -c "show running-config"` (normalised, for drift), `show ip route json`, `show ip route vrf all json`,
   `show ipv6 route json`, `show interface json`, `show version`, `show vrf json`. Expose `RegisterStateReader` so P12 adds `show bgp summary json`.
5. Events where the daemon exposes them (poll at 1 Hz otherwise): poll `show ip route json` route count / `show interface json` link state and emit on change;
   protocol pollers register through the same hook.
6. Unit tests with golden files (`testdata/*.golden`) — every template path covered (empty config, vrfs, ipv4+ipv6 statics with each nexthop type, descriptions),
   including hostile strings: `"; rm -rf /`, `\nrouter bgp 65000\n`, `!`, `#`, unicode, 300-char description → rejected or escaped, asserted.
7. Integration test on this host, **never through the system units or `/etc` paths**: inside the rig namespace `ns-w<N>-a` (P04 `tools/lab rig`; kernel
   routes then land only in that namespace) start as child processes with a test-scoped config dir and pidfiles: `mgmtd`, then
   `zebra -N w<N> -f <cfg> -i <dir>/zebra.pid -z <dir>/zserv.api --vty_socket <dir> -A 127.0.0.1 -P 0`, then `staticd -N w<N> …` (same flags; `-P 0`
   disables the telnet vty; vtysh talks unix sockets). Bound **only to `127.0.0.1:<slot port>` or to rig veths inside `ns-<prefix>-*`** — assert the
   rendered interface/listen list before starting; never `ens192`. Flow: render two static routes (`10.<N>.200.0/24 via 10.<N>.1.1`, one blackhole) →
   Validate → Apply → `show ip route json` shows both → change one → Apply → frr-reload diff applied, no daemon restart (same PIDs) → remove all →
   Retrieve shows none. You own frr for this task; kill by the PID you spawned; the system units stay stopped and disabled.

## Acceptance (paste the evidence)
- [ ] `go test ./internal/renderers/frr/...` green, integration included (paste the `show ip route json` excerpt and the frr-reload diff)
- [ ] `grep -rn "sh -c\|bash -c" internal/renderers/frr` is empty; `ALLOWLIST.md` updated with every binary above
- [ ] A rendered config with `"; rm -rf /` in a description field is rejected or escaped (test present, golden shows the result)
- [ ] No child daemon left running after tests (`pgrep -f /run/vrx-test/w<N>/frr` empty); `systemctl is-active frr` still `inactive`; `/etc/frr` untouched (`stat` before/after)
- [ ] zebra/staticd PIDs identical before and after a config change (reload, not restart)

## Out of scope (do not build)
API/UI, schema changes beyond an additive `contract/<id>` for framework fields, FRR routing-protocol semantics (`router bgp/ospf/isis/bfd/pim` —
that is P12/F-*), linux-cp interface pairs and FIB verification in VPP (DF-8/P12), the `/etc/frr/daemons` file and unit enablement (P10 packaging),
strongSwan build (P11), full-table performance, VRF↔netns mapping policy (P12), running any protocol daemon in tests, any write under `/etc/frr`,
`systemctl start/enable frr`, binding to `ens192`.
