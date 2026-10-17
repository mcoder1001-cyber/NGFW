# Task envelope — F-security-page

Role: developer (web only). Started 2026-10-17 by the manager session; branch `claude/F-security-page`, worktree
`/root/NGFW/.claude/worktrees/wizardly-turing-057676`, base `origin/main` 99a774734.

Task: build the Security domain screen so `/firewall/security` is no longer "Not yet available" — a
`DomainTabsPage` with an **Auto-block policy** tab (edits the committed `security.autoBlock` through the same
schema-form / domain-candidate save path the Management tabs use, API problems through `ProblemAlert`) and a
**Blocked sources** tab (the existing `AutoBlockPage`); `/firewall/auto-block` keeps working (redirect allowed);
`security` in `BUILT_DOMAINS` and a lazy route; en/fa `security` namespace; auto-block intro/allow-list hints point at
the Policy tab; tests (App, nav, policy tab jsdom); user docs; WIP status file; small commits, each pushed; PR
against main with the test output.

Owned files: `apps/web/src/domains/security/**`, `apps/web/src/nav/nav.ts`, `apps/web/src/nav/nav.test.ts`,
`apps/web/src/router.tsx`, `apps/web/src/App.test.tsx`, `apps/web/src/i18n.ts`,
`apps/web/src/locales/{en,fa}/security.json` and `auto-block.json`, `docs/user/security/**`,
`docs/status/tasks/F-security-page-*.md`.

Out of scope: `packages/schema`, `packages/proto`, `apps/api`, `apps/agent` (no contract, API or agent change).
