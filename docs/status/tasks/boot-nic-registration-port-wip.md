# Boot-time NIC registration port — checkpoint

Branch: `claude/boot-nic-registration-port`; base `origin/main` `99a774734e2cf455f71563ff0dba5afb58051685`.
Remote: `origin/claude/boot-nic-registration-port`. Checkpoints: iteration 1 = `814532721a1e5728ff24f9419ca48e0332243c24`
(quick gate PASSED locally and hosted, below); iteration 2 = `db5042e38803304c1f5ed6756dc3720fb4d53928` (review findings
R2/R7/R8; quick gate PASSED locally, below; hosted run 39022880662); iteration 3 = `5fa3c01e4` (R7/R8 verify-round and R6
findings; not gated separately — superseded within minutes by iteration 4); iteration 4 = `9574d997a` (R1 and R4
findings; quick gate PASSED locally and hosted, below); iteration 5 = `c5356454c` (R4 verify-round findings: kernel-name
guard, doc exception; its local gate ran while iteration 6's test edits were being written, so it is superseded);
iteration 6 = `ab6d223b2` (R1 verify-round tests; **final head**, gated below).
PR: https://github.com/mcoder1001-cyber/NGFW/pull/238 (draft until the final gate is green and the review rounds are combined).
Owned files: see `boot-nic-registration-port-envelope.md`.

Manager decision (2026-10-17, after the review rounds): the NIC-registration decision is **D-252** (it was logged as D-249 on
this branch; PR #240 keeps D-249, #242 D-250, #241 D-251). Merge order: #240 → #242 → #241 → #238. PR #241 (the R4 agent
fix) is on `main` as `c3ef9bf36`, so this branch was re-based onto `origin/main` (`c3ef9bf36`) as **one commit** (D112 single-commit
integration; the reviewed nine-commit history is archived at `refs/heads/archive/boot-nic-registration-port-reviewed-009f9a423`).
Conflicts resolved: `docs/decisions/LOG.md` keeps D-249, D-250, D-251 and appends D-252; `docs/tech-debt.md` keeps the PR #241
section and adds this branch's rows under their own heading. Post-#241 changes: the R4 debt row is closed (`[x]`, pointer to
PR #241), the *Awaiting engine* exception paragraph and the released-row parenthetical are gone from the user doc (replaced by
the fixed behaviour: all-interfaces block lists and auto-block skip unbound rows; released NICs are never covered by engine
ACLs), DEFERRED case (e) and the PENDING note name PR #241. Final head and its gate: below.

## Completed code

