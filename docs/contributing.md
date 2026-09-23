# Contributing — the gate, the contract rule, the shared host, how merges happen

Git on the dev host is **local-only**: no remote, no pull requests, no CI service. `tools/ci.sh` **is** the CI. The manager runs
it in every worker's worktree before merging and on `main` after merging; a worker runs it before declaring a task done. This
page is the contract around that script. Architecture and security rules live in `prompts/00-CONTEXT.md`; shared-host rules in
`docs/lab/shared-host-rules.md`; the merge procedure in `prompts/MANAGER-PROMPT.md` §2.

## TL;DR for a worker

```bash
tools/ci.sh check --base main   # ~2 s: forbidden patterns + contract guard — run before every commit
tools/ci.sh                     # quick gate (unit-only), same as `tools/ci.sh quick`
tools/ci.sh --base main         # what the manager runs before merging your branch — must print CI GATE PASSED
pnpm gen:check                  # regenerate + verify you did not hand-edit generated code
```

Nothing is merged unless `tools/ci.sh --base main` ends with the line `CI GATE PASSED` in your worktree.
`main` after a merge must pass a bare `tools/ci.sh` too — with golangci-lint installed, because `apps/agent/Makefile` `lint`
fails on linter findings (D-031). A lint finding is fixed in the code or reported in the status file, never by weakening the
linter or its configuration.

## The gate: `tools/ci.sh`

| Command | What it does | Budget |
|---|---|---|
| `tools/ci.sh` / `tools/ci.sh quick` | unit-only gate, see the step table below | < 6 min; < 2 min on an unchanged tree |
| `tools/ci.sh --base <ref>` (any mode) | + contract guard and branch checks against `<ref>`; the manager uses `--base main` | +1 s |
| `tools/ci.sh full` | quick, then integration under the exclusive lab lock on the CI slot (12) with the packet rig | depends on suites |
| `tools/ci.sh check [--base <ref>]` | only the forbidden-pattern greps (+ contract guard) | ~2 s |
| `tools/ci.sh gen-check` | only `pnpm gen` + the generated-output gate (`pnpm gen:check` calls this) | ~15 s |
| `tools/ci.sh install-tools` | golangci-lint + gitleaks, pinned versions, sha256-verified, into `/usr/local/bin` | network |
| `tools/ci.sh install-hooks [repo]` | `.git/hooks/pre-merge-commit` that runs `quick` on the merged tree | — |

Backward compatibility is a rule: a bare `tools/ci.sh` is `quick`, and `tools/ci.sh --base main` keeps working — the manager's
procedure depends on both.

### Steps of `quick`, in order

