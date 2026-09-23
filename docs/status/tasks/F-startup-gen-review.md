# F-startup-gen — review (independent review agent, 2026-09-24)

Branch `task/F-startup-gen` @ 2888b94, base main. Reviewed on the host (read-only `/etc/vpp/startup.conf`, VPP source
`/root/vpp` read for semantics, nothing written under `/etc`, VPP untouched).

## Checklist summary

| # | check | result |
|---|---|---|
| 1 | contract | no diff under `packages/schema`, `packages/proto`, `apps/agent/gen`, generated clients. Four stand-in fields are read from the raw JSON (D-055), listed in Q1 — OK, but see F3/F6 on how safe that is until the additive contract change lands |
| 2 | real verification | pure generator; the live-file semantic test (`render_test.go:151`) reads `/etc/vpp/startup.conf`. Adequate for a renderer that never applies |
| 3 | restart safety | N/A (no VPP objects); `Retrieve` → `ErrRetrieveUnsupported`, `Apply` → `ErrManagerStep`, asserted in `render_test.go:221` |
| 4 | binapi | not used, not touched |
| 5 | shared host | no processes, no daemons, no writes under `/etc` in tests |
| 6 | security | no `exec.Command`; template input passes strict validators + `ident`/`pathtok` + `CheckRendered`. Injection attempts (below) all rejected |
| 7 | transaction semantics | renderer not registered anywhere; only the CLI imports the package (`grep` over `apps/agent`) — (f) holds |
| 8/10 | UI / i18n | N/A |
| 9 | scope creep | `Renderer` (dry-run use, Q3) and the in-process LCS diff are small and justified; none to remove |
| 11 | CI | `tools/ci.sh --base main` run by the reviewer: **CI GATE PASSED** (mode quick, wall 1m04s, logs `/root/ngfw-wt/logs/ci/F-startup-gen-20260924-014651-1560129`); matches the pasted run |

Reviewer probes (built `vrx-startupgen`, fed documents on stdin): newline / `}` / `{` / `#` / U+2028 / overlong logical
names, brace+newline in `devices` keys and `plugins` keys, `../` plugin paths, `default` plugin, 1e300 numbers,
`0b:00.0`, `" 0000:0b:00.0"`, Arabic-Indic and full-width digits, `0000:0B:00.0` vs `0000:0b:00.0` in every
combination of `managementPci`/`pciWhitelist`/`devices`, duplicate JSON keys → **all rejected** with one-line errors.
`--diff` / `--diff --semantic` against the live file: only the header comments, dropped stock comments, and the
explicit `statseg { socket-name /run/vpp/stats.sock }` differ (semantic: exactly `+ statseg {}`, `+ statseg > socket-name …`).

## Findings (by severity)

### F1 — HIGH (blocking): the management NIC is whatever the document says; any document can put 0000:0b:00.0 into `dev`
`apps/agent/internal/renderers/vppstartup/model.go:449-460, 524-527, 538-540`

The mgmt check only compares `devices`/`pciWhitelist` against `dataplane.managementPci` **from the same document**.
The host's real management NIC is never consulted. Reproduced:

```
{"dataplane":{"managementPci":["0000:04:00.0"],"devices":{"0000:0b:00.0":{"name":"lan"}}}}
→ exit 0:   dev 0000:0b:00.0 { name lan }   blacklist 0000:04:00.0      (ens192 blacklist gone)
```
Also `{}` / any document without `managementPci` renders **no** `blacklist 0000:0b:00.0` (harmless only because
`no-pci` follows) — the prompt's "is always blacklisted" is not met. A swapped entry while typing the pending NIC →
port-group mapping (Q2) is exactly this failure. Today VPP itself would still skip binding ens192 while its kernel
interface is UP (`/root/vpp/src/vlib/linux/pci.c:566`) and blacklists un-whitelisted vmxnet3 by default
(`plugins/dpdk/device/init.c:743`), but neither covers an explicitly whitelisted NIC that is down at VPP start, and the
generator's own invariant must not depend on that.

