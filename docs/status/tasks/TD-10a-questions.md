# TD-10a — questions for the manager

1. **Hunks outside `files_owned`.** No other row owns these files, and each hunk is small:
   - `apps/api/src/agent/agent.client.ts`: `apply`/`dryRun` take an optional `timeoutMs` (the 2.4a budget), and a new `watchReady()` provides the ARCH-01 agent-reconnect trigger. I kept these away from the W-seed anchor blocks.
   - `apps/api/src/datastore/pg-repo.ts`: `readPending` maps `restore_secrets`/`warnings`. This is a free function, not TD-15's PgConfigTx hunk.
   - `apps/web/src/locales/{en,fa}/revisions.json`: adds the `rollback.outcome.*` strings.

   If you want another owner for any of these, the hunks are self-contained.
2. **Decisions D-TD10a-1…5** are in TD-10a.md. The one that departs from the literal review text is D-TD10a-1: `commit-busy` comes after a 1 s lock wait, not at 0 ms. Please fold them into LOG, or give them D numbers.
3. **CommitDialog / queries.ts** (the pending-change bar) are not mine. They should reuse `applyOutcome()` from net.ts on a lost commit answer, the same way RevisionsPage does now. Which row should carry that one-hunk change?
4. **Salvage commit 5374c4a** reformatted the whole of RevisionsPage.tsx and net.ts with prettier (width 100). 5ed9589 undoes that and keeps only the hunks. When you squash (D-112), the net diff is hunks only.
