# Identity resume — R2 carry-forward verification

Current product HEAD `721ecc1dc15a99c8d93bee6a9126c545676fc89f`, tree `1a46cbd6e1dc7b89043a3040c5e8015d73b06848`.

This is a historical security-approval carry-forward assessment, separate from fresh R1 tests. Read `identity-finish-independent-review.md` and old read-only `/workspace/scratch/96b8b6fbc8a7/NGFW-identity-integration-review/docs/status/tasks/identity-integration-review-20261002.md` (review commit `9ddb0c7d93031f9f05e6eb98291838748e83c2ec`, reviewed integration head `30de41fd4f9a7913a2614c46a2ec5557a3bfcadc`). Historical review inspects owner isolation, bounded observations, protected state and committed-only public banner, and reports no unresolved code blockers.

Executed Python comparison of `git ls-tree -r <ref> -- apps packages` mappings (path to mode/type/blob) across old read-only and current repositories:
```text
Latest historical integration product differences: []
```
The earlier `3ccabd...` head is not identical to current product (subsequent formatting/corrections exist); no direct carry-forward from that older tree alone is asserted. Latest reviewed integration tree `30de41fd...` is the exact product match.

Fresh static inspection plus current route-inventory/literal-banner/owner-isolation tests confirms the security properties remain: fixed bounded file reads, fixed error identifiers, no candidate/banner/secrets exposed through identity RPC, slot hostname/resolver isolation, configured committed login text only at the expressly public endpoint, no-store header and React text escaping. `tools/ci.sh check --base main` passed in the independent review worktree including current branch gitleaks; no secret finding. No new privilege/session/storage policy, dependency, or shell operation is introduced by this recovery delta.

Assessment: **R2 historical code APPROVE carries forward** to the exact current product above. This is not an independent rerun of every historical security test or full CI, and does not waive mandatory complete quick gate or owner-deferred appliance acceptance.
