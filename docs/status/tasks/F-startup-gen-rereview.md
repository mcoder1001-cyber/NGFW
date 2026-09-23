# F-startup-gen — re-review after fix round 1 (independent review agent, 2026-09-24)

Scope: `task/F-startup-gen` @ 130cc33 (fixes since the BLOCK at e049b31) and `contract/F-startup-gen` @ b662ff8.
Run on the host, read-only: `/etc/vpp/startup.conf`, `/sys`, `/proc` read; nothing written under `/etc`; VPP not
restarted; no NIC bound/unbound; `apply-startup.sh` never run in `--apply` mode against real paths (one `--stage rollback`
run with every `VRX_*` path pointed at scratch fakes, see N1). Probes that needed a fake `/sys`+`/proc` ran through
`go test -overlay` (no file added to the worktree).

## What I ran

| check | result |
|---|---|
| `tools/ci.sh --base main` on task branch @ 130cc33 | **CI GATE PASSED** (quick, 1m21s, logs `/root/ngfw-wt/logs/ci/F-startup-gen-20260924-023006-2029831`); contract guard lists the 4 `contract(` commits; only warning = the manager's `review(F-startup-gen): findings` subject |
| `tools/ci.sh --base main` on contract branch @ b662ff8 (temp worktree in scratchpad, removed afterwards) | **CI GATE PASSED** (quick, 1m40s, logs `/root/ngfw-wt/logs/ci/contract-wt-20260924-023156-2050590`) |
| drift guard on contract branch | `876 scalar leaves and 193 messages compared, 4 accepted difference(s), 0 finding(s)` — matches the pasted output |
| `go test ./internal/renderers/vppstartup/ ./cmd/vrx-startupgen/` | ok / ok |
| `deploy/vpp/test-apply-startup.sh <built generator>` | `36 passed, 0 failed` — matches |
| `shellcheck deploy/vpp/apply-startup.sh deploy/vpp/test-apply-startup.sh` | clean |
| contract branch additive? | yes: proto fields 8–11 + new messages `PluginSet`, `DataplaneDevice`; schema adds 4 optional/defaulted keys + 4 semantic validators; one existing test expectation (`parse({})` defaults), no rename/reshape of a field on main. Merges cleanly into current main (`git merge-tree`) |
| renderer registered anywhere? | no — only `cmd/vrx-startupgen` imports the package; `Apply` = `ErrManagerStep` |

## Original findings

