# Task: Integration check   (prepend 00-CONTEXT.md)

You verify `main` after the manager's merges. You do **not** merge, revert, or touch worktrees. Git is local-only; VPP on this host is shared
and must not be restarted while handover is pending.

1. `git -C /root/ngfw log --since=24h --oneline` — list what the manager merged. `git status --porcelain` must be empty; if not, report it.
2. On `main`: `pnpm install --frozen-lockfile && pnpm gen` — generated paths must be clean (`git status --porcelain -- packages/proto/gen apps/agent/gen packages/schema/dist packages/api-client/src/generated`). If not, a merge brought hand-edited generated code: report the commit.
3. `tools/ci.sh quick`, then `tools/ci.sh full` (it takes the exclusive lab lock and uses slot 12 and the veth rig; never `tools/lab down` in local mode — `vrx-a` is this host). If `test/topology/tri.yml` lists reachable VMs run the `tri` suite; otherwise record `tri: not available`.
4. Leftover check: `tools/lab rig gc` for every slot prefix; `vppctl show interface` must list no `host-w*` interfaces after; report offenders by prefix (the board says which slot belongs to which task).
5. Chaos: **only if `docs/lab/host-vrx-a.md` says `handover: done`**, and only under `flock -x /run/lock/vrx-vpp.lock`: `systemctl kill -s KILL vpp`; verify the main agent reconciles every object within 30 s by comparing `Retrieve` before/after. Otherwise write `chaos: skipped (handover pending)`.
6. Write `docs/status/integration-<YYYY-MM-DD>.md`: what merged, what is red (with the exact failing command and log tail), leftovers, chaos result, and for each failure a row appended to `docs/tech-debt.md` plus `docs/status/tasks/<id>-fail.md` naming the originating branch. Commit on `main` **only these status files** (`git add docs/status docs/tech-debt.md && git commit`), nothing else.
7. Do not attempt fixes — your job is to keep `main` honest, not to write features.
