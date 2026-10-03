# SDK generator collision audit

- Branch: `codex/sdk-generator-audit-20261003`.
- Worktree: `/root/Documents/Codex/2026-10-03/new-chat/work/NGFW-sdk`.
- Task envelope: audit concrete generator bugs and add bounded compatible fixes; own `sdk/python/**` and this status file only. No generated client edits, new dependencies, schema/API contracts, runtime changes, other-agent worktrees, push, PR or merge.
- Base SHA: `19aa88a5cdbe35954079c563fe5143dc18fa5dba` (`origin/main` when assigned). Product checkpoint: `ceb1effa6b6682baa146bd2d0b784a39b4affb5d`; latest status checkpoint: resolve with `git rev-parse HEAD`.
- Remote checkpoint: none. Manager observed GitHub helper push HTTP 403 permission denied; publication blocked, no alternative write-route retries authorized.
- Owned changed files: `sdk/python/tools/gen.py`, `sdk/python/tests/test_gen_hostile.py`, this file.

## Completed code and evidence

Distinct valid operationIds `Get_x` and `get_x` previously generated two `get_x` methods; the generator succeeded while one method silently shadowed the other. Parameter names `itemId` and `item_id` generated duplicate arguments; `self` or `body` with a request body likewise collided with generated arguments. Reject these collisions with `GenError` before writing any output. Preserve a query parameter named `body` when the operation has no request body.

Both negative regressions fail against the baseline generator, and pass with this fix. Existing injection tests remain passing. This is a bounded correctness fix with no schema contract or dependency changes. Options considered: silently rename, accept shadowing, or fail explicitly; explicit failure preserves operation/parameter identity and avoids an ambiguous public method mapping. Reversal cost is limited to this generator and tests.

## Actual tests

- Prescribed `sdk/python/lock.sh venv`: Python 3.14.4, pytest 9.1.1; existing hash-pinned dependencies installed only in this worktree venv. System Python lacked pytest.
- `sdk/python/.venv/bin/pytest -p no:cacheprovider sdk/python/tests/test_gen_hostile.py`: **5 passed in 1.84s**.
- `cd sdk/python && .venv/bin/pytest -p no:cacheprovider`: **37 passed, 1 skipped in 1.94s**. Skip is live integration, no API/lab execution requested.
- Baseline generator loaded from `git show HEAD:sdk/python/tools/gen.py` into a temporary directory: both negative tests failed as expected.
- `tools/ci.sh check --base main`: **check PASSED (0m10s)**; manager subsequently identified local main as stale, so use origin/main for all further gates.
- `tools/ci.sh check --base origin/main`: **check PASSED (0m08s)**; full quick gate awaits serialized slot from manager.

## Remaining work and exact next command

Independent review, serialized full quick gate, manager publication when access is restored. No remaining scoped product code.

Next command once the full-gate slot is granted: `tools/ci.sh quick --base origin/main`.
