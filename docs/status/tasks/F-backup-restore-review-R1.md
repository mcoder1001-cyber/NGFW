# F-backup-restore independent R1 — APPROVE — final integration verified

Initial reviewed SHA c4ad647cf6df3dd951630b843e738fc4d60ac7fe (combined5eb + author test/docs). Verified source after authored fixes: 49393f40a668b4acd90e3acbfe009b6211820e4e, product/test paths identical to root243f44f23591c83cb0f7dffb7bc8b88f1f6b1746 (only reviewer/history docs differ). Reviewer owns evidence only; no product edits. Independent R2 previously completed separately.

## Confirmed findings

- **BLOCKER**, apps/api/src/features/backup-restore/backup-restore.service.ts:159: account subtree read before locked candidate import permits a concurrently committed new account to be deleted by restore/commit. Actual PostgreSQL probe twice: `Accounts before restore commit: admin,race-account`; `Accounts after restore commit: admin`. Author9c1f1f3a9 moves current-user preservation into locked datastore edit. Independently CLOSED: original external realDB race now passes; new account remains.
- **BLOCKER**, apps/api/src/commit/commit.service.ts:1134: unrelated in-flight commit preserves newer restored candidate payload but saveCandidate omits restoreSecrets; database pins become null. Actual PostgreSQL promise-barrier probe twice: `Candidate pins before unrelated promotion: {"password/r1pin":2}`; `Candidate pins after unrelated promotion: null`. Author9c1f1f3a9 retains c.restoreSecrets. Independently CLOSED: original external realDB promise-barrier race now passes; nonempty pins remain identical.
- **MAJOR**, apps/api/src/features/backup-restore/templates.ts:43: supplied string placeholder recursively evaluated; x='${x}' causes RangeError Maximum call stack size exceeded (reproduced twice). Manager28c439b57 now treats substituted values as terminal literals and tests self/cross parameter strings plus controls. Independently CLOSED: focused renderer4/4 testsPASS.
- **BLOCKER quick gate**, apps/web/src/nav/nav.test.ts:34: exact availability fixture omitted implemented backup-restore and upgrade pages. Full unchanged gate reproduced1 failed/618 passed webtests. Manager095dcdd99 adds both names without weakening the exact assertion. Independently focused nav5/5PASS; final complete gate pending.

## Acceptance mapping

| Acceptance | Implementation and evidence | Current outcome |
|---|---|---|
| encrypted backup, wipe, restore, commit same hash; wrong password no staging | archive.ts; service/importCandidate; actual PostgreSQL six-case suite, recovered SHA256 ad27929269906644d14d795f096a6dd441780cbe241acf04821e88df696974e4 | standard and both concurrency flowsPASS |
| no plaintext secret; support rejects hash/secret | archive authentication tests + realDB archive byte test; template support scan tests; fixed collector tests | R2 independently13/13PASS on prior sameproduct, finalfullgate pending |
| scheduled local export and retention | schedule.ts durable minute claim, local file retention; realDB suite | PASS independently; remote fixtures author-run only |
| typed template diff and newline/prototype refusal | renderer tests and realDB candidate API test | standard and terminal substitution4/4PASS |
| English/Persian Backup and Upgrade screenshots real endpoints | separate T4 assigned by manager | not run by R1/T1/T2; do not claim browserPASS |
| complete unchanged quick gate | tools/ci.sh --base origin/main | initialFAIL exact navfixture; finalrun pending |

RealDB tests assert actual SQL rows/candidate pins/accounts and file content; agent apply is an in-process fake, no actual host upgrade/VPP mutation. Existing normal commit rollback/restart engine tests inspected; dedicated new race probes described in T2 report use deterministic promises, not test sleeps.

Independent final verification: PostgreSQL authored8/8PASS, external races2/2PASS, renderer4/4PASS, nav5/5PASS. Full restart on493 was stopped via verified ownprocessgroupTERM, exit143 at generation on manager request to wait for fresh R3 contract review; no pass claimed.

Verdict: **APPROVE**. All confirmed findings closed and complete final integration quick gate passed.

## Exact final integration closure, 2026-10-05

Source 4c8640695c8ac9877819817506979e361e801216; tree 79c94d41e9a5e0fc71b7a8837ed9956d661258fd; includes main f57424992, descending history index and appliance packaging fixes. No reviewer product edits. Full unchanged quick gate exit0, CI GATE PASSED,21m16s. PostgreSQL9/9, original external race2/2, independent security probe1/1, production audit0 advisories all passed. Earlier pending statements above are historical checkpoints superseded by this final closure. R5/R6/R8 and browser acceptance are separately reviewed; no implied independent browser/lab execution by this reviewer.
