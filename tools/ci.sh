#!/usr/bin/env bash
# tools/ci.sh — THE CI gate. Git on this host is local-only (no remote, no PRs, no CI service), so this
# script is what "CI" means: the manager runs it in every worker's worktree before merging and on main
# after merging; workers run it before declaring done. Never weaken it — extend it (owner: P09).
#
#   tools/ci.sh [quick]                    unit-only gate (default). Budget: < 6 min, < 2 min on an unchanged tree
#   tools/ci.sh full                       quick + integration: flock -x /run/lock/vrx-lab.lock (barrier) → held shared, CI slot 12,
#                                          tools/lab rig up w12, Go/TS suites with VRX_INTEGRATION=1, rig down
#                                          (tools/lab absent = P04 not merged: loud WARNING, integration NOT RUN, gate still passes)
#   tools/ci.sh [quick|full] --base <ref>  + contract guard and branch checks against <ref> (manager: --base main)
#   tools/ci.sh gen-check                  only `pnpm gen` + the generated-output dirty gate (what `pnpm gen:check` runs)
#   tools/ci.sh check [--base <ref>]       only the forbidden-pattern greps (+ contract guard with --base): ~2 s, run before committing
#   tools/ci.sh install-tools              golangci-lint + gitleaks, pinned + checksum-verified → /usr/local/bin
#   tools/ci.sh install-hooks [repo]       .git/hooks/pre-merge-commit that runs `quick` (manager: in /root/ngfw)
#   tools/ci.sh --help
#
# Each step's output is captured to a log file; on failure the tail is printed. The very last line is exactly
# `CI GATE PASSED`, otherwise the script exits non-zero after `CI GATE FAILED — <reason>`.
# Order of quick: [contract guard] → tools → install → gen + dirty gate → forbidden patterns (+gitleaks)
#                 → lint/typecheck/unit tests/build (turbo, VRX_INTEGRATION unset) → apps/agent make lint test build
#                 → every Go module under test/ (gofmt, go vet, go test -count=1; integration tests skip without VRX_INTEGRATION)
set -euo pipefail

usage() {
  sed -n '2,/^set -euo pipefail/{/^set -euo pipefail/!p}' "$0" | sed -E 's/^# ?//'
  cat <<'EOF'
environment (all optional):
  VRX_CI_VERBOSE=1               stream every step's output (same as --verbose / -v)
  VRX_CI_LOG_DIR=<dir>           step logs (default /root/ngfw-wt/logs/ci when writable, else $TMPDIR/vrx-ci)
  VRX_CI_TOOLS_DIR=<dir>         where install-tools puts binaries (default /usr/local/bin; sudo used when needed)
  VRX_CI_ALLOW_MISSING_TOOLS=1   warn instead of fail when golangci-lint/gitleaks are absent and cannot be downloaded
  VRX_CI_LOCK=<file>             lab lock for `full` (default /run/lock/vrx-lab.lock)
  VRX_CI_LOCK_TIMEOUT=<seconds>  how long `full` waits for the exclusive lab lock (the barrier before rig up; default 1800)
  VRX_CI_SLOT=<n>                slot used by `full` (default 12 = the CI slot, docs/lab/shared-host-rules.md)
  VRX_CI_REQUIRE_INTEGRATION=1   make `full` fail (instead of warn) when tools/lab is not available
  VRX_CI_HEAD_REF=<ref>          the branch tip for --base (default HEAD; the pre-merge-commit hook passes the ref being merged)
  GOLANGCI_LINT_VERSION / GITLEAKS_VERSION   override the pinned tool versions for install-tools
  TURBO_CACHE_DIR                shared turbo cache (default ~/.cache/vrx-turbo); GOCACHE: go's default (~/.cache/go-build)
EOF
}

# ---------------------------------------------------------------- pinned tools (recorded in docs/contributing.md)
GOLANGCI_LINT_VERSION="${GOLANGCI_LINT_VERSION:-2.13.2}"
GITLEAKS_VERSION="${GITLEAKS_VERSION:-8.30.1}"

# ---------------------------------------------------------------- constants
# generated output that is committed and must equal what `pnpm gen` produces (never hand-edited)
GEN_PATHS=(packages/proto/gen apps/agent/gen packages/schema/dist packages/api-client/src/generated)
# the contract: changing any of these needs a commit whose subject starts with `contract(` (see docs/contributing.md)
CONTRACT_PATHS=(packages/schema packages/proto apps/agent/gen packages/api-client/src/generated)
# the control plane: never shells out, never talks to VPP directly (00-CONTEXT rules 1 and 9)
CONTROL_PLANE_PATHS=(apps/api/src apps/web/src 'packages/*/src')

TIP="${VRX_CI_HEAD_REF:-HEAD}"    # the branch under test for --base: HEAD, or the branch being merged inside the pre-merge-commit hook
LOCK_FILE="${VRX_CI_LOCK:-/run/lock/vrx-lab.lock}"
LOCK_TIMEOUT="${VRX_CI_LOCK_TIMEOUT:-1800}"
CI_SLOT="${VRX_CI_SLOT:-12}"
TOOLS_DIR="${VRX_CI_TOOLS_DIR:-/usr/local/bin}"

