# F-startup-gen — VPP startup.conf generator (WBS D0.6)

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