Fix: make the management NIC(s) a **host fact**, not (only) a document field: `Host.ManagementPCI`, filled by the CLI
from the PCI device behind the interface(s) carrying the default route / the address the manager is connected on
(`/sys/class/net/<if>/device` → basename), plus an explicit `--mgmt-pci` flag that is required with `--no-host`.
`BuildModel`: blacklist = host mgmt ∪ `managementPci`, always rendered (also with `no-pci`); reject any device in that
union; reject rendering (not just devices) when host mgmt is unknown. Hostile tests: document mgmt ≠ host mgmt, empty
document still renders `blacklist 0000:0b:00.0`.

### F2 — HIGH (blocking, docs): the manager apply/rollback procedure is not sound
`docs/agent/renderers/vppstartup.md:84-118`

1. **Rollback dies with the SSH session.** The script runs in the manager's SSH shell; if the new file does take the
   management path down, sshd's session gets SIGHUP and the `bash -c` (including the rollback branch) is killed — the
   one failure the rollback must survive. Run the restart+verify+rollback detached (`systemd-run --unit=vrx-startup-apply
   --collect …` or `setsid nohup`), and add a dead-man timer (`systemd-run --on-active=180 …restore-and-restart…`) that
   the manager cancels only after reconnecting.
2. **Interface verification is vacuous:** `vppctl show interface "$n"` (line 109) exits 0 even for an unknown name —
   vppctl's status reflects only the socket session (`/root/vpp/src/vpp/app/vppctl.c:497-503`). Parse output instead,
   e.g. `vppctl show interface | awk '{print $1}' | grep -qx "$n"`, likewise check `show hardware-interfaces` for the PCI.
3. **Rollback does not undo a driver steal:** if VPP unbound a NIC from `vmxnet3` (vfio/uio), restoring the file and
   restarting VPP does not rebind it to the kernel — ens192 stays gone. Record `readlink /sys/bus/pci/devices/<pci>/driver`
   for the mgmt NIC before, and on failure `driverctl`/`bind` it back and `ip link set ens192 up` / `netplan apply`.
4. `verify` never checks that plugins the new file **disables** are unloaded, nor that plugins enabled by the old file
   are still loaded (see F3); `before=$(… NRestarts …)` (line 99) is computed and never used; the backup `cp -p` (line 96)
   and the `--diff` review (93-94) happen outside the lock, so a concurrent edit between review and `install` is lost
   silently — compare a checksum of `/etc/vpp/startup.conf` taken at review time inside the lock before `install`.
5. `DOC=/root/ngfw/…/running.json` (line 86): there is no source for a running document that carries the stand-in fields
   (the API rejects them, strictObject) — see F3.

### F3 — MEDIUM: stand-in reading silently drops the D-060 plugins and the mgmt blacklist for any schema-valid document
`model.go:178-258`, `docs/agent/renderers/vppstartup.md:86`, questions Q1

Until the additive contract change lands, every document that passed the API (strict schema) lacks `plugins` and
`managementPci`. Rendering it yields no `plugins {}` block → after restart `linux_cp`/`linux_nl`/`npt66` are off
(P12/NPTv6 break) and F2's verify still passes (it checks only plugins the *new* file enables). Typed `DesiredState`
input has the same effect for the dry-run `Renderer`. Fail-closed for devices (mgmt required) is good; plugins are not
fail-closed. Fix (until the contract change): warn loudly / exit non-zero in `--diff` mode when the existing file enables
plugins the rendering does not mention (`--allow-plugin-removal` to override), and let F1's host mgmt fact cover the
blacklist. Ask for the Q1 contract change before any real apply.

### F4 — MEDIUM: CPU placement model does not match VPP; Q5 check can pass while VPP puts workers on housekeeping CPUs
`model.go:392-403, 421-438`

VPP 26.06 (`/root/vpp/src/vlib/threads.c:255-256, 380-400`): an unset `main-core` becomes `sched_getcpu()` (the CPU VPP
happened to start on, not "1"), and `workers N` without a corelist takes the **lowest free CPUs excluding CPU 0 and the
main core**, not `main+1…main+N`. Reproduced:
- `{"mainCore":5,"workers":2}` with `--isolcpus 6-7 --cpus 8` → accepted (generator assumes 6,7); VPP pins workers to 1,2
  (not isolated) — the D-SG-3/Q5 rule is silently violated.
