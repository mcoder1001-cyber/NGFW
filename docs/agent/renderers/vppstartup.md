# VPP startup.conf generator — dataplane domain ↔ startup.conf (F-startup-gen, WBS D0.6)

Code: `apps/agent/internal/renderers/vppstartup` (pure generator, host-fact reader, `renderers.Renderer` for dry runs),
and the CLI `apps/agent/cmd/vrx-startupgen`. File:
`/etc/vpp/startup.conf`, mode 0644. VPP reads it only at start, so every change needs a **VPP restart**, which is a
**manager step** (`deploy/vpp/apply-startup.sh`, procedure below; D-088). The agent never applies it: `Renderer.Apply` returns `ErrManagerStep`, `Retrieve`
returns `ErrRetrieveUnsupported` (VPP has no API that reports its start-up config; compare with `vrx-startupgen --diff`).

## Pipeline

```
config document (JSON, strict)  or  DesiredState / DataplaneConfig (proto)
        ──Desired()──▶ DataplaneConfig  ──BuildModel(Host)──▶ Model (validated, sorted)  ──template──▶ startup.conf
Host = ReadHost(/sys, /proc, plugin dir, current startup.conf)  — required, never defaulted (ErrHost)
```

`Generate()` is pure (no I/O): the same document and host facts give byte-identical output (golden tests). The
**host facts** are part of the input and all of them are required (`Host.Check`):

| fact | source (`ReadHost`) | used for |
|---|---|---|
| management NIC(s) | every interface with an IPv4/IPv6 default route (`/proc/net/route`, `/proc/net/ipv6_route`) **plus** the interface the route lookup picks for the peer of every established control connection (`/proc/net/tcp{,6}`, local port in `--control-ports`, default 22 = sshd — so a management subnet that is only directly connected is covered), plus `--mgmt-if` / `--mgmt-pci`; each resolved via `/sys/class/net/<if>/device`, bonds/VLANs via their `lower_*` members. A tun/tap without a device (a **linux-cp tap, VPP-owned**) and unreachable/blackhole defaults (`*`) are skipped with a note, never an error; an interface that cannot be resolved is an error unless `--mgmt-pci` names the NIC(s). The CLI prints every NIC found and why | always `blacklist`ed; never a DPDK device, **whatever the document says** |
| online / isolated CPUs | `/sys/devices/system/cpu/{online,isolated}` | core placement |
| NUMA nodes | `/sys/devices/system/node/node*` | buffer budget |
| hugepage reservation | `/proc/meminfo` HugePages_Total × Hugepagesize (0 ⇒ refused) | buffer budget |
| on-disk plugins | `/usr/lib/x86_64-linux-gnu/vpp_plugins/*.so` | plugin names |
| current plugin switches | `plugins { }` of the current `/etc/vpp/startup.conf` (`--current`, `none` = no file) | kept when the document has no `dataplane.plugins`; otherwise only compared (warnings for removed switches) |

## File layout (section order)

