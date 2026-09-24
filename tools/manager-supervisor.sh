#!/usr/bin/env bash
# Keeps the manager agent alive across Claude usage limits (the 5-hour window), crashes, cycle
# caps and normal exits. All manager state lives in the repo (board, status files, envelopes),
# so a restart resumes from disk. Run via systemd (deploy/systemd/ngfw-manager.service) or tmux.
set -uo pipefail
REPO="${REPO:-/root/ngfw}"
STATE="${STATE:-/root/.ngfw-manager}"; mkdir -p "$STATE"
KICKOFF="$REPO/prompts/KICKOFF.md"
RESUME_PROMPT='You are the NGFW manager agent resuming after an interruption (usage limit, crash or restart). Follow prompts/MANAGER-PROMPT.md section "Interruptions and resume": reconcile plan/tasks.yaml with reality (git status on main, worktrees, tmux sessions, docs/status/tasks/*), respawn dead workers with CONTINUE envelopes, then continue the loop. Do not ask questions; log decisions.'
BACKOFF_MIN="${BACKOFF_MIN:-15}"        # minutes between retries while the usage limit is in force
MAX_CYCLE_MIN="${MAX_CYCLE_MIN:-240}"   # hard cap per manager run; it is restarted and resumes
export HOME="${HOME:-/root}" PATH="$PATH:/root/go/bin:/usr/local/bin"

log() { printf '%s %s\n' "$(date -Is)" "$*" | tee -a "$STATE/supervisor.log"; }
cd "$REPO" || { log "repo missing: $REPO"; exit 1; }
command -v claude >/dev/null || { log "claude CLI not installed (npm i -g @anthropic-ai/claude-code) — exiting"; exit 1; }

run_claude() {   # $1 = mode flag set: "skip" | "settings"
  local mode="$1"; shift
  local flags=(--output-format json)
  [[ "$mode" == "skip" ]] && flags+=(--dangerously-skip-permissions)
  # Root refuses --dangerously-skip-permissions unless IS_SANDBOX=1; the repo's .claude/settings.json
  # allowlist is the fallback when the flag is rejected.
  IS_SANDBOX=1 timeout "${MAX_CYCLE_MIN}m" claude "${flags[@]}" "$@" > "$STATE/last-run.json" 2> "$STATE/last-run.err"
}

MODE="skip"
while true; do
  SID=""; [[ -f "$STATE/session_id" ]] && SID="$(<"$STATE/session_id")"
  if [[ -n "$SID" ]]; then ARGS=(-p --resume "$SID" "$RESUME_PROMPT"); else ARGS=(-p "$(<"$KICKOFF")"); fi
  log "starting manager (mode=$MODE session=${SID:-new})"
  run_claude "$MODE" "${ARGS[@]}"; RC=$?
  ERR="$(tr -d '\0' < "$STATE/last-run.err" | tail -c 3000)"
  OUT="$(tr -d '\0' < "$STATE/last-run.json" | tail -c 6000)"

  if [[ "$MODE" == "skip" ]] && grep -qiE "dangerously-skip-permissions.*(root|sudo)|cannot be used with root" <<<"$ERR"; then
    log "flag rejected as root — switching to settings.json allowlist mode"; MODE="settings"; continue
  fi
  NEWSID="$(python3 - "$STATE/last-run.json" <<'PY' 2>/dev/null
import json,sys
try: print(json.load(open(sys.argv[1])).get("session_id",""))
except Exception: print("")
PY
)"
  [[ -n "$NEWSID" ]] && printf '%s' "$NEWSID" > "$STATE/session_id"

  if grep -qiE "usage limit|hit your limit|rate.?limit|too many requests|\b429\b|overloaded|resets? (at|in)|quota|out of (extra )?usage" <<<"$OUT$ERR"; then
    log "usage/rate limit detected (rc=$RC) — sleeping ${BACKOFF_MIN}m, then retrying"; sleep "${BACKOFF_MIN}m"; continue
  fi
  if grep -qiE "No conversation found|session.*not found|Failed to resume" <<<"$OUT$ERR"; then
    log "stored session unusable — starting fresh from KICKOFF (state is in the repo)"; rm -f "$STATE/session_id"; sleep 10; continue
  fi
  if grep -qiE "not logged in|please run /login|authentication|invalid api key" <<<"$OUT$ERR"; then
    log "claude is not authenticated — run 'claude' once as root on this host and log in; retrying in 10m"; sleep 10m; continue
  fi
  if [[ $RC -eq 124 ]]; then log "cycle cap ${MAX_CYCLE_MIN}m reached — restarting (resume)"; sleep 30; continue; fi
  if [[ $RC -ne 0 ]]; then log "manager exited rc=$RC — restarting in 2m (see $STATE/last-run.err)"; sleep 2m; continue; fi
  log "manager cycle finished normally — restarting in 60s"; sleep 60
done