| # | verdict | evidence |
|---|---|---|
| F1 mgmt NIC from the document | **PARTIAL** | Host fact now authoritative (`hostfacts.go:90-118`, `model.go:505-534, 594-596`). Reviewer repro → `managementPci … does not match the host's management NIC(s) 0000:0b:00.0` exit 2; `{}` renders `blacklist 0000:0b:00.0`; doc `managementPci` as a strict subset of a 2-NIC host set → rejected; upper-case `0000:0B:00.0` canonicalised and accepted as equal; no route files → `ErrHost` "management NIC is unknown"; IPv6-only default → protected. **Not covered:** the management path that is *not* a default route (N4) — a document can still hand the NIC the manager is SSH'd on to DPDK when that NIC only carries a connected subnet. The original fix text asked for "default route / the address the manager is connected on"; only the first half was done |
| F2 apply procedure | **PARTIAL** | Fixed: detached via `systemd-run --unit … --collect` (fallback `setsid nohup`) (`apply-startup.sh:217-221`); locks before sha/backup/diff (231-240); sha check inside the lock (233-234); dead-man armed before `install`/restart (255-263) and cancelled only after the full window (273-274); interface check by API not `vppctl` exit code; plugin checks by `show plugins` content; NRestarts used; driver rebind on rollback (173-184, matches VPP's own `driver_override`/`bind` sequence in `vlib/linux/pci.c:577-605`). **Still broken:** dead-man cannot roll back a hung run (N1); on vrx-a the rollback cannot restore the management address (N2); the interface checker cannot run on vrx-a, so every apply with a logical name rolls back (N3); the reviewed rendering is not pinned (N5) |
| F3 stand-ins drop plugins / mgmt | **FIXED** | Contract fields real (D-081) and `plugins` wrapped (D-084). Probed with live host facts: absent → current switches kept with warnings; `{"plugins":{}}` → authoritative empty, one warning per D-060 plugin and per removed switch, exit 0; bare map `{"plugins":{"linux_cp_plugin.so":true}}` → `unknown field` exit 2; `dpdk_plugin.so: false` with devices → exit 2. Strict decode confirmed (`maincore`, `Devices` rejected). Residual inconsistencies in N6/N12 |
| F4 CPU model | **FIXED** | Always explicit `main-core` + `corelist-workers` (`model.go:402-503`). Checked against VPP 26.06 `vlib/threads.c:226-400`: non-relative mode uses the *online* bitmap (not the affinity mask), so isolcpus does not make pinned workers "unavailable"; explicit values leave VPP no choice. `{"mainCore":5,"workers":2}` + isolcpus 6-7 → `corelist-workers 6-7`; corelist 40 on a 0-31 host rejected; mainCore in corelist rejected |
| F5 hugepages | **FIXED** | `hugepagesGb: 0` → `0 not in 1..1024`; `hugepagesGb: 64, buffersPerNuma: 500000` → error against the 2 GiB host reservation; `hugepagesGb: 1` → error against 1 GiB; `HugePages_Total = 0` → `ErrHost` |
| F6 bounds / unknown keys | **FIXED** | `txQueues 100000`, `mainCore 4294967295`, device `rxQueues 0`, unknown keys → exit 2 (`model.go:286-332`, strict protojson) |
| F7 Renderer without host | **FIXED** | `New(host Host, …)`, `Host.Check` in `BuildModel` (`model.go:94-115, 239`) |
| F8 comment rule | **FIXED** | `semantic.go:71-76`: `#` anywhere ends the line (VPP rule) |
| F9 live file in unit tests | **FIXED** | no test reads `/etc/vpp/startup.conf`; `testdata/host-startup.conf` only |

## New findings (by severity)

### N1 — HIGH (blocking): a hung VPP leaves the run blocked forever, holding the locks, and the dead-man cannot roll back
`deploy/vpp/apply-startup.sh:98, 135, 155` (`vppctl` without timeout), `:258` (dead-man), `:283` + `:127-128` (`take_locks`)

`vppctl` waits for output with `epoll_wait(efd, &event, 1, -1)` (`/root/vpp/src/vpp/app/vppctl.c:369`), with no timeout.
When VPP accepts the CLI socket but its main loop is stuck (e.g. DPDK device init hanging in a process node after
the new `dev` lines — exactly the case a start-up change causes), `wait_api` / `check_health` / `loaded_plugins`
block forever. The run keeps `flock -x` on `vrx-vpp.lock` + `vrx-lab.lock`. The dead-man fires after
`window+api_wait+120` s, calls `take_locks` with the **default 1800 s** timeout (no `--lock-timeout` is passed at :258),
then gives up with exit 3 and **never rolls back**. Reproduced with scratch paths (lock held by another process):
```
$ VRX_VPP_LOCK=…/vpp.lock … apply-startup.sh --stage rollback --work …/work --lock-timeout 3
apply-startup: 2026-09-24 02:34:56 REFUSED: …/vpp.lock busy for 3s
exit=3          (startup.conf still the new file)
```
`systemctl restart vpp` itself does not hang (`Type=simple`), and a crash is caught (`Restart=always` → NRestarts), so
the hang is the uncovered failure mode. Fix: wrap every `vppctl`/`ip`/checker call in `timeout 10`; make the dead-man
**stop the run unit first** (`systemctl kill --signal=KILL vrx-startup-apply-<stamp>`, or kill the recorded run PID
from the setsid fallback) so its locks are released, then take the locks with a short timeout; add a fake-host
scenario "vppctl hangs" (fake `vppctl` that `sleep infinity`s).

### N2 — HIGH (blocking): on vrx-a the rollback cannot restore the management address (no netplan; ifupdown host)
`deploy/vpp/apply-startup.sh:191-196`

vrx-a has no `netplan` binary and no `/etc/netplan`; `ens192` is configured by ifupdown
(`/etc/network/interfaces.d/ens192.cfg`: `auto ens192` / `iface ens192 inet static` / `gateway 172.30.126.1`;
`networking.service` active, networkd/NetworkManager inactive). After a driver steal is undone (vfio-pci → vmxnet3),
the kernel re-creates `ens192` **without** its address and default route; `auto` (not `allow-hotplug`) means nothing
re-applies it. The rollback does `ip link set up`, skips netplan (`command -v` fails), and ends with
"ROLLBACK INCOMPLETE — console access needed" — the exact F2.3 scenario, on the very host the script was written for.
The fake-host scenario 8 passes only because the test provides a fake `netplan`. Fix: record `ip -o addr show dev
<mgmt>` and `ip route show default` before the restart and re-add them verbatim on rollback (independent of the network
manager), then additionally try `ifup --force <dev>` / `netplan apply` / `networkctl reconfigure` whichever exists;
fake-host test without netplan.

### N3 — HIGH: the interface checker cannot run on vrx-a → every apply with a logical interface name rolls back
`deploy/vpp/vpp-iface-check.py:14`, `apply-startup.sh:165-168`

```
$ python3 deploy/vpp/vpp-iface-check.py local0
vpp-iface-check: python3-vpp-api missing: No module named 'vpp_papi'
exit=2
```
`dpkg -l` lists `python3-vpp-api 26.06-release`, but `/usr/lib/python3/dist-packages/vpp_papi/` does not exist on this
host (Python 3.14). Exit 2 → `check_health` "logical interface(s) missing" → rollback, i.e. two VPP restarts for every
apply that names a NIC (every D-069 apply). The checker was never executed against a real VPP (the fake-host test
substitutes `VRX_IFACE_CHECK`). Fix: preflight in the dry run **and** in `stage_run` before arming/installing — run the
checker against the running VPP (`local0` must be found), refuse with exit 3 otherwise; paste one real read-only run
(`sw_interface_dump` is read-only) in the status file. Remove the committed `__pycache__` (N10).

### N4 — MEDIUM: "management NIC = default-route interface" misses real management paths and breaks on an operational router
`apps/agent/internal/renderers/vppstartup/hostfacts.go:90-106, 123-154`; `apply-startup.sh:100-102, 244-250`

Probed via `go test -overlay` on a fake root:
- SSH on a directly connected management subnet (`ens193`, 0000:0c:00.0), default route on another NIC →
  `mgmt=[0000:04:00.0]`, and `devices: {"0000:0c:00.0": {name: lan}}` is **accepted** (blacklist only 04:00.0). The apply
  script also watches only the default-route device, so it would commit with the SSH path gone (VPP's own
  "host interface is up" skip in `vlib/linux/pci.c:565` is the only guard).
- A default route through a linux-cp tap (`wan`, no `device` link) → `ErrHost: management interface wan has no PCI device`.
  linux_cp is enabled (D-060) with no default netns (V21 note), so once FRR/static routes put the host's default route
  on an LCP tap, the generator refuses to render at all on the running router.
- `unreachable`/`blackhole` default in the main table (`/proc/net/route` iface `*`) → same hard error.
Fix: add the interface that carries the manager's session (`ip route get <SSH_CLIENT addr>` in the script, a
`--mgmt-if` from it for the generator), ignore default routes on non-PCI virtual devices (tun/tap, `*`) instead of
failing, and let the script watch every protected interface, not the first IPv4 default route.

