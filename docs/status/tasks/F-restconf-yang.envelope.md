# TASK ENVELOPE — F-restconf-yang
id: F-restconf-yang   branch: task/F-restconf-yang   worktree: /root/ngfw-wt/F-restconf-yang   base: main@<BASE>   started: <STARTED>
title: S5 system (day 16-18): RESTCONF (RFC 8040) + YANG 1.1 generated from the Zod schema
prompt: prompts/features/F-restconf-yang.md   (template: prompts/FEATURE-TEMPLATE.md; refreshed on task/prep-rest against main@11a175b)   wbs: D8.8
scope: `packages/yang` (Zod → YANG generator, generated modules, golden tests), a RESTCONF layer in vrx-api that maps data resources onto the existing candidate/commit engine, a "download YANG" card, docs. NETCONF is cut (D-085) and listed as a have-not
merged deps you can rely on: P06, P07b, P08
  - P06: config pointer routes (`apps/api/src/config/`), commit engine (`apps/api/src/commit/commit.service.ts`: validate/commit/confirm/rollback), candidate lock, problem+json (`apps/api/src/common/problem.ts`), global AuthGuard + AuditInterceptor, RBAC on the resulting document (D-091)
  - P07b: web shell, SchemaForm, system pages · P08: the `apps/web/src/domains/` layout and the feature-module pattern
  - packages/schema: `RootConfig`, `withUi` meta, `secret: true` + `redactSecrets`/`secretPointers` (D-046, D-070), `pnpm gen` (gen.ts → JSON Schema / OpenAPI)
  - also on main by then: TD-2/TD-4 (auth, users, `route-guard.test.ts`), W-seed (wave-A anchors), F-sdk-terraform-ansible (sdk/ is generated from OpenAPI)
read first: prompts/features/F-restconf-yang.md · docs/status/wave-BC-numbers.md (section "S5 system": pack rules SY1–SY9 + "F-restconf-yang") · docs/status/wave-A-hotspots.md (§0 rules; P1 W1–W3 C7 D3 D4) · docs/04-api-datamodel.md · docs/contracts/schema.md · apps/api/src/app.ts · docs/decisions/LOG.md D-045, D-046, D-053, D-070, D-085, D-091, D-106
slot: <SLOT> → VRX_SLOT=<SLOT> VRX_TEST_PREFIX=w<SLOT> VRX_HTTP_PORT=3000+100·<SLOT> VRX_WEB_PORT=5000+100·<SLOT> VRX_METRICS_PORT=9100+10·<SLOT>+1 VRX_AGENT_SOCKET=/run/vrx-test/w<SLOT>/agent.sock VRX_PG_DATABASE=vrx_w<SLOT> VRX_VALKEY_DB=<SLOT> VRX_VPP_TABLE_BASE=<SLOT>000 VRX_LAB_LOCK=/run/lock/vrx-lab.lock
  - source of truth: `eval "$(tools/lab env <SLOT>)"`
  - no VPP work: the slot API (and the fake agent, or a slot agent creating only w<SLOT> objects) run only for the evidence
  - slots 1–11 only; 12 is CI
daemon-owner: none
obligations:
  - one source of truth (00-CONTEXT rule 5): YANG is generated from the Zod root schema, deterministic and checked in. The generator is a `gen` script in packages/yang, so `pnpm gen` regenerates it with the other generated files. Never hand-edit a module
  - no schema edits: YANG hints live in a side table in packages/yang keyed by JSON pointer (the 13 domain files are being extended by other features at the same time). A `contract(schema)` commit only if that is impossible, questions file first
  - D-046/D-070: `secret: true` leaves are write-only in YANG (`nacm:default-deny-all`) and never returned by RESTCONF GET: reuse `redactSecrets`/`secretPointers`; paste grep evidence over responses and generated modules
  - D-045/D-053: records → `list` keyed by the record key; arrays with itemKey meta → keyed `list`
  - RESTCONF writes take the SAME path as /api/v1: candidate + candidate lock + RBAC (checked on the resulting document and at commit, D-091) + audit. No second write path into the datastore. Default: an explicit `vrx:commit` operation (open question)
  - `/restconf/**` is excluded from OpenAPI by default (`@ApiExcludeController()`), so api-client, the CLI operations table and the SDK do not grow generic operations. If you include it: regenerate C7 + `make -C apps/cli gen docs` + `sdk/gen.sh`, and log the choice with options
  - errors: RFC 8040 §7 `ietf-restconf:errors` with `error-path` (the pointer equivalent), HTTP status per RFC 8040
  - pyang/yanglint are not installed: no host package installs. A worktree-local venv under `.scratch/` only if PyPI is reachable (pinned version), else the documented manual gate
