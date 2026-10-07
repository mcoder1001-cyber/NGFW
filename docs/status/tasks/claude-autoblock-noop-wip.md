# claude-autoblock-noop — slot14 MPLS/SRv6 real-API/browser replay

- Branch: `claude/autoblock-noop-20261006` (local only, not pushed per coordinator instruction)
- Product SHA under test: `774b82548` (origin/main `75434a3e5` + cherry-pick of the AutoBlockSet
  confirmed-inactive no-op in `apps/agent/internal/agent/rpc_autoblock.go`). `tools/ci.sh quick --base origin/main` PASSED earlier.
- Artifacts (worktree builds, sha256): `apps/agent/bin/ngfw-agent` f3a0d051…98cd, `apps/api/dist/main.js` 27bfd889…9b7f,
  `apps/web/dist` (worktree build). Browser: previously verified Chrome bundle
  `/root/.codex/worktrees/6189/NGFW/artifacts/test-closeout/browser/root`.
- Runner: unchanged `test/topology/mpls-srv6-browser-live/run.py` (execs `management-dataplane/acceptance.py` with slot14
  substitutions) under `test/topology/hardware-smoke/isolated-vpp.py`, exclusive `flock -n /run/lock/ngfw-acceptance-slot14.lock`.
  No assertion, deadline, fixture or product code was changed.

## Purpose

Show that the intermittent "drift recommit HTTP 504" seen in the original campaign (closeout-mpls-srv6-browser-live-wip.md
attempts 7 and 9, agent deadline exceeded while AutoBlock ACL reconciles ran 10.6 s and restarted) does not recur with the fix.

## Commands

From the worktree root:

- `python3 .scratch/mpls-replay.py <n>` — launcher (env = original attempt11 launcher, artifact paths pointed at this worktree;
  output `.scratch/mpls-srv6-replay<n>/`).
- `.scratch/replay-and-check.sh <n>` — runs the launcher with `.scratch/diag-watch.sh <n>` (diagnostic only: copies the owned
  `agent.log`/`api.log` from the private bind-mounted `/run/ngfw-test` = `.scratch/isolated-vpp-<pid>/test-run/w14/` into
  `.scratch/diag<n>/` because the runner deletes them on exit), then checks cleanup.
- `.scratch/series.sh <from> <to>` — sequential attempts until 3 consecutive PASS.

Cleanup checks after each attempt: `ip netns list | grep w14` (none), no `vpp -c …claude-autoblock-noop…` process, no owned
agent/API process, `systemctl show vpp -p MainPID -p NRestarts` (= 1014/0), slot14 lock free.

## Root cause of replay 1 (and 2, 7) "Connection refused"