- `apps/api/src/features/default-vpp-nics/seed.service.ts`: `SeedService` registers the non-management host NICs
  missing from the running document at every API start through one system commit. `planRegistration` (`seedDocument`
  = its `next`) matches NICs by PCI (case-insensitive), keeps `dataplane.devices.<pci>.name`, device tuning, the
  whitelist, existing rows and released (`owner: host`) NICs, adopts an existing row of the same name (settings kept,
  marker added) unless it is a physical row, a bond or a PPPoE client, skips reserved (generator `reservedNames` +
  `reservedStems`) and invalid names, never writes a management PCI the configuration hands to the engine into
  `managementPci` (reported as `management-engine-owned`) and never a fifth address (`management-limit`), never a 65th
  whitelisted address (`whitelist-limit`), never a logical name another PCI's saved device name or physical row holds
  (`name-in-use`, R1 #1) nor a name any other host NIC currently carries as its kernel name (`name-in-use`, R4 verify #9:
  the agent would take the row for that kernel NIC), never a kernel NIC under a saved device name that differs from its
  kernel name (`name-mismatch`, R4 #2: the agent tells unbound NICs by their kernel name) and never a bound NIC without a
  saved device name
  (`bound-unnamed`: VPP's name for it is unknown) — so the golden document no longer carries the `enp29s0f0` row. Saved
  `dataplane.managementPci` flags NICs as management before the no-management check. Skips are carried in the
  `system.seed-defaults` payload (with the NICs taken as management) or, when nothing could be registered, in the warning
  event `system.seed-defaults-skipped`. A commit the validators reject (4xx ProblemError) is final for the boot: one
  `system.seed-defaults-failed` warning event (reason, pointers, issues, type), no timer — final only for the deterministic
  problem types `validation` and `license-required`; every other error (agent refusal, failed apply, 5xx, database)
  and an empty data inventory retry with exponential backoff (`retryMs` 60 s → `maxRetryMs` 1 h). The failed event is
  recorded once per reason + pointers; an overlapping attempt (`skipped`) leaves the schedule alone and the latest
  outcome sets the next delay (a deferral after a long backoff is retried in `retryMs`). Log/event text passes
  `reasonOnly()`. `OnModuleDestroy` stops the watch and timer; `start()` is idempotent.
- `apps/api/src/datastore/physical-nics.ts`: guard comments and the 403 texts say "boot-time registration" and state the
  rollback behaviour under D-252 (wording only, no logic change).
- `deploy/systemd/ngfw-api.service`: comment on the kept `NGFW_SEED_DEFAULT_NICS=1` (precedence, not line order);
  `deploy/debian/ngfw/tests/test_packaging.py`: `test_api_unit_enables_default_nic_registration` shell-splits every
  `Environment=` value of the unit and the hardening drop-in (last assignment wins ∈ {1, true}; no `UnsetEnvironment=`).
- `apps/api/src/config.ts`, `main.ts`, `features/default-vpp-nics/index.ts`: comments.
- Tests: `seed.service.test.ts` (33 cases; every planned document is checked with `validateConfig`; mutation-driven cases
  for the PPPoE guard, upper-case `physical.pci` rows, backoff growth/reset/replacement, 409/503 problems retried,
  `reasonOnly` on the inventory error, the name-claim and name-mismatch/bound-unnamed rules, the whitelist limit),
  `commit.service.test.ts` (+2 cases through real gRPC / fake agent, incl. deferral while a commit awaits confirmation
  and registration after the confirm), regenerated `seed.golden.json` (bound unnamed NIC no longer registered), e2e
  wording.
- Docs: `docs/decisions/PENDING-automatic-network-bootstrap.md` (implemented state, owner instruction 2026-10-10, not-run
  list, operator-visible behaviour changes), `docs/decisions/LOG.md` D-252 (amends HIST-NATIVE-D-164),
  `docs/user/interfaces/default-dataplane-nics.md`, `docs/user/interfaces/basics.md`, `docs/install/bare-metal.md`,
  `docs/install/upgrade.md`, `docs/status/DEFERRED-ACCEPTANCE.md` (appliance reboot case), `docs/tech-debt.md` (HA sync
  vs node-local physical rows; rollback warning for dropped built-in rows; systemCommit vs configure lock).

Not ported (already on `main`): `apps/agent/.../hostfacts.go` `netdevPCI` virtio handling and
`TestHostNICsVirtioPCIManagementAndData`; `seed.golden.json` (main's copy already carries the newer tunnel domains).

## Actual test results

Iteration 1 (`814532721`):

```
$ python3 -m unittest discover -s deploy/debian/ngfw/tests -p test_packaging.py -v
Ran 17 tests in 1.263s
OK

$ pnpm turbo run lint typecheck test --filter=@ngfw/api
@ngfw/api:test:  ✓ src/commit/commit.service.test.ts (26 tests) 6454ms
@ngfw/api:test:  ✓ src/features/default-vpp-nics/seed.service.test.ts (19 tests) 528ms
@ngfw/api:test:  Test Files  113 passed (113)
@ngfw/api:test:       Tests  806 passed (806)
 Tasks:    10 successful, 10 total   Cached:    3 cached, 10 total   Time:    2m38.844s

$ tools/ci.sh quick          (branch claude/boot-nic-registration-port @ 814532721)
  install (pnpm --frozen-lockfile --prefer-offline)   0m00s
  generate + generated-output gate                   1m26s
  forbidden patterns (+ gitleaks)                    0m15s
  packet-trace ban on the shared VPP (D-128)         0m00s
  ip classify reset after every shell interface create (D-185)   0m02s
  slot resource scheme (1..32, no collisions)        0m04s
  lint · typecheck · unit tests · build (turbo)   7m21s   (Tasks: 35 successful, 35 total)
  apps/agent: make lint test build                   2m54s
  apps/cli: make lint test build                     0m15s
  test/ Go modules, unit mode (27 modules)           1m29s
  deploy/vpp: shellcheck + apply-startup fake-host harness   4m34s
  mode quick · wall time 18m22s · logs /root/ngfw-wt/logs/ci/priceless-lewin-09c0d6-20261017-091540-1233287
CI GATE PASSED
```

Hosted checks on `814532721` (`gh pr checks 238`):

```
Mandatory quick gate                                   pass  16m12s  https://github.com/mcoder1001-cyber/NGFW/actions/runs/39020868049/job/117362229500
Offline packaging and temporary-key signing fixtures   pass  50s     https://github.com/mcoder1001-cyber/NGFW/actions/runs/39020803916/job/117362050625
Offline packaging and temporary-key signing fixtures   pass  1m8s    https://github.com/mcoder1001-cyber/NGFW/actions/runs/39020868110/job/117362229673
```

Iteration 2 (this tree, before the gate):

```
$ pnpm --filter @ngfw/api exec vitest run src/features/default-vpp-nics src/commit/commit.service.test.ts
 ✓ src/commit/commit.service.test.ts (26 tests) 6322ms
 ✓ src/features/default-vpp-nics/seed.service.test.ts (25 tests) 792ms
 ✓ src/features/default-vpp-nics/drift.test.ts (1 test)
$ pnpm --filter @ngfw/api typecheck      → $ tsc -p tsconfig.json   (no errors)
$ pnpm --filter @ngfw/api lint           → $ eslint src test        (no findings)
$ python3 -m unittest discover -s deploy/debian/ngfw/tests -p test_packaging.py -k enables_default_nic -v
Ran 1 test in 0.001s
OK
```

Iteration 2 (`db5042e38`) complete quick gate:

```
$ tools/ci.sh quick          (branch claude/boot-nic-registration-port @ db5042e38)
  tools (golangci-lint, gitleaks)                    0m01s
  install (pnpm --frozen-lockfile --prefer-offline)   0m00s
  generate + generated-output gate                   1m20s
  forbidden patterns (+ gitleaks)                    0m15s
  packet-trace ban on the shared VPP (D-128)         0m01s
  ip classify reset after every shell interface create (D-185)   0m02s
  slot resource scheme (1..32, no collisions)        0m04s
  lint · typecheck · unit tests · build (turbo)   7m19s
  apps/agent: make lint test build                   1m19s
  apps/cli: make lint test build                     0m08s
  test/ Go modules, unit mode (27 modules)           1m05s
  deploy/vpp: shellcheck + apply-startup fake-host harness   0m07s
  mode quick · wall time 11m43s · logs /root/ngfw-wt/logs/ci/priceless-lewin-09c0d6-20261017-094745-1427436
CI GATE PASSED
exit=0
```

Hosted checks on `db5042e38` (`gh pr checks 238` at 10:05 UTC): Mandatory quick gate run 39022880662 pending (result on the
PR); "Offline packaging and temporary-key signing fixtures" pass (49s, 1m8s).

Iteration 3 (this tree, before the gate): `pnpm --filter @ngfw/api exec vitest run src/features/default-vpp-nics` →
`✓ seed.service.test.ts (26 tests)`, `Tests 27 passed (27)`; `python3 -m unittest … -k enables_default_nic` → OK (1 test).
Iteration 4 (this tree, before the gate):

```
$ pnpm --filter @ngfw/api exec vitest run src/features/default-vpp-nics src/commit/commit.service.test.ts
 ✓ src/features/default-vpp-nics/drift.test.ts (1 test)
 ✓ src/features/default-vpp-nics/seed.service.test.ts (33 tests) 1176ms
 ✓ src/commit/commit.service.test.ts (26 tests) 6528ms
      Tests  60 passed (60)
$ pnpm --filter @ngfw/api typecheck / lint   → see the next checkpoint (run before the commit)
$ PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s deploy/debian/ngfw/tests -p test_packaging.py → 17 tests OK
```

Iteration 4 (`9574d997a`) complete quick gate:

```
$ tools/ci.sh quick          (branch claude/boot-nic-registration-port @ 9574d997a)
  lint · typecheck · unit tests · build (turbo), apps/agent, apps/cli, test/ Go modules, deploy/vpp harness — all passed
CI GATE PASSED
exit=0
```
(step durations in the log directory named in the gate output; hosted run on `9574d997a` recorded on the PR)

Hosted checks on `9574d997a` (`gh pr checks 238`): Mandatory quick gate pass 14m37s (run 39025185586); packaging fixtures pass.

Iteration 6 (`ab6d223b2`, final head), before the gate:

```
$ pnpm --filter @ngfw/api exec vitest run src/features/default-vpp-nics src/commit/commit.service.test.ts
 ✓ src/features/default-vpp-nics/seed.service.test.ts (36 tests) 2104ms
 ✓ src/commit/commit.service.test.ts (26 tests)
$ pnpm --filter @ngfw/api typecheck → $ tsc -p tsconfig.json (clean) · $ pnpm --filter @ngfw/api lint → $ eslint src test (clean)
$ PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s deploy/debian/ngfw/tests -p test_packaging.py → 17 tests OK
```

`tools/ci.sh quick` on `ab6d223b2` (pre-rebase history, archived):

```
$ tools/ci.sh quick          (branch claude/boot-nic-registration-port @ ab6d223b2)
  tools (golangci-lint, gitleaks)                    0m01s
  install (pnpm --frozen-lockfile --prefer-offline)   0m00s
  generate + generated-output gate                   1m31s
  forbidden patterns (+ gitleaks)                    0m15s
  packet-trace ban on the shared VPP (D-128)         0m00s
  ip classify reset after every shell interface create (D-185)   0m02s
  slot resource scheme (1..32, no collisions)        0m04s
  lint · typecheck · unit tests · build (turbo)   7m42s
  apps/agent: make lint test build                   1m22s
  apps/cli: make lint test build                     0m09s
  test/ Go modules, unit mode (27 modules)           (passed)
  deploy/vpp: shellcheck + apply-startup fake-host harness   0m07s
  mode quick · wall time 12m22s · logs /root/ngfw-wt/logs/ci/priceless-lewin-09c0d6-20261017-105336-1731319
CI GATE PASSED
exit=0
```
(the only uncommitted files during that run were this wip file and the combined review file — docs; the commit that carries
them is docs-only and is covered by the hosted Mandatory quick gate on the PR head)

Hosted checks on `ab6d223b2` (`gh pr checks 238`, 11:08 UTC): Mandatory quick gate run 39026899248 pending (result on the PR);
packaging fixtures pass (1m15s).

Final head (one commit on `origin/main` `c3ef9bf36`, after PR #241): the single commit that carries this file (SHA on PR #238 and in the manager's report)
`tools/ci.sh quick` on it:

```
$ tools/ci.sh quick          (branch claude/boot-nic-registration-port @ e41f496ee — the integration commit before this
                              docs-only amendment; identical tree apart from the gate lines in this file and the combined review)
  tools (golangci-lint, gitleaks)                    0m02s
  install (pnpm --frozen-lockfile --prefer-offline)   0m01s
  generate + generated-output gate                   1m28s
  forbidden patterns (+ gitleaks)                    0m15s
  packet-trace ban on the shared VPP (D-128)         0m01s
  ip classify reset after every shell interface create (D-185)   0m02s
  slot resource scheme (1..32, no collisions)        0m04s
  lint · typecheck · unit tests · build (turbo)   7m51s
  apps/agent: make lint test build                   2m26s
  apps/cli: make lint test build                     0m09s
  test/ Go modules, unit mode (27 modules)           (passed)
  deploy/vpp: shellcheck + apply-startup fake-host harness   0m08s
  mode quick · wall time 13m37s · logs /root/ngfw-wt/logs/ci/priceless-lewin-09c0d6-20261017-140334-2197868
CI GATE PASSED
exit=0
```
Hosted Mandatory quick gate on the published head: on the PR (`gh pr checks 238`).

## Not run

- `apps/api/test/e2e/default-vpp-nics.e2e.test.ts` (PostgreSQL e2e; only under `test:integration` / `tools/ci.sh full`).
- `tools/ci.sh full` (integration under the lab lock).
- A boot of a real appliance with existing revisions, an HA pair, a rollback across a registration revision → deferred
  (`docs/status/DEFERRED-ACCEPTANCE.md`, *Active feature acceptance*). No appliance, lab host or VPP instance was touched.

## Review

Panel: R1, R2, R7 (mandatory), R8 (systemd unit / packaging test), R6 (`docs/user/**` changed), R4 (dataplane ownership
of NICs). Round 1 on `814532721`: R2 APPROVE WITH CHANGES, R7 BLOCK (evidence gap + not-run list + panel), R8 BLOCK (MAJOR
#1: unbounded retry of a failing commit without an operator-visible event) — reports and developer answers in
`boot-nic-registration-port-review-R{2,7,8}.md`. Iteration 2 addressed every finding except the follow-ups recorded in
`docs/tech-debt.md`. Round 2 on `db5042e38`: R7 verify APPROVE WITH CHANGES (conditions: gate-2 evidence pasted, PR body
updated, PENDING wording — all done in iteration 3), R8 verify APPROVE (new MINOR N1: finality limited to deterministic
problems — done), R6 APPROVE WITH CHANGES (docs precision — done), R1 BLOCK (MAJOR #1 planner name claim across PCIs,
MAJOR #2 4xx finality — both fixed in iteration 4 with tests; MINORs fixed), R4 BLOCK (MAJOR #1: pre-existing agent
defect — `allInterfaces` block lists / auto-block over unbound and released rows — widened by D-252: handled as a
`docs/tech-debt.md` row with a proposed separate agent fix plus the corrected user-doc sentences, option (b) of the
reviewer; MINOR #2 fixed in the planner (`name-mismatch`, `bound-unnamed`); #3–#8 documented) — reports and answers in
`boot-nic-registration-port-review-R{1,4}.md`. Round 3 on `9574d997a`: R4 verify — BLOCK pending (1) the manager's
acceptance of the agent-defect debt row (`docs/tech-debt.md`, owner + date; marked "proposed" until then) and (2) the
reworded *Awaiting engine* exception (done in iteration 5); new MINOR #9 (bound NIC adopting another NIC's current kernel
name) fixed in iteration 5 with the test. R1 verify — APPROVE WITH CHANGES: both MAJORs fixed with tests; the requested
tests (timer replacement on reconnect, overlapping attempt, management-name clash, licence finality) landed in iteration 6.
Combined verdict: `boot-nic-registration-port-review.md` — APPROVE WITH CHANGES with the one condition above for the manager. Pre-existing, not touched here:
`tools/docs/generate-reference.py --check` reports `docs/user/README.md` drift on `origin/main` (four pages missing).

## Remaining

`tools/ci.sh quick` + hosted gate on `ab6d223b2`, PR body, PR out of draft
(`boot-nic-registration-port-review.md`), PR body update, PR out of draft. No merge without the owner's go-ahead.

Current failure: none observed.
Exact next command: `gh pr checks 238` (hosted gate on the final head); then the manager merges (D112) on the owner's go-ahead.
