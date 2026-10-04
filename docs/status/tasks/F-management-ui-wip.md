# F-management-ui durable completion checkpoint

Branch: codex/complete-management-ui-20261004
Base: origin/main 121c09747. Reviewed local checkpoint: d76af4eab.
Remote source checkpoint: 1300e0573ac3914f40ce84a8e628e481303b2711, published successfully through GitHub connector.
Its tree 65bf47a6f13e46da4060c2b038848505696c1156 exactly matches local d76af4eab^{tree}.
This status followup is published separately on the same branch.
Owned files: see F-management-ui-envelope.md.
Completed: active HTTPS removal guard; rollback removal closes HTTPS and upgrades, clears context;
late replacement restarts listener; failed rotation retains actual loaded revision/minimum;
secret-store exceptions sanitized; bilingual accurate listener/error status and operator docs.
Remaining source: none within F-management-ui prompt. AAA belongs to F-aaa; edge TLS excluded by prompt.
Actual verification:
- pnpm --filter @ngfw/api exec vitest run src/features/mgmt-tls/mgmt-tls.test.ts: 15 passed, including real TLS and authenticated WSS.
- pnpm --filter @ngfw/web exec vitest run src/domains/system/management/ManagementPage.test.tsx src/domains/system/management/locale.test.ts: 11 passed.
- pnpm --filter @ngfw/api typecheck: exit 0.
- focused API eslint: exit 0 (existing root package module warning).
- tools/ci.sh check --base origin/main: check PASSED; git diff --check: exit 0.
- Workspace dependency build: 12 successful tasks; generated tracked files unchanged.
Current failure: none. Full CI waived by owner; real deployed API/DB/browser acceptance deferred.
Next command: git show --stat HEAD; publish GitHub branch, compare remote commit tree with local HEAD^{tree}, independent review.