# ---------------------------------------------------------------- arguments (backward compatible: no arg = quick, --base <ref>)
MODE=quick; BASE=""; VERBOSE="${VRX_CI_VERBOSE:-0}"; HOOK_REPO=""
while (($#)); do
  case "$1" in
    quick|full|gen-check|check|install-tools|install-hooks) MODE=$1 ;;
    --base) if [[ -n ${2:-} && ${2:0:1} != - ]]; then BASE=$2; shift; else BASE=main; fi ;;
    --base=*) BASE=${1#--base=} ;;
    -v|--verbose) VERBOSE=1 ;;
    -h|--help) usage; exit 0 ;;
    *) if [[ $MODE == install-hooks && -d $1 ]]; then HOOK_REPO=$1
       else echo "tools/ci.sh: unknown argument '$1' (try --help)" >&2; exit 2; fi ;;
  esac
  shift
done

# ---------------------------------------------------------------- environment
ROOT="$(git rev-parse --show-toplevel)"; cd "$ROOT"
export PATH="$PATH:$HOME/go/bin:/usr/local/go/bin:/usr/local/bin"
export CI=1                                   # non-interactive everywhere (pnpm, vitest, turbo)
export GOTOOLCHAIN=local                      # never auto-download a Go toolchain (dl.google.com is unreachable here)
export TURBO_CACHE_DIR="${TURBO_CACHE_DIR:-${XDG_CACHE_HOME:-$HOME/.cache}/vrx-turbo}"   # shared across worktrees
export TURBO_TELEMETRY_DISABLED=1 TURBO_NO_UPDATE_NOTIFIER=1
SECONDS=0

if [[ -t 1 ]]; then B=$'\033[1;34m' G=$'\033[1;32m' R=$'\033[1;31m' Y=$'\033[1;33m' D=$'\033[2m' N=$'\033[0m'
else B='' G='' R='' Y='' D='' N=''; fi

LOG_DIR=""; CUR_LOG=""; STEP_N=0; STEP_NAME=""; STEP_T0=0; MERGE_BASE=""
declare -a SUMMARY=() WARNINGS=()
INTEGRATION_STATUS=""; RIG_UP=0; RIG_PREFIX=""

say()  { printf '%s\n' "$*"; }
note() { printf '%s%s%s\n' "$D" "$*" "$N"; }
warn() { printf '%sWARN%s %b\n' "$Y" "$N" "$*"; WARNINGS+=("$*"); }
fmt_dur() { printf '%dm%02ds' $(( $1 / 60 )) $(( $1 % 60 )); }

step() {
  end_step
  STEP_N=$((STEP_N + 1)); STEP_NAME=$*; STEP_T0=$SECONDS
  printf '\n%s== %s ==%s\n' "$B" "$*" "$N"
}
end_step() {
  [[ -n $STEP_NAME ]] || return 0
  SUMMARY+=("$(printf '%-48s %7s' "$STEP_NAME" "$(fmt_dur $((SECONDS - STEP_T0)))")")
  STEP_NAME=""
}
# run <label> <command...>: capture output to $LOG_DIR/<NN>-<label>.log (stream it when verbose); returns the command's rc
run() {
  local label=$1; shift
  CUR_LOG="$LOG_DIR/$(printf '%02d' "$STEP_N")-${label//[^A-Za-z0-9._-]/_}.log"
  printf '$ %s\n' "$*" >>"$CUR_LOG"
  local rc=0
  if [[ $VERBOSE == 1 ]]; then
    set +e; "$@" 2>&1 | tee -a "$CUR_LOG"; rc=${PIPESTATUS[0]}; set -e
  else
    set +e; "$@" >>"$CUR_LOG" 2>&1; rc=$?; set -e
  fi
  return "$rc"
}
fail() {
  end_step
  printf '\n%sCI GATE FAILED%s — %b\n' "$R" "$N" "$*" >&2
  if [[ $VERBOSE != 1 && -n $CUR_LOG && -s $CUR_LOG ]]; then
    printf '%s--- last 60 lines of %s ---%s\n' "$D" "$CUR_LOG" "$N" >&2; tail -n 60 "$CUR_LOG" >&2
  fi
  [[ -z $LOG_DIR ]] || printf '%slogs: %s%s\n' "$D" "$LOG_DIR" "$N" >&2
  exit 1
}
on_err() { printf '\n%sCI GATE FAILED%s — unexpected error (exit %s) at %s:%s; logs: %s\n' "$R" "$N" "$1" "$0" "$2" "${LOG_DIR:-<none>}" >&2; exit 1; }
trap 'on_err $? $LINENO' ERR
cleanup() {
  if [[ $RIG_UP == 1 ]]; then
    warn "cleanup: bringing the rig down for prefix $RIG_PREFIX after a failure"
    tools/lab rig down "$RIG_PREFIX" >>"$LOG_DIR/99-rig-down-cleanup.log" 2>&1 || true
  fi
}
trap cleanup EXIT

init_logs() {
  local base="${VRX_CI_LOG_DIR:-}"
  if [[ -z $base ]]; then
    if [[ -d /root/ngfw-wt/logs && -w /root/ngfw-wt/logs ]]; then base=/root/ngfw-wt/logs/ci; else base="${TMPDIR:-/tmp}/vrx-ci"; fi
  fi
  LOG_DIR="$base/$(basename "$ROOT")-$(date +%Y%m%d-%H%M%S)-$$"
  mkdir -p "$LOG_DIR"
}

