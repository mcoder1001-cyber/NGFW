# UX-structure-stage1 — WIP

- Branch: `codex/ux-structure-stage1-20261007`; isolated worktree matches envelope.
- Local base: `19052bb130ab46977dc5b7aa7be25e976fb5aaf9`; remote checkpoint: `885e2c1909c2f1095297bf6eacb90745f29b3d28`, successfully pushed to GitHub; PR #206 draft.
- Completed: Policy route alias and titles, independent Objects tab links, dynamic routing and routing object groups, reused routing object editors; reference/backend semantics preserved.
- Tests: nav regression suite 7/7 PASS; React test Persian heading expectation needed updating after label change; initial `tools/ci.sh check --base origin/main` PASS (14s); targeted tests initially could not collect because workspace dependency dist outputs were absent; building dependencies before retry; first implementation checkpoint prepared.
- Remaining: finish React regressions and unchanged quick gate; final evidence and owner review. R1/R2/R6/R7 APPROVE on `885e2c190`; reports preserved on published reviewer branches.
- Open semantic mismatch: reference zone-pair policies and Route/Bridge/Intra Zone Blocking do not map directly to current NGFW objects; excluded pending owner decision.
- Current validation: new navigation/component regressions PASS: 2 files / 10 tests (27.50s). Isolated existing collapse regression PASS (3.19s). Web lint PASS after constants fix. Earlier combined run had new Persian test settings mismatch (fixed), and two existing App test timeouts; no test timeout or gate setting was relaxed. New tests moved to a separate file for isolation. Full quick gate still running (first run captured lint before its fix).
- Next command: `pnpm --filter @ngfw/web exec vitest run src/nav/nav.test.ts src/App.test.tsx src/domains/firewall/acl/AclPage.test.tsx`.
- Merge: forbidden until owner approval; no deployment requested.
