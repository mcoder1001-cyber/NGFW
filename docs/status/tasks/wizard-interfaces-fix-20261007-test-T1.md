# Independent wizard interface test evidence

Test date: 2026-10-07. Tester branch: `codex/wizard-test-20261007`; no lab slot assigned.

Product source tested: `3947362b6` and later documentation-only checkpoints. Tester source `840c4fdf6a88bb177393494dab2bb2601cec9b8a`; durable tester envelope checkpoint `c8563850153081b25304528d093de5fe678b6622`. `git diff --quiet codex/wizard-interfaces-fix-20261007 -- apps packages tools test deploy` exited 0 after targeted execution (developer ref then `02e2d1fbc4711c98da50c077b6580f87ddaa295a`). Product `apps` tree object `cee4e8c06d70e75f8ae24252a621721a1e6f919f`.

I authored only independent test cases and test evidence. Developer source corrections were cherry-picked without modification. Three independent regression cases were published first as `84d230778`; the developer then corrected their fixture state to match the final eligibility rules. No product failure is asserted from these fixture corrections.

## Scenario evidence

| Scenario | Expected | Observed | Result |
|---|---|---|---|
| Empty running config, live physical NICs | Live WAN/LAN choices; no config write before summary | Original web regression passes | PASS |
| WAN selected; LAN options | WAN excluded from LAN | Independent English and Persian cases pass | PASS |
| Live discovery returns 503 with configured NICs | Configured choices stay usable; retry recovers new NICs | Independent fallback/retry case passes | PASS |
| Live discovery succeeds with no NICs | Empty guidance and translated selection validation, no undefined Zod error | Independent empty-state case passes | PASS |
| Persian WAN and LAN unselected | Persian error, continued current step, RTL direction | Independent Persian case passes; DOM `dir=rtl` asserted | PASS |
| Preview/stage adopt live NICs absent from running | Candidate gains both interfaces; running unchanged | API controller test passes | PASS |
| NIC disappears, becomes subinterface/managed orphan, or drifts VRF | Rejected without staging | API tests pass | PASS |
| Host-owned/local NICs | Refused without agent lookup | API test passes | PASS |
| Existing candidate/stale revision/password failure | No overwrite or unauthorized staging | API tests pass | PASS |
| Real PostgreSQL/Valkey API + browser light/dark screenshots | Slot-isolated live T2/T4 runs | No assigned slot/provisioned isolated live stack; not run and not represented by mocks | BLOCKED-ENV |

## Targeted API command and actual output

`TMPDIR=/root/ngfw-wt/logs/wizard-test-tmp-20261007 pnpm --filter @ngfw/api exec vitest run src/features/setup/controller.test.ts`

```text

 RUN  v3.2.7 /root/ngfw-wt/wizard-test-20261007/apps/api

 ✓ src/features/setup/controller.test.ts (13 tests) 1290ms
   ✓ setup staging and security > previews and stages live interfaces absent from running without changing running  867ms

 Test Files  1 passed (1)
      Tests  13 passed (13)
   Start at  09:43:33
   Duration  31.44s (transform 11.92s, setup 0ms, collect 29.10s, tests 1.29s, environment 1ms, prepare 268ms)

```

## Targeted web command and actual output

`TMPDIR=/root/ngfw-wt/logs/wizard-test-tmp-20261007 pnpm --filter @ngfw/web exec vitest run src/domains/system/setup/SetupWizardPage.test.tsx src/domains/system/setup/SetupWizardPage.regression.test.tsx`

