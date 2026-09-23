# F-startup-gen — VPP startup.conf generator (WBS D0.6)

> Fix round after review BLOCK (e049b31): see **Review fixes** at the end — it supersedes the evidence and decisions
> D-SG-2/D-SG-5 of the first round (management NIC now a host fact, plugins overlay, explicit pinning, strict bounds).

branch `task/F-startup-gen` · worktree `/root/ngfw-wt/F-startup-gen` · base main@ff6b91a · worker ran directly on the host
(slot 12). Nothing written under `/etc`, VPP not restarted or touched (no API calls at all); `/etc/vpp/startup.conf` and
the plugin directory only read.

## What was built

| path | what |
|---|---|
| `apps/agent/internal/renderers/vppstartup/` | pure generator: `Desired` (DesiredState / DataplaneConfig / JSON document) → `BuildModel` (all validation against `Host` facts) → `templates/startup.conf.tmpl` (strings only via `ident` / `pathtok`, `renderers.Execute` backstop). `Renderer` implements `renderers.Renderer` for dry runs (Render + structural Validate); `Apply` → `ErrManagerStep`, `Retrieve` → `ErrRetrieveUnsupported`. `semantic.go` (section/entry parser + `SemanticDiff`), `udiff.go` (unified diff, hunks byte-identical to GNU `diff -u` on the live file) |
| `apps/agent/cmd/vrx-startupgen/` | CLI: document (file or stdin) → stdout or `-o` (atomic, 0644); `--diff <existing>` (exit 1 when different), `--semantic`, `--check`; host facts from `/sys`, `/proc/meminfo`, plugin dir, or flags (`--no-host --cpus --isolcpus --numa-nodes --hugepages-mb --plugin-dir`). Runs no process, never talks to VPP |
| `docs/agent/renderers/vppstartup.md` | mapping table (schema field → startup.conf line → validation), CLI, **manager apply procedure** (backup, render, `--diff` + `--semantic`, `flock -x` vrx-vpp + vrx-lab locks, restart, verify plugins / logical interfaces / mgmt NIC, automatic rollback — same shape as D-060; `bash -n` clean) |
| `apps/agent/internal/renderers/ALLOWLIST.md` | 2-line note: vppstartup runs no binary |

