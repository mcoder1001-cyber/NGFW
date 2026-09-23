# VPP startup.conf generator — dataplane domain ↔ startup.conf (F-startup-gen, WBS D0.6)

Code: `apps/agent/internal/renderers/vppstartup` (pure generator + `renderers.Renderer` for dry runs) and the CLI
`apps/agent/cmd/vrx-startupgen`. File: `/etc/vpp/startup.conf`, mode 0644. VPP reads it only at start, so every change
needs a **VPP restart**, which is a **manager step** (procedure below). The agent never applies it: `Renderer.Apply`
returns `ErrManagerStep`, `Retrieve` returns `ErrRetrieveUnsupported` (VPP has no API that reports its start-up config;
compare files with `vrx-startupgen --diff`).

## Pipeline

```
config document (JSON) ──Desired()──▶ DataplaneConfig (proto) + Extensions (D-055 stand-ins)
        ──BuildModel(host facts)──▶ Model (validated, sorted)  ──template + ident/pathtok──▶ startup.conf
```

`Generate()` is pure (no I/O, no clock): the same document and host facts give byte-identical output (golden tests).
Host facts (`Host`: CPU count, isolcpus, NUMA nodes, hugepage reservation, on-disk plugin list) are an input; the CLI
reads them from `/sys`, `/proc/meminfo` and the plugin directory, or takes them from flags (`--no-host`).

## File layout (section order)

| section | content | source |
|---|---|---|
| header comments | "generated, do not edit"; `# hugepages: N GiB …` when `hugepagesGb` is set | — |
| `unix { }` | `nodaemon`, `log /var/log/vpp/vpp.log`, `full-coredump`, `cli-listen /run/vpp/cli.sock`, `gid vpp` | product constants (`Settings`) |
| `api-trace { on }`, `api-segment { gid vpp }`, `socksvr { default }` | fixed | product constants |
| `statseg { socket-name /run/vpp/stats.sock }` | explicit VPP default | product constants |
| `cpu { }` | `main-core`, `corelist-workers` or `workers` | `dataplane` |
| `buffers { }` | `buffers-per-numa` (only when set) | `dataplane` (stand-in) |
| `dpdk { }` | `dev default { … }`, `dev <pci> { name … }`, `blacklist <mgmt pci>`, `no-pci` when there are no devices; **omitted** when `dpdk_plugin.so` is disabled (VPP rejects the section of an unloaded plugin) | `dataplane` |
| `plugins { }` | `plugin <file> { enable|disable }`, sorted (only when set) | `dataplane` (stand-in) |

## Mapping

