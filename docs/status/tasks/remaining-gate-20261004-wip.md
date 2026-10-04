# Independent remaining quick-gate checks — 2026-10-04

- Branch: `test/remaining-gate-20261004`; isolated worktree `remaining-gate-tests`.
- Product tree tested: `e0d7e8195d88be444970c29300ae5a4fd6403ede` (PR145 integration).
- Owned files: this report and `remaining-gate-20261004-evidence/*.log` only.
- Local report commit: see branch HEAD. Remote publication: pending manager connector publication; no remote SHA claimed.
- Scope: host-independent phases after the previously failing agent phase in unchanged `tools/ci.sh`. No product, workflow, gate, generated-code or board edits; no live lab/shared-host operations.

## Results

Runtime: Go `go1.26.0 linux/amd64`; pinned `ci-tools/golangci-lint` on PATH.

| Phase | Actual result |
|---|---|
| `make -C apps/cli lint test build` | Formatting, REST-only guard, vet, golangci-lint (`0 issues.`), and race unit tests passed. Exact build stopped with `error obtaining VCS status: exit status 128`. |
| CLI supplementary compile | `GOFLAGS=-buildvcs=false make -C apps/cli build` passed; this is compilation evidence, not an exact gate pass. |
| All 19 `test/` Go modules | Every module passed `gofmt -l` (empty), `go vet ./...`, and `env -u NGFW_INTEGRATION go test -count=1 ./...`. Integration tests remain skipped by their existing guards; no lab acceptance claimed. |
| `deploy/vpp/*.sh` shellcheck | Not run: `shellcheck` is not installed/on PATH; it is absent from pinned ci-tools. |
| Startup generator exact build | Same environment VCS failure. Supplementary `go build -buildvcs=false` passed and supplied the harness binary. |
| Apply-startup fake-host harness, two shards | Both shards exited 1 before a check result: fixture setup attempts an AF_UNIX socket and Python raises `PermissionError: [Errno 1] Operation not permitted`. Shard 1 stopped at scenario 1, shard 2 at scenario 2. No product assertion failures or final harness result lines. |

The Go `-x` diagnostic selected `/workspace`, outside the checkout, for `git status --porcelain`, which failed. Git status in the correct isolated worktree succeeds.

Evidence files retain exact outputs. Module names/results are in the individually named `test_*.log`; CLI original and supplementary build logs are distinct. No genuine new product failure was observed. The original complete hosted gate is still required: these partial checks do not certify `CI GATE PASSED`.

## Remaining and next command

Manager should publish this report branch using the authorized connector and retain the remote SHA. Await unchanged hosted gate on the exact integration product tree for CLI build, shellcheck, complete apply-startup harness, and every other required phase. No duplicate local green phase rerun is needed. Do not rerun the complete local quick gate or live integration here.

Task envelope: independent test-only verification requested by manager pursuant to AGENTS parallel independent testing requirement; no new board task. Shellcheck/harness environment limits are not deferred product acceptance and do not justify weakening the gate.