| section | content | source |
|---|---|---|
| header comments | "generated, do not edit"; `# hugepages: N GiB …` when `hugepagesGb` is set | — |
| `unix { }` | `nodaemon`, `log /var/log/vpp/vpp.log`, `full-coredump`, `cli-listen /run/vpp/cli.sock`, `gid vpp` | product constants (`Settings`) |
| `api-trace { on }`, `api-segment { gid vpp }`, `socksvr { default }` | fixed | product constants |
| `statseg { socket-name /run/vpp/stats.sock }` | explicit VPP default | product constants |
| `cpu { }` | **always** `main-core N`; `corelist-workers …` whenever there are workers (never VPP's own placement) | `dataplane` + host CPUs |
| `buffers { }` | `buffers-per-numa` (only when set) | `dataplane.buffersPerNuma` |
| `dpdk { }` | `dev default { … }`, `dev <pci> { name … }`, `blacklist <host mgmt pci>` (always), `no-pci` when there are no devices; **omitted** when `dpdk_plugin.so` is disabled (VPP rejects the section of an unloaded plugin) | `dataplane` + host |
| `plugins { }` | `plugin <file> { enable|disable }`, sorted: exactly `dataplane.plugins.switches` when `plugins` is present; the current file's switches when it is absent (D-084) | `dataplane.plugins` or current file |

## Mapping

| desired state (JSON path) | rendered | validation (error ⇒ nothing is rendered) |
|---|---|---|
| `dataplane` (whole object) | — | decoded strictly: unknown keys (typos such as `maincore`) are an error |
| `dataplane.mainCore` | `cpu { main-core N }` | 0–1023, an **online** CPU, not a worker, not isolated when workers exist and the host isolates CPUs. **Unset:** the lowest online, non-isolated CPU ≠ 0 (CPU 0 as last resort) — rendered explicitly, with a warning |
| `dataplane.corelist` | `cpu { corelist-workers 2-3,6 }` (sorted, ranges) | ≤ 256 entries, each 0–1023, online, unique; `workers` (if set) = its length; ⊆ isolcpus when the host isolates CPUs |
| `dataplane.workers` (no corelist) | turned into `corelist-workers`: the N lowest online CPUs except the main core (CPU 0 last; only isolated CPUs when isolcpus is set) — VPP's own rule, made explicit; a warning shows the choice | 0–255; enough CPUs available |
| `dataplane.rxQueues` | `dpdk { dev default { num-rx-queues N } }` | 1–256; **RSS: ≤ max(workers, 1)** |
| `dataplane.txQueues` | `dpdk { dev default { num-tx-queues N } }` | 1–256 |
| `dataplane.hugepagesGb` | comment only — hugepages are reserved by `vm.nr_hugepages` (`/etc/sysctl.d/80-vpp.conf`) | 1–1024 (0 is an error, not "unknown"); warning when above the host reservation |
| `dataplane.buffersPerNuma` | `buffers { buffers-per-numa N }` | 1024–4194304; budget below |
| `dataplane.pciWhitelist[i]` | `dpdk { dev <pci> }` (merged with `devices`) | ≤ 64; canonical `dddd:bb:dd.f` (lower-cased), device ≤ 1f, function ≤ 7; unique after canonicalisation; never a host management NIC |
| `dataplane.managementPci[i]` | nothing extra — the blacklist comes from the **host** | ≤ 4, PCI as above, unique; when set it must **equal** the host's management NIC set (mismatch = error, e.g. a swapped entry) |
| `dataplane.devices.<pci>` | `dpdk { dev <pci> { … } }`, sorted by PCI | ≤ 64; key: PCI as above, unique after canonicalisation (`0000:0C:00.0` = `0000:0c:00.0`); never a host management NIC |
| `dataplane.devices.<pci>.name` | `name <logical>` — the logical interface name (D-069: equals the `interface/<name>` key) | `[a-z][a-z0-9_-]{0,14}` (15 = Linux IFNAMSIZ−1), not ending in `-`/`_`; not `local0`/`default`/`none`/`any`/`all`; not a VPP-created stem followed by digit/`_`/`-` (`loop0`, `gre1`, `host-x`, `tap0`, `vxlan_tunnel0`, …); unique. A device without a name gets a warning |
| `dataplane.devices.<pci>.rxQueues` / `.txQueues` | `num-rx-queues` / `num-tx-queues` | 1–256; rxQueues ≤ max(workers, 1) |
| `dataplane.devices.<pci>.rxDesc` / `.txDesc` | `num-rx-desc` / `num-tx-desc` | power of two, 64–16384 |
| no device at all | `dpdk { no-pci }` + the host blacklist (the current vrx-a semantics) | — |
| `dataplane.plugins` **present** | authoritative: `plugin <file> { enable }` (`true`) / `{ disable }` (`false`) for exactly `switches.<file>`; switches of the current file that are not listed disappear (warning each); `{}` / `{switches:{}}` = no plugins block | names `[a-z0-9][a-z0-9_-]*_plugin.so`, **must exist in the plugin directory**; ≤ 128; `dpdk_plugin.so: false` is rejected while devices are listed; warning for each D-060 plugin (`linux_cp`, `linux_nl`, `npt66`) not listed |
| `dataplane.plugins` **absent** | the current file's switches are kept (a warning names each) — a document without `plugins` never drops the D-060 block | the kept names must exist on disk too |

**Hugepage budget:** `buffersPerNuma (default 16384) × NUMA nodes × 2560 B` must fit in `min(hugepagesGb, host reservation)`
(always checked — the host value is required). 2560 B is a conservative per-buffer figure (2048 B data + metadata +
headroom + mempool overhead). Default on vrx-a: 16384 × 2 × 2560 B = 80 MiB of 2 GiB.

**Escaping:** every string passes a validator above *and* a template helper (`ident` for PCI/names/plugins, `pathtok`
for paths); numbers are bounded (VPP's `~0` sentinel 4294967295 is rejected). `renderers.Execute` runs `CheckRendered` as a
backstop. Error messages quote hostile keys, so an error text is always one line.

Contract: the four fields `managementPci`, `devices`, `buffersPerNuma`, `plugins` are in the schema and the proto
(`contract/F-startup-gen`, D-081; `docs/status/tasks/F-startup-gen-contract.md`). `plugins` is the wrapper
`{ switches: {<file>: bool} }` / `optional PluginSet` so its presence survives the proto (D-084).

## CLI

```
vrx-startupgen [flags] [document.json|-]
  -o <path>            write atomically (temp file + rename, 0644) instead of stdout
  --diff <existing>    unified diff existing → rendering; exit 1 when different
  --semantic           with --diff: compare sections/entries, ignoring comments ('#' anywhere, as VPP), order, indentation
  --check              validate only (host NIC + warnings on stderr)
  --control-ports <p>  local TCP ports of control connections whose peers mark their NIC as management (default 22)
  stderr always: management NICs found (and why), warnings, "rendered sha256 <hex>" of exactly this rendering
  --current <file>     current start-up file whose plugin switches are kept (default /etc/vpp/startup.conf; "none")
  --mgmt-if <ifs>      extra management interfaces   --mgmt-pci <pcis>  extra management NICs
  --online-cpus L  --isolcpus L  --numa-nodes N  --hugepages-mb N  --plugin-dir D   host fact overrides
  --no-host            read nothing from /sys and /proc: every fact from flags (--mgmt-pci mandatory)
exit: 0 ok / identical · 1 different · 2 invalid input, missing host facts or error
```

The CLI never restarts VPP, never talks to VPP and runs no other process. Build: `cd apps/agent && go build -o bin/vrx-startupgen ./cmd/vrx-startupgen`.

## Manager apply procedure — `deploy/vpp/apply-startup.sh` (F-startup-apply, manager-only)

Only the manager applies a start-up file, and only when a VPP restart is allowed: the script refuses `--apply` unless
`docs/lab/host-<vm>.md` says `handover: done` in the canonical `/root/ngfw` copy **and** the planner's tree (same rule as
`tools/lab`, D-012), or the product owner approved this change: `--i-have-product-owner-approval PENDING-<slug>`, which
must name `docs/decisions/PENDING-<slug>.md` **on main** and a `LOG.md` D-row on main that references it (both are
resolved and recorded: file blob + D ids, in the run log, `<work>/gate` and syslog tag `vrx-startup-apply`). Workers and
tests never touch `/etc/vpp/startup.conf`; the script is tested only against a fake host (`deploy/vpp/test-apply-startup.sh`).

1. Build both tools from a known tree: `cd apps/agent && go build -o bin/vrx-startupgen ./cmd/vrx-startupgen &&
   go build -o bin/vrx-vppcheck ./cmd/vrx-vppcheck` (or point `VRX_STARTUPGEN` / `VRX_VPPCHECK` at absolute paths).
2. Dry run (changes nothing): `deploy/vpp/apply-startup.sh --doc running.json [-- <generator flags>]`. Review the diffs,
   the drivers of every PCI device involved, the **protected management interfaces** (every default route v4/v6, the path
   to the SSH peer — shown —, `--mgmt-if`) with their network manager and exact `ip … replace` restore plan, the
   **management reachability check** that will be used, the VPP preflight + boot identity and the gate. Exit 3 = `--apply`
   would refuse (e.g. no viable reachability check on this host).
3. Reachability check (`--mgmt-probe`, review H2): `auto` = `ssh-peer` (the manager's TCP session from `$SSH_CONNECTION` /
   `--mgmt-peer` is ESTABLISHED in `ss`) when available, else `gateway-ping` only when the gateway answers ICMP **now**,
   else refuse; or explicitly `tcp:HOST:PORT` (a TCP connect through the management path). On vrx-a the gateway drops
   ICMP, so apply from an SSH session (and keep it open for the window) or pass `--mgmt-probe tcp:…`. In every mode the
   management interface must also keep its addresses and routes exactly as snapshotted.
4. Apply with both sums the dry run printed:
   `apply-startup.sh --doc running.json --apply --expect-sha256 <live> --expect-new-sha256 <rendered> [-- <same flags>]`.
   The planner copies the document, both binaries and the script into `/var/lib/vrx/startup-apply/<stamp>/`, records every
   setting in `<work>/settings`, seals the plan (`plan.sha256` over settings, document, binaries and the gate record) and
   starts the run detached (`systemd-run`, settings also as `--setenv`; fallback `setsid`); the caller returns at once —
   follow `journalctl -fu vrx-startup-apply-<stamp>` or `<work>/log`. `--foreground` is refused over SSH (`--console`).
5. The detached run: verifies the seal and re-evaluates the gate (must equal the sealed record) → a **lock holder** (own
   unit/session) takes `vrx-vpp.lock` then `vrx-lab.lock` and keeps them until commit or the end of the rollback →
   both sha256 re-checked → VPP preflight → reachability baseline → snapshot (addresses/routes, drivers, plugins, D-080
   boot identity) → backup → dead-man timer → install → `systemctl restart vpp` → a **new** boot identity whose PID is
   vpp.service's MainPID → watch `--window` s: unit active, MainPID + ActiveEnterTimestampMonotonic and the boot identity
   unchanged (no crash since the restart; `NRestarts` is not used — systemd resets it on a manual restart), plugins,
   logical names, management state + reachability.
6. Any failure → rollback (one attempt inside the run): stop VPP (SIGKILL if the stop hangs), restore the backup, rebind
   changed NICs, re-apply the recorded addresses/routes verbatim, then only if still broken `ifup --force` /
   `networkctl reconfigure` / `netplan apply`, `reset-failed` + start VPP, verify the same way. Healthy → locks released.
   Incomplete → the locks stay held and the dead-man retries.
7. **Timeouts and the dead-man:** every VPP/kernel/systemd call runs under `timeout` (`--cmd-timeout` 10 s,
   `--svc-timeout` 120 s); the dead-man fires after the run budget plus a worst-case rollback (both printed in the log).
   It kills the run's process tree (never the lock holder), rolls back with `--rollback-retries` (3) attempts and
   exponential backoff from `--retry-backoff` (10 s), then releases the locks. If the holder is gone it takes the locks
   itself, exclusively and bounded, before touching the run; only a foreign holder makes it roll back without them
   (FORCED, holder from `lslocks` logged).
8. Record the result in `docs/decisions/LOG.md` (what changed, backup name) and update `docs/lab/host-<vm>.md`.

Exit codes: 0 committed / nothing to do · 1 failed and rolled back · 2 usage · 3 refused before any change.
Needs `jq`, `flock`, `timeout`, `ss`, `git` (approval check) on the target host. Not restored automatically: multipath
routes, policy-routing tables, address lifetimes. Plugins the old file enabled and the new document no longer mentions
(D-084 omission) may disappear after the restart without failing the apply.

A `dataplane.plugins` that leaves out a plugin the current file enables (D-084 "present = authoritative") is accepted by
the generator with a warning; if the plugin is `default_disabled` in VPP (linux_cp, linux_nl, npt66) it will not load after
the restart — expected, not a failure, when the omission was intended.

After a successful apply, the agent's reconcile (P05) recreates everything else. `show hardware-interfaces` then lists
the data NICs under their logical names; the `interface/<name>` alias keys (D-065/D-069) resolve without further mapping.

## Known limits

- No offline checker exists for startup.conf; `Validate` is structural (one file, balanced braces, no control characters).
- The semantic diff compares entries line by line in the layout VPP ships and we render; a review aid, not VPP's parser.
- NIC → port-group mapping on vrx-a is still unknown: the six-NIC golden (`testdata/six-nic-sample.golden`) uses a
  **SAMPLE** mapping (`04:00.0 wan`, `0c:00.0 lan`, `13:00.0 dmz`, `14:00.0 p2p`, `1b:00.0 lan2`, `1c:00.0 sync`).
- A management path through a bond/VLAN is resolved through its `lower_*` members; anything else without a PCI device
  (and not a linux-cp tap) needs `--mgmt-pci`.
- Control connections are recognised by local port (`--control-ports`, default 22). A manager connected through a
  jump host is protected only via the NIC its jump-host session arrives on.
- `tools/lab provision` still renders remote startup.conf files with its own shell template (tech-debt, D-081).
- NIC driver binding for DPDK (vfio-pci / `driverctl set-override`) is outside the file and the generator.
