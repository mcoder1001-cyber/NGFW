# WORKER-OPS — how a worker agent operates in this project (read after 00-CONTEXT.md)

You are running on the operator's desktop. **The repository, the worktrees, VPP and every toolchain live on the host `ngfw`**
(`ssh ngfw`, root, multiplexed; alias for 172.30.126.195). Nothing product-related is built or committed on the desktop.

## Your worktree
Your TASK ENVELOPE names it: `/root/ngfw-wt/<id>` on the host, branch `task/<id>`. It already exists.

## Two ways to work (mix freely)
1. **Local copy + sync** — `WT=/home/eshagghi/atest/new/tools/operator/wt.sh`
   - `$WT pull <id>` → files appear in `~/atest/wt/<id>` (no .git, no node_modules). Edit them with your normal file tools.
   - `$WT push <id>` → rsync back to the host worktree (never deletes there).
   - `$WT run <id> '<command>'` → runs inside the host worktree (PATH includes Go). Example: `$WT run P04 'tools/ci.sh --base main'`.
   - `$WT commit <id> "<conventional commit message>"` → `git add -A && git commit` on the host.
   - `$WT log <id>` → commits on your branch + working-tree status.
   Always `pull` again before editing after you ran generators or `pnpm install` on the host, so your local copy matches.
2. **Direct SSH** — `ssh ngfw 'cd /root/ngfw-wt/<id> && …'`, heredocs (`cat > file <<'EOF'`), `scp file ngfw:/root/ngfw-wt/<id>/…`.
   Quote carefully; prefer method 1 for multi-line files.

## Rules that matter on a shared host (docs/lab/shared-host-rules.md)
- Only your worktree. Never `/root/ngfw` (main), never another `/root/ngfw-wt/*`, never `/etc/vpp`, never `/root/vpp`.
- Everything you create on VPP, in PostgreSQL/Valkey, or as processes carries your slot prefix from the envelope
  (`VRX_TEST_PREFIX`, ports, table range). Kill only PIDs you started. No `pkill`/`killall`. No VPP restarts.
- `pnpm install` on the host uses the shared store; run `pnpm install` (not frozen) only if you added a dependency, and commit the lockfile.
- Long commands: `$WT run` has no time limit but your tool call does — for anything over ~8 minutes run it with
  `nohup … > /root/ngfw-wt/logs/<id>-<step>.log 2>&1 &` and poll the log.

## Finishing (mandatory)
1. `$WT run <id> 'tools/ci.sh --base main'` prints `CI GATE PASSED` (fix until it does; if impossible, say exactly why).
2. Write `docs/status/tasks/<id>.md` in the worktree: what you built · how you verified (**paste real command output**, redact
   secrets) · what is out of scope / left undone · open questions · decisions you made (they will be copied to the LOG).
3. Commit everything (`$WT commit`). Never leave uncommitted work. Do not merge; the manager merges.
4. Your final message: a 10-line summary — branch, last commit hash, CI result, evidence paths, open questions.

## If you get blocked
Write `docs/status/tasks/<id>-questions.md`, commit, and keep going on everything else. If truly nothing is left, finish (step 3–4)
and say so. Never wait for a human.