tool_ver() {
  case $1 in
    node) node -v 2>/dev/null ;;
    pnpm) pnpm -v 2>/dev/null ;;
    go) go version 2>/dev/null | awk '{print $3}' ;;
    buf) buf --version 2>/dev/null ;;
    golangci-lint) golangci-lint version --short 2>/dev/null || golangci-lint version 2>/dev/null | sed -nE 's/.* version v?([0-9][0-9.]*).*/\1/p' ;;
    gitleaks) gitleaks version 2>/dev/null | sed -E 's/^v//' | head -1 ;;
  esac
}

# ---------------------------------------------------------------- install-tools
fetch() { curl -fsSL --retry 3 --connect-timeout 15 --max-time 600 -o "$2" "$1"; }
# shellcheck disable=SC2120  # called with arguments through run() in ensure_tools
install_tools() {  # install_tools [golangci-lint] [gitleaks]  (default: both). Pinned versions, sha256-verified.
  local want=("$@"); ((${#want[@]})) || want=(golangci-lint gitleaks)
  local arch; case "$(uname -m)" in x86_64|amd64) arch=amd64 ;; aarch64|arm64) arch=arm64 ;; *) echo "unsupported arch $(uname -m)" >&2; return 1 ;; esac
  local sudo=""; if [[ ! -w $TOOLS_DIR ]]; then command -v sudo >/dev/null && sudo=sudo; fi
  $sudo mkdir -p "$TOOLS_DIR"
  local tmp; tmp=$(mktemp -d)
  local rc=0 t
  for t in "${want[@]}"; do
    case $t in
      golangci-lint)
        local gv=$GOLANGCI_LINT_VERSION gbase="https://github.com/golangci/golangci-lint/releases/download/v$GOLANGCI_LINT_VERSION"
        local gtgz="golangci-lint-$gv-linux-$arch.tar.gz"
        echo "downloading $gbase/$gtgz"
        fetch "$gbase/$gtgz" "$tmp/$gtgz" && fetch "$gbase/golangci-lint-$gv-checksums.txt" "$tmp/golangci.sums" \
          && (cd "$tmp" && grep -E " \*?$gtgz\$" golangci.sums | sha256sum -c --quiet) \
          && tar -xzf "$tmp/$gtgz" -C "$tmp" "golangci-lint-$gv-linux-$arch/golangci-lint" \
          && $sudo install -m 0755 "$tmp/golangci-lint-$gv-linux-$arch/golangci-lint" "$TOOLS_DIR/golangci-lint" \
          && echo "installed $TOOLS_DIR/golangci-lint: $("$TOOLS_DIR/golangci-lint" version 2>/dev/null | head -1)" || { echo "golangci-lint install FAILED" >&2; rc=1; } ;;
      gitleaks)
        local lv=$GITLEAKS_VERSION lbase="https://github.com/gitleaks/gitleaks/releases/download/v$GITLEAKS_VERSION"
        local larch=$arch; [[ $arch == amd64 ]] && larch=x64
        local ltgz="gitleaks_${lv}_linux_${larch}.tar.gz"
        echo "downloading $lbase/$ltgz"
        mkdir -p "$tmp/gitleaks"
        fetch "$lbase/$ltgz" "$tmp/$ltgz" && fetch "$lbase/gitleaks_${lv}_checksums.txt" "$tmp/gitleaks.sums" \
          && (cd "$tmp" && grep -E " \*?$ltgz\$" gitleaks.sums | sha256sum -c --quiet) \
          && tar -xzf "$tmp/$ltgz" -C "$tmp/gitleaks" gitleaks \
          && $sudo install -m 0755 "$tmp/gitleaks/gitleaks" "$TOOLS_DIR/gitleaks" \
          && echo "installed $TOOLS_DIR/gitleaks: $("$TOOLS_DIR/gitleaks" version 2>/dev/null | head -1)" || { echo "gitleaks install FAILED" >&2; rc=1; } ;;
      *) echo "unknown tool $t" >&2; rc=1 ;;
    esac
  done
  rm -rf "$tmp"
  return $rc
}

