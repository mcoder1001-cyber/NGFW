# vppstartup — VPP startup.conf generator (F-startup-gen, WBS D0.6)

Mapping table, CLI and the manual manager apply procedure: `docs/agent/renderers/vppstartup.md`.

| file | what |
|---|---|
| `model.go` | `Desired` (proto or strict JSON document → `DataplaneConfig`), `Host` facts + `Host.Check`, `BuildModel` (all validation, explicit CPU pinning, plugin overlay) |
| `hostfacts.go` | `ReadHost` (management NIC from the default route → `/sys/class/net/<if>/device`, CPUs, NUMA, hugepages, plugins, current plugin switches), `PluginSwitches` |
| `validate.go` | `PCIAddress`, `LogicalName`, `PluginName`, CPU list parse/format |
| `renderer.go` | `Generate` (pure), `RenderModel`, `Renderer` (Render/Validate; Apply/Retrieve refuse) |
| `templates/startup.conf.tmpl` | the file; strings only through `ident` / `pathtok` |
| `semantic.go` | section/entry parser + `SemanticDiff` (comments, order, indentation ignored) |
| `udiff.go` | unified diff for `ngfw-startupgen --diff` (no external `diff` process) |
| `testdata/cases/*.json` → `testdata/*.golden` | golden inputs/outputs (`go test -update` rewrites); `six-nic-sample` uses a **SAMPLE** port-group mapping |
| `testdata/host-startup.conf` | copy of ngfw-a's hand-written `/etc/vpp/startup.conf` (2026-09-24) for the semantic test and the current plugin switches — unit tests never read the live file |
| `testdata/plugins-ngfw-a.txt` | the 94 on-disk plugin names of ngfw-a, so tests are hermetic |

No process is started (VPP has no offline config checker); nothing writes the live file.
