# Task: F-startup-gen — VPP startup.conf generator   (prepend 00-CONTEXT.md)

## Goal
Generate `/etc/vpp/startup.conf` from the `dataplane` domain of the config document (WBS D0.6 in `plan/wbs.csv`):
hugepages, main core / workers / isolcpus, RSS queues, NUMA-aware buffers, the DPDK device list with **logical names**
(`dpdk { dev <pci> { name <logical> num-rx-queues … } }`, D-069), the plugin enable/disable list (D-060), socksvr/cli/statseg/log.
The generator is a pure function plus a small CLI; applying it to a live host (write file + restart VPP) is a **manager step**
under `flock -x /run/lock/vrx-lab.lock` and is never done by this task.

## Inputs to read first
- `packages/schema/src/domains/dataplane.ts` (P02a, merged) and `system.ts` — the model you render from; the dataplane proto message
- `/etc/vpp/startup.conf` on this host (read only) — the current hand-written file incl. the D-060 plugins block and the
  `blacklist 0000:0b:00.0` rule for the management NIC; the generator must be able to reproduce its semantics
- `docs/lab/host-vrx-a.md` (NIC inventory: ens192 = mgmt 0000:0b:00.0; data NICs ens161 0000:04:00.0, ens193 0000:0c:00.0,
  ens224 0000:13:00.0, ens225 0000:14:00.0, ens256 0000:1b:00.0, ens257 0000:1c:00.0)
- `apps/agent/internal/renderers/{renderer.go,README.md,ALLOWLIST.md}` and the merged FRR renderer (`renderers/frr`) for escaping style
- VPP 26.06 startup.conf reference: https://s3-docs.fd.io/vpp/26.06/configuration/reference.html

## Scope — build exactly this
1. **Renderer** `apps/agent/internal/renderers/vppstartup/`: Go template + strict escaping; deterministic output (sorted devices/plugins);
   input = the dataplane domain (typed Go struct from the proto; D-055 stand-in allowed for missing fields, listed in questions).
2. **Validation** (return errors, never render): management NIC must never appear in `dpdk { dev }` and is always blacklisted;
   PCI addresses well-formed and unique; logical names unique and valid VPP interface names; workers + main core within host CPU count
   and disjoint from isolcpus rules; hugepage budget ≤ configured hugepages; RSS queues ≤ workers; plugin names from the on-disk list
   (`/usr/lib/x86_64-linux-gnu/vpp_plugins/*.so`), never disabling `dpdk_plugin.so` while devices are listed.
3. **CLI** `apps/agent/cmd/vrx-startupgen`: reads a config document (JSON) → writes the rendered file to stdout or `-o <path>`;
   `--diff <existing>` prints a unified diff and exits 1 when different; `--check` validates only. It never restarts VPP and never
   writes under `/etc` in tests.
4. **Tests**: golden files (single-core lab, 2-worker lab, the six-NIC vrx-a layout with a sample port-group mapping, plugins block);
   hostile inputs (newline/brace injection in logical names, duplicate PCI, mgmt NIC in dev list); a test that renders the equivalent of the
   current host file and diffs semantically (plugins, blacklist, no-pci when no devices).
5. **Docs**: `docs/agent/renderers/vppstartup.md` — the mapping table schema field → startup.conf line, the manager apply procedure
   (backup, render, `--diff`, flock -x, restart, verify plugins/interfaces, automatic rollback — mirror D-060's script).

## Acceptance (paste the evidence)
- [ ] golden + hostile tests green; `--diff` against the live host file shows only expected differences (pasted)
- [ ] the six-NIC example renders `dpdk { dev 0000:04:00.0 { name <logical> } … }` with the mgmt NIC blacklisted (pasted)
- [ ] `tools/ci.sh --base main` green in your worktree

## Out of scope (do not build)
Applying to the host / restarting VPP (manager); NIC driver binding (vfio/uio); the UI/API screens (the dataplane domain is served by
the generic config routes); CPU auto-tuning heuristics beyond validation; the Debian packaging of the tool (P10).

## Open questions to surface, not to decide silently
The NIC → port-group (logical name) mapping is still pending from the product owner — use a clearly marked sample mapping in tests.
