# TD-4 — questions for the manager

1. **Re-enable and API keys (prompt's open question).** Default kept: disabling an account does **not** delete its API
   keys. They are refused while the account is disabled (`authenticateApiKey` checks `app_user.disabled`) and work again
   after a re-enable (e2e `(3)`: `apiKey: 200` after the re-enable commit). If a re-enable should require re-issued keys,
   the disable path would delete them like the D-102 reset does (one line in `syncUsers`) — needs a LOG decision.

2. **Flows that reach the API over plain HTTP from a non-loopback address (prompt's open question).** I searched `tools/`,
   `test/`, `deploy/dev/`, `apps/web/test`, `apps/cli`, `sdk/`:
   - **`tools/app`** (the running product stack): the web UI is `vite preview` on **`0.0.0.0:8080`, plain HTTP**, and
     vite proxies `/api` to the API on `127.0.0.1:3000`. The API sees a **loopback** peer, so logins through `:8080` are
     accepted (not `tls-required`), while the password crosses the LAN in clear between the browser and vite. This is the
     same relay pattern D-100 (1) forbids for the product nginx (P10): the D-100 assumption "every local relay is
     TLS-terminated" does not hold for `tools/app`. Not changed here (out of scope). Suggest: `tools/app` gets TLS
     (self-signed) or binds the web UI to 127.0.0.1 by default, or D-100 records `tools/app` as a lab-only exception.
   - `tools/app` with `VRX_APP_API_HOST=0.0.0.0` exposes the API directly on plain HTTP: remote logins, password sets and
     JWT key creations now get **403 `tls-required`**.
   - CLI `--insecure-http` to a remote host: login now gets 403 `tls-required` (the CLI refuses remote `http://` without
     that flag anyway).
   - Not affected (all loopback): the api e2e harness (`app.inject`, 127.0.0.1), `apps/cli/test/devstack.sh` + the REPL
     e2e, `test/topology/sdk-terraform-ansible/live.sh` (`http://127.0.0.1:$VRX_HTTP_PORT`), the Playwright flow
     (`apps/web/test/e2e/flow.e2e.mjs`, vite on 127.0.0.1 proxying to 127.0.0.1). The SDK and the Terraform provider use
     API keys only (no login, no key creation).

3. **Gap, not built (not in D-100):** role **demotion** and user **deletion** through the config API do not revoke
   access tokens. A deleted user's access token (≤ 15 min) keeps working (`verifyAccess` does not read app_user; refresh
   fails because the row is gone); a demoted user keeps the old role in the JWT until it expires. WebSockets already
   close on both (`usersChanged` re-check in the relay). The disable hook added here would cover both with a few lines
   (`syncUsers` returns the user with reason `demoted`/`deleted`, `configResets` calls `revokeUser`). Needs a decision.

4. **CLI text outside the minimal hunk:** `--password-file` help (`app.go`) still says "with --user: …", and the CLI
   reference row of `api-key create` (registry `Summary` in `cmd_op.go` `init`, rendered into
   `docs/user/cli/reference.md`) does not mention the current-password prompt. Only `apiKeyCreate` was mine to touch, so
   both are unchanged. Suggest one line each in a follow-up (or allow it at merge).

5. **Web UI:** there is no API-key screen in `apps/web` today (no caller of `POST /auth/api-keys`). Whoever adds one
   (P07b / F-aaa) must send `current` for a logged-in user.

6. **Generated drift from TD-2:** `make -C apps/cli gen` and `sdk/gen.sh` also regenerated TD-2's own OpenAPI changes
   (`Users_setPassword`, `Config_revisionDiff`, `Auth_password` summary, `secretChanges`, `/health` schema), which TD-2
   had not regenerated into the Python SDK. Committed on this branch as `chore(cli)` / `chore(sdk)` because the prompt
   asks for clean generated outputs; TD-2's later ff9811f made the same `operations_gen.go` change, so the merge was clean.

7. **CLI e2e `TestReviewFixesOnTheRealStack` (P13 H1) is red since TD-2 — not TD-4.** It commits
   `comment "cli review\x1b]0;PWNED\x07\x1b[2K\rinnocuous"` to prove the CLI shows server text visibly; TD-2's safe-text rule on
   `comment` now refuses that with `400 invalid query … /comment` (TD-4.md, REPL section). `commit.controller.ts` and `common/text.ts`
   are untouched here. The test needs a P13/CLI follow-up: expect the 400, or put the control bytes into a server text that can still
   carry them (a legacy revision comment seeded in the slot DB). `TestREPLConfirmedCommitAutoRevertAndRBAC` (the TD-4 hunk) passes.

8. **Agent binaries built before TD-5 fail on the shared VPP.** My first REPL run used an agent built from the pre-merge tree (no TD-5):
   `create interface.loopback/loop801: … no clean sw_if_index obtained (VPP V19 quarantine): placeholder cap reached before every freed
   classify table index was resurrected (16 placeholders)`. With main's agent (TD-5 / D-105 cap) it passed. Other speculative branches
   based before `3048991` should merge main (and rebuild `apps/agent/bin`) before any devstack or agent-int run.

9. **Fix round 1 — the same "locked" wording split exists on the self-service password change (TD-2 code, not changed here).**
   `UsersService.setPassword` (self) answers a wrong `current` that locks the account with 403 `forbidden` "the current password is
   wrong; the account is now locked", and a guess refused while locked with "the account is locked after too many failed password
   checks". Parallel guesses there are already bounded by `pwset:<caller>` / `pwset:target:<id>` before argon2 (≤ 5/min), so it is
   not the H1 hole, but the two texts still tell a checked guess from an unchecked one. The step-up now answers every `locked`
   with one body and shares the `pwset:<user id>` budget with this route. Suggest the same one-body rule for `setPassword` in a
   follow-up (a two-line change in an owned file, left out because the envelope scopes H1 to key creation).

10. **`tools/ci.sh` contract guard: false "no contract commit" under `pipefail` (not TD-4 code; manager-owned file).**
    `do_contract_guard` runs `git log --format=%s "$mb..$TIP" | grep -qiE '^contract(\(|:|!)'` under `set -euo pipefail`. `grep -q`
    exits at the first match; when `git log` still has output to write it dies of SIGPIPE (141), `pipefail` makes the pipeline fail and
    the gate reports `CONTRACT FILES CHANGED WITHOUT A CONTRACT COMMIT`. On this branch the newest contract commit is line 2 of 45, so
    the race is frequent: the same pipeline in `bash -c 'set -euo pipefail; …'`, 200 runs → `ok=90 false-negative=110`. My 19:00 CI
    run failed that way; the re-run is below. The manager's pre-merge-commit hook runs the same guard on `task/TD-4`, so the merge can
    hit it too. One-line fix: `git log … | grep -iE … >/dev/null` (grep reads to EOF), or read the subjects into a variable first.