- `{"mainCore":31,"workers":2}` on this 32-CPU host → rejected ("worker core 32 does not exist"); VPP would boot (1,2).
- `{"workers":2}` without `mainCore` → worker placement depends on the CPU VPP starts on (non-deterministic).
Fix: model VPP's algorithm exactly (main = mainCore; workers = first N of online∖{0, main}, CPU 0 last resort), and
require `mainCore` whenever `workers > 0` or the host isolates CPUs (or render `corelist-workers` computed from the
isolated set). Also `h.CPUs = max(online)+1` (`cmd/vrx-startupgen/main.go:224-227`) ignores holes in the online list:
keep the online set and check `mainCore`/corelist membership, since VPP errors on an offline CPU ("cpu %u is not available").

### F5 — MEDIUM: hugepage budget can be bypassed → VPP fails buffer allocation at start
`model.go:557-568`
- `hugepagesGb: 0` (schema says min 1, but the CLI never runs Zod) sets `configured = 0` → "unknown" → check skipped
  even though the host reserves 2 GiB: `{"hugepagesGb":0,"buffersPerNuma":1000000}` → `ok` (4.8 GiB of buffers).
- `hugepagesGb` larger than the host reservation is only a warning, and the budget is checked against the *document*
  number: `{"hugepagesGb":64,"buffersPerNuma":500000}` → `ok` with a warning; 2.4 GiB of buffers vs 2 GiB reserved.
Fix: budget against `min(hugepagesGb, host reservation)` whenever the host value is known (error, not warning, when the
budget exceeds what the host actually reserves); treat `hugepagesGb == 0` as invalid.

### F6 — MEDIUM: generator relies on schema bounds it does not enforce; unknown keys are dropped
`model.go:166` (`DiscardUnknown: true`), `model.go:338-346, 380-386`

The CLI's input never passes the Zod schema, yet the generator assumes its bounds (D-049 input hardening):
`txQueues: 100000` → ok; `rxQueues`/`workers`/`corelist` length unbounded when host CPUs are unknown;
`mainCore: 4294967295` with `--no-host` renders `main-core 4294967295`, which is VPP's `~0` "unset" sentinel.
Typos are silently ignored: `{"maincore":3,"Devices":{…}}` renders an empty `cpu {}` and `no-pci`, exit 0.
Fix: re-check the schema bounds in `BuildModel` (workers ≤ 255, corelist ≤ 256, queues 1..256, hugepagesGb 1..1024,
mainCore < 4096) and decode `dataplane` strictly (reject unknown keys; the schema is a strictObject).

### F7 — LOW: the agent-side `Renderer` validates against "all unknown" host facts by default
`renderer.go:122-134`. `New()` without `WithHost` skips every CPU and hugepage check, so a commit dry-run (Q3) would
report "valid" for configs the CLI rejects. Fix: require host facts in the constructor (or return a warning "not
validated against host") and share the CLI's `hostFacts` reader from the package.

### F8 — LOW: semantic diff comment handling differs from VPP
`semantic.go:69-76`. VPP drops everything from **any** `#` to end of line (`/root/vpp/src/vpp/vnet/main.c:216`);
`stripComment` only at token start. `no-pci#x` (VPP: `no-pci`) is reported as a difference — spurious diffs only (rendered
files never contain a mid-token `#`, so no false "identical" found), but align it with VPP's rule.

### F9 — LOW: `TestLiveHostFileSemantics` binds unit tests to the live host file
`render_test.go:151-161`. After any legitimate manager change of `/etc/vpp/startup.conf` (e.g. the first real apply
with devices), `make test` fails on this host. Gate it behind `VRX_INTEGRATION` or compare against the checked-in
`testdata/host-startup.conf` only.

## Answers to the focus questions
- (a) Yes — F1 (document-controlled mgmt identity). All formatting tricks (case, leading zeros/short forms, whitespace,
  unicode digits, duplicate keys) are correctly rejected or canonicalised.
- (b) No injection found: strict ASCII allowlists + `ident`/`pathtok` + `CheckRendered`; error texts quote hostile keys.
- (c) Unbootable configs reachable: F5 (buffers vs hugepages), F6 (`~0` main-core, unbounded queues); CPU model wrong
  (F4). corelist beyond CPUs and main-core == worker are correctly rejected; main-core is required with corelist.
- (d) Unified diff correct against the live file; semantic mode fine as a review aid, F8.
- (e) Not sound yet — F2.
- (f) Holds: `Apply` always returns `ErrManagerStep` (`renderer.go:171`), tested, and nothing registers the renderer.

**BLOCK**