```text

 RUN  v3.2.7 /root/ngfw-wt/wizard-test-20261007/apps/web

stdout | src/domains/system/setup/SetupWizardPage.regression.test.tsx
🌐 i18next is made possible by our own product, Locize — consider powering your project with managed localization (AI, CDN, integrations): https://locize.com 💙

stdout | src/domains/system/setup/SetupWizardPage.test.tsx
🌐 i18next is made possible by our own product, Locize — consider powering your project with managed localization (AI, CDN, integrations): https://locize.com 💙

stderr | src/domains/system/setup/SetupWizardPage.test.tsx > setup wizard > keeps all seven steps local until reviewed final commit and uses confirmation
No `HydrateFallback` element provided to render during initial hydration

stderr | src/domains/system/setup/SetupWizardPage.regression.test.tsx > setup wizard independent network-picker regression > retains configured choices when live state fails and retry loads new interfaces
No `HydrateFallback` element provided to render during initial hydration

 ✓ src/domains/system/setup/SetupWizardPage.regression.test.tsx (3 tests) 13709ms
   ✓ setup wizard independent network-picker regression > retains configured choices when live state fails and retry loads new interfaces  4910ms
   ✓ setup wizard independent network-picker regression > shows empty-state guidance without exposing undefined schema validation  2603ms
   ✓ setup wizard independent network-picker regression > renders Persian WAN and LAN selection errors and excludes the selected WAN from LAN  6191ms
 ✓ src/domains/system/setup/SetupWizardPage.test.tsx (4 tests) 14411ms
   ✓ setup wizard > keeps all seven steps local until reviewed final commit and uses confirmation  5119ms
   ✓ setup wizard > loads live WAN/LAN interfaces on an empty configuration and requires a selection  3362ms
   ✓ setup wizard > refuses a weak password before advancing  2067ms
   ✓ setup wizard > factory login redirects dashboard to setup  3859ms

 Test Files  2 passed (2)
      Tests  7 passed (7)
   Start at  09:43:32
   Duration  57.40s (transform 6.99s, setup 12.13s, collect 23.69s, tests 28.12s, environment 47.99s, prepare 403ms)

```

These API tests use the in-memory repository and stubbed agent; web tests use scripted HTTP responses under jsdom. They establish host-independent regression coverage, not laboratory acceptance. No production or laboratory configuration changed and no host service was started/restarted.

## Environment recovery

Initial independent web test collection failed before any tests with:

```text
Error: ENOSPC: no space left on device, open '/tmp/xOtUDCu84v15Z0zTsy0Gi/web/84f004b27ee7221e483bcaa51daff17064bb7e8f'
Test Files  1 failed (1)
Tests  no tests
```

Actual `df -i` then showed `/tmp` 1,047,839 used / 1,048,576 inodes (737 free, 100%), while `/` had 4,591,332 free inodes. Retried with an own root-filesystem TMPDIR; tests passed. The obsolete initial quick gate was stopped only by its spawned PID 2456714 and observed child PIDs after manager authorization. Its partial execution is not a pass.

## Mandatory complete quick gate

Final command: `TMPDIR=/root/ngfw-wt/logs/wizard-test-tmp-20261007 tools/ci.sh --base origin/main`.

Gate implementation and checks unchanged. No missing-tools override or weakened check. Fixture-only correction `3947362b6` was adopted during generation before tests scheduled; all product source code remained stable through final execution. Tester envelope/WIP documentation was committed during execution. Full log directory: `/root/ngfw-wt/logs/ci/wizard-test-20261007-20261007-094007-2511477`.

The gate above exposed a real TypeScript test compilation failure (not an environment failure). It was stopped after that failure using only its spawned PID 2511477 and observed descendants. A direct `pnpm --filter @ngfw/api typecheck` on the same source independently reproduced it with exit 1; gate task exited 2. Actual output:

```text
$ tsc -p tsconfig.json
src/features/setup/controller.test.ts(121,50): error TS2352: Conversion of type '{ doc: { system: { setup: { completed: boolean; completedAt?: string | undefined; }; hostname: string; timezone: string; banner: { login?: string | undefined; motd?: string | undefined; }; dns: { servers: string[]; searchDomains: string[]; vrf: string; }; }; ... 12 more ...; security: { ...; }; }; revision: { ...; }...' to type 'Running' may be a mistake because neither type sufficiently overlaps with the other. If this was intentional, convert the expression to 'unknown' first.
  Types of property 'revision' are incompatible.
    Type '{ id: number; }' is missing the following properties from type 'Revision': payload, createdAt, authorId, author, and 6 more.
[ELIFECYCLE] Command failed with exit code 2.
Error: ERR_PNPM_RECURSIVE_RUN_FIRST_FAIL

  × "pnpm recursive run" failed in /root/ngfw-wt/wizard-test-20261007/apps/api

```

