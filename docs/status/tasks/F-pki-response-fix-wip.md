# PKI external-response repair — 2026-10-03

Branch `codex/pki-response-fix-20261003`, isolated worktree `/root/.codex/worktrees/0b16/developers/PKI-response-fix`, based on frozen generated DTO checkpoint `3ca241a4`. Root owns heavy execution, generation and publication; no push/merge or developer heavy commands.

Fixed the standalone client negative probe by placing its `@ts-expect-error` directly above the `body` member where the missing passphrase diagnostic occurs. Public parsed-CSR response metadata now has a bounded string subject/SAN array and optional key specification, allowing dotted-OID subjects and parser omission without defaulting a key. Only response staging-patch metadata changed: the editable configuration CSR schema remains strict and unchanged.

Added meaningful external OpenSSL CSR regressions for P-256 and secp521r1, both carrying `title=Role` (parsed as `2.5.4.12=Role`). The actual signing service returns `staged:false` because no matching stored key exists; the tests validate the entire sign response, preserve optional omitted key metadata for P-521, reject the same metadata through the strict configuration schema and check no private-material banner appears in the public response. Keys are generated only in temporary test scratch files and removed by cleanup.

Unverified current source checkpoint. Root command from this worktree: `/root/.codex/worktrees/0b16/NGFW/tools/heavy.sh bash .scratch/pki-response-check.sh > .scratch/pki-response-check.log 2>&1`. Runs repository scheduler client generation/prerequisites, focused PKI/global route guards, API typecheck/scoped lint, client build and PKI consumer typecheck. Expected 4–10 minutes plus queue. Generated client updates must be committed after generator execution; none were hand-edited.

Separate generator parity issue (RSA bits YANG union lacking literal ranges) was reported to developer_iso; this patch does not modify generator, YANG or config contracts. Remaining: root current validation/consumer generation, independent re-review and final quick CI; materializer/P11 runtime/UI/live acceptance are separate unfinished work.
