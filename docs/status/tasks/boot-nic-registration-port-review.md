# Combined review — boot-nic-registration-port (PR #238)

Branch `claude/boot-nic-registration-port`, base `origin/main` `99a774734`. Panel per `prompts/REVIEW-PROMPT.md`: R1, R2, R7
(mandatory), R8 (systemd unit / packaging test), R6 (`docs/user/**`), R4 (data-plane ownership of NICs, added on R7's request).
Reviewers were read-only agents (Opus 5.5); their reports were handed back in chat and saved unedited by the developer with a
"Developer answers" section in each file. Final head: `ab6d223b2` (iteration 6); quick gate: `tools/ci.sh quick` CI GATE PASSED on `ab6d223b2` (12m22s, local); hosted Mandatory quick gate on the PR.

| reviewer | round 1 (`814532721`) | round 2 (`db5042e38`) | round 3 (`9574d997a`) | BLOCKER / MAJOR / MINOR (open) | file |
|---|---|---|---|---|---|
| R1 correctness & tests | — | BLOCK (2 MAJOR, 4 MINOR, 4 NIT) | verify: APPROVE WITH CHANGES — both MAJORs fixed with tests; requested tests landed in iteration 6 | 0 / 0 / 0 | `boot-nic-registration-port-review-R1.md` |
| R2 security | APPROVE WITH CHANGES (3 MINOR, 3 NIT) | fixes answered (iteration 2/3) | — | 0 / 0 / 0 (one pre-existing item → tech-debt) | `…-review-R2.md` |
| R4 data plane / shared host | — | BLOCK (1 MAJOR pre-existing agent defect, 4 MINOR, 3 NIT) | verify: BLOCK → APPROVE WITH CHANGES once the manager accepts the debt row (doc rewording done; new MINOR #9 fixed in iteration 5) | 0 / 1 pending manager acceptance / 0 | `…-review-R4.md` |
| R6 user docs | — | APPROVE WITH CHANGES (8 MINOR, 9 NIT) | fixes in iteration 3 | 0 / 0 / 0 | `…-review-R6.md` |
| R7 docs & evidence | BLOCK (1 BLOCKER evidence, 2 MAJOR) | verify: APPROVE WITH CHANGES (conditions: gate evidence, PR body, wording) | conditions met in iterations 3–6 | 0 / 0 / 0 | `…-review-R7.md` |
| R8 operability & packaging | BLOCK (1 MAJOR) | verify: APPROVE (new MINOR N1 fixed in iteration 3) | — | 0 / 0 / 0 | `…-review-R8.md` |

## Combined verdict: APPROVE WITH CHANGES — one condition open for the manager, merge only on the owner's go-ahead

1. **R4 #1 (MAJOR, pre-existing on `main`, widened by D-249):** the agent's `allInterfaces` block lists and automatic blocking target
   unbound and released physical rows (`apps/agent/internal/desired/global_blocking.go`). Not fixed in this branch (agent code, outside
   the port's scope); handled as reviewer option (b): `docs/tech-debt.md` row with the fix and the projection test (owner manager,
   proposed Due 2026-10-31) plus the user-doc exception. **The manager must confirm the row (owner + date, drop "proposed") or have
   the small agent fix land first.**
2. Every other BLOCKER/MAJOR is fixed in-branch with a test; every MINOR is fixed or answered; NITs answered.
3. Not run (all reviewers agree): `apps/api/test/e2e/default-vpp-nics.e2e.test.ts` (integration only), `tools/ci.sh full`, real
   appliance evidence (`docs/status/DEFERRED-ACCEPTANCE.md` row with data-plane evidence (a)–(g)).
4. Tester reports: none (no lab access; deferred per AGENTS.md). Pre-existing on `main`, untouched: `docs/user/README.md` drift.

The owner decides whether this port lands or the behaviour is set aside (audit 2026-10-17 §1); the go-ahead is to be recorded on the PR.
