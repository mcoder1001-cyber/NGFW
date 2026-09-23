# vppstartup — VPP startup.conf generator (F-startup-gen, WBS D0.6)

Mapping table, CLI and the manager apply procedure: `docs/agent/renderers/vppstartup.md`.

| file | what |
|---|---|
| `model.go` | `Desired` (proto or JSON document → `DataplaneConfig` + D-055 stand-in `Extensions`), `Host` facts, `BuildModel` (all validation) |
| `validate.go` | `PCIAddress`, `LogicalName`, `PluginName`, CPU list parse/format |
| `renderer.go` | `Generate` (pure), `RenderModel`, `Renderer` (Render/Validate; Apply/Retrieve refuse) |
| `templates/startup.conf.tmpl` | the file; strings only through `ident` / `pathtok` |
| `semantic.go` | section/entry parser + `SemanticDiff` (comments, order, indentation ignored) |
| `udiff.go` | unified diff for `vrx-startupgen --diff` (no external `diff` process) |
| `testdata/cases/*.json` → `testdata/*.golden` | golden inputs/outputs (`go test -update` rewrites); `six-nic-sample` uses a **SAMPLE** port-group mapping |
| `testdata/host-startup.conf` | copy of vrx-a's hand-written `/etc/vpp/startup.conf` (2026-09-24, read only) for the semantic test |
| `testdata/plugins-vrx-a.txt` | the 94 on-disk plugin names of vrx-a, so tests are hermetic |

No process is started (VPP has no offline config checker); nothing writes the live file.
