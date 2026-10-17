# Combined review — boot-nic-registration-port (PR #238)

Branch `claude/boot-nic-registration-port`, base `origin/main` `99a774734`. Panel per `prompts/REVIEW-PROMPT.md`: R1, R2, R7
(mandatory), R8 (systemd unit / packaging test), R6 (`docs/user/**`), R4 (data-plane ownership of NICs, added on R7's request).
Reviewers were read-only agents (Opus 5.5); their reports were handed back in chat and saved unedited by the developer with a
"Developer answers" section in each file. Final head: the single commit that carries this file (SHA on PR #238 and in the manager's report) (one commit on `origin/main` `c3ef9bf36`); quick gate: `tools/ci.sh quick` CI GATE PASSED on `e41f496ee`, the integration commit before the docs-only amendment that carries this line (13m37s, clean tree); hosted Mandatory quick gate on the PR. Pre-rebase: CI GATE PASSED on `ab6d223b2` (12m22s).

| reviewer | round 1 (`814532721`) | round 2 (`db5042e38`) | round 3 (`9574d997a`) | BLOCKER / MAJOR / MINOR (open) | file |
|---|---|---|---|---|---|
| R1 correctness & tests | — | BLOCK (2 MAJOR, 4 MINOR, 4 NIT) | verify: APPROVE WITH CHANGES — both MAJORs fixed with tests; requested tests landed in iteration 6 | 0 / 0 / 0 | `boot-nic-registration-port-review-R1.md` |
| R2 security | APPROVE WITH CHANGES (3 MINOR, 3 NIT) | fixes answered (iteration 2/3) | — | 0 / 0 / 0 (one pre-existing item → tech-debt) | `…-review-R2.md` |
| R4 data plane / shared host | — | BLOCK (1 MAJOR pre-existing agent defect, 4 MINOR, 3 NIT) | verify: BLOCK → APPROVE WITH CHANGES; the MAJOR is fixed by PR #241 (merged before this branch), doc rewording done, new MINOR #9 fixed in iteration 5 | 0 / 0 / 0 | `…-review-R4.md` |
| R6 user docs | — | APPROVE WITH CHANGES (8 MINOR, 9 NIT) | fixes in iteration 3 | 0 / 0 / 0 | `…-review-R6.md` |
| R7 docs & evidence | BLOCK (1 BLOCKER evidence, 2 MAJOR) | verify: APPROVE WITH CHANGES (conditions: gate evidence, PR body, wording) | conditions met in iterations 3–6 | 0 / 0 / 0 | `…-review-R7.md` |
| R8 operability & packaging | BLOCK (1 MAJOR) | verify: APPROVE (new MINOR N1 fixed in iteration 3) | — | 0 / 0 / 0 | `…-review-R8.md` |

## Combined verdict: APPROVE WITH CHANGES — the R4 condition is closed by PR #241 (merged first); merge only on the owner's go-ahead

1. **R4 #1 (MAJOR, pre-existing on `main`):** the agent's `allInterfaces` block lists and automatic blocking targeted unbound and
   released physical rows (`apps/agent/internal/desired/global_blocking.go`). Fixed by **PR #241** (reviewer option (a)), merged to
   `main` as `c3ef9bf36` before this branch; the branch is re-based onto it, the debt row is closed with a pointer to #241 and the
   user-doc exception is removed.
2. Every other BLOCKER/MAJOR is fixed in-branch with a test; every MINOR is fixed or answered; NITs answered.
3. Not run (all reviewers agree): `apps/api/test/e2e/default-vpp-nics.e2e.test.ts` (integration only), `tools/ci.sh full`, real
   appliance evidence (`docs/status/DEFERRED-ACCEPTANCE.md` row with data-plane evidence (a)–(g)).
4. Tester reports: none (no lab access; deferred per AGENTS.md). Pre-existing on `main`, untouched: `docs/user/README.md` drift.

The owner decides whether this port lands or the behaviour is set aside (audit 2026-10-17 §1); the go-ahead is to be recorded on the PR.
