# F-backup-restore — independent R5 performance and scale review

Source: local integration `f42df2cd685e87c21bb240c652b0868b3c331457` (runtime feature source `9bd8f2949`). Isolated worktree `/root/ngfw-wt/f-backup-performance-review-20261005`, branch `codex/f-backup-performance-review-20261005`. Reviewer owns this report only; no product files changed. Task envelope: independent R5 bounded archive/snapshot/streaming/scheduler/list review; no real upgrade, 8 GiB allocation, or full CI rerun.

## MAJOR: scheduled history performs an ever-growing full scan

`apps/api/src/features/backup-restore/schedule.ts:74` queries `ORDER BY f_backup_run.at DESC LIMIT 100`. `apps/api/src/db/schema.ts:339` and `apps/api/migrations/0010_f_backup_restore.sql:1` define only the minute primary key, with no index on `at`. One row per scheduled minute is retained indefinitely: 525,600 rows/year for every-minute schedules. Each history request must inspect the entire table and sort top-N; LIMIT bounds response memory but not DB work. Add an index on `at` (descending appropriate) in the Drizzle schema and generated migration/snapshot. No new retention requirement is imposed.

Actual isolated PostgreSQL probe, run from this review worktree:

```sql
CREATE TEMP TABLE r5_backup_run(minute text PRIMARY KEY, at timestamptz NOT NULL, result text);
INSERT INTO r5_backup_run SELECT i::text, now()-i*interval '1 minute','success' FROM generate_series(1,100000) i;
ANALYZE r5_backup_run;
EXPLAIN (ANALYZE, BUFFERS) SELECT * FROM r5_backup_run ORDER BY at DESC LIMIT 100;
CREATE INDEX ON r5_backup_run(at DESC);
EXPLAIN (ANALYZE, BUFFERS) SELECT * FROM r5_backup_run ORDER BY at DESC LIMIT 100;
```

Executed using `sudo -u postgres psql -X -d postgres -v ON_ERROR_STOP=1`; exit 0. Before: Seq Scan 100000 rows, top-N heapsort 32 kB, local hit637, Execution Time39.867ms. After: Index Scan returns100 rows, local hit1/read2, Execution Time0.171ms. These are a bounded query-plan demonstration, not appliance throughput claims. TEMP table/index vanished on session exit; no persistent DB changes. Host-name sudo warning did not prevent execution.

## Other observations

Archive input/output caps32MiB, fixed asynchronous scrypt parameters, fail-fast exclusive archive operations, repeatable-read SQL byte preflight before fetching payloads, revision1000/audit10000/secret4096 caps constrain archive memory and CPU. SQL secret predicates are capped8192 parameters. Restore searches are bounded by these limits, though repeated array searches could later use maps. Upload pipeline uses a Transform byte counter and file stream backpressure, removes partial files on failure, does not collect the8GiB body. Scheduler interval15s, one durable claim per minute, bounded in-memory100run history, cleared/unref timer, HTTPS absolute60s and SFTP60s deadlines constrain background work. Local/SFTP retention directory lists depend on target directory contents; normal successful scheduled retention keeps matching backup files bounded.

No full CI or hardware benchmark run by this reviewer; root/R1 owns unchanged full gate. Other panels own functional/security/packaging acceptance. Static commands inspected archive/service/controller/schedule, schema and migration, and the task prompt. No secret data or real upgrades accessed.

Verdict: **BLOCK** (0 BLOCKER, 1 MAJOR, 0 MINOR). Verify index schema/migration and source SHA after fix; no broad rerun needed for this finding.

## Focused closure — 2026-10-05

Developer fix `0de1bd078`, integrated product source `f193013d0`; independent review worktree cherry-pick `4ed1d4251bdd28903568cb9af7916312a8c6a8f7`. Schema defines `index('f_backup_run_at_idx').on(t.at.desc().nullsFirst())`; generated single0010 migration and snapshot agree. DESC NULLS FIRST matches PostgreSQL ORDER BY at DESC default, including planner ordering. No product code edited by reviewer.

Commands actually run:

```
pnpm install --frozen-lockfile --prefer-offline
NGFW_CI_TASK_CONCURRENCY=2 pnpm exec turbo run build --filter='@ngfw/api^...' --concurrency=2
NGFW_TEST_PREFIX=w28 NGFW_VALKEY_DB=10 NGFW_INTEGRATION=1 pnpm --filter @ngfw/api exec vitest run -c vitest.e2e.config.ts test/e2e/backup-restore.e2e.test.ts -t 'uses the generated descending index'
```

Install exit0 (8.4s); prerequisites6/6 successful (38.366s). Focused e2e exit0, 1passed and8explicitly skipped by name filter,29.21s. Real PostgreSQL18.6 harness applies actual production migration to owned w28 database; production `f_backup_run` populated100000rows. EXPLAIN ANALYZE actualquery used `Index Scan` on `f_backup_run_at_idx`, no Seq Scan/sort, returned100rows with3 shared hit blocks, Execution Time0.106ms. This independently closes the original MAJOR on the real schema, not the initial temporary lookalike. Teardown deleted4 own-prefixed Valkey keys inDB10, dropped ownedw28database/role; no flush or host upgrade. Logs `/tmp/fbr-r5-index.log` contain full plan. Other8cases/fullCI are not claimed by this focused run.

Final verdict: **APPROVE** (original MAJOR closed; 0 open BLOCKER/MAJOR/MINOR).
