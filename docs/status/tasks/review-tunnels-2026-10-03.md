# Tunnels acceptance-driver review — 2026-10-03

Reviewer 2 read `test/topology/tunnels/run.sh` and `delete-owned.go` independently without editing product or fixture code. `bash -n test/topology/tunnels/run.sh` passed. The three-kind state/drift checks, exact duplicate-error pointer check, bounded recovery poll and historical revision rollback are useful acceptance coverage. Disposal guard and stopped-agent loss injection substantially contain mutations. The helper gathers all candidates before any deletion and rejects ambiguous or missing candidates.

**Changes requested to strengthen acceptance evidence:**

1. **P2 — failed unchanged commit can pass the driver.** `run.sh:145` embeds `commit all-kinds-unchanged` inside `say` command substitution. A refused commit's nonzero exit is consumed by the successful logger; the following state/drift checks can pass against the unchanged previous state. Assign the result with an explicit checked command first, then log. The same pattern occurs in the baseline unchanged commit and cleanup logging; prioritize the acceptance assertion.
2. **P2 — CLI failures are treated as proof of absence.** `run.sh:162` and `:182` put `V show ... | grep` in an `if`. A timeout or CLI failure makes the condition false and therefore passes the loss/rollback absence check. Capture CLI output with a checked assignment, then assert absence. Empty API state alone cannot prove native VPP residue disappeared.
3. **P2 — loss selectors are weaker than the helper's “fully identified” claim.** `delete-owned.go:57`, `:77`, `:97` check instance/source (and VXLAN VNI), but omit expected destination and underlay table; VXLAN also omits the expected ports/decapsulation. A mismatched object with those identifiers can be deleted and counted as the intended fixture. Pass or derive the exact expected fixture tuple and reject mismatches before mutation. Disposable VPP confines the risk, but tightening selectors makes the ownership/evidence claim accurate.

Additional nonblocking cleanup weakness: process shutdown uses unbounded `wait`, and cleanup failures are logged under `set +e` without changing a previously successful result. The private VPP launcher provides ultimate VPP cleanup, but API/agent shutdown and DB cleanup should preferably be bounded and failure-visible. Completed old-agent PID is removed before restart, correctly avoiding a later kill of that old PID. No shared VPP change was performed in this review.

## Final re-review

The final driver/helper resolves the three P2 requests above. **Scoped approval of the revised acceptance scaffolding**, subject to the separately running final live acceptance result:

- Unchanged commits are checked before logging and must return applied/unchanged with an actual empty results array.
- Native absence checks first require a successful bounded CLI call, then search its output. API drift checks also require an actual array, preventing a missing-field false success.
- The helper derives reserved source/destinations from the validated slot and matches default underlay tables, GRE type/mode, IPIP mode, and VXLAN VNI/ports/multicast index/decap next index before deleting. It gathers all matches before mutation and reuses each dumped object's delete fields. VXLAN next index 1 matches the pinned fixture behavior; this helper is intentionally specific to this fixture, not a generic owner deletion utility.
- Top-level step invocation no longer wraps the entire all-mode function in an `||` conditional that disables shell errexit. Owned process stops are bounded with a force-kill fallback, old-agent PID is removed before restart, and cleanup preserves an original failure or reports tracked cleanup failure. Historical rollback additionally verifies persisted tunnel metadata is empty.

Read-only inspection of `apps/agent/internal/agent/tunnels_integration_test.go` confirms canonical protobuf Retrieve equality, zero-operation unchanged applies before/after restart, bounded loss injection, persisted metadata emptiness and checked native absence after rollback. No independent hardware rerun was performed by this reviewer.

Remaining nonblocking nit: the cleanup tunnel PATCH does not itself set `cleanup_failed` on failure, unlike its adjacent interface PATCH. The later checked cleanup commit and disposable VPP destruction contain this, but marking that PATCH failure explicitly would make cleanup diagnostics consistent. Syntax verification passed again; no fixture/product edits were made.

## Final evidence audit and closure

Independently read the final live evidence: [all-kind-final-output.txt](F-tunnels-host-2026-10-03-evidence/all-kind-final-output.txt) proves checked unchanged commit, duplicate HTTP 400, native loss followed by 2-second agent recovery, historical rollback with empty state/drift and persisted metadata, cleanup status 0/original status 0, and disposable VPP shutdown. [go-integration-final.txt](F-tunnels-host-2026-10-03-evidence/go-integration-final.txt) records `TestTunnelsOnDisposableVPP` PASS in 1.53 seconds, metadata entries=0 and private VPP shutdown.

The remaining cleanup PATCH nit is resolved: its failure now sets `cleanup_failed=1`. This error-reporting-only change was made after the successful live driver run, as disclosed by the acceptance report; the successful path tested above is unchanged. Independently reran `bash -n` and verified all three current source hashes against `source.sha256`; all passed.

Final disposition: **APPROVE for review/integration**, no remaining actionable finding from this review. This is an evidence audit, not a reviewer-owned hardware rerun, merge or appliance deployment.
