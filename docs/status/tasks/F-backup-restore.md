# F-backup-restore acceptance evidence

Author branch codex/f-backup-restore-20261005. Historical author checkpoint c8ec544708e1dcf06dfc225059a276d0a756112e corresponds to local a6fd289a6. Final integration product checkpoint is 2560e8511 atop main f57424992; final integrated quick gate is pending. Manager owns combined frontend integration, independent reviews and final quick/hosted CI verdict.

- Encrypted full backup, bounded authenticated import, revision/secret pins and normal candidate/commit recovery: PostgreSQL e2e6/6 PASS (/tmp/fbr-e2e8.log). Wiped running document recovers SHA256 ad27929269906644d14d795f096a6dd441780cbe241acf04821e88df696974e4. Wrong passphrase stages nothing; live ciphertext remains unchanged until promotion; discard clears pins. Current accounts remain unchanged.
- Fail-closed write-ahead audit: actual API audit.begin rejection prevents restore version/candidate writes and upgrade agent calls in PostgreSQL e2e. Scheduled credential export also calls write-ahead audit.
- Scheduling and retention: committed UTC schedule executes once per minute, durable run log and local retention2/3 PASS in e2e. Production SFTP transport against owned localhost12682 sshd verifies key upload, retention2/3 and rejection of wrong host-key pin (/tmp/fbr-sftp.log). Production HTTPS transport against owned localhost12683 fixture rejects untrusted TLS and accepts explicitly trusted authenticated PUT/retention header (/tmp/fbr-https.log). Remote HTTPS deletion is receiver-owned, not claimed implemented.
- Templates: typed candidate diff, newline/prototype injection refusal PASS. Definitions use normal staging/commit.
- Support: archive/templates unit5/5 PASS; collector Python2/2 PASS; injected quoted credentials/hash output rejected. Export contains redacted running config, value-free audit summaries, bounded fixed collector and subsystem health.
- Upgrade: streamed10MiB upload and JSON restore body larger than8MiB PASS; archive bounds refusal PASS. Agent fixed operation tests2/2 and race PASS; helper allowlist, root request handoff and fixed systemd unit preserve privilege boundaries. Actual A/B reboot, signature rejection on appliance and power-loss recovery NOT RUN; require laboratory acceptance under F-ab-upgrade. No host upgrade executed.
- Contracts: pnpm gen13/13 PASS; Go schema/proto drift PASS; API lint/typecheck and focused auth route tests PASS; CLI operations regenerated from OpenAPI. Complete unchanged quick gate running; no PASS claimed until manager records final result.
- English/Persian pages: independent UI author branch integrated by manager. Independent R6/T4 approved eight English/Persian light/dark page screenshots plus actual candidate-only template staging; API fixture12600 is real authenticated private DB with fixed upgrade status mock that refuses host mutations.

Recovery commands: tools/ci.sh --base origin/main; NGFW_TEST_PREFIX=w26 NGFW_VALKEY_DB=10 pnpm -C apps/api exec vitest run -c vitest.e2e.config.ts test/e2e/backup-restore.e2e.test.ts. Harness owns/drops ngfw_w26 only, cleans its prefix only, never flushes shared Valkey.

Final contract supersedes historical UI checkpoint notes: OpenAPI now has explicit successful response schemas for templates, run history, upgrade and restore; restore/apply responses include diff. Generated client consumers no longer depend on missing-success-schema workarounds. API/schema/CLI regeneration passed after this correction. The earlier UI status file records an intermediate checkpoint, not a current defect.

R1/R2 concurrency correction: PostgreSQL8/8 PASS (/tmp/fbr-race-fix2.log). Restore account preservation uses transaction-current running while holding candidate lock, not a service pre-read. A normal API account commit injected after archive preparation survives restore+commit with all app_user rows unchanged. Promotion of an earlier document preserves restoreSecrets on a newer retained candidate; real PostgreSQL regression asserts payload and pins retained. Earlier6/6 evidence remains historical;8/8 is current.

Final concurrency validation supersedes the initial direct-promotion probe: full API agent-response barrier PostgreSQL8/8 PASS31.33s (/tmp/fbr-race-fix4.log), typecheck PASS (/tmp/fbr-race-typecheck2.log), datastore/template unit25/25 PASS (/tmp/fbr-race-unit.log). Barrier holds normal commit response, imports snapshot concurrently, then releases promotion and verifies both candidate payload and nonempty restored pins survive.

R5 run-history correction: generated migration creates f_backup_run_at_idx on at DESC NULLS FIRST. PostgreSQL9/9 PASS34.91s (/tmp/fbr-index-e2e2.log); 100000-row actual ORDER BY at DESC LIMIT100 uses Index Scan,3 shared buffers,0.206ms execution. First nulls-last declaration was rejected by this meaningful query-plan test and corrected before checkpoint. API typecheck result recorded in final WIP.


Final integration evidence (2026-10-05)

The following excerpts are actual saved command output, not simulated expected output. Source-specific reports in this directory retain commands, boundaries and independent verdicts. Final fresh-main T1/R1 and R7 remain pending; no merge or Done claim is made here.

```text
# Author actual PostgreSQL regression: source 0de1bd078, slot26
NGFW_TEST_PREFIX=w26 NGFW_VALKEY_DB=10 pnpm -C apps/api exec vitest run -c vitest.e2e.config.ts test/e2e/backup-restore.e2e.test.ts
   ✓ backup recovery on PostgreSQL with the normal commit API > preserves accounts committed after archive preparation but before the restore transaction  801ms
   ✓ backup recovery on PostgreSQL with the normal commit API > retains restored secret pins when an earlier promotion leaves a newer candidate intact  771ms
   ✓ backup recovery on PostgreSQL with the normal commit API > uses the generated descending index for recent history with 100000 runs  2074ms

 Test Files  1 passed (1)
      Tests  9 passed (9)
   Start at  17:04:32
   Duration  34.91s (transform 13.66s, setup 0ms, collect 21.60s, tests 10.79s, environment 1ms, prepare 260ms)

e2e teardown: deleted 4 Valkey keys ngfw:w26:e2e:* in db 10
drop   database ngfw_w26
drop   role ngfw_w26
ok     nothing named ngfw_w26 / ngfw_w26 remains

# Actual faithful packaging fixture suite: source2560e8511

----------------------------------------------------------------------
Ran 35 tests in 43.512s

OK
Packaging fixtures: 35 run, 0 failures, 0 errors, 0 skipped, 0 expected failures, 0 unexpected successes
```

R5 independently verifies the production history query uses the generated index on100000 rows (0.106ms,3sharedbuffers). R8 independently closes helper manifest, lsb-release dependency and interrupted-export/downgrade documentation findings. R2/R3/R4/R5/R6 reports are preserved separately. Real appliance A/B reboot, power-loss and signature rejection remain explicitly NOT RUN in the authorized laboratory acceptance scope; the UI and fixed dispatch evidence do not substitute for them. Implementation choices and alternatives are recorded in D236.
