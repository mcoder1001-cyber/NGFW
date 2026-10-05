# SECURITY-REVIEW task envelope
Branch: codex/security-final-20261005
Worktree: /root/ngfw-wt/security-final-20261005
Baseline: d314f0728 (origin/main at start).
Ownership: prompts/SECURITY-REVIEW.md; docs/status/tasks/SECURITY-REVIEW*.md and DOCS-GEN-review-R2.md; apps/api/package.json; pnpm-lock.yaml; pnpm-workspace.yaml; apps/api/src/auth/security-freeze.test.ts. Product/test ownership expressly approved by root before changes.
Scope: whole-tree adversarial review, auth delta after SEC-auth, existing abuse regression tests, secret scanning and complete unchanged quick gate. No shared-host mutations. Root owns board, independent review and integration.
Recovery: checkpoint and publish coherent changes immediately, at least every 15 minutes. No unexecuted lab acceptance called passing.

Additional root-approved transport ownership: test/topology/private_http.py,test_private_http.py; traffic-a/execute.py; management-dataplane/acceptance.py; ipsec/api-flow.py; dataplane-ui/run.py; management-ui/run.py; capture-trace/live.py,test_live.py; hardware-smoke/reachability-live.py. Only credential transport and necessary adjacent regression fixtures.
