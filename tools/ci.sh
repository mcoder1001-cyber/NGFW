#!/usr/bin/env bash
# Local CI gate. Git is local-only on this host, so this script IS the CI: the manager runs it on
# every worker branch before merge and on main after merge. P09 extends it; do not weaken it.
#   tools/ci.sh            → full gate in the current worktree
#   tools/ci.sh --base main → also runs the contract guard against that ref
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"
export PATH="$PATH:$HOME/go/bin:/usr/local/go/bin"
BASE=""; [[ "${1:-}" == "--base" ]] && BASE="${2:-main}"

step() { printf '\n\033[1;34m== %s ==\033[0m\n' "$*"; }

step "install (frozen lockfile)"
pnpm install --frozen-lockfile --prefer-offline >/dev/null

step "generated output must be clean"
pnpm gen >/dev/null
if [[ -n "$(git status --porcelain -- . ':!pnpm-lock.yaml')" ]]; then
  echo "generated output is dirty — run pnpm gen and commit:"; git status --short; exit 1
fi

if [[ -n "$BASE" ]]; then
  step "contract guard vs $BASE"
  changed="$(git diff --name-only "$BASE"...HEAD -- packages/schema packages/proto 'apps/agent/gen' 'packages/proto/gen' 'packages/api-client/src/generated' || true)"
  if [[ -n "$changed" ]]; then
    # a contract change is allowed only when the branch carries a commit whose subject starts with 'contract'
    if ! git log --format=%s "$BASE"..HEAD | grep -qiE '^contract(\(|:|!)'; then
      echo "contract files changed without a 'contract:' commit:"; echo "$changed"; exit 1
    fi
  fi
fi

step "forbidden patterns"
if grep -rnE "child_process|execSync|exec\.Command|sh -c|bash -c" apps/api/src apps/web/src packages/*/src 2>/dev/null | grep -v "^.*ALLOW:" ; then
  echo "shell execution found in the control plane — not allowed"; exit 1
fi
if grep -rnE "^(FROM |services:)" --include=Dockerfile --include='*compose*.yml' -r . 2>/dev/null | grep -v node_modules; then
  echo "Docker/compose files are not allowed in this repo"; exit 1
fi

step "typecheck"; pnpm typecheck >/dev/null
step "lint";      pnpm lint >/dev/null
step "test";      pnpm test >/dev/null
step "build";     pnpm build >/dev/null
step "agent";     (cd apps/agent && make lint >/dev/null && make test >/dev/null && make build >/dev/null)
echo; echo "CI GATE PASSED"