| # | Step | Fails when |
|---|---|---|
| 0 | contract guard (`--base` only; git-only, ~1 s, so a contract-less branch fails with the root-cause message before anything is built) | see [The contract rule](#the-contract-rule) |
| 1 | tools | `golangci-lint` / `gitleaks` missing and the download from GitHub fails (`VRX_CI_ALLOW_MISSING_TOOLS=1` downgrades to a warning — never for a merge) |
| 2 | `pnpm install --frozen-lockfile --prefer-offline` | the lockfile is stale (you added a dependency: run `pnpm install` once, commit `pnpm-lock.yaml`) |
| 3 | `pnpm gen` + **generated-output gate** | anything under `packages/proto/gen`, `apps/agent/gen`, `packages/schema/dist`, `packages/api-client/src/generated` differs from what the generators produce (working tree vs index after `pnpm gen`, plus untracked files there — so a staged merge inside the hook is not "dirty"), or `go mod tidy` (run by the proto generator) changed `apps/agent/go.mod`/`go.sum`. Only these paths are checked — other uncommitted files just produce a warning. A file in a generated directory that no generator writes (`.gitkeep`) is covered by the contract guard, not by this step |
| 5 | forbidden patterns | see [Forbidden patterns](#forbidden-patterns) |
| 6 | `turbo run lint typecheck test build` | ESLint, `buf lint`, `tsc`, Vitest unit tests or a build fails. Run as one turbo invocation so `gen` (uncached by design) runs once, not four times; `VRX_INTEGRATION` is unset — this is unit-only |
| 7 | `make -C apps/agent lint test build` | `go vet`, `golangci-lint run` (config `apps/agent/.golangci.yml`), `go test -race -count=1`, `go build` fail — or golangci-lint silently did not run (D-031: the Makefile's `lint` fails on linter findings) |
| 8 | every Go module under `test/` in unit mode (`gofmt -l`, `go vet ./...`, `go test -count=1 ./...`) | a `gofmt`/`vet`/compile regression in e.g. `test/integration/smoke` (its own module, `replace ngfw/agent => ../../../apps/agent`), or a stale `go.sum` there after `apps/agent/go.mod` grew (`missing go.sum entry` — run `go mod tidy` in the module and commit). The integration tests inside `t.Skip` without `VRX_INTEGRATION`; this step only proves they compile and vet (P04 review F6). Skipped silently while `test/` has no `go.mod` |

Every step's output goes to a log file (`/root/ngfw-wt/logs/ci/<worktree>-<timestamp>-<pid>/NN-<step>.log`, or `$TMPDIR/vrx-ci`
elsewhere); only the failing step's tail is printed. `--verbose` / `VRX_CI_VERBOSE=1` streams everything. The summary at the end
lists each step with its duration and the total wall time. The final line is exactly `CI GATE PASSED`; any failure prints
`CI GATE FAILED — <reason>` and exits non-zero.

### `full` — integration on the CI slot

`full` runs the whole quick gate first (no lock held), then:

1. takes **`flock -x /run/lock/vrx-lab.lock`** (integration harnesses hold it shared; VPP restarts — manager-only, after handover —
   exclusive), waiting up to `VRX_CI_LOCK_TIMEOUT` (1800 s);
2. exports **slot 12** — the CI slot from `docs/lab/shared-host-rules.md` §1 — from `tools/lab env 12` (values are validated,
   never `eval`'d); anything it does not print comes from the same arithmetic `tools/lab` uses (D-025 for the metrics port):
   `VRX_SLOT=12 VRX_TEST_PREFIX=w12 VRX_HTTP_PORT=4200 VRX_WEB_PORT=6200 VRX_METRICS_PORT=9221 VRX_AGENT_SOCKET=/run/vrx-test/w12/agent.sock
   VRX_PG_DATABASE=vrx_w12 VRX_VALKEY_DB=12 VRX_VPP_TABLE_BASE=12000`;
3. `tools/lab status`, then `tools/lab rig up w12` (idempotent: a suite that runs `rig up`/`rig down` on the same prefix itself,
   like the smoke test, just reuses it);
4. **converts its lock to shared** (`flock -s` on the same descriptor) and exports `VRX_LAB_LOCK_HELD=1 VRX_CI_FULL=1`, then runs
   `VRX_INTEGRATION=1 go test -race -count=1 -timeout 20m ./...` in every Go module under `apps/agent` and `test/`, then
   `VRX_INTEGRATION=1 pnpm -r --workspace-concurrency=1 --if-present run test:integration`.
   Why shared: every integration harness takes its own `flock -s /run/lock/vrx-lab.lock` (the convention in `00-CONTEXT.md`;
   `test/integration/smoke/smoke_test.go` does it on a fresh file description). Against the gate's exclusive lock that call would
   block until `go test` times out — flock is per open file description, the process tree does not matter. Holding the lock
   shared keeps the protection that matters (no VPP restart — those take the exclusive lock — can start underneath the suites)
   while other harnesses may run beside the gate on their own prefixes; harnesses may skip their own lock when
   `VRX_LAB_LOCK_HELD=1`;
5. re-acquires the **exclusive** lock (same timeout), `tools/lab rig down w12` (also on failure, via the exit trap — the rig is
   torn down whatever happened), release the lock.

It **never restarts VPP**. If `tools/lab` is not in the tree (P04 not merged) it prints a loud `WARNING: integration NOT RUN` and
still passes — the quick gate ran; set `VRX_CI_REQUIRE_INTEGRATION=1` to turn that into a failure. Integration tests follow the
conventions in `00-CONTEXT.md`: Go tests `t.Skip` unless `VRX_INTEGRATION=1`; TS suites live behind a `test:integration` script;
every object created on VPP/PostgreSQL/Valkey carries `VRX_TEST_PREFIX`, and cleanup happens in `t.Cleanup`. Slot 12 belongs to
the gate: no worker uses it. Only the manager runs `full`, on `main`, at most once per hour.

## Generated code is never hand-edited

`packages/proto/gen`, `apps/agent/gen`, `packages/schema/dist` and `packages/api-client/src/generated` are outputs of `pnpm gen`
(`buf generate` → Go/TS stubs; Zod → JSON Schema/OpenAPI components; NestJS → OpenAPI → `openapi-typescript`). They are committed
so that a checkout builds without generators, and the gate regenerates them and fails if the committed files differ. Change the
source (`.proto`, `packages/schema/src`, controllers in `apps/api`), run `pnpm gen`, commit the result. If the gate says
`GENERATED OUTPUT IS DIRTY`, that is what happened — or a generator version differs from the one that produced the committed
files (protoc-gen-go, protoc-gen-go-grpc, ts-proto, buf are pinned on the host; do not upgrade them inside a task).

## The contract rule

`packages/schema`, `packages/proto`, `apps/agent/gen` and `packages/api-client/src/generated` are the **contract** between the
UI, the API and the agent. When a branch changes any of them, it must carry at least one commit whose subject starts with
`contract(<pkg>): …` (`contract:` / `contract!:` also match). `tools/ci.sh --base main` fails otherwise:

```
CI GATE FAILED — CONTRACT FILES CHANGED WITHOUT A CONTRACT COMMIT. [...] Changed files: packages/proto/vrx/v1/dataplane.proto ...
```

How to do it right (FAST MODE, `00-CONTEXT.md`): commit the contract change **first** as `contract(schema): add dhcp.server`
(or `contract(proto): …`, `contract(api): …` for a REST surface change that regenerates the client), tell the manager in
`docs/status/tasks/<id>-questions.md`, keep building on your branch. *Additive* changes that follow `docs/04-api-datamodel.md`
are ordinary; *renaming or reshaping* existing fields is always a PENDING decision (`docs/decisions/decision-policy.md` #1).
The manager reviews every `contract(` commit before merging.

## Forbidden patterns

All greps run on tracked and untracked (not ignored) files with `git grep`, so `node_modules`/`dist` never leak in. A hit fails
the gate; the offending `file:line` is printed.

| Rule | Where | Pattern (summary) | Why |
|---|---|---|---|
| no shell, no direct VPP, no FFI in the control plane | `apps/api/src`, `apps/web/src`, `packages/*/src` | `child_process`, `execSync`, `execFileSync`, `spawnSync`, `exec.Command`, `sh -c`, `bash -c`, `vppctl`, `/run/vpp/`, `govpp`, `ffi-napi`, `node-ffi`, `koffi` | 00-CONTEXT rules 1 (Node never talks to VPP) and 9 (no user input reaches a shell). Escape hatch: `ALLOW: <justification>` **on the same line** as the hit (e.g. a trailing comment) — the gate prints every allowed line as a warning for the reviewer |
| no Docker | whole tree | file names `Dockerfile*`, `Containerfile*`, `.dockerignore`, `*compose*.yml/yaml` | D-002: VMware VMs, never Docker |
| no kill-by-pattern | every file except `docs/`, `prompts/`, `wbs/`, `*.md` | an *invocation* of either process-killing-by-name command: at command position (start of line, after `;`/`&`/`|`/`(`, `sudo …`, `$(…`) or as a quoted program name (`["…", "-f"]`, `os.system("… -f x")`); prose mentions do not count | shared host: kill only PIDs you spawned (`shared-host-rules.md` §5) |
| no secret shapes | whole tree except `pnpm-lock.yaml` | private-key blocks, AWS/GitHub/Slack/Stripe token shapes, JWTs, URLs with an embedded password | secrets never enter the repo; `VRX_TEST_PSK_<id>` and `<redacted>` are exempt |
| gitleaks | commit history: the branch's commits (`merge-base..HEAD`) with `--base`, else the last 500 commits of `HEAD` (never other refs — a leak on an unmerged branch must not fail `main`) | default gitleaks rules + `.github/gitleaks.toml` allowlist (placeholder PSKs, `<redacted>`, lockfile, `wbs/`, `apps/web/src/locales/`) | a secret in history stays in history — the branch must be recreated, not fixed forward |

Do not widen `.github/gitleaks.toml` or add `ALLOW:` to make a branch pass; state the reason in your status file and let the
manager decide.

## Shared host rules (summary — the full text is `docs/lab/shared-host-rules.md`)

Up to 12 workers share one VPP, PostgreSQL, Valkey and disk. Your envelope gives you a slot `N`: prefix everything you create with
`VRX_TEST_PREFIX=w<N>`, use your ports (`3<N>00`, `5<N>00`, `91<N>1`), your table range and your database. Only your worktree and
branch; never `/root/ngfw`, other worktrees, `/etc/vpp`, `/root/vpp`. Kill only PIDs you spawned. No VPP restarts while
`docs/lab/host-vrx-a.md` says `handover: pending`. Integration tests only with `VRX_INTEGRATION=1` under the shared lock; `pnpm test`
and `make test` are unit-only. Slot 12 and the exclusive lock are the gate's.

## How the manager merges (from `prompts/MANAGER-PROMPT.md` §2)

1. `git -C /root/ngfw status --porcelain` is empty (board and status committed first).
2. In the worker's worktree: `tools/ci.sh --base main` → must end with `CI GATE PASSED` (contract guard included).
3. `git -C /root/ngfw merge --no-ff task/<id>`. With the hook installed (`cd /root/ngfw && tools/ci.sh install-hooks`),
   `pre-merge-commit` runs `tools/ci.sh quick --base HEAD` with `VRX_CI_HEAD_REF=MERGE_HEAD`: the quick gate on the merged
   tree plus the contract guard and gitleaks on the commits being merged. A red gate aborts the merge commit; the working tree
   keeps the merge result for inspection (`git merge --abort` to drop it). `git merge --no-verify` bypasses the hook — only
   deliberately, and logged in the status entry. Git runs this hook only when it creates a merge commit: never for
   fast-forward or `--squash` merges, which is why the procedure says `--no-ff`.
4. If the merge touched `pnpm-lock.yaml` or `--frozen-lockfile` fails: `pnpm install` once on `main`, commit the lockfile.
5. `tools/ci.sh` on `main`; red → `git revert -m 1 <merge>` and reopen the task with the log attached.
6. `tools/ci.sh full` on `main`, serialized under the lab lock, at most once per hour, never while a worker's envelope says it is in
   its integration phase; record the result in `docs/status/`.
7. Green → `git worktree remove /root/ngfw-wt/<id>`, `git branch -d task/<id>`, board → merged.

## Speed and caches

- **pnpm store** is shared by every worktree (root's default store, `pnpm store path` → `/root/.local/share/pnpm/store/v11`);
  `--prefer-offline` avoids registry round-trips. A worktree costs ~300 MB, not 1.5 GB.
- **Go build cache** `/root/.cache/go-build` (Go's default) and the module cache are shared per user; `GOTOOLCHAIN=local` so Go never
  tries to download a toolchain (`dl.google.com` is not reachable from the host).
- **turbo cache** is shared across worktrees through `TURBO_CACHE_DIR=~/.cache/vrx-turbo` (exported by the gate); identical package
  inputs hit the cache in every worktree, which is what makes quick on an unchanged tree take about a minute.
- golangci-lint keeps its own cache under `~/.cache/golangci-lint`.

Measured on the dev host (30 vCPU), 2026-09-23: quick with a cold turbo cache 1m04s; the hook-guarded merge of task/P09 into
main (16/30 turbo tasks cached) 52 s; quick on the unchanged merged main (24/30 cached) **39 s**; `check --base main` 2–3 s
(the outputs are pasted in `docs/status/tasks/P09.md`).

## Tool versions (pinned in `tools/ci.sh`; `tools/ci.sh install-tools` installs exactly these)

| Tool | Version | Source | Where |
|---|---|---|---|
| golangci-lint | 2.13.2 (built with go1.27.0) | github.com/golangci/golangci-lint releases, sha256 verified against the release checksums | `/usr/local/bin/golangci-lint`, used by `apps/agent/Makefile` `lint` |
| gitleaks | 8.30.1 | github.com/gitleaks/gitleaks releases, sha256 verified | `/usr/local/bin/gitleaks`, config `.github/gitleaks.toml` |
| buf | 1.73.0 | installed on the host (P01) | `/usr/local/bin/buf` |
| Node / pnpm | 22.23.2 / 12.5.1 (`packageManager` in `package.json`) | host | — |
| Go | 1.26.0 (`apps/agent/go.mod`: `go 1.26`) | host | `/usr/local/go` |
| turbo | 2.11.x (root devDependency) | lockfile | — |

Bumping a pinned version is a one-line change in `tools/ci.sh` plus this table; do it in its own commit.

## GitHub Actions

`.github/workflows/ci.yml` is a thin wrapper that installs the toolchain and calls `tools/ci.sh quick` (`--base origin/<base>` on
pull requests). Nothing runs it today — the repository has no remote — and it is not part of any acceptance. Keep the logic in
`tools/ci.sh`; the workflow only sets up node/pnpm/go/buf/protoc plugins and uploads the step logs as an artifact.