Not a wrong path/env and not DB/Valkey: the owned API never reached `listen` inside the fixture's fixed ~30 s `/health`
wait (150 x 0.2 s). Attempt 7 (with log capture) shows the agent up and listening at 07:08:58 and stopped at 07:09:33 while
`api.log` stayed **empty** for the whole 35 s, i.e. Node was still loading modules (`createApp` logs "Starting Nest
application" first thing). Measured on this host: `import('apps/api/dist/app.js')` alone takes 3.5–19 s wall for 4–8 s CPU
(IO/contention on a busy shared host with a cold page cache); the original frozen live-product API shows the same
(4.1 s and 19.0 s measured back-to-back). Passing attempts show agent→"Starting Nest" gaps of 5–10 s.
So the failure is host-load-dependent API cold start, unrelated to the agent fix and to AutoBlock.

Launcher-only fix (from attempt 11): before taking the slot, `mpls-replay.py` imports `apps/api/dist/app.js` once with Node
(no env, no listen, no side effects) and prints `API_IMPORT_WARM_MS`, so the measured run starts from a warm cache. The
fixture's health wait is unchanged.

## Attempts

| # | Launcher | Result | Notes |
|---|----------|--------|-------|
| 1 | no warm-up | FAIL exit 1 | `urllib.error.URLError: <urlopen error [Errno 111] Connection refused>` at `auth/login` after health wait (API not listening). No service logs retained. |
| 2 | no warm-up | FAIL exit 1 | Same `Connection refused`. Watcher pointed at host `/run/ngfw-test` (wrong; namespace bind mount) so no logs. |
| 3 | no warm-up + log capture | **PASS** exit 0 | All markers PASS, browser 16/16. |
| 4 | no warm-up + log capture | FAIL exit 1 | Browser fixture: `BROWSER_ACCEPTANCE_ERROR=page evaluation failed` (`browser.mjs:57` via `until` at `:92`). All API/native/drift/recommit/restart steps passed; en 0–7 and fa 0–5 screenshots captured; failed at fa-6 steering while the SPA was blank (failed-page.png shows an empty loading page, `lang=en`), so the value-wait expression threw on a null root. No HTTP ≥500 in browser responses (only the expected initial `auth/refresh` 401). Not a 504; browser-fixture race. |
| 5 | no warm-up + log capture | **PASS** | |
| 6 | no warm-up + log capture | **PASS** | |
| 7 | no warm-up + log capture | FAIL exit 1 | `Connection refused` again; `api.log` empty for 35 s (diagnosis above). |
| 8 | no warm-up + log capture | **PASS** | |
| 9 | no warm-up + log capture | **PASS** | |
| 10 | no warm-up + log capture | **PASS** | 3 consecutive PASS (8, 9, 10). |
| 11 | warm-up (6126 ms) | **PASS** | |
| 12 | warm-up (4148 ms) | **PASS** | |
| 13 | warm-up (5843 ms) | **PASS** | 3 consecutive PASS with the corrected launcher (11, 12, 13). |

PASS = `MPLS_SRV6_REAL_API_LIFECYCLE=PASS`, `BROWSER_MPLS_SRV6_CONFIGURED_EN_FA=PASS routes=16`,
`DATAPLANE_REAL_API_ACCEPTANCE=PASS`, `MANAGEMENT_REAL_API_ACCEPTANCE=PASS`, `SHARED_VPP_BEFORE/AFTER=MainPID=1014,NRestarts=0`,
launcher exit 0.

Failures preserved verbatim (attempt 4):

```
BROWSER_ACCEPTANCE_ERROR=page evaluation failed
AssertionError [ERR_ASSERTION]: page evaluation failed
    at evaluate (file:///root/ngfw-wt/claude-autoblock-noop/test/topology/mpls-srv6-browser-live/browser.mjs:57:5)
    at async until (file:///root/ngfw-wt/claude-autoblock-noop/test/topology/mpls-srv6-browser-live/browser.mjs:19:44)
    at async file:///root/ngfw-wt/claude-autoblock-noop/test/topology/mpls-srv6-browser-live/browser.mjs:92:7 {
RuntimeError: owned browser exited 1
```

Attempts 1, 2, 7:

```
ConnectionRefusedError: [Errno 111] Connection refused
urllib.error.URLError: <urlopen error [Errno 111] Connection refused>
```

## 504 result

- HTTP 504 count across all 13 attempts: **0** (runner logs: no `"status": 5xx` problem other than the expected 503
  "Agent unavailable" polls during the deliberate owned-agent restart window; captured `api.log`: 0). The raw "504"
  substring hits in attempts 5/7 are nanosecond timestamps, not statuses.
- Every attempt that reached the drift step (3–6, 8–13) got `DRIFT_RECOMMIT` applied (`"code": "ok"`).
- AutoBlock ACL reconciles per run (captured agent logs, attempts 3, 5, 6, 8–13): exactly two, `created:1` then
  `unchanged:1`, each 72–278 ms (attempt 9 of the original campaign: 10.666 s followed by another that never finished).

## Cleanup

After every attempt: no `w14` netns; disposable VPP `DISPOSABLE_VPP_STOPPED` (all 13) and no process of ours left (another
agent's `claude-pppoe` disposable VPP was seen running and left alone); owned DB/role dropped and owned Valkey keys removed
by the runner (22–23 keys per full run); system VPP MainPID=1014 NRestarts=0 before and after every attempt; slot14 lock free.
Generated `test/topology/__pycache__/` (runner byproduct) removed.

## Open items

- The 30 s API-health wait in the fixture is tight on a loaded shared host (cold Node import up to 19 s wall measured, >35 s in
  failing attempts). Consider a launcher warm-up as standard or a separate, reviewed decision on the fixture budget.
- Browser fixture `until()` treats a thrown page evaluation (transient blank SPA) as fatal (attempt 4); 1 of 11 runs that
  reached the browser.

## Next command

None required. To re-run: copy `docs/status/tasks/claude-autoblock-noop-evidence/*` into `.scratch/` and run `.scratch/series.sh 14 21` from the worktree root (.scratch is untracked; launcher and helper scripts are kept in the evidence directory).
