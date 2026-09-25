# TD-10b verify: fix round 1 (focused)

verifier: the reviewer of `175bed1c`, who did not write the code · branch `task/TD-10b` @ `c9f5d296` (fixes `fe41a982`, `89e5ee5f`,
`f3011e01`, `cb74843d`, `37611df9`) · slot 5 as `w5b` · 2026-09-25 09:15–09:32. Read: the net diff `1480e6b5..c9f5d296` (code, tests,
tools/app), the fix-round section of TD-10b.md, the questions file, and the SEC-auth, TD-22 and P10 rows on main.

## Verdict: APPROVE

C1, C2 (M1 and L5), C3, L1, L4, L7 and the rate-limit flake are fixed as asked, and each has a test that fails on the old code. I re-ran
the new cases. Nothing regressed. The hand-offs are sound. One new nit (N1) is not a blocker and goes to TD-22.

## Checks

| item | result | where |
|---|---|---|
| **C1** | Correct. The API line in tools/app gains only `VRX_TRUST_PROXY=none`, plus a comment naming PENDING-tools-app-transport. The banner now says passwords cross the LAN in clear and offers the tunnel. With `none`, Fastify's `trustProxy=false`, `req.ips` is undefined, the protocol comes from the socket and the client is vite's 127.0.0.1. That is exactly what main does today, so remote lab logins are accepted as before (unit test "review C1" with vite 7's real appended headers). The rate-limit and lockout keys are also 127.0.0.1 for every lab browser, as on main. The one difference is an improvement: the last admin is now throttled instead of locked. | `tools/app:115-119`, `:144-148`; `transport.test.ts` "review C1" |
| **C2 / M1** | Correct. The file may belong to root or `VRX_API_USER` (default `vrx`, read from /etc/passwd), the owner and group are kept, and the mode stays 0600. Everything else is still refused: a foreign owner (unit test: `owned by uid 65534; it must belong to root or the API user 'td10b-no-such-user'`), a symlink (`O_NOFOLLOW` → ELOOP → "is a symbolic link"), and group/other bits (`mode 0620 … refused`). A dangling symlink is replaced by `rename`, never written through. `VRX_API_USER` comes only from root's environment or env file. The test ran as root here: `M1: … {"keys":2,"uid":65534,"mode":"600"}`. | `break-glass.ts:170-211`, `key-file.ts:20-99` |
| **L5** | Correct. `auth.jwt-key-rotated` writes an audit row with `username: root (break-glass)` holding the file, the key count, the kid and the owner uid, plus a `JWT_KEY_RING_ROTATED` system_event. The e2e asserts that the key is not in the row. A failed audit write leaves the rotation standing and prints a warning. | `break-glass.ts:217-245`, `break-glass-cli.ts` rotate branch; lockout e2e "review L5" |
| **L1** | Correct. `pg_advisory_xact_lock(7310100001)` sits in the transaction before the UPDATE, so the `NOT EXISTS` sees the other admin's committed lock. The single-bigint key space does not overlap TD-10a's two-int keys (PostgreSQL documents the two spaces as disjoint). Both callers run outside any transaction, so it cannot deadlock against a held row lock. e2e: `1,1,1,1,1,1,1,1,1,1`; the worker's pre-fix run gave `2,2,1,2,…`. | `auth.service.ts:22`, `:223-241` |
| **L4** | Correct. `CONSUME_SCRIPT` does GET, DEL and SET rtused in one step. The family comes from the token prefix, which is authentic because `rt:<hash>` exists only for tokens the API issued. A racing logout always finds one of the two keys, and a refresh cannot recreate a deleted `rtfam`, because ISSUE_SCRIPT continues a chain only if it exists. e2e: `{"logout":204,"refresh":200,"oldToken":401,"newChain":401,"newToken":401}`. | `tokens.service.ts:311-333`, `:552-563` |
| **L7** | Correct. The file is opened once with O_NOFOLLOW, then checked and read through that same file handle (`fstat`), so the path is never looked up twice. The swap test fails on the old path-based check. | `key-file.ts:69-99`, `tokens.service.ts:153` |
| **C3** | Correct. `f3011e01` takes the hunk out: datastore == the base `c05183a6`, and the diff is empty. `cb74843d` puts it back **byte-identical**: the `+/-` lines of `c05183a6..1480e6b5` and `f3011e01..cb74843d` for `apps/api/src/datastore` are the same. The two e2e cases moved unchanged into their own suite. **I checked the revert:** `git merge-tree --write-tree --merge-base cb74843d HEAD cb74843d~1` → clean (tree `6da55256`). Its datastore equals `c05183a6`, and only the 3 files of the commit change. I exported that tree to scratch and ran it there: unit **134/134, typecheck clean**. (Cloning from /root/ngfw was refused by the git rule, so I used merge-tree plus `git archive` instead.) | `cb74843d` |
| **Rate-limit flake** | Sound. The limiter is a fixed per-minute window, and the test now waits out the last 10 s of a minute before its 7 logins. It is test-only. At most a 10 s wait, inside the test timeout. | `td10b-audit.e2e.test.ts:66-68` |
| **Hand-offs** | Sound. **M2, L2, L6, L9 → SEC-auth.** These are design-level hardening items with no merge risk, and SEC-auth is the D-125 auth review row; the M2 residual must still be written into the D-line (review, proposed D-136). **L3 → P10.** Questions 6 now says **overwrite** X-Forwarded-Proto, adds X-Forwarded-Host, and gives 127.0.0.2 as a dedicated trusted address. The manager should copy that into P10's row notes so the P10 worker sees it. **L8 → TD-22.** The row exists and already has "merge after TD-10a", which is what the call site needs. | `TD-10b-questions.md:33-39`; plan rows SEC-auth, TD-22, P10 |
| **Regressions** | None. The net diff touches only the files listed, plus the move of the option-1 suite. Unit tests are 134/134 and typecheck and lint are clean at HEAD. The e2e (slot 5 as `w5b`; ports 3500 and 3550 checked free) covered td10b lockout, session, session-revocation and audit: **23/23**. The worker's run of 45/45 includes td4 and auth. | below |

## N1 (new, L, not blocking → TD-22): opening the key file can block on a FIFO
`key-file.ts:73` calls `openSync(path, O_RDONLY | O_NOFOLLOW)` with no `O_NONBLOCK`. `TokensService.refreshRing`
(`tokens.service.ts:165`) runs `checkKeyFile` on the request path at most every 5 s. If the key path is ever replaced by a named pipe,
`open` blocks the event loop and the API hangs. The old lstat-first code would have refused the FIFO before reading. The precondition is
write access to the key's directory (root or `vrx`), so the risk is low.

**Fix (one flag):** add `constants.O_NONBLOCK`, which is harmless for regular files, together with a unit test that uses `mkfifo`.

## Evidence (verifier runs)
```
HEAD c9f5d296 — apps/api: vitest run → Test Files 16 passed (16), Tests 134 passed (134); tsc --noEmit exit 0; eslint src test exit 0
  M1: rotate as root, file owned by the API user → {"keys":2,"uid":65534,"mode":"600"}
reverted (merge-tree tree 6da55256 = HEAD minus cb74843d, exported to scratch):
  vitest run → Test Files 16 passed (16), Tests 134 passed (134); tsc --noEmit exit 0
  git diff --quiet c05183a6 6da55256 -- apps/api/src/datastore → identical
e2e: tools/lab lock shared … test:integration td10b-{lockout,session,session-revocation,audit} (VRX_TEST_PREFIX=w5b, port 3550)
  2.3b 7 logins from P: 401,401,401,401,401,401,429; then one from Q: 401
  L1 admins locked account-wide per trial (2 parallel failures at MAX-1): 1,1,1,1,1,1,1,1,1,1
  L5 rotate: exit 0; rotated …/jwt.keys: 1 key(s), new signing key <kid> first, owner uid 65534; the API reloads it within 5 s
  L4 logout racing a refresh: {"logout":204,"refresh":200,"oldToken":401,"newChain":401,"newToken":401}
  demotion: {"demoMe":401,"demoRefresh":401,"promoMe":200} · deletion: … → 401
  Test Files 4 passed (4) · Tests 23 passed (23) · ok nothing named vrx_w5b / vrx_w5b remains
```
Cleanup: I removed the `dist/` directories I built and the scratch copy. The worktree is clean apart from this file. Nothing named `w5b`
is left, and I did not touch tools/app or ports 3000/8080/9101.

## For the merger
The rebase plan in the review (item 12) is unchanged: `reset --soft c05183a6`, one `contract(api-client):` commit, `rebase --onto main
c05183a6`, regenerate `schema.d.ts`, run `ci.sh`.

**C3 and the squash.** If the product owner has not answered PENDING-session-revocation by merge time, **revert `cb74843d` before the
squash**. The squash would otherwise fold option 1 into the single commit (D-112), and it could no longer be dropped on its own. If the
answer is option 1, keep it and log the DEC.