# ---------------------------------------------------------------- install-hooks
install_hooks() {
  local repo="${1:-$ROOT}"
  local gitdir common
  gitdir=$(readlink -f "$(git -C "$repo" rev-parse --absolute-git-dir)")
  common=$(readlink -f "$(cd "$repo" && git rev-parse --git-common-dir)")
  if [[ $gitdir != "$common" && -z ${1:-} ]]; then
    cat >&2 <<EOF
tools/ci.sh install-hooks: $repo is a linked worktree. Hooks live in $common/hooks and apply to EVERY worktree
of the repository, so install them from the main worktree (the manager: cd /root/ngfw && tools/ci.sh install-hooks)
or pass the repository path explicitly: tools/ci.sh install-hooks /root/ngfw
EOF
    exit 2
  fi
  local hooks; hooks=$(git -C "$repo" rev-parse --git-path hooks); [[ $hooks == /* ]] || hooks="$repo/$hooks"
  mkdir -p "$hooks"
  local f="$hooks/pre-merge-commit"
  if [[ -e $f ]] && ! grep -q 'installed by tools/ci.sh' "$f"; then
    cp -p "$f" "$f.bak.$(date +%Y%m%d-%H%M%S)"; echo "existing $f backed up"
  fi
  cat >"$f" <<'HOOK'
#!/usr/bin/env bash
# pre-merge-commit — installed by tools/ci.sh install-hooks (VRX). Runs the quick CI gate on the MERGED tree before
# git creates the merge commit; a red gate aborts the merge (the working tree keeps the merge result: fix, or
# `git merge --abort`). Bypass only deliberately and log it: `git merge --no-verify`.
set -euo pipefail
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE
cd "$(git rev-parse --show-toplevel)"
[[ -x tools/ci.sh ]] || { echo "pre-merge-commit: tools/ci.sh not found — refusing the merge" >&2; exit 1; }
# Which branch is being merged? git writes MERGE_HEAD only AFTER this hook, but it exports GIT_REFLOG_ACTION="merge <ref>"
# with the ref named on the command line (git merge --no-ff task/<id> → "merge task/<id>").
tip=""
case "${GIT_REFLOG_ACTION:-}" in "merge "*) tip=${GIT_REFLOG_ACTION#merge }; tip=${tip%% *} ;; esac
if [[ -n $tip ]] && git rev-parse -q --verify "$tip^{commit}" >/dev/null 2>&1; then
  # the quick gate on the merged tree + the contract guard / gitleaks on the commits of the branch being merged
  echo "pre-merge-commit: tools/ci.sh quick --base HEAD on the merged tree; contract guard + gitleaks on '$tip' ($(git rev-parse --short "$tip")) (git merge --no-verify bypasses this)" >&2
  VRX_CI_HEAD_REF=$tip exec tools/ci.sh quick --base HEAD
fi
echo "pre-merge-commit: cannot tell which ref is being merged (GIT_REFLOG_ACTION='${GIT_REFLOG_ACTION:-}') — running tools/ci.sh quick on the merged tree without the contract guard (git merge --no-verify bypasses this)" >&2
exec tools/ci.sh quick
HOOK
  chmod 0755 "$f"
  echo "installed $f  → runs 'tools/ci.sh quick --base HEAD' (contract guard on the branch being merged) before every merge commit in $(readlink -f "$repo")"
  echo "note: git runs pre-merge-commit only when it creates a merge commit — not for fast-forward or --squash merges (the manager merges with --no-ff)"
}

# ---------------------------------------------------------------- the gate steps
preflight() {
  local branch head
  branch=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo '?'); head=$(git rev-parse --short HEAD 2>/dev/null || echo '?')
  printf '%s== VRX CI gate: %s%s%s ==%s\n' "$B" "$N" "$MODE" "$B" "$N"
  say "worktree  $ROOT"
  say "branch    $branch @ $head${BASE:+   (base: $BASE)}${VRX_CI_HEAD_REF:+   (tip: $TIP)}"
  say "tools     node $(tool_ver node) · pnpm $(tool_ver pnpm) · $(tool_ver go) · buf $(tool_ver buf) · golangci-lint ${GOLANGCI_LINT_VERSION} (pinned) · gitleaks ${GITLEAKS_VERSION} (pinned)"
  say "caches    pnpm store $(pnpm store path 2>/dev/null || echo '?') · turbo $TURBO_CACHE_DIR · go $(go env GOCACHE 2>/dev/null || echo '?')"
  say "logs      $LOG_DIR"
  if [[ -n ${VRX_INTEGRATION:-} ]]; then warn "VRX_INTEGRATION was set in the environment — ignored: the quick gate is unit-only"; fi
  unset VRX_INTEGRATION
  local gen_re dirty; gen_re=$(IFS='|'; echo "${GEN_PATHS[*]}")
  if [[ -n ${VRX_CI_HEAD_REF:-} && $TIP != HEAD ]] || [[ -f $(git rev-parse --git-path MERGE_HEAD) ]]; then
    say "merge     in progress: $TIP into $branch — the gate runs on the merged tree (pre-merge-commit hook)"
    dirty=""
  else
    dirty=$(git status --porcelain --untracked-files=normal | grep -vE "^.. ($gen_re)" || true)
  fi
  if [[ -n $dirty ]]; then
    warn "uncommitted changes in the worktree — the gate checks the working tree, but only commits get merged:\n$(sed 's/^/      /' <<<"$dirty" | head -n 15)"
  fi
}

ensure_tools() {
  step "tools (golangci-lint, gitleaks)"
  local missing=() t
  for t in golangci-lint gitleaks; do command -v "$t" >/dev/null 2>&1 || missing+=("$t"); done
  if ((${#missing[@]})); then
    say "missing: ${missing[*]} — installing the pinned versions into $TOOLS_DIR (GitHub releases)"
    if ! run install-tools install_tools "${missing[@]}"; then
      if [[ ${VRX_CI_ALLOW_MISSING_TOOLS:-0} == 1 ]]; then
        warn "could not install ${missing[*]} (VRX_CI_ALLOW_MISSING_TOOLS=1: continuing with reduced checks)"
      else
        fail "${missing[*]} not installed and the download failed. Run 'tools/ci.sh install-tools' with network access, or set VRX_CI_ALLOW_MISSING_TOOLS=1 to run a reduced gate (not for merges)."
      fi
    fi
  fi
  for t in golangci-lint gitleaks; do
    command -v "$t" >/dev/null 2>&1 || continue
    local have want; have=$(tool_ver "$t"); want=$GOLANGCI_LINT_VERSION; [[ $t == gitleaks ]] && want=$GITLEAKS_VERSION
    if [[ $have == "$want" ]]; then say "$t $have"; else warn "$t $have installed, $want pinned — run 'tools/ci.sh install-tools' to align"; fi
  done
}

do_install() {
  step "install (pnpm --frozen-lockfile --prefer-offline)"
  run pnpm-install pnpm install --frozen-lockfile --prefer-offline \
    || fail "pnpm install --frozen-lockfile failed. If you added a dependency: run 'pnpm install' (not frozen) once and commit pnpm-lock.yaml."
  note "$(grep -E '^(Progress|Done|Already up to date|Lockfile is up to date)' "$CUR_LOG" | tail -n 2 | tr '\n' ' ')"
}

do_gen_check() {
  step "generate + generated-output gate"
  local before after
  before=$(git hash-object apps/agent/go.mod apps/agent/go.sum 2>/dev/null | tr '\n' ' ' || true)
  run pnpm-gen pnpm gen || fail "'pnpm gen' failed"
  after=$(git hash-object apps/agent/go.mod apps/agent/go.sum 2>/dev/null | tr '\n' ' ' || true)
  # what did the generators change? working tree vs INDEX (plus untracked files) — never vs HEAD, so a staged but not yet
  # committed merge (the pre-merge-commit hook runs before the merge commit exists) is not mistaken for hand-edited output
  local dirty
  dirty=$( { git diff --name-status -- "${GEN_PATHS[@]}"; git ls-files --others --exclude-standard -- "${GEN_PATHS[@]}" | sed 's/^/??\t/'; } )
  if [[ -n $dirty ]]; then
    say "$dirty"
    git --no-pager diff --stat -- "${GEN_PATHS[@]}" | tail -n 20 || true
    fail "GENERATED OUTPUT IS DIRTY: the committed files under [${GEN_PATHS[*]}] differ from what 'pnpm gen' produces.
  Generated code is never hand-edited. Change the source instead (.proto in packages/proto, Zod in packages/schema/src,
  controllers in apps/api), run 'pnpm gen' and commit the regenerated files. Offending paths are listed above."
  fi
  if [[ $before != "$after" ]]; then
    fail "'pnpm gen' (go mod tidy) changed apps/agent/go.mod or go.sum — commit them:\n$(git status --short -- apps/agent/go.mod apps/agent/go.sum)"
  fi
  say "clean: ${GEN_PATHS[*]}"
}

do_contract_guard() {
  step "contract guard: $TIP vs $BASE"
  git rev-parse --verify -q "$BASE^{commit}" >/dev/null || fail "--base $BASE: no such ref in this repository"
  git rev-parse --verify -q "$TIP^{commit}" >/dev/null || fail "VRX_CI_HEAD_REF=$TIP: no such ref in this repository"
  local mb; mb=$(git merge-base "$BASE" "$TIP") || fail "no merge base between $BASE and $TIP"
  MERGE_BASE=$mb
  local n; n=$(git rev-list --count "$mb..$TIP")
  local changed; changed=$(git diff --name-only "$mb" "$TIP" -- "${CONTRACT_PATHS[@]}")
  if [[ -z $changed ]]; then
    say "no contract files changed in the $n commit(s) of $TIP since $BASE ($(git rev-parse --short "$mb"))"
  else
    say "contract files changed in $TIP since $BASE:"; sed 's/^/  /' <<<"$changed"
    if git log --format=%s "$mb..$TIP" | grep -qiE '^contract(\(|:|!)'; then
      say "ok — contract commit(s) on the branch:"; git log --format='  %h %s' "$mb..$TIP" | grep -iE '^  [0-9a-f]+ contract(\(|:|!)'
    else
      fail "CONTRACT FILES CHANGED WITHOUT A CONTRACT COMMIT. [${CONTRACT_PATHS[*]}] are the contract between packages;
  a branch that changes them must carry a commit whose subject starts with 'contract(<pkg>): …' so the manager reviews it
  (docs/contributing.md → Contract rule; FAST MODE: additive changes are fine, renaming/reshaping needs a PENDING decision).
  Changed files:\n$(sed 's/^/    /' <<<"$changed")"
    fi
  fi
  local bad
  bad=$(git log --format=%s "$mb..$TIP" | grep -vE '^(feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert|contract|status|wip)(\([^)]*\))?!?: ' | grep -vE '^(Merge|Revert) ' || true)
  [[ -z $bad ]] || warn "commit subject(s) not in Conventional Commits form (type(scope): subject):\n$(sed 's/^/      /' <<<"$bad" | head -n 10)"
}

do_forbidden() {
  step "forbidden patterns (+ gitleaks)"
  local hits allowed pat

  # 1. control plane: no shell execution, no direct VPP access, no FFI (rules 1 and 9). Escape hatch: a line comment `ALLOW: <why>`.
  pat='child_process|execSync|execFileSync|spawnSync|exec\.Command|\bsh -c\b|\bbash -c\b|vppctl|/run/vpp/|govpp|ffi-napi|node-ffi|\bkoffi\b'
  hits=$(git grep -nIE --untracked -e "$pat" -- "${CONTROL_PLANE_PATHS[@]}" 2>/dev/null || true)
  allowed=$(grep 'ALLOW:' <<<"$hits" || true); hits=$(grep -v 'ALLOW:' <<<"$hits" || true)
  [[ -z $allowed ]] || warn "control-plane lines exempted with 'ALLOW:' — reviewer, check each justification:\n$(sed 's/^/      /' <<<"$allowed")"
  [[ -z $hits ]] || fail "shell execution / direct VPP access in the control plane (Node never talks to VPP; no user input reaches a shell — go through vrx-agent gRPC):\n$(sed 's/^/    /' <<<"$hits")"
  say "ok: no shell/VPP/FFI access in ${CONTROL_PLANE_PATHS[*]}"

  # 2. no Dockerfiles / compose files anywhere (D-002: VMware VMs, never Docker)
  hits=$(git ls-files --cached --others --exclude-standard | grep -iE '(^|/)(Dockerfile[^/]*|Containerfile[^/]*|\.dockerignore|[^/]*compose[^/]*\.ya?ml)$' || true)
  [[ -z $hits ]] || fail "Docker/compose files are not allowed in this repository (D-002):\n$(sed 's/^/    /' <<<"$hits")"
  say "ok: no Dockerfile/compose files"

  # 3. no kill-by-pattern in any script (shared host: kill only PIDs you spawned). Matches invocations — at the start of a
  #    command (`p*kill …`, `sudo p*kill`, `; p*kill`, `$(p*kill`) or as a quoted program name (`["p*kill", …]`, `os.system("p*kill …`) —
  #    not prose mentions; docs, prompts and *.md are exempt anyway.
  pat='(^|[;&|(`]|\$\()[[:space:]]*(sudo[[:space:]]+)?(p[k]ill|killa[l]l)\b|["'"'"'](p[k]ill|killa[l]l)\b'
  hits=$(git grep -nIE --untracked -e "$pat" -- . ':(exclude)docs' ':(exclude)prompts' ':(exclude)wbs' ':(exclude)*.md' 2>/dev/null || true)
  [[ -z $hits ]] || fail "kill-by-pattern found (p*kill / kill*all) — on the shared host you kill only PIDs you spawned (docs/lab/shared-host-rules.md §5):\n$(sed 's/^/    /' <<<"$hits")"
  say "ok: no kill-by-pattern in scripts"

  # 4. secrets, built-in high-confidence shapes (gitleaks below does the broad scan). Fixtures use VRX_TEST_PSK_<id>; docs use <redacted>.
  pat='-----BEGIN [A-Z ]*PRIVATE KEY-----|AKIA[0-9A-Z]{16}|gh[pousr]_[A-Za-z0-9]{36,}|xox[baprs]-[0-9A-Za-z-]{10,}|sk_(live|test)_[0-9A-Za-z]{10,}|eyJ[A-Za-z0-9_-]{10,}\.eyJ[A-Za-z0-9_-]{10,}|(postgres(ql)?|redis|valkey|mysql|amqp|mongodb(\+srv)?)://[^:/@$<[:space:]]+:[^@$<[:space:]]+@'
  hits=$(git grep -nIE --untracked -e "$pat" -- . ':(exclude)pnpm-lock.yaml' 2>/dev/null | grep -vE 'VRX_TEST_PSK_|<redacted>' || true)
  [[ -z $hits ]] || fail "secret-shaped content in committed/working files (private key block, cloud/API token, JWT, URL with embedded password). Secrets never go into the repository — redact as <redacted>, fixtures use VRX_TEST_PSK_<id>:\n$(sed 's/^/    /' <<<"$hits")"
  say "ok: no secret-shaped strings"

  # 5. gitleaks over the commit history: the branch's commits with --base, otherwise HEAD's history (last 500 commits).
  #    Scoped explicitly — gitleaks' default is every ref, and a leak on some unmerged branch must not fail main.
  if command -v gitleaks >/dev/null 2>&1; then
    local opts=(git . --no-banner --redact --exit-code 1 --report-format json --report-path "$LOG_DIR/gitleaks-report.json")
    [[ -f .github/gitleaks.toml ]] && opts+=(--config .github/gitleaks.toml)
    if [[ -n $MERGE_BASE ]]; then opts+=(--log-opts="$MERGE_BASE..$TIP"); else opts+=(--log-opts="-n 500 HEAD"); fi
    run gitleaks gitleaks "${opts[@]}" \
      || fail "gitleaks found secrets in the commit history (report: $LOG_DIR/gitleaks-report.json). A secret in history stays there: the manager must not merge this branch; recreate the commits without it."
    say "ok: gitleaks — $(sed 's/\x1b\[[0-9;]*m//g' "$CUR_LOG" | grep -oE '(no leaks found|leaks found: [0-9]+|scanned ~?[0-9]+ bytes \([^)]*\) in [0-9.a-z]+)' | head -n 2 | tr '\n' ' ')"
  else
    warn "gitleaks not installed — built-in secret grep only"
  fi
}

do_turbo() {
  step "lint · typecheck · unit tests · build (turbo)"
  run turbo pnpm turbo run lint typecheck test build --continue --output-logs=errors-only \
    || fail "lint / typecheck / unit tests / build failed — the failing task's output is above."
  note "$(grep -E '^\s*(Tasks|Cached|Time):' "$CUR_LOG" | sed 's/^ *//' | tr '\n' ' ')"
}

do_agent() {
  step "apps/agent: make lint test build"
  run agent make -C apps/agent lint test build || fail "apps/agent lint/test/build failed"
  if grep -q 'golangci-lint not installed' "$CUR_LOG"; then
    if [[ ${VRX_CI_ALLOW_MISSING_TOOLS:-0} == 1 ]]; then warn "golangci-lint did not run (go vet only)"
    else fail "golangci-lint did not run in apps/agent (Makefile fell back to go vet) — run 'tools/ci.sh install-tools'"; fi
  fi
  note "$(grep -E '^(ok|FAIL)\s' "$CUR_LOG" | head -n 12 | tr '\n' ';' | sed 's/;/; /g')"
}

# every Go module under test/ (e.g. test/integration/smoke, its own module with `replace ngfw/agent => ../../../apps/agent`)
# is compiled, vetted and run in unit mode: its integration tests t.Skip without VRX_INTEGRATION, but a gofmt/vet/compile
# regression or a stale go.sum ("missing go.sum entry" after apps/agent/go.mod grew) fails the gate here, not in someone's
# integration run (P04 review F6). Nothing to do when test/ has no go.mod yet.
do_test_modules() {
  local mods=() mod
  while IFS= read -r mod; do mods+=("$(dirname "$mod")"); done \
    < <(find test -name go.mod -not -path '*/node_modules/*' 2>/dev/null | sort)
  ((${#mods[@]})) || return 0
  step "test/ Go modules, unit mode (${mods[*]})"
  local unformatted
  for mod in "${mods[@]}"; do
    unformatted=$(gofmt -l "$mod" 2>/dev/null | grep -v '/node_modules/' || true)
    [[ -z $unformatted ]] || fail "gofmt: files in $mod are not formatted (run gofmt -w):\n$(sed 's/^/    /' <<<"$unformatted")"
    run "vet-${mod//\//_}" go -C "$mod" vet ./... || fail "go vet failed in $mod (a stale $mod/go.sum after apps/agent/go.mod changed? run 'go mod tidy' there and commit it)"
    run "test-${mod//\//_}" env -u VRX_INTEGRATION go -C "$mod" test -count=1 ./... || fail "go test (unit mode) failed in $mod"
    say "$mod: gofmt ok · go vet ok · $(grep -E '^(ok|FAIL|\?)\s' "$CUR_LOG" | head -n 3 | tr '\n' ';' | sed 's/;/; /g')"
  done
  note "integration tests inside these modules skip here (VRX_INTEGRATION unset); 'tools/ci.sh full' runs them on the CI slot"
}

# export the VRX_* slot variables for slot $1: from `tools/lab env <slot>` when present and parseable (values are
# never eval'd), missing ones from the formulas in docs/lab/shared-host-rules.md §1 (as `tools/lab env` computes them)
slot_env() {
  local n=$1 line k v
  declare -A got=()
  if [[ -x tools/lab ]]; then
    while IFS= read -r line; do
      line=${line#export }; line=${line%%#*}
      [[ $line =~ ^[[:space:]]*(VRX_[A-Z0-9_]+)=(.*)$ ]] || continue
      k=${BASH_REMATCH[1]}; v=${BASH_REMATCH[2]}; v=${v%%[[:space:]]}; v=${v#[\"\']}; v=${v%[\"\']}
      [[ $v =~ ^[A-Za-z0-9_./:@-]*$ ]] || { warn "tools/lab env: ignoring unsafe value for $k"; continue; }
      export "$k=$v"; got[$k]=1
    done < <(tools/lab env "$n" 2>/dev/null || true)
  fi
  # same arithmetic as `tools/lab env` (D-025: metrics = 9100 + 10·N + 1, so slots 10–12 stay valid ports)
  declare -A def=([VRX_SLOT]=$n [VRX_TEST_PREFIX]=w$n [VRX_HTTP_PORT]=$((3000 + n * 100)) [VRX_WEB_PORT]=$((5000 + n * 100))
                  [VRX_METRICS_PORT]=$((9100 + n * 10 + 1)) [VRX_AGENT_SOCKET]=/run/vrx-test/w$n/agent.sock
                  [VRX_PG_DATABASE]=vrx_w$n [VRX_VALKEY_DB]=$n [VRX_VPP_TABLE_BASE]=$((n * 1000)))
  for k in "${!def[@]}"; do [[ -n ${got[$k]:-} ]] || export "$k=${def[$k]}"; done
  say "slot $n exports: $(env | grep '^VRX_' | sort | tr '\n' ' ')"
}

do_integration() {
  step "integration (slot $CI_SLOT, exclusive lab lock)"
  if [[ ! -x tools/lab ]]; then
    local msg="tools/lab is not present in this tree (P04 not merged yet) — the integration phase was NOT RUN; only the quick gate ran"
    if [[ ${VRX_CI_REQUIRE_INTEGRATION:-0} == 1 ]]; then fail "$msg (VRX_CI_REQUIRE_INTEGRATION=1)"; fi
    warn "$msg"; INTEGRATION_STATUS="NOT RUN — tools/lab absent"; return 0
  fi
  mkdir -p "$(dirname "$LOCK_FILE")" 2>/dev/null || true
  exec 9>>"$LOCK_FILE" || fail "cannot open lock file $LOCK_FILE"
  say "acquiring exclusive $LOCK_FILE (timeout ${LOCK_TIMEOUT}s) — integration harnesses hold it shared, VPP restarts exclusive"
  local t0=$SECONDS
  flock -x -w "$LOCK_TIMEOUT" 9 || fail "could not acquire the exclusive lab lock within ${LOCK_TIMEOUT}s; holders:\n$(lslocks 2>/dev/null | grep -F "$(basename "$LOCK_FILE")" || echo '  unknown')"
  say "exclusive lock held (waited $(fmt_dur $((SECONDS - t0)))) — barrier passed: no VPP restart and no harness is running right now"
  slot_env "$CI_SLOT"
  unset VRX_INTEGRATION
  RIG_PREFIX=$VRX_TEST_PREFIX
  # From here on the gate holds the lock SHARED, like every integration harness (00-CONTEXT, shared-host-rules §1b), because
  #  - `tools/lab rig up` refuses to touch VPP while the lock is held exclusively (it reads that as "VPP restart / CI in progress"),
  #  - the suites take their own `flock -s` on a fresh file description (P04's smoke_test.go does) — against our exclusive lock
  #    that would block until `go test` times out; flock is per open file description, the process tree does not matter.
  # Shared still gives the protection that matters: a VPP restart (exclusive) cannot start underneath the rig or the suites.
  # Other harnesses may run beside the gate on their own prefixes. VRX_LAB_LOCK_HELD=1 / VRX_CI_FULL=1 tell tools/lab and the
  # harnesses that the gate holds the lock for them.
  flock -s 9 || fail "could not convert the lab lock to shared"
  export VRX_LAB_LOCK_HELD=1 VRX_CI_FULL=1
  say "lab lock converted to shared for rig up → suites → rig down"
  if run lab-status tools/lab status; then sed 's/^/  /' "$CUR_LOG" | tail -n 15; else warn "tools/lab status failed (non-fatal)"; fi
  run rig-up tools/lab rig up "$RIG_PREFIX" || fail "tools/lab rig up $RIG_PREFIX failed"
  RIG_UP=1
  local mod
  while IFS= read -r mod; do
    mod=$(dirname "$mod")
    say "go integration: $mod"
    # -p 1: one package at a time — packages share the CI slot's prefix/instance ranges on one VPP (D-087)
    run "go-integration-${mod//\//_}" env VRX_INTEGRATION=1 go -C "$mod" test -p 1 -race -count=1 -timeout 30m ./... \
      || fail "Go integration tests failed in $mod"
    grep -E '^(ok|FAIL)\s' "$CUR_LOG" | sed 's/^/  /' || true
  done < <(find apps/agent test -name go.mod -not -path '*/node_modules/*' 2>/dev/null | sort)
  say "ts integration: pnpm -r run test:integration (packages that define it)"
  run ts-integration env VRX_INTEGRATION=1 pnpm -r --workspace-concurrency=1 --if-present run test:integration \
    || fail "TS integration tests failed"
  run rig-down tools/lab rig down "$RIG_PREFIX" || fail "tools/lab rig down $RIG_PREFIX failed — objects with prefix $RIG_PREFIX may be left on VPP; run 'tools/lab rig gc $RIG_PREFIX'"
  RIG_UP=0
  unset VRX_LAB_LOCK_HELD VRX_CI_FULL
  exec 9>&-
  INTEGRATION_STATUS="ran on slot $CI_SLOT (prefix $RIG_PREFIX): rig up → Go + TS suites with VRX_INTEGRATION=1 → rig down"
}

passed() {
  end_step
  printf '\n%s== summary (%s) ==%s\n' "$B" "$MODE" "$N"
  printf '  %s\n' "${SUMMARY[@]}"
  if ((${#WARNINGS[@]})); then printf '  %swarnings:%s\n' "$Y" "$N"; printf '    - %b\n' "${WARNINGS[@]}"; fi
  [[ -z $INTEGRATION_STATUS ]] || say "  integration: $INTEGRATION_STATUS"
  say "  mode $MODE · wall time $(fmt_dur "$SECONDS") · logs $LOG_DIR"
  if [[ $INTEGRATION_STATUS == NOT\ RUN* ]]; then printf '%sWARNING: integration NOT RUN (%s)%s\n' "$Y" "$INTEGRATION_STATUS" "$N"; fi
  printf '\n%sCI GATE PASSED%s\n' "$G" "$N"
}

# ---------------------------------------------------------------- dispatch
case $MODE in
  install-tools)
    install_tools; exit $? ;;
  install-hooks)
    install_hooks "$HOOK_REPO"; exit 0 ;;
  gen-check)
    init_logs; do_gen_check; end_step; say "gen-check PASSED ($(fmt_dur "$SECONDS"))" ;;
  check)
    init_logs; [[ -z $BASE ]] || do_contract_guard; do_forbidden; end_step; say "check PASSED ($(fmt_dur "$SECONDS"))" ;;
  quick|full)
    init_logs
    preflight
    [[ -z $BASE ]] || do_contract_guard      # git-only, ~1 s: a contract-less branch fails with the root-cause message first
    ensure_tools
    do_install
    do_gen_check
    do_forbidden
    do_turbo
    do_agent
    do_test_modules
    [[ $MODE != full ]] || do_integration
    passed ;;
esac