Developer repair `4ba92c948` preserves complete stored running metadata in the test fixture, replacing the incomplete revision cast. Tester adopted it as `d06a83cae`; no tester product edit. Corrected targeted API typecheck exited 0 (`$ tsc -p tsconfig.json`). Replacement full gate passed all 35 Turbo tasks, API 109 files / 716 tests, and web 107 files / 623 tests. It then failed Go tests because the long temporary directory exceeded Unix socket/path constraints; no Go product source changed. These observed failures are preserved below; they do not count as a green mandatory gate. The prior gate does not count as a pass.

Final verdict pending.

## Temporary-directory Unix socket recovery

The long root-backed TMPDIR avoided inode exhaustion but caused deterministic Unix socket failures in existing agent/unbound/chrony tests and SNMP path validation. The unbound failure reproduced with the same long TMPDIR in a targeted official `go test -race -count=1 ./internal/renderers/unbound -run '^TestDescriptorRestartPendingUntilActedOn$'`:

```text
--- FAIL: TestDescriptorRestartPendingUntilActedOn (0.00s)
    descriptor_test.go:110: listen unix /root/ngfw-wt/logs/wizard-test-tmp-20261007/TestDescriptorRestartPendingUntilActedOn3426847055/001/unbound.ctl: bind: invalid argument
FAIL
FAIL	ngfw/agent/internal/renderers/unbound	0.155s
FAIL
```

Using short root-backed `TMPDIR=/root/wztmp`, affected package checks (unchanged sources, `go test -race -count=1`) now report:

```text
ok  	ngfw/agent/internal/renderers/unbound	3.629s
ok  	ngfw/agent/internal/renderers/chrony	3.902s
ok  	ngfw/agent/internal/renderers/snmpd	4.581s
ok  	ngfw/agent/internal/subsystems	35.463s
ok  	ngfw/agent/internal/agent	1.560s
```

Subsystems package subsequently passed: `ok ngfw/agent/internal/subsystems 35.463s`. Next full gate will use manager-approved `/wzt` (same length as baseline `/tmp`) with every original check intact. No product Go patch or test-policy change. Final verdict remains pending mandatory complete gate.

## Short-TMPDIR full gate and HA timing observation

`TMPDIR=/wzt tools/ci.sh --base origin/main` passed all 35 Turbo tasks and resolved every earlier Unix socket/path failure. It then observed one existing HA timing failure:

```text
--- FAIL: TestResyncCorrelatesCompletionAndReportsMisses (0.18s)
    action_test.go:41: observation={CompletedAt:0001-01-01 00:00:00 +0000 UTC MissedCount:0 Completed:0}
FAIL
FAIL ngfw/agent/internal/actions/ha-state-sync 0.318s
```

This test fixture configures a 50 ms runtime deadline. It did not reproduce in isolated official race mode nor ten package repetitions; classify this observed occurrence as FLAKY, not silently omit it:

```text
ok  	ngfw/agent/internal/actions/ha-state-sync	1.126s
ok  	ngfw/agent/internal/actions/ha-state-sync	1.771s
```

Commands: `TMPDIR=/wzt go test -race -count=1 ./internal/actions/ha-state-sync -run '^TestResyncCorrelatesCompletionAndReportsMisses$'`; `TMPDIR=/wzt go test -race -count=10 ./internal/actions/ha-state-sync`. Manager notified to record tech debt and arrange final gate. Full failed log: `/root/ngfw-wt/logs/ci/wizard-test-20261007-20261007-100757-2601696`. No Go sources changed. Final mandatory gate verdict still pending.

Manager accepted HA timing occurrence as FLAKY after isolated and ten-package race repetition passed, and committed `docs/tech-debt.md` row in `30eb90243` (HA maintainers/integration manager; review 2026-10-08). Manager authorized test-process scheduling `GOMAXPROCS=4` with short root-backed `TMPDIR=/wzt`; every check remains unchanged. Complete final gate is currently through all TS tasks, agent/CLI lint-race-test-build and 27 test-module unit checks, with final fake-host harness in progress.
