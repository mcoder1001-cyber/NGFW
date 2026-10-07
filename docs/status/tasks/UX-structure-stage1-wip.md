# UX-structure-stage1 — WIP

- Branch: `codex/ux-structure-stage1-20261007`; isolated worktree matches envelope.
- Local base: `19052bb130ab46977dc5b7aa7be25e976fb5aaf9`; remote task SHA: not yet published.
- Completed: Policy route alias and titles, independent Objects tab links, dynamic routing and routing object groups, reused routing object editors; reference/backend semantics preserved.
- Tests: initial `tools/ci.sh check --base origin/main` PASS (14s); targeted tests initially could not collect because workspace dependency dist outputs were absent; building dependencies before retry; first implementation checkpoint prepared.
- Remaining: navigation/localization/component placement, backward-compatible URLs, regressions, quick gate, independent review.
- Open semantic mismatch: reference zone-pair policies and Route/Bridge/Intra Zone Blocking do not map directly to current NGFW objects; excluded pending owner decision.
- Next command: `pnpm --filter @ngfw/web exec vitest run src/nav/nav.test.ts src/App.test.tsx src/domains/firewall/acl/AclPage.test.tsx`.
- Merge: forbidden until owner approval; no deployment requested.