Validation (error ⇒ nothing rendered, every error wraps `ErrInput`, names the JSON path, is one line): mgmt NIC never a
`dev` and always `blacklist`ed (and required once devices exist); PCI canonical + unique (case-insensitive, across
`pciWhitelist` and `devices`); logical names `[a-z][a-z0-9_-]{0,14}`, not VPP-reserved / VPP-created stems, unique;
main core + workers (corelist or VPP's auto-pinning) within host CPUs, disjoint, main-core required with corelist (VPP rule),
workers ⊆ isolcpus and main ∉ isolcpus when the host isolates CPUs; buffer hugepage budget ≤ hugepagesGb (or host
reservation); RSS rx queues ≤ max(workers,1) (global and per device); descriptor counts power of two; plugin names from the
on-disk directory only; `dpdk_plugin.so` cannot be disabled while devices are listed (when disabled with no devices the
`dpdk {}` section is omitted — VPP rejects sections of unloaded plugins).

## Evidence (real output, 2026-09-24, on vrx-a)

### Tests (golden + hostile + host-equivalence)
```
$ cd apps/agent && go test -count=1 -v ./internal/renderers/vppstartup/ ./cmd/vrx-startupgen/ | grep -E '^(--- |ok|FAIL|PASS)'
--- PASS: TestParseCanonical
--- PASS: TestSemanticDiffIgnoresLayout
--- PASS: TestUnifiedDiff
--- PASS: TestHostile
--- PASS: TestTemplateBackstop
--- PASS: TestLogicalNameAccepts
--- PASS: TestGolden
--- PASS: TestSixNICSample
--- PASS: TestHostEquivalentSemantics
--- PASS: TestLiveHostFileSemantics
--- PASS: TestRendererInterface
--- PASS: TestTypedInputMatchesDocument
--- PASS: TestPluginListMatchesHost
--- PASS: TestWarnings
--- PASS: TestCPUList
PASS
ok  	ngfw/agent/internal/renderers/vppstartup	0.153s
--- PASS: TestRenderToStdoutMatchesGolden
--- PASS: TestStdinAndOutputFile
--- PASS: TestDiffAgainstHostFile
--- PASS: TestCheckAndErrors
--- PASS: TestHostFactsFromSysRoot
PASS
ok  	ngfw/agent/cmd/vrx-startupgen	0.164s

$ go test -count=1 -v -run TestGolden ./internal/renderers/vppstartup/ | grep -E '^    --- '
    --- PASS: TestGolden/dpdk-disabled
    --- PASS: TestGolden/empty
    --- PASS: TestGolden/host-equivalent
    --- PASS: TestGolden/plugins
    --- PASS: TestGolden/single-core
    --- PASS: TestGolden/six-nic-sample
    --- PASS: TestGolden/two-worker-auto
    --- PASS: TestGolden/two-worker
    --- PASS: TestGolden/whitelist-only
$ go test -count=1 -v -run TestHostile ./internal/renderers/vppstartup/ | grep -c '    --- PASS'
111
```
TestHostile covers: the framework hostile table (`"; rm -rf /`, LF, CRLF, NUL, ESC, U+2028; invalid UTF-8 via
TestTemplateBackstop) plus brace/newline injection (`lan\n}\nunix { exec /tmp/x }`, `lan }`, `lan{`, `lan # comment`) in
logical names, PCI keys, whitelist, managementPci and plugin names; duplicate PCI (exact, by case, whitelist and devices);
mgmt NIC in whitelist / devices / by case; devices without managementPci; reserved names/stems; plugin not on disk /
path traversal / dpdk disabled with devices; CPU range, isolcpus, corelist rules; RSS; hugepage budget.
TestTemplateBackstop feeds hostile strings straight into the model (bypassing validation): the template helpers still refuse.

### `--diff` against the live `/etc/vpp/startup.conf` (host-equivalent document)
Input `testdata/cases/host-equivalent.json`:
`{"dataplane":{"managementPci":["0000:0b:00.0"],"plugins":{"linux_cp_plugin.so":true,"linux_nl_plugin.so":true,"npt66_plugin.so":true}}}`
```
$ vrx-startupgen --diff /etc/vpp/startup.conf --semantic testdata/cases/host-equivalent.json
+ statseg > socket-name /run/vpp/stats.sock
+ statseg {}
exit=1

$ vrx-startupgen --diff /etc/vpp/startup.conf testdata/cases/host-equivalent.json > ud.txt; echo exit=$?
exit=1
$ head -30 ud.txt
--- /etc/vpp/startup.conf
+++ rendered
@@ -1,3 +1,6 @@
+# Generated by vrx-startupgen from the `dataplane` domain of the VRX configuration.
+# Do not edit by hand: changes need a VPP restart, applied by the manager procedure in
+# docs/agent/renderers/vppstartup.md.
 
 unix {
   nodaemon
@@ -5,32 +8,10 @@
   full-coredump
   cli-listen /run/vpp/cli.sock
   gid vpp
-
-  ## run vpp in the interactive mode
-  # interactive
-
-  ## do not use colors in terminal output
-  # nocolor
-
-  ## do not display banner
-  # nobanner
 }
 
 api-trace {
-## This stanza controls binary API tracing. Unless there is a very strong reason,
-## please leave this feature enabled.
   on
-## Additional parameters:
-##
...
$ grep -E '^[+-]' ud.txt | grep -vE '^(\+\+\+|---)' | grep -vE '^-\s*#|^-\s*$|^\+\s*#|^\+\s*$'     # non-comment, non-blank changes
+statseg {
+  socket-name /run/vpp/stats.sock
+}
$ diff -u /etc/vpp/startup.conf rendered.conf | tail -n +3 | cmp - <(tail -n +3 ud.txt) && echo "hunks identical to GNU diff -u"
hunks identical to GNU diff -u
```
Expected differences only: the upstream sample comments disappear, a generated-file header appears, and `statseg`
names its **default** socket explicitly (semantic no-op). Plugins block, `blacklist 0000:0b:00.0`, `no-pci`, unix/api/
socksvr/cpu sections are semantically identical (asserted by TestHostEquivalentSemantics on the committed copy and
TestLiveHostFileSemantics on the live file).

### Six-NIC vrx-a render (SAMPLE port-group mapping — real mapping pending, Q2)
```
$ vrx-startupgen testdata/cases/six-nic-sample.json        # live host facts: 32 CPUs, 2 NUMA, 2 GiB hugepages, 94 plugins
# Generated by vrx-startupgen from the `dataplane` domain of the VRX configuration.
# Do not edit by hand: changes need a VPP restart, applied by the manager procedure in
# docs/agent/renderers/vppstartup.md.
# hugepages: 2 GiB of 2 MB pages, reserved by the host (vm.nr_hugepages), not by this file.

unix {
  nodaemon
  log /var/log/vpp/vpp.log
  full-coredump
  cli-listen /run/vpp/cli.sock
  gid vpp
}

api-trace {
  on
}

api-segment {
  gid vpp
}

socksvr {
  default
}

statseg {
  socket-name /run/vpp/stats.sock
}

cpu {
  main-core 1
  corelist-workers 2-3
}

dpdk {
  dev default {
    num-rx-queues 2
  }
  dev 0000:04:00.0 {
    name wan
  }
  dev 0000:0c:00.0 {
    name lan
  }
  dev 0000:13:00.0 {
    name dmz
  }
  dev 0000:14:00.0 {
    name p2p
  }
  dev 0000:1b:00.0 {
    name lan2
    num-rx-queues 1
  }
  dev 0000:1c:00.0 {
    name sync
    num-rx-desc 512
    num-tx-desc 512
  }
  # management NIC(s): never handed to DPDK
  blacklist 0000:0b:00.0
}

plugins {
  plugin linux_cp_plugin.so { enable }
  plugin linux_nl_plugin.so { enable }
  plugin npt66_plugin.so { enable }
}
exit=0
```

### Hostile input through the CLI
```
$ echo '{"dataplane":{"managementPci":["0000:0b:00.0"],"pciWhitelist":["0000:0b:00.0"]}}' | vrx-startupgen --check
vrx-startupgen: vppstartup: invalid dataplane configuration: dataplane: management NIC 0000:0b:00.0 must never be a DPDK device (it is blacklisted; remove it from pciWhitelist/devices)
exit=2
$ echo '{"dataplane":{"managementPci":["0000:0b:00.0"],"devices":{"0000:04:00.0":{"name":"lan\n}\nunix { exec /tmp/x }"}}}}' | vrx-startupgen --check
vrx-startupgen: vppstartup: invalid dataplane configuration: dataplane.devices.0000:04:00.0.name: renderers: unsafe value: logical name "lan\n}\nunix { exec /tmp/x }": length 26 not in 1..15
exit=2
$ echo '{"dataplane":{"managementPci":["0000:0b:00.0"],"pciWhitelist":["0000:04:00.0","0000:04:00.0"]}}' | vrx-startupgen --check
vrx-startupgen: vppstartup: invalid dataplane configuration: dataplane.pciWhitelist[1]: duplicate PCI address 0000:04:00.0
exit=2
$ echo '{"dataplane":{"plugins":{"evil_plugin.so":true}}}' | vrx-startupgen --check
vrx-startupgen: vppstartup: invalid dataplane configuration: dataplane.plugins.evil_plugin.so: plugin evil_plugin.so is not installed (not in the on-disk plugin directory)
exit=2
$ echo '{"dataplane":{"managementPci":["0000:0b:00.0"],"devices":{"0000:04:00.0":{"name":"wan"}},"plugins":{"dpdk_plugin.so":false}}}' | vrx-startupgen --check
vrx-startupgen: vppstartup: invalid dataplane configuration: dataplane.plugins.dpdk_plugin.so: dpdk_plugin.so cannot be disabled while DPDK devices are listed (1)
exit=2
$ echo '{"dataplane":{"mainCore":1,"corelist":[2,3],"rxQueues":4}}' | vrx-startupgen --check
vrx-startupgen: vppstartup: invalid dataplane configuration: dataplane.rxQueues: 4 RX queues exceed the 2 worker thread(s) that can poll them
exit=2
```

### CI gate
```
$ tools/ci.sh --base main      # at 0a14832 (code complete; later commits are docs/status only)
== VRX CI gate: quick ==
worktree  /root/ngfw-wt/F-startup-gen
branch    task/F-startup-gen @ 0a14832   (base: main)
tools     node v22.23.2 · pnpm 12.5.1 · go1.26.0 · buf 1.73.0 · golangci-lint 2.13.2 (pinned) · gitleaks 8.30.1 (pinned)
caches    pnpm store /root/.local/share/pnpm/store/v11 · turbo /root/.cache/vrx-turbo · go /root/.cache/go-build
logs      /root/ngfw-wt/logs/ci/F-startup-gen-20260924-014406-1519297

== contract guard: HEAD vs main ==
...
== lint · typecheck · unit tests · build (turbo) ==
Tasks:    30 successful, 30 total Cached:    24 cached, 30 total Time:    19.22s  

== apps/agent: make lint test build ==
ok  	ngfw/agent/cmd/vrx-startupgen	1.390s; ok  	ngfw/agent/internal/agent	1.105s; ok  	ngfw/agent/internal/contracttest	1.854s; ok  	ngfw/agent/internal/descriptors/abf	1.117s; ok  	ngfw/agent/internal/descriptors/acl	1.271s; ok  	ngfw/agent/internal/descriptors/adl	1.111s; ok  	ngfw/agent/internal/descriptors/af_packet	1.103s; ok  	ngfw/agent/internal/descriptors/arp	1.096s; ok  	ngfw/agent/internal/descriptors/bond	1.120s; ok  	ngfw/agent/internal/descriptors/classify	1.151s; ok  	ngfw/agent/internal/descriptors/df2	1.103s; ok  	ngfw/agent/internal/descriptors/df2/idempotency	1.121s; 

== test/ Go modules, unit mode (test/integration/smoke) ==
test/integration/smoke: gofmt ok · go vet ok · ok  	ngfw/test/integration/smoke	0.020s; 
integration tests inside these modules skip here (VRX_INTEGRATION unset); 'tools/ci.sh full' runs them on the CI slot

== summary (quick) ==
  contract guard: HEAD vs main                       0m00s
  tools (golangci-lint, gitleaks)                    0m02s
  install (pnpm --frozen-lockfile --prefer-offline)   0m06s
  generate + generated-output gate                   0m18s
  forbidden patterns (+ gitleaks)                    0m03s
  lint · typecheck · unit tests · build (turbo)   0m20s
  apps/agent: make lint test build                   0m33s
  test/ Go modules, unit mode (test/integration/smoke)   0m02s
  mode quick · wall time 1m25s · logs /root/ngfw-wt/logs/ci/F-startup-gen-20260924-014406-1519297

CI GATE PASSED
```

## Out of scope / not done
Applying to the host and restarting VPP (manager, procedure in the doc); vfio/uio driver binding; UI/API (generic config
routes); CPU auto-tuning; Debian packaging of the tool (P10); schema/proto fields for the stand-ins (contract change, Q1);
switching `tools/lab provision` to this generator (Q4).

## Open questions
`docs/status/tasks/F-startup-gen-questions.md`: Q1 contract fields (managementPci, devices, buffersPerNuma, plugins) ·
Q2 NIC → port-group mapping (product owner) · Q3 Renderer must not be in the commit apply list · Q4 tools/lab duplicate
template · Q5 isolcpus interpretation.

## Decisions (for the LOG)
| id | decision | options | why |
|---|---|---|---|
| D-SG-1 | Stand-in fields (D-055) `dataplane.managementPci[]`, `dataplane.devices{<pci>:{name,rxQueues,txQueues,rxDesc,txDesc}}`, `dataplane.buffersPerNuma`, `dataplane.plugins{<file>:bool}` read from the JSON document; `devices` merges with `pciWhitelist` | (a) records keyed by natural key (b) arrays (c) wait for P03b | (a) follows D-045/D-053; (c) blocks the task |
| D-SG-2 | Management NIC is mandatory (`managementPci`) once any DPDK device is listed; always rendered as `blacklist`, also with `no-pci` | (a) optional (b) mandatory with devices (c) CLI flag | a DPDK whitelist without an explicit mgmt exclusion is one typo from an unreachable box |
| D-SG-3 | isolcpus: workers ⊆ isolated set, main core ∉ isolated set (only when the host isolates CPUs) | (a) literal "all disjoint" (b) workers in, main out (c) no check | (b) is the standard VPP layout; (a) forbids it (Q5) |
| D-SG-4 | Logical names `[a-z][a-z0-9_-]{0,14}`, no VPP-reserved names or created-interface stems + digit/`_`/`-` | (a) framework `ident` only (b) strict VPP/Linux-safe rule | 15 = IFNAMSIZ−1 for linux-cp; `.` is the sub-if separator; stems would collide with VPP-created names |
| D-SG-5 | Hugepage budget = buffersPerNuma × NUMA nodes × 2560 B vs hugepagesGb (else host reservation); `hugepagesGb` is rendered as a comment only (the reservation is `vm.nr_hugepages`) | (a) also emit sysctl (b) comment + validation | startup.conf cannot reserve hugepages; sysctl is outside this file and task |
| D-SG-6 | `dpdk_plugin.so: false` with no devices ⇒ the `dpdk {}` section is omitted | (a) keep section (b) omit | VPP rejects config sections of unloaded plugins |
| D-SG-7 | `statseg { socket-name /run/vpp/stats.sock }` rendered explicitly (VPP default) | (a) omit (b) explicit | the prompt lists statseg; explicit default is a semantic no-op (only expected `--diff` difference) |
| D-SG-8 | `Renderer` implements the frozen interface with `Apply` → `ErrManagerStep`, `Retrieve` → `ErrRetrieveUnsupported` | (a) CLI only (b) Renderer for dry-run + CLI | lets the commit engine report validation / "restart required" without ever restarting VPP (Q3) |
| D-SG-9 | Unified diff implemented in Go (LCS, 3-line context), no `diff` process | (a) exec `/usr/bin/diff` (b) in-process | no allowlisted binary needed; output verified identical to GNU diff -u |

## Review fixes (fix round 1, 2026-09-24) — findings in docs/status/tasks/F-startup-gen-review.md

`git merge main` first (b733933). Branches: `contract/F-startup-gen` @ 9cd24eb (from main 1ff8f3b) and
`task/F-startup-gen` (contract branch merged in 3579ab9).

| finding | fix |
|---|---|
| **F1** mgmt NIC from the document | `Host.ManagementPCI` is a **host fact**: `ReadHost` resolves every IPv4/IPv6 default-route interface (`/proc/net/route`, `/proc/net/ipv6_route`) via `/sys/class/net/<if>/device` to its PCI address (+ `--mgmt-if`/`--mgmt-pci`). It is **always** blacklisted, any device/whitelist entry equal to it is refused, `dataplane.managementPci` must equal it when set (mismatch = error), an empty document keeps the blacklist, and nothing renders without it (`--no-host` requires `--mgmt-pci`). Regression tests: `TestManagementFromHost` (reviewer repro + `{}`), hostile table, CLI `reviewer repro`, apply-script scenario 13 |
| **F2** apply procedure | `deploy/vpp/apply-startup.sh` (+ `vpp-iface-check.py`): dry run by default; `--apply --expect-sha256` re-runs itself detached (`systemd-run --unit=vrx-startup-apply-<stamp>`, fallback `setsid nohup`); flock -x vrx-vpp then vrx-lab **before** sha check, backup and diff; dead-man timer (`systemd-run --on-active`); drivers of every involved PCI recorded; watch window checks vpp active + NRestarts, `show plugins` content (enabled/disabled/previously loaded), interfaces via the VPP API (`sw_interface_dump` name filter, exact match), mgmt iface UP + address + driver + gateway ping; rollback stops VPP, restores the backup, rebinds changed drivers (driverctl unset-override, sysfs unbind/driver_override/bind), `ip link set up`/`netplan apply`, starts VPP. Tested only against a fake host (`deploy/vpp/test-apply-startup.sh`, 13 scenarios / 36 checks); never executed for real |
| **F3** stand-ins drop plugins/mgmt | four fields in the contract (`contract(schema)` 98d02be, `contract(proto)` 9cd24eb, `F-startup-gen-contract.md`, drift guard 0 findings); document decoded **strictly** from the proto; plugins = current file's switches **overlaid** by `dataplane.plugins` (a document without `plugins` keeps the D-060 block, warnings name every kept switch); current switches are a required host fact (`--current`, default `/etc/vpp/startup.conf`) |
| **F4** CPU model | always `main-core N` and, with workers, `corelist-workers`; unset main core → lowest online non-isolated CPU ≠ 0; `workers: N` → N lowest online CPUs except main (CPU 0 last, only isolated CPUs when isolcpus is set) — VPP's rule made explicit; online **set** (holes) instead of a count |
| **F5** hugepages | host reservation required (0 ⇒ ErrHost); budget always checked against `min(hugepagesGb, host)`; `hugepagesGb: 0` rejected |
| **F6** bounds / unknown keys | `checkBounds` re-applies every schema bound (workers ≤ 255, cores ≤ 1023 incl. the `~0` sentinel, queues 1–256, hugepagesGb 1–1024, buffersPerNuma 1024–4194304, list/record sizes); strict decode rejects unknown keys (`maincore`, `Devices`) |
| **F7** Renderer without host | `New(host Host, …)`; `Host.Check` requires every fact; `Render` fails with `ErrHost` otherwise (`TestHostCheckRequiresEveryFact`) |
| **F8** comments | `#` anywhere ends the line (VPP rule) |
| **F9** live file in unit tests | `TestLiveHostFileSemantics` removed; tests use `testdata/host-startup.conf` only (also as the current plugin switches) |

Also: `apps/agent/internal/renderers/ALLOWLIST.md` note names the plugin directory literal (the allowlist scanner treats
`/usr/lib/…` strings as binaries); `packages/schema/src/domains/group-a.test.ts` one expectation (new defaults) — in the
contract commit; `docs/contracts/schema.md` dataplane table.

### Tests
```
$ cd apps/agent && go test -count=1 -v ./internal/renderers/vppstartup/ ./cmd/vrx-startupgen/ | grep -E '^(--- |ok|FAIL|PASS)'
--- PASS: TestParseCanonical
--- PASS: TestSemanticDiffIgnoresLayout
--- PASS: TestUnifiedDiff
--- PASS: TestReadHost
--- PASS: TestHostCheckRequiresEveryFact
--- PASS: TestPluginSwitches
--- PASS: TestCPUPlacementExplicit
--- PASS: TestManagementFromHost
--- PASS: TestPluginOverlay
--- PASS: TestHostile
--- PASS: TestTemplateBackstop
--- PASS: TestLogicalNameAccepts
--- PASS: TestGolden
--- PASS: TestSixNICSample
--- PASS: TestHostEquivalentSemantics
--- PASS: TestRendererInterface
--- PASS: TestTypedInputMatchesDocument
--- PASS: TestPluginListMatchesHost
--- PASS: TestWarnings
--- PASS: TestCPUList
PASS
ok  	ngfw/agent/internal/renderers/vppstartup	0.226s
--- PASS: TestRenderToStdoutMatchesGolden
--- PASS: TestStdinAndOutputFile
--- PASS: TestDiffAgainstHostFile
--- PASS: TestCheckAndErrors
--- PASS: TestHostFactsFromSysRoot
PASS
ok  	ngfw/agent/cmd/vrx-startupgen	0.188s
$ go test -count=1 -v -run TestHostile ./internal/renderers/vppstartup/ | grep -c '    --- PASS'
127
```

### Generator on this host (live host facts, read only)
```
$ echo '{"dataplane":{"managementPci":["0000:04:00.0"],"devices":{"0000:0b:00.0":{"name":"lan"}}}}' | vrx-startupgen --check      # reviewer F1 repro, live host facts
vrx-startupgen: vppstartup: invalid dataplane configuration: dataplane.managementPci: 0000:04:00.0 does not match the host's management NIC(s) 0000:0b:00.0
exit=2
$ echo '{"dataplane":{"devices":{"0000:0b:00.0":{"name":"lan"}}}}' | vrx-startupgen --check
vrx-startupgen: vppstartup: invalid dataplane configuration: dataplane: 0000:0b:00.0 is the host's management NIC and must never be a DPDK device (remove it from pciWhitelist/devices)
exit=2
$ echo {} | vrx-startupgen | sed -n "/^cpu/,\$p"      # empty document
vrx-startupgen: host management NIC(s) 0000:0b:00.0 (always blacklisted)
vrx-startupgen: warning: plugins: linux_cp_plugin.so { enable } kept from the current start-up file (not in dataplane.plugins)
vrx-startupgen: warning: plugins: linux_nl_plugin.so { enable } kept from the current start-up file (not in dataplane.plugins)
vrx-startupgen: warning: plugins: npt66_plugin.so { enable } kept from the current start-up file (not in dataplane.plugins)
vrx-startupgen: warning: dataplane.mainCore not set: main-core 1 chosen (lowest online, non-isolated CPU)
cpu {
  main-core 1
}

dpdk {
  # host management NIC(s): never handed to DPDK
  blacklist 0000:0b:00.0
  # no DPDK devices configured: probe no PCI device at all
  no-pci
}

plugins {
  plugin linux_cp_plugin.so { enable }
  plugin linux_nl_plugin.so { enable }
  plugin npt66_plugin.so { enable }
}
exit=0
$ echo {} | vrx-startupgen --diff /etc/vpp/startup.conf --semantic
vrx-startupgen: host management NIC(s) 0000:0b:00.0 (always blacklisted)
vrx-startupgen: warning: plugins: linux_cp_plugin.so { enable } kept from the current start-up file (not in dataplane.plugins)
vrx-startupgen: warning: plugins: linux_nl_plugin.so { enable } kept from the current start-up file (not in dataplane.plugins)
vrx-startupgen: warning: plugins: npt66_plugin.so { enable } kept from the current start-up file (not in dataplane.plugins)
vrx-startupgen: warning: dataplane.mainCore not set: main-core 1 chosen (lowest online, non-isolated CPU)
+ cpu > main-core 1
+ statseg > socket-name /run/vpp/stats.sock
+ statseg {}
exit=1
$ echo '{"dataplane":{"mainCore":5,"workers":2}}' | vrx-startupgen --check
vrx-startupgen: host management NIC(s) 0000:0b:00.0 (always blacklisted)
vrx-startupgen: warning: plugins: linux_cp_plugin.so { enable } kept from the current start-up file (not in dataplane.plugins)
vrx-startupgen: warning: plugins: linux_nl_plugin.so { enable } kept from the current start-up file (not in dataplane.plugins)
vrx-startupgen: warning: plugins: npt66_plugin.so { enable } kept from the current start-up file (not in dataplane.plugins)
vrx-startupgen: warning: dataplane.workers=2 without corelist: corelist-workers 1-2 chosen
vrx-startupgen: ok (0 DPDK device(s), 3 plugin switch(es))
exit=0
$ echo '{"dataplane":{"hugepagesGb":0,"buffersPerNuma":1000000}}' | vrx-startupgen --check
vrx-startupgen: vppstartup: invalid dataplane configuration: dataplane.hugepagesGb: 0 not in 1..1024
exit=2
$ echo '{"dataplane":{"hugepagesGb":64,"buffersPerNuma":500000}}' | vrx-startupgen --check
vrx-startupgen: vppstartup: invalid dataplane configuration: dataplane.buffersPerNuma: buffer memory 2441.4 MiB (500000 buffers × 2 NUMA node(s) × 2560 B) exceeds the 2 GiB of the host's hugepage reservation
exit=2
$ echo '{"dataplane":{"txQueues":100000}}' | vrx-startupgen --check
vrx-startupgen: vppstartup: invalid dataplane configuration: dataplane.txQueues: 100000 not in 1..256
exit=2
$ echo '{"dataplane":{"mainCore":4294967295}}' | vrx-startupgen --check
vrx-startupgen: vppstartup: invalid dataplane configuration: dataplane.mainCore: 4294967295 not in 0..1023
exit=2
$ echo '{"dataplane":{"maincore":3,"Devices":{}}}' | vrx-startupgen --check
vrx-startupgen: vppstartup: invalid dataplane configuration: dataplane: proto: (line 1:2): unknown field "Devices"
exit=2
$ echo '{"dataplane":{"mainCore":5,"workers":2}}' | vrx-startupgen --no-host --mgmt-pci 0000:0b:00.0 --online-cpus 0-7 --isolcpus 6-7 --numa-nodes 1 --hugepages-mb 2048 --plugin-dir /usr/lib/x86_64-linux-gnu/vpp_plugins --current none | grep -A3 "^cpu"
vrx-startupgen: host management NIC(s) 0000:0b:00.0 (always blacklisted)
vrx-startupgen: warning: dataplane.workers=2 without corelist: corelist-workers 6-7 chosen
cpu {
  main-core 5
  corelist-workers 6-7
}
$ echo {} | vrx-startupgen --no-host --online-cpus 0-3      # no management NIC fact
vrx-startupgen: vppstartup: host facts: the management NIC is unknown (detect it from the default route or pass --mgmt-pci)
exit=2
$ vrx-startupgen testdata/cases/six-nic-sample.json | sed -n "/^cpu/,\$p"
vrx-startupgen: host management NIC(s) 0000:0b:00.0 (always blacklisted)
cpu {
  main-core 1
  corelist-workers 2-3
}

dpdk {
  dev default {
    num-rx-queues 2
  }
  dev 0000:04:00.0 {
    name wan
  }
  dev 0000:0c:00.0 {
    name lan
  }
  dev 0000:13:00.0 {
    name dmz
  }
  dev 0000:14:00.0 {
    name p2p
  }
  dev 0000:1b:00.0 {
    name lan2
    num-rx-queues 1
  }
  dev 0000:1c:00.0 {
    name sync
    num-rx-desc 512
    num-tx-desc 512
  }
  # host management NIC(s): never handed to DPDK
  blacklist 0000:0b:00.0
}

plugins {
  plugin linux_cp_plugin.so { enable }
  plugin linux_nl_plugin.so { enable }
  plugin npt66_plugin.so { enable }
}
exit=0
```

### Apply script against a fake host
```
$ deploy/vpp/test-apply-startup.sh bin/vrx-startupgen 2>/dev/null
== 1. dry run (default) changes nothing and prints the sha256
  ok   unified diff shows the new dev lines
  ok   semantic diff shows the logical names
  ok   drivers of the PCI devices involved are listed
  ok   sha256 of the live file printed
  ok   live file untouched, VPP not restarted
== 2. --apply without --expect-sha256 is refused
  ok   refused (exit 2), file unchanged
== 3. file changed since the review → refused inside the lock, nothing restarted
  ok   exit 3, file unchanged, no restart
  ok   reason logged
== 4. healthy apply commits
  ok   exit 0, committed
  ok   new file installed
  ok   backup kept (work dir + next to the file)
  ok   VPP restarted
  ok   dead-man timer armed
  ok   dead-man timer cancelled after commit
  ok   logical interfaces verified through the API checker
  ok   drivers recorded before the restart
  ok   locks taken before backup and diff
== 5. a logical interface missing in VPP → rollback
  ok   exit 1, rolled back
  ok   original file restored
  ok   reason logged
  ok   VPP stopped and started on the old file
== 6. VPP steals the management NIC (vfio-pci) → rollback rebinds it to vmxnet3
  ok   exit 1, file restored
  ok   detected by the management check
  ok   unbound from vfio-pci, bound back to vmxnet3
  ok   driverctl override cleared
  ok   management interface brought up
== 7. a plugin the new file enables is not loaded → rollback
  ok   exit 1, reason: npt66 not loaded
== 8. gateway stops answering → rollback; address lost → netplan apply
  ok   exit 1, management path failure detected
  ok   netplan apply during rollback
== 9. VPP crash-restarts during the window → rollback
  ok   exit 1, NRestarts change detected
== 10. dead-man timer: no-op after commit, rollback when the run died mid-way
  ok   committed run: timer does nothing
  ok   uncommitted run: timer restores the backup (exit 1)
== 11. lab lock held by someone else → refused, nothing changed
  ok   exit 3, lock busy, file unchanged
== 12. without --foreground the run is detached through systemd-run
  ok   systemd-run --unit=vrx-startup-apply-… --stage run
  ok   caller returns at once; file untouched by the caller
== 13. the generator refuses the management NIC as a device (host facts) → nothing changed
  ok   dry run fails with the generator's error (exit 2)

apply-startup tests: 36 passed, 0 failed
$ shellcheck deploy/vpp/apply-startup.sh deploy/vpp/test-apply-startup.sh && echo clean
clean
```

### Contract branch (`contract/F-startup-gen` @ 9cd24eb)
```
$ go test -count=1 -v -run 'TestSchemaProtoDrift$' ./internal/contracttest/ | grep 'drift guard'
drift_test.go:486: drift guard: 876 scalar leaves and 192 messages compared, 4 accepted difference(s), 0 finding(s)
$ (cd packages/schema && npx vitest run --coverage)   → Test Files 37 passed, Tests 1214 passed, thresholds met (exit 0)
$ (cd packages/proto && npx vitest run)               → Tests 68 passed
$ tools/ci.sh --base main
== VRX CI gate: quick ==
worktree  /tmp/claude-0/-root-ngfw/7d2208cf-6822-4d52-a974-bcfd33679c79/scratchpad/contract-wt
branch    contract/F-startup-gen @ 9cd24eb   (base: main)
tools     node v22.23.2 · pnpm 12.5.1 · go1.26.0 · buf 1.73.0 · golangci-lint 2.13.2 (pinned) · gitleaks 8.30.1 (pinned)
caches    pnpm store /tmp/.pnpm-store/v11 · turbo /root/.cache/vrx-turbo · go /root/.cache/go-build
logs      /root/ngfw-wt/logs/ci/contract-wt-20260924-015909-1731541
...
== summary (quick) ==
  contract guard: HEAD vs main                       0m00s
  tools (golangci-lint, gitleaks)                    0m02s
  install (pnpm --frozen-lockfile --prefer-offline)   0m05s
  generate + generated-output gate                   0m26s
  forbidden patterns (+ gitleaks)                    0m04s
  lint · typecheck · unit tests · build (turbo)   1m16s
  apps/agent: make lint test build                   0m39s
  test/ Go modules, unit mode (test/integration/smoke)   0m03s
  mode quick · wall time 2m37s · logs /root/ngfw-wt/logs/ci/contract-wt-20260924-015909-1731541

CI GATE PASSED
```

### CI gate — task branch
```
$ tools/ci.sh --base main
== VRX CI gate: quick ==
worktree  /root/ngfw-wt/F-startup-gen
branch    task/F-startup-gen @ 4937438   (base: main)
tools     node v22.23.2 · pnpm 12.5.1 · go1.26.0 · buf 1.73.0 · golangci-lint 2.13.2 (pinned) · gitleaks 8.30.1 (pinned)
caches    pnpm store /root/.local/share/pnpm/store/v11 · turbo /root/.cache/vrx-turbo · go /root/.cache/go-build
logs      /root/ngfw-wt/logs/ci/F-startup-gen-20260924-021440-1865328
...
== summary (quick) ==
  contract guard: HEAD vs main                       0m00s
  tools (golangci-lint, gitleaks)                    0m01s
  install (pnpm --frozen-lockfile --prefer-offline)   0m01s
  generate + generated-output gate                   0m25s
  forbidden patterns (+ gitleaks)                    0m03s
  lint · typecheck · unit tests · build (turbo)   0m26s
  apps/agent: make lint test build                   0m29s
  test/ Go modules, unit mode (test/integration/smoke)   0m02s
  warnings:
    - commit subject(s) not in Conventional Commits form (type(scope): subject):
      review(F-startup-gen): findings
  mode quick · wall time 1m29s · logs /root/ngfw-wt/logs/ci/F-startup-gen-20260924-021440-1865328

CI GATE PASSED
```
(The only warning is the manager's own `review(F-startup-gen): findings` commit subject on this branch.)

### Decisions (fix round)
| id | decision | options | why |
|---|---|---|---|
| D-SG-10 | Management NIC = host fact (default-route interfaces → PCI) ∪ `--mgmt-if/--mgmt-pci`; document `managementPci` must equal it | (a) document only (b) host only (c) host authoritative, document cross-checked | (c): the host cannot be talked out of its own NIC; a document mismatch reveals a wrong mapping early |
| D-SG-11 | `plugins` overlay: current file's switches + document (document wins per key) | (a) document authoritative (drops D-060 if absent) (b) fail when absent (c) overlay | proto3 map has no presence (Q6); (c) never silently drops, `false` still disables |
| D-SG-12 | Always explicit `main-core` + `corelist-workers`; `workers: N` resolved to CPUs by VPP's own rule | (a) keep `workers N` (b) require corelist (c) resolve explicitly | (c) keeps the simple field and makes pinning = validated model |
| D-SG-13 | Unset main core → lowest online non-isolated CPU ≠ 0 | (a) require mainCore (b) default | (b) keeps `{}` renderable (the current host file has no main-core) |
| D-SG-14 | Apply script: `--expect-sha256` mandatory; locks in tools/lab order (vpp, then lab) | (a) lab lock only (b) both, tools/lab order | (b) avoids a lock-order deadlock with `tools/lab restart-vpp` |
| D-SG-15 | Interface check in Python (`vpp_papi`, python3-vpp-api from our debs) | (a) Go helper in apps/agent (b) papi script | (b) stays inside the task's file set; API, not vppctl exit codes |