files you own exclusively:
  - packages/yang/** (new workspace package: package.json with `gen` + `test` scripts, src/ generator + hints side table, modules/*.yang generated output, golden tests)
  - apps/api/src/features/restconf-yang/** (index.ts exports {controllers, providers}; `RestconfYangController` + host-meta; the `application/yang-data+json` parser registered from this module via `HttpAdapterHost`)
  - apps/api/test/e2e/restconf-yang*
  - apps/web/src/domains/system/restconf-yang/** (download-YANG link card only), apps/web/src/locales/{en,fa}/restconf-yang.json
  - docs/user/system/restconf-yang.md, test/topology/restconf-yang/**, docs/status/tasks/F-restconf-yang*
shared hotspots (append-only, conflicts resolved by the manager at merge; ids from docs/status/wave-A-hotspots.md §1 and docs/status/wave-BC-numbers.md "S5 system"):
  - protocol: docs/status/wave-A-hotspots.md §0. Insert only directly below your own anchor `// wave-BC: F-restconf-yang` (the manager seeds it before spawn; if absent, at the end of the block). Never reorder or reformat other lines. List every hunk under "Shared hunks" in docs/status/tasks/F-restconf-yang.md
  - P1 apps/api/src/app.module.ts: one import + one spread per array
  - SY2 apps/api/src/app.ts: only if the yang-data+json parser cannot be registered from your module, one entry under the anchor
  - SY1 apps/api/src/auth/route-guard.test.ts: only if `/.well-known/host-meta` becomes public (default: guarded, no edit)
  - W1 apps/web/src/router.tsx · W2 apps/web/src/nav/nav.ts (+ nav.test.ts): one non-domain NavItem `restconf-yang` in the `groups.get('system')!.push(…)` block · W3 apps/web/src/i18n.ts
  - SY8 pnpm-lock.yaml: the new workspace importer (plus pinned devDeps only if needed): `pnpm install` once, commit; the manager re-resolves at merge
  - SY9 + C7: `packages/yang/modules/**` is generated. Ask the manager (questions file) to add it to `GEN_PATHS` in tools/ci.sh; after that, every schema change regenerates it
contract numbers: none (docs/status/wave-BC-numbers.md "F-restconf-yang"). If a schema hint is unavoidable: a `contract(schema): …` commit on YOUR branch + docs/status/tasks/F-restconf-yang-contract.md; tell the manager and keep building. No own branches
files you must not touch:
  - everything else
  - never: /root/ngfw (main), other worktrees, /etc, /root/vpp
  - packages/schema/src/** (read-only; see contract numbers), packages/proto/**, apps/agent/** (not needed)
  - apps/api/src/{config,commit,datastore,auth,users,secrets}/** (consume the services, do not edit them; a need → questions file), apps/api/src/actions/** (F-vrf-static-ecmp)
  - apps/agent/binapi, tools/ci.sh, tools/lab, plan/tasks.yaml, docs/decisions/LOG.md, docs/status/PROGRESS.md, packages/ui-kit/**, generated files by hand
host facts:
  - Node 22.23, pnpm store /root/.pnpm-store (`--prefer-offline`); pyang/yanglint absent
  - the dev stack `tools/app` (port 8080) proxies only `/api`: take the RESTCONF evidence against your slot API port (VRX_HTTP_PORT). The product nginx route for `/restconf` is P10's (its prompt says so)
coordination:
  - P10: nginx proxies `/restconf` and `/.well-known/host-meta` over TLS; mention it in your status file
  - F-sdk-terraform-ansible (merged): if `/restconf` enters OpenAPI, the sdk/ output changes (default: excluded)
  - F-aaa and F-backup-restore (same stage): they share only the system nav block, router/i18n and app.module anchors with you
evidence: curl outputs (GET, PATCH, commit, rollback, error body) with tokens redacted. Playwright is not installed: take the YANG-card screenshots (en + fa/RTL) with the headless Chrome approach from P07a/P07b/P08 (`test/topology/interfaces/shots_test.go`, kept outside the product code), and say so
time box: 15 h — when exceeded: stop, commit WIP, write docs/status/tasks/F-restconf-yang.md with what is left
WIP: commit at least every 45 min; keep docs/status/tasks/F-restconf-yang-wip.md current
CI: `TMPDIR=/tmp/g-w<SLOT> tools/ci.sh --base main` — short TMPDIR (unix socket paths ≤ 108 chars); no host-wide CI lock: golangci-lint serializes itself since main fc0fe68 (D-106 rejected serialising whole gates). Ports 3000/8080/9101 and /run/vrx/agent.sock belong to the running product stack (tools/app) — never touch them
finish: `tools/ci.sh --base main` green in the worktree · docs/status/tasks/F-restconf-yang.md with pasted real output · everything committed · final message = 10-line summary (branch, last commit, CI result, evidence, open questions, decisions taken with options)
cleanup: stop every process you started (API/agent/vite), by PID · lab lock released if taken · vrx_w<SLOT> dropped · `.scratch/` venv removed · dist/ removed
questions: docs/status/tasks/F-restconf-yang-questions.md — write and keep going; never wait for a human
never: merge · restart/kill VPP · Docker · pkill · secrets in files · edit files you do not own
