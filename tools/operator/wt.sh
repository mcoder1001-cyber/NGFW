#!/usr/bin/env bash
# Operator-desktop helper for working on server worktrees (all git lives on the host).
#   wt.sh new <id> [base]        create /root/ngfw-wt/<id> + branch task/<id> from base (default main)
#   wt.sh pull <id>              rsync server worktree -> $WT_LOCAL/<id>   (no .git/node_modules/dist)
#   wt.sh push <id>              rsync $WT_LOCAL/<id> -> server worktree   (never deletes on the server)
#   wt.sh run <id> <cmd...>      run a command inside the server worktree
#   wt.sh commit <id> <message>  git add -A && git commit on the server worktree
#   wt.sh log <id>               last commits + status of the worktree
set -euo pipefail
HOST="${WT_HOST:-ngfw}"; WT=/root/ngfw-wt; LOCAL="${WT_LOCAL:-$HOME/atest/wt}"
cmd="${1:-}"; id="${2:-}"; [[ -n "$cmd" && -n "$id" ]] || { sed -n 2,9p "$0"; exit 1; }
EXCL=(--exclude .git --exclude node_modules --exclude dist --exclude .turbo --exclude 'apps/agent/bin' --exclude coverage)
case "$cmd" in
  new)    base="${3:-main}"; ssh "$HOST" "git -C /root/ngfw worktree add $WT/$id -b task/$id $base && echo created $WT/$id" ;;
  pull)   mkdir -p "$LOCAL/$id"; rsync -rlptDz --delete "${EXCL[@]}" "$HOST:$WT/$id/" "$LOCAL/$id/"; echo "pulled -> $LOCAL/$id" ;;
  push)   rsync -rlptDz --no-o --no-g "${EXCL[@]}" "$LOCAL/$id/" "$HOST:$WT/$id/"; ssh "$HOST" "chown -R root:root $WT/$id"; echo "pushed -> $HOST:$WT/$id" ;;
  run)    shift 2; ssh "$HOST" "cd $WT/$id && export PATH=\$PATH:\$HOME/go/bin && $*" ;;
  commit) shift 2; ssh "$HOST" "cd $WT/$id && git add -A && git commit -qm \"$*\" && git log --oneline | head -1" ;;
  log)    ssh "$HOST" "cd $WT/$id && git log --oneline main..HEAD | head -20; git status --short | head -20" ;;
  *)      echo "unknown command $cmd"; exit 1 ;;
esac
