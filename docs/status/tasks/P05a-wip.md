# P05a — WIP log

Branch `task/P05a`, worktree `/root/ngfw-wt/P05a`, slot 2 (`w2`). Task: `prompts/P05a-interfaces.md`.

## Done so far

- `apps/agent/internal/scheduler/` — `descriptor.go` (Key/KV/Dependency/ErrRecreate/Descriptor/Registry/Plan/Result + AD-3
  transaction semantics in the package doc), `registry.go` (`MapRegistry`: Add/Register/Get/ForKey/Descriptors/Names, name
  validation, duplicate error), `registry_test.go`, `example_descriptor_test.go` (loopback descriptor against the fake VPP:
  create / retrieve == desired / update / ErrRecreate / delete / owner filtering / VPP errors).
- `apps/agent/internal/vpp/` — `client.go` (`Client` = superset of govpp `api.Connection` + `Connected()`, compile-time
  asserted), `tag.go` (`OwnerTag`/`ParseOwnerTag`), `fake/` (recording in-memory client: Invoke, dump streams with
  control-ping replay, WatchEvent/Emit, handlers by message name), `vpptest/` (VRX_INTEGRATION gate, slot-derived
  prefix/instances/tables/NAT pool, shared lab flock).
- `apps/agent/internal/renderers/` — `renderer.go` (`Renderer`, `Files`, `File{Mode,Owner,Content,Secret}`, Validate/
  Paths/Redacted), `helpers_files.go` (WriteFileAtomic, WriteFiles, TakeSnapshot/Restore, Stage), `helpers_exec.go`
  (Allowlist, Command/Output, SystemRunner fixed-argv + minimal env + timeout + bounded output, RecordingRunner,
  ExitError), `helpers_template.go` (Ident/Line/Quoted/JSONString/Addr/Prefix/Network, NewTemplate with missingkey=error,
  Execute + CheckRendered backstop), tests incl. hostile strings and `TestAllowlistDocumented`, `README.md`, `ALLOWLIST.md`.
- `apps/agent/internal/descriptors/README.md`.
- `go.mod`/`go.sum`: `go.fd.io/govpp v0.13.0` (api package only).

`go build ./... && go vet ./... && go test -race ./...` green on the host (see P05a.md for pasted output).

## Left

- `make lint test build` + `tools/ci.sh --base main` evidence, `docs/status/tasks/P05a.md`, `P05a-questions.md`, final commit.