### N5 — MEDIUM: `--expect-sha256` pins the live file but not the rendering that gets installed
`apply-startup.sh:54, 212-217, 235`

The dry run reviews a rendering; the detached run renders **again** from a document copied at `--apply` time (not at
dry-run time), with whatever generator is at `STARTUPGEN`. `systemd-run` starts the unit with systemd's environment, not
the caller's (no `-E`/`--setenv`), so a `VRX_STARTUPGEN=…` override used for the dry run is silently dropped and the
detached run uses `/root/ngfw/apps/agent/bin/vrx-startupgen` — in the shared main tree that another agent rebuilds.
Host facts (default route, hugepages) can also change in between. Fix: print `sha256(new.conf)` in the dry run, require
`--expect-new-sha256` and compare inside the lock before `install`; pass the generator path explicitly
(`--startupgen <abs path>` recorded in `$WORK`) instead of via the environment.

### N6 — MEDIUM: D-084 "present = authoritative" removals are un-appliable — the plugin check rolls them back
`apply-startup.sh:160-164`

A plugin that was loaded before and is merely **not listed** in the new file (D-084 authoritative omission) is treated as
"loaded before and is gone" → rollback. `linux_cp`, `linux_nl`, `npt66` are `default_disabled = 1`
(`plugins/linux-cp/lcp_api.c:388`, `lcp_nl.c:1046`, `npt66/npt66_api.c:60`), so dropping them by omission — the case the
generator explicitly warns about and accepts — always fails the apply after a VPP restart. Only an explicit `false`
works. Fix: exempt plugins that the old file enabled explicitly and the new file no longer mentions (or have the dry run
refuse such documents with the same message), and add the scenario to the fake-host test.

