#!/usr/bin/env bash
# Manager: gate + merge a finished task branch into main.  Usage: tools/manager/merge.sh <id>
set -uo pipefail
id="$1"; REPO=/root/ngfw; WT=/root/ngfw-wt/$id
cd "$REPO"
[[ -z "$(git status --porcelain)" ]] || { echo "main is dirty — commit board/status first"; git status --short; exit 1; }
echo "== gate in worktree $WT =="
( cd "$WT" && chown -R root:root . && tools/ci.sh --base main > /root/ngfw-wt/logs/$id-gate.log 2>&1 ) || { echo "GATE FAILED in worktree (see /root/ngfw-wt/logs/$id-gate.log)"; tail -15 /root/ngfw-wt/logs/$id-gate.log; exit 2; }
echo "== merge =="
git merge --no-ff -m "merge($id): $(git log -1 --format=%s task/$id | cut -c1-90)" task/$id || { echo "MERGE CONFLICT"; git merge --abort; exit 3; }
if grep -q "pnpm-lock.yaml" <(git diff --name-only HEAD~1 HEAD); then pnpm install --prefer-offline >/dev/null 2>&1 && git add pnpm-lock.yaml && git commit -qm "chore: lockfile after merge($id)" || true; fi
echo "== gate on main =="
tools/ci.sh > /root/ngfw-wt/logs/$id-main-gate.log 2>&1 || { echo "MAIN GATE FAILED — reverting merge"; tail -15 /root/ngfw-wt/logs/$id-main-gate.log; git revert --no-edit -m 1 HEAD; exit 4; }
python3 tools/board.py --set "$id" merged --note "merged $(date +%F-%H%M)" >/dev/null
git worktree remove --force "$WT" && git branch -d "task/$id" >/dev/null
git add -A && git commit -qm "board: $id merged" && echo "MERGED $id → $(git log -1 --format=%h)"
python3 tools/board.py
