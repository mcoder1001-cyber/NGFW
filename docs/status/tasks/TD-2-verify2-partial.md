# TD-2: verify round 2 (PARTIAL, stopped on the coordinator's order during the manager handover)

Reviewer: independent verify agent. Branch `task/TD-2` @ `84210b7`, base `main` (`f3008cd`). I read the rules, REVIEW-PROMPT, the
envelope `TD-2.verify2.md`, TD-2-verify.md (V1–V5), the "Fix round 2" section of TD-2.md, LOG D-097/D-100/D-102, TD-2-questions
Q6/Q7 and TD-2-contract.md. I also reviewed the code diff `ae52906..84210b7` (apps/api/src) and the new e2e file. No verdict is
given: the runs the envelope requires did not finish.

## Runs

| run | state |
|---|---|
| `tools/ci.sh --base main` (HEAD 84210b7), log `/root/ngfw-wt/logs/ci/TD-2-20260924-131851-10708` | **did not complete, so it is not valid evidence.** Stages 1–6 passed: the contract guard, install, the generate/generated-output gate, gitleaks, and turbo with 30/30 successful. The agent stage failed with `Error: parallel golangci-lint is running`. That failure comes from a concurrent CI run on the host (TD-3), not from the TD-2 code. Also, my captured stdout log was overwritten by the TD-3 run's header. It needs a re-run when the host is quiet. |
| api e2e suite (slot 7) | **not run.** I stopped before starting it. |
| negative controls for V2/V3/V5 (new e2e against the round-1 `apps/api/src`) | **not run.** They were planned. The worker pasted negative controls only for V1 and V4. |

The worktree is clean. No files were modified and no processes were left behind.

## V1–V5 (from reading the code, not yet confirmed by my own runs)

| finding | code review | test that fails without the fix |
|---|---|---|
| V1 keys minted during a reset | Looks fixed. `auth.service.ts` `createApiKey`: one transaction, `SELECT credential_gen … FOR SHARE`, then a JWT generation check (`sessionCurrent`) or a check that the ApiKey row still exists, then `INSERT`. The reset's `UPDATE app_user` (`users.service.ts`, `credentialGen + 1`) conflicts with that lock, so the two are ordered. Login uses `UPDATE … WHERE credential_gen = read` (`auth.service.ts` ~L93-100). Refresh compares against the column. | e2e `V1 — 40 runs`. The worker pasted a negative control (314 survivors). I did not re-run it. |
| V2 config-path reset (D-102) | Looks fixed. `pg-repo.ts` `syncUsers` compares hashes before and after, and then, in the same promote transaction, bumps the generation, clears the lockout, deletes the keys and calls `releaseKeyLocks`. `commit.service.ts` `configResets` revokes and audits after the commit. Confirm resets at confirm. The same hash staged again, or a new user, is not treated as a reset. | e2e V2 ×3. There is no pasted negative control, but the round-1 probe P2 already showed the pre-fix behaviour. |
| V3 Valkey fails after the commit | Looks fixed. `tokens.service.ts` `revokeUser` sets the in-process revocation and calls `bus.sessions` before Valkey, inside a try/catch. Refresh and minting check the PostgreSQL generation. The audit row gets `revocationPersisted:false`. | e2e V3 (simulated `eval` failure). No negative control. |
| V4 inflight hash before commit | Looks fixed. `users.service.ts` calls `replaceInflightHash` after `db.transaction` resolves, still inside `exclusive`. The lock order is now app_user → api_key → candidate. | e2e V4 (a forced deadlock that relies on PostgreSQL `deadlock_timeout`). The worker pasted a negative control. |
| V5 audit of the opt-out and the discard | Looks fixed. `users.controller.ts:71-88` adds `keepApiKeys`, `apiKeysKept` and `discardedCandidate`, and puts `discardedCandidate` in the 200 body. | e2e V5 |

**Contract:** `7ab9c83 contract(api-client): …` touches only `schema.d.ts` (+2 lines), which adds `discardedCandidate`. The field is additive, on
a route that exists only on this branch, and TD-2-contract.md lists it. Migration `0003_td2_credential_gen.sql` is additive (`ADD COLUMN … DEFAULT 0 NOT NULL`),
and no migration numbers collide with main (main has only 0000 and 0001). `git merge-tree main task/TD-2` merges cleanly.

## Findings so far (provisional)
- **Info / manager decision (Q6):** when an admin changes their own hash through the config API, D-102 ends all of the committer's own sessions and keys
  (`commit.service.ts` `configResets`, no `keep`). This is the literal reading of D-102, and the manager should confirm it in the LOG.
- **Info:** `pg-repo.ts` `syncUsers` reads the "before" hashes without a row lock. This is safe only because the commit mutex serialises the hash
  writers of one process. It is not safe across several API processes. This matches the existing single-process assumption.
- **Info:** the V4 e2e relies on PostgreSQL choosing the reset as the deadlock victim (the reset waits ~300 ms longer). If the
  timing changes, the test fails loudly rather than passing falsely.
- No blocking issue found in the code read so far.

## Still to do (next verifier)
1. Re-run `tools/ci.sh --base main` when no other CI or golangci-lint run is active on the host.
2. `eval "$(tools/lab env 7)"; tools/lab lock shared pnpm --filter @ngfw/api test:integration`, then compare with the pasted 61 passed / 3 skipped.
3. Optional: run the new e2e file against `git checkout ae52906 -- apps/api/src` to see V2, V3 and V5 fail, then restore with `git checkout HEAD -- apps/api/src`.
4. Give the verdict.

**No verdict (partial).** It leans toward APPROVE, pending steps 1 and 2.