### N7 — LOW: bond/VLAN management — the documented workaround does not work
`hostfacts.go:100-104`, `docs/agent/renderers/vppstartup.md` "Known limits"

The doc says "name the NIC(s) with `--mgmt-pci`", but `ReadHost` returns the "has no PCI device" error for the
default-route interface before `MgmtPCI` is applied. Probe: default route on `bond0` + `MgmtPCI: [0000:0b:00.0]` →
error. Only `--no-host` with every fact by hand works. Fail-closed, but fix the code (skip an interface when `--mgmt-pci`
/ `--mgmt-if` supplies the NIC, or resolve bond slaves via `/sys/class/net/<bond>/lower_*`) or the doc.

### N8 — LOW: the script does not enforce the handover gate
`apply-startup.sh:290-304`. `tools/lab restart-vpp` refuses while `docs/lab/host-vrx-a.md` says `handover: pending`
(D-012; it still says so). The apply script restarts VPP with only a comment saying "manager only". Fix: the same
`handover_state` check (or `--i-have-a-decision D-xxx` recorded in the log).

### N9 — LOW: the setsid dead-man fallback keeps the VPP and lab locks held after commit
`apply-startup.sh:259`. The background child inherits fds 8/9 (the flocks) and sleeps `window+api_wait+120` s, so after
a successful commit `vrx-vpp.lock`/`vrx-lab.lock` stay locked for up to ~3.5 min (CI and `tools/lab` block). Fix:
`… 8>&- 9>&- &`.

### N10 — LOW: compiled Python committed
`deploy/vpp/__pycache__/vpp-iface-check.cpython-314.pyc` (commit e382a65). Remove; ignore `__pycache__/`.

### N11 — LOW: rollback may hit systemd's start limit
`apply-startup.sh:197`. `vpp.service` has `Restart=always` and default `StartLimitBurst=5/10s`; a new file that makes VPP
exit at start can leave the unit `start-limit-hit`, and `systemctl start vpp` then fails (swallowed by `|| true`).
Today the `API_WAIT` ≥ 10 s delay usually outlasts the interval; make it explicit with `systemctl reset-failed vpp`
before `start`.

### N12 — LOW: small contract/generator mismatches
- CLI accepts `"plugins": null` (→ absent, overlay) and `"switches": null` (→ present-empty, drops every switch); Zod
  rejects both. Reject `null` in the generator so it matches the schema.
- `docs/contracts/schema.md` gives the logical-name pattern as `[a-z][a-z0-9_-]{0,14}`; the schema regex also forbids a
  trailing `_`/`-`.
- `pciAddress` accepts device `20`–`ff`; `PCIAddress` rejects device > `1f`. A schema-valid document can fail at render
  time; align (either side).

## Summary

The generator side is in good shape: F3–F9 fixed, strict decode and bounds hold against every probe, the contract
branch is additive with the drift guard green and CI green on both branches. The contract branch on its own can be
merged. `deploy/vpp/apply-startup.sh` — the fix for F2, and the tool that would do the first real start-up change on a
remote host with a single management NIC — still fails in three of the cases it exists for: a hung VPP (N1), recovering
the management address on this ifupdown host (N2), and verifying interfaces on this host (N3). N1–N3 must be fixed
(each with a fake-host scenario, N3 with one real read-only checker run) before the branch merges; N4–N6 before the
first real apply.

**BLOCK**
