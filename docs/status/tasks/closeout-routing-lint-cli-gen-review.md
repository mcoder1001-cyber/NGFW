# Independent CLI generation and gate review

Verdict: **APPROVE scoped generated CLI correction and mandatory generation guard**. No blocking findings; complete frozen quick still required.

Read-only review covered root `tools/ci.sh` blob `172866df9cb97f7415058c69f90ccd2184c800ca` and generated operation correction commit `6e08240d395b712c9daf916389cf536c71655e36` / operations blob `b4d0bd438771005b920a9730d5ca2d371ee6ebbc`. Reviewer did not edit root product code or gate.

The gate retains all previous generated paths, adds `apps/cli/internal/api/operations_gen.go`, runs official `make -C apps/cli gen` only after `pnpm gen` has emitted current OpenAPI, and fails when either generator fails. Existing index-relative generated output/untracked-file checks now cover CLI output as well. Agent module hash checks remain and CLI go.mod/go.sum join the before/after invariant. No lint, security, unit, build or existing generation checks were removed or weakened. Updated failure messages identify the correct two generation commands and both module sets. `bash -n tools/ci.sh` independently passed; no unrelated shellcheck-baseline changes were reviewed or waived.

All nine generated additions were independently matched against current OpenAPI operation IDs, HTTP methods, paths, summaries, path/query parameters and request-body flags: four IS-IS/RIP state readers, owner tunnel state, and four VRRP/config-sync operations. They expose existing documented REST operations; no endpoint implementation or authentication boundary changed.

Independent reproduction used the official ngfw-opgen from the reviewer's isolated worktree, reading root OpenAPI only and writing the reviewer's scratch file. It matched root operations_gen.go byte-for-byte, SHA256 `30b921a54049b848c27ebd46b894fe306fc2556603c37664d10dd130ae3afeaf`:

```
tools/heavy.sh go -C apps/cli run ./cmd/ngfw-opgen -in /root/.codex/worktrees/6189/NGFW/packages/api-client/openapi.json -out /root/ngfw-wt/codex-closeout-routing-lint/.scratch/review-operations.go
```

This verifies provenance and strengthens drift detection. It does not replace the unchanged complete quick gate on the final integration tree.