| desired state (JSON path) | rendered | validation (error ⇒ nothing is rendered) |
|---|---|---|
| `dataplane.mainCore` | `cpu { main-core N }` | < host CPUs; not a worker core; not an isolated CPU when isolcpus is set and workers exist; **required with `corelist`** (VPP: "main-core must be specified when using corelist-*") |
| `dataplane.corelist` | `cpu { corelist-workers 2-3,6 }` (sorted, ranges compressed) | unique; each < host CPUs; `workers` (if set) = its length; ⊆ isolcpus when the host isolates CPUs |
| `dataplane.workers` (no corelist) | `cpu { workers N }` (omitted when 0) | main core + N ≤ host CPUs; the auto-pinned cores (main+1 … main+N, VPP's rule) exist and are isolated when isolcpus is set |
| `dataplane.rxQueues` | `dpdk { dev default { num-rx-queues N } }` | **RSS: ≤ max(workers, 1)** |
| `dataplane.txQueues` | `dpdk { dev default { num-tx-queues N } }` | 1–256 (schema) |
| `dataplane.hugepagesGb` | comment only (`# hugepages: N GiB …`) — hugepages are reserved by `vm.nr_hugepages` (`/etc/sysctl.d/80-vpp.conf`), not by startup.conf | budget below; warning when larger than the host's current reservation |
| `dataplane.pciWhitelist[i]` | `dpdk { dev <pci> }` (merged with `devices`) | canonical `dddd:bb:dd.f` (lower-cased), device ≤ 1f, function ≤ 7; unique after canonicalisation; never a management NIC |
| `dataplane.managementPci[i]` *(stand-in)* | `dpdk { blacklist <pci> }` — always, also with `no-pci` | PCI as above, unique; **required when any device is listed**; may never appear in `pciWhitelist`/`devices` |
| `dataplane.devices.<pci>` *(stand-in)* | `dpdk { dev <pci> { … } }`, sorted by PCI | key: PCI as above, unique after canonicalisation (`0000:0C:00.0` = `0000:0c:00.0`); unknown fields rejected |
| `dataplane.devices.<pci>.name` *(stand-in)* | `name <logical>` — the logical interface name (D-069: equals the `interface/<name>` key) | `[a-z][a-z0-9_-]{0,14}` (15 = Linux IFNAMSIZ−1 so linux-cp can mirror it), not ending in `-`/`_`; not `local0`/`default`/`none`/`any`/`all`; not a VPP-created stem followed by digit/`_`/`-` (`loop0`, `gre1`, `host-x`, `tap0`, `vxlan_tunnel0`, …); unique across devices. A device without a name gets a warning (VPP names it after the PCI slot) |
| `dataplane.devices.<pci>.rxQueues` / `.txQueues` *(stand-in)* | `num-rx-queues` / `num-tx-queues` | ≤ 256; rxQueues ≤ max(workers, 1) |
| `dataplane.devices.<pci>.rxDesc` / `.txDesc` *(stand-in)* | `num-rx-desc` / `num-tx-desc` | power of two, 64–16384 |
| no device at all | `dpdk { no-pci }` (DPDK probes nothing — the current vrx-a semantics) | — |
| `dataplane.buffersPerNuma` *(stand-in)* | `buffers { buffers-per-numa N }` | hugepage budget |
| `dataplane.plugins.<file>` *(stand-in)* | `plugins { plugin <file> { enable } }` (`true`) / `{ disable }` (`false`) | `[a-z0-9][a-z0-9_-]*_plugin.so`, **must exist in the plugin directory** (`/usr/lib/x86_64-linux-gnu/vpp_plugins/*.so`); `dpdk_plugin.so: false` is rejected while devices are listed |

**Hugepage budget:** `buffersPerNuma (default 16384) × NUMA nodes × 2560 B` must fit in `hugepagesGb` (or, when unset,
the host's `HugePages_Total × Hugepagesize`). 2560 B is a conservative per-buffer figure (2048 B data + metadata +
headroom + mempool overhead). Default on vrx-a: 16384 × 2 × 2560 B = 80 MiB of 2 GiB.

**Escaping:** every string passes a validator above *and* a template helper (`ident` for PCI/names/plugins, `pathtok`
for paths); numbers come from typed fields. `renderers.Execute` runs `CheckRendered` as a backstop. Error messages quote
hostile record keys, so an error text is always one line.

*Stand-ins (D-055):* `managementPci`, `devices`, `buffersPerNuma` and `plugins` are not in the Zod schema / proto yet;
they are read from the JSON document (`*structpb.Struct`) at the paths above. Adding them is a contract change
(`docs/status/tasks/F-startup-gen-questions.md` Q1). Typed `DesiredState` input renders only the proto fields.

## CLI

```
vrx-startupgen [flags] [document.json|-]
  -o <path>           write atomically (temp file + rename, 0644) instead of stdout
  --diff <existing>   unified diff existing → rendering; exit 1 when different
  --semantic          with --diff: compare sections/entries, ignoring comments, order and indentation
  --check             validate only (warnings on stderr)
  --plugin-dir <dir>  --cpus N  --isolcpus LIST  --numa-nodes N  --hugepages-mb N  --no-host
exit: 0 ok / identical · 1 different · 2 invalid input or error
```

The CLI never restarts VPP, never talks to VPP and runs no other process. Build: `cd apps/agent && go build -o bin/vrx-startupgen ./cmd/vrx-startupgen`.

## Manager apply procedure (vrx-a; after handover, or on an explicit PENDING decision — D-012, D-060)

Same shape as the D-060 plugin change (backup kept, one restart under the exclusive lab lock, verify, roll back on failure).
Only the manager runs this; workers and tests never write `/etc/vpp/startup.conf`.

```bash
set -euo pipefail
DOC=/root/ngfw/…/running.json            # the running configuration document (or `pnpm`/API export of it)
NEW=/run/vrx-startupgen/startup.conf; STAMP=$(date +%Y%m%d-%H%M%S)
install -d -m 0750 /run/vrx-startupgen
cd /root/ngfw/apps/agent && go build -o bin/vrx-startupgen ./cmd/vrx-startupgen
# 1. render + validate against this host (plugins, CPUs, isolcpus, hugepages)
bin/vrx-startupgen -o "$NEW" "$DOC"
# 2. review the change — both views; stop here if anything is unexpected
bin/vrx-startupgen --diff /etc/vpp/startup.conf "$DOC" || true
bin/vrx-startupgen --diff /etc/vpp/startup.conf --semantic "$DOC" || true
# 3. backup, install, restart under the exclusive locks (waits for running integration tests), verify, auto-rollback
cp -p /etc/vpp/startup.conf "/etc/vpp/startup.conf.bak-$STAMP"
flock -x /run/lock/vrx-vpp.lock flock -x /run/lock/vrx-lab.lock bash -c '
  new="$1" bak="$2"
  before=$(systemctl show vpp -p NRestarts --value)
  install -m 0644 "$new" /etc/vpp/startup.conf
  verify() {
    for i in $(seq 1 30); do [[ -S /run/vpp/api.sock ]] && vppctl show version >/dev/null 2>&1 && break; sleep 1; done
    vppctl show version >/dev/null 2>&1 || return 1
    # every plugin the document enables is loaded
    for p in $(grep -oP "plugin \K\S+(?= \{ enable \})" /etc/vpp/startup.conf); do
      vppctl show plugins | grep -q "$p" || { echo "plugin $p not loaded"; return 1; }; done
    # every DPDK device came up under its logical name
    for n in $(grep -oP "^\s+name \K\S+" /etc/vpp/startup.conf); do
      vppctl show interface "$n" >/dev/null 2>&1 || { echo "interface $n missing"; return 1; }; done
    # the management NIC is still the kernel'"'"'s (host reachable)
    ip -br addr show ens192 | grep -q UP || { echo "mgmt NIC down"; return 1; }
  }
  if systemctl restart vpp && verify; then echo "startup.conf applied"; exit 0; fi
  echo "verification failed — rolling back" >&2
  install -m 0644 "$bak" /etc/vpp/startup.conf
  systemctl restart vpp; verify && echo "rolled back to $bak" >&2; exit 1
' _ "$NEW" "/etc/vpp/startup.conf.bak-$STAMP"
# 4. record in docs/decisions/LOG.md (what changed, backup name, NRestarts before/after) and update docs/lab/host-vrx-a.md
```

After a successful apply, the agent's reconcile (P05) recreates everything else — restart-safety is already required of
every descriptor. `vppctl show hardware-interfaces` then lists the data NICs under their logical names; the `interface/<name>`
alias keys (D-065/D-069) resolve without any further mapping.

## Known limits

- No offline checker exists for startup.conf; `Validate` is structural (one file, balanced braces, no control characters).
- The semantic diff compares entries line by line in the layout VPP ships and we render; it is a review aid, not VPP's parser.
- NIC → port-group mapping on vrx-a is still unknown: the six-NIC golden (`testdata/six-nic-sample.golden`) uses a
  **SAMPLE** mapping (`04:00.0 wan`, `0c:00.0 lan`, `13:00.0 dmz`, `14:00.0 p2p`, `1b:00.0 lan2`, `1c:00.0 sync`).
- `tools/lab provision` still renders remote startup.conf files with its own shell template (not this generator).
- NIC driver binding (vfio-pci / `driverctl`) is outside the file and outside this tool.
