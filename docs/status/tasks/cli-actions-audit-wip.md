# CLI actions audit — isolated developer checkpoint

- Branch: `codex/cli-actions-audit-20261003`.
- Base: `19aa88a5cdbe35954079c563fe5143dc18fa5dba` (`origin/main` at assignment).
- Local implementation SHA: `415015cb939b65f66c14516c775228ecd0fb8b49`; subsequent status-only checkpoint records that tested implementation.
- Remote implementation SHA: none. Manager reports GitHub publication blocked by HTTP 403 permission denied; no publication attempted by this worker.
- Ownership: `apps/cli` excluding generated `internal/api/operations_gen.go`, `docs/user/cli/reference.md`, this status file. Other chat/worktrees untouched.
- Task envelope: audit ping/traceroute requests against existing API without changing contracts; add actual HTTP JSON-body and validation regression tests; correct stale action support docs; no live VPP/runtime operations, no merge/push/PR by worker.

## Code

`action()` previously discarded its only argument and sent an empty POST body. API's strict PingBody/TracerouteBody requires `target`, making both commands fail validation. Both commands now send exactly `{ "target": "<address>" }`, accept IPv4/IPv6 literals and reject hostnames, CIDRs, scoped/bracketed addresses, malformed addresses and wrong argument counts before API requests. Existing API response/error mapping remains unchanged; traceroute still returns exit 10 on HTTP 501.

The registry and documentation generator now describe ping's default-VRF support and traceroute's existing unsupported status. Regenerated reference through docgen. Defaults verified in API controller and agent ping implementation (5 requests, 1000 ms interval).

## Verification

- `tools/ci.sh check --base origin/main`: PASS (`check PASSED (0m10s)`), contract/forbidden-pattern/secret/slot checks.
- `git diff --check`: PASS.
- `make -C apps/cli docs test lint`: PASS. Docgen regenerated reference; all Go packages passed `go test -race -count=1 ./...`; REST-only acceptance check passed; `go vet ./...` passed; golangci-lint reported `0 issues.`.
- `go test ./internal/cli -run 'TestActions|TestTraceroute|TestDocs' -count=1` (inside apps/cli): PASS after final documentation whitespace correction.
- Focused tests assert serialized HTTP request body and content type for both actions with IPv4, IPv6 and mapped IPv6; invalid targets produce zero requests; traceroute HTTP 501 retains exit 10.
- No live integration performed. Unit HTTP server is regression coverage, not data-plane acceptance.
- Full quick gate deferred to manager's coordinated queue because another developer's gate is running; required before merge.

## Remaining / recovery

Implementation and targeted verification complete; full repository quick gate and independent review remain before merge. Manager runs coordinated full gate and publishes when credentials permit. Exact next command: `git show --stat HEAD` for review, then coordinated `tools/ci.sh --base origin/main`.
