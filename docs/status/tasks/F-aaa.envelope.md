# TASK ENVELOPE — F-aaa
id: F-aaa   branch: task/F-aaa   worktree: /root/ngfw-wt/F-aaa   base: main@<BASE>   started: <STARTED>
title: S5 system (day 16-18): external AAA for management login (RADIUS, TOTP MFA, LDAP, OIDC, TACACS+, SAML)
prompt: prompts/features/F-aaa.md   (template: prompts/FEATURE-TEMPLATE.md; refreshed on task/prep-rest against main@11a175b + task/TD-2, task/TD-4)   wbs: D10.2
scope: vrx-api only: an `AuthBackend` interface with RADIUS → TOTP MFA → LDAP → OIDC → TACACS+ → SAML in that priority order (stop at the time box, list what is left); `management.aaa` contract + proto mirror; login/MFA/OIDC routes; AAA settings and MFA screens; the Users page password change moves to `POST /api/v1/users/{name}/password` (D-102). The agent is not touched (D-040)
merged deps you can rely on: P06, P07b, P08 (+ TD-2 and TD-4 — the manager should add TD-4 as a board dep: until it merges it owns apps/api/src/{auth,users}/**)
  - P06: argon2id local users, JWT + rotating refresh cookie, API keys, roles admin/operator/readonly, lockout; the secrets service (AES-GCM with the name as AAD, `<kind>/<name>` refs, admin-only writes, versioned, D-091); audit; RBAC
  - TD-2: `app_user` credential generation, D-097 reset path (sessions + refresh chains + API keys revoked), `POST /api/v1/users/{name}/password`, the secure-transport check, D-102 post-promote hook
  - TD-4: D-100 (1)–(3): login transport check, API-key creation from a JWT session needs the current password, disabling bumps the credential generation
  - P07b: `apps/web/src/pages/{LoginPage,UsersPage}.tsx`, `apps/web/src/shell/UserMenu.tsx`, `apps/web/src/auth/{session.ts,AuthProvider.tsx}` · P08: the `apps/web/src/domains/` layout
read first: prompts/features/F-aaa.md · docs/status/wave-BC-numbers.md (section "S5 system": pack rules SY1–SY9 + "F-aaa") · docs/status/wave-A-hotspots.md (§0 rules; C1–C7 P1 W1–W3 D4) · docs/status/tasks/TD-2.md + TD-4.md (merged) · apps/api/src/auth/route-guard.test.ts · apps/agent/internal/contracttest/drift_test.go (header) · docs/decisions/LOG.md D-003, D-040, D-046, D-051, D-091, D-097, D-100, D-102
slot: <SLOT> → VRX_SLOT=<SLOT> VRX_TEST_PREFIX=w<SLOT> VRX_HTTP_PORT=3000+100·<SLOT> VRX_WEB_PORT=5000+100·<SLOT> VRX_METRICS_PORT=9100+10·<SLOT>+1 VRX_AGENT_SOCKET=/run/vrx-test/w<SLOT>/agent.sock VRX_PG_DATABASE=vrx_w<SLOT> VRX_VALKEY_DB=<SLOT> VRX_VPP_TABLE_BASE=<SLOT>000 VRX_LAB_LOCK=/run/lock/vrx-lab.lock
  - source of truth: `eval "$(tools/lab env <SLOT>)"`
  - in-process fake servers listen on 127.0.0.1 only: RADIUS udp 3000+100·<SLOT>+71, LDAP tcp +72, TACACS+ tcp +73, OIDC provider (discovery/token/JWKS) +74 — never 1812/389/49/443
  - test users and groups start with w<SLOT>; test shared secrets are the literals `VRX_TEST_PSK_FAAA_*`
  - slots 1–11 only; 12 is CI
daemon-owner: none (freeradius, slapd and tac_plus are not installed and are never installed; in-process fakes are test fixtures)
obligations:
  - D-040: nothing crosses the API↔agent boundary; the proto mirror exists only for the drift guard, and the agent ignores it
  - drift guard: every new non-secret leaf is mirrored in `ManagementAaa` 4–9 + `Aaa*` messages (wave-BC-numbers "F-aaa"); a `secret: true` leaf gets no proto field. Secret refs are mirrored strings
  - D-046/D-051: shared secrets, bind passwords and client secrets are `psk/…`, `password/…`, `token/…` refs, resolved from the secret store at use time. They never appear in logs, audit rows, problem+json, fixtures or status files, and are never cached. TOTP seeds are encrypted at rest; recovery codes are hashed (argon2id) and shown once
  - D-097/D-102: hashes live only in `app_user`; external principals have no local hash, and an admin reset or disable still revokes their sessions and keys. The Users page stops staging `passwordHash` and calls `POST /api/v1/users/{name}/password`
  - D-100: API-key creation from a JWT session needs a step-up. For external principals, either re-authenticate against the backend that logged them in or refuse; pick one and log it with options. Login keeps the transport check
  - defaults (open questions, flag them): local login only when every external server is unreachable (not on reject); several mapped groups → highest privilege; unmapped → 403 problem+json; `aaa.order: [radius]` with no servers → 400 with `pointer`
  - no shell (rule 9): RADIUS (RFC 2865 PAP + Message-Authenticator per RFC 3579) and TACACS+ codecs in TS on `node:crypto`, or a maintained npm client. New npm deps are D4 (questions file; they need the registry)
  - failed external logins count toward P06's lockout; the rate limit is unchanged
files you own exclusively:
  - apps/api/src/features/aaa/** (index.ts exports {controllers, providers}; `AaaController`; backends, TOTP, OIDC, SAML if time allows; unit-test fakes)
  - apps/api/test/e2e/aaa*
  - packages/schema/src/domains/ext/aaa*.ts, packages/schema/src/semantic/aaa*.ts (rule ids `management.aaa-…`), packages/schema/examples/aaa-*.json, packages/proto/test/fixtures/aaa-*.json
  - apps/web/src/domains/system/aaa/**, apps/web/src/locales/{en,fa}/aaa.json
  - docs/user/system/aaa.md, test/topology/aaa/**, docs/status/tasks/F-aaa*
  - minimal named hunks (no other S5 task touches them; list each in F-aaa.md): apps/api/src/auth/{auth.controller,auth.service}.ts (login walks `aaa.order` through ONE call into features/aaa; the `mfaRequired` challenge) · apps/web/src/pages/LoginPage.tsx (MFA step) · apps/web/src/pages/UsersPage.tsx (password change → `POST /users/{name}/password`) · apps/web/src/shell/UserMenu.tsx (MFA enrolment entry) · apps/web/src/auth/{session.ts,AuthProvider.tsx} (the challenge step)
shared hotspots (append-only, conflicts resolved by the manager at merge; ids from docs/status/wave-A-hotspots.md §1 and docs/status/wave-BC-numbers.md "S5 system"):
  - protocol: docs/status/wave-A-hotspots.md §0. Insert only directly below your own anchor `// wave-BC: F-aaa` (the manager seeds it before spawn; if absent, at the end of the block). Never reorder or reformat other lines. List every hunk under "Shared hunks" in docs/status/tasks/F-aaa.md
  - SY4 packages/schema/src/domains/management.ts: `AaaSchema` only. Widen `AuthMethod` (+ `ldap`, `oidc`, `saml`) and `order.max(3)` in place; one key line per new key (sub-schemas in ext/aaa.ts). `ManagementSchema` and `SyslogTargetSchema` are other tasks'
  - C2 packages/schema/src/semantic/index.ts: one spread · C3 packages/schema/src/index.ts: one export if needed · C4: new fixture files only
  - C5 packages/proto/vrx/v1/dataplane.proto: `ManagementAaa` 4–9 as one-line insertions inside P02a's block (you are its only toucher); `Aaa*` messages in a `// ----- F-aaa -----` section at the end
  - C7 generated, regenerated and never hand-edited: apps/agent/gen/**, packages/proto/gen/ts/**, packages/api-client/src/generated/**, apps/cli/internal/api/operations_gen.go, docs/user/cli/reference.md (`packages/proto/gen.sh`, `pnpm gen`, `make -C apps/cli gen docs`)
  - SY1 apps/api/src/auth/route-guard.test.ts: PUBLIC += MFA verify, OIDC start, OIDC callback; ADMIN_ONLY += `POST /api/v1/actions/aaa/test`
  - SY3 apps/api/src/db/schema.ts + apps/api/migrations/**: your tables under the anchor; migration via `pnpm -C apps/api db:generate --name f_aaa_mfa`; regenerated on top of main at merge, never hand-merged
  - SY8 pnpm-lock.yaml: e.g. `ldapts`, `openid-client`, a QR helper; `@node-saml/node-saml` only if SAML is reached
  - P1 apps/api/src/app.module.ts · W1 apps/web/src/router.tsx · W2 apps/web/src/nav/nav.ts (+ nav.test.ts): non-domain system NavItem `aaa` · W3 apps/web/src/i18n.ts
contract: `contract(schema): aaa ldap/oidc/saml/mfa` and `contract(proto): aaa mirror` as separate commits on YOUR branch first (the ci.sh contract guard checks the subject), plus docs/status/tasks/F-aaa-contract.md. Tell the manager in the questions file and keep building. No own branches
contract numbers: **ManagementAaa 4 `ldap` · 5 `oidc` · 6 `saml` · 7 `role_map` · 8 `mfa` · 9 `fallback_local`** (docs/status/wave-BC-numbers.md "F-aaa"; reusing one is a merge blocker); nothing else
files you must not touch:
  - everything else
  - never: /root/ngfw (main), other worktrees, /etc (incl. /etc/pam.d, /etc/ssh), /root/vpp
  - apps/agent/** (D-040), apps/api/src/{secrets,commit,datastore,config,users}/** (use the services; a need → questions file), apps/api/src/actions/** (F-vrf-static-ecmp), other features' directories
  - packages/ui-kit/**, apps/agent/binapi, tools/ci.sh, tools/lab, plan/tasks.yaml, docs/decisions/LOG.md, docs/status/PROGRESS.md
host facts:
  - freeradius, slapd, ldapsearch and tac_plus are not installed → in-process fakes only, never apt-get
  - no external IdP: OIDC is tested against an in-process fake provider on the slot port; SAML likewise, or skipped at the time box
  - new npm packages need the registry; if it is unreachable, write it in the questions file and build the codecs on `node:crypto`
coordination:
  - F-backup-restore: TOTP seeds and recovery codes are account data, not config. Name the table that holds them in F-aaa.md; F-backup-restore does not restore accounts by default (D-097)
  - F-hardening-lite decides OS/SSH login policy (PAM-RADIUS is out of scope here) · F-pki: LDAPS CAs are `cert/<name>` refs, no CA UI
  - F-restconf-yang and F-backup-restore (same stage) share only the system nav block, router/i18n, app.module and route-guard anchors
evidence: RADIUS login → JWT with the mapped role, wrong password → 401, server down + `fallbackLocal` → local admin (pasted); TOTP enrol/login/replay/recovery test output; grep over logs, fixtures, audit rows and GET `/config/management` showing no secret or seed. Playwright is not installed: screenshots (AAA page + MFA login step, en + fa/RTL) with the headless Chrome approach from P07a/P07b/P08, kept outside the product code, and say so
time box: 15 h — when exceeded: stop, commit WIP, write docs/status/tasks/F-aaa.md with what is left
WIP: commit at least every 45 min; keep docs/status/tasks/F-aaa-wip.md current
CI: `TMPDIR=/tmp/g-w<SLOT> tools/ci.sh --base main` — short TMPDIR (unix socket paths ≤ 108 chars); no host-wide CI lock: golangci-lint serializes itself since main fc0fe68 (D-106 rejected serialising whole gates). Ports 3000/8080/9101 and /run/vrx/agent.sock belong to the running product stack (tools/app) — never touch them
finish: `tools/ci.sh --base main` green in the worktree · docs/status/tasks/F-aaa.md with pasted real output · everything committed · final message = 10-line summary (branch, last commit, CI result, evidence, open questions, decisions taken with options)
cleanup: stop every process you started (API/agent/vite/fake servers), by PID · lab lock released if taken · vrx_w<SLOT> dropped · Valkey db <SLOT> keys of your runs deleted · dist/ removed
questions: docs/status/tasks/F-aaa-questions.md — write and keep going; never wait for a human
never: merge · restart/kill VPP · Docker · pkill · secrets in files · edit files you do not own
