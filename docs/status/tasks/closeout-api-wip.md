# API NAT46 validation fixture closeout, 2026-10-04

تنها شکست مجموعهٔ API مربوط به انتظار قدیمی تست NAT46 بود.
آدرس IPv4 تکراری و prefix غیر۹۶ اکنون در مرحلهٔ PATCH رد می‌شوند.
هم‌پوشانی MAP و رابط ناشناخته همچنان در COMMIT رد می‌شوند.
هر چهار تست روی PostgreSQL و Valkey واقعی موفق شد؛ agent این مجموعه fake است.
تغییر محصولی انجام نشد؛ اجرای کامل مجموعه و quick gate با مدیر است.

Branch `codex/closeout-api-nat46`, base `af83737b2`.
Local acceptance checkpoint: committed alongside this status; retrieve exact SHA with `git log -1`.
Remote publication: HTTP403 blocked (manager observed); no push workaround or publication claimed.
Owned: NAT46 e2e fixture and `closeout-api*` documents/evidence.

Root aggregate run had291passed/1failed across61files. Reproduced failing first case against actual PostgreSQL18.6/Valkey with existing fake-agent harness. Before the fix, duplicate IPv4 PATCH returned400 with problem detail `the document does not match the schema`, pointer `/nat/nat46/mappings/1/ipv4`, message `IPv4 service address 10.1.2.80 used twice`. This matches `Nat46Schema.superRefine`, not a product defect. Non-/96 client prefix is the same schema boundary. MAP overlap and unknown interface require cross-domain semantic validation at commit.

Test now explicitly assigns the expected rejection boundary per case (never accepts either boundary opportunistically). All four retain exact400, problem content-type and pointer assertions. Adds strict checks that invalid cases never reach fake-agent Apply, schema-rejected edits leave candidate payload unchanged, and discard succeeds.

Command:

```bash
NGFW_TEST_PREFIX=w9 NGFW_SLOT=9 NGFW_HTTP_PORT=3900 NGFW_VALKEY_DB=9 NGFW_INTEGRATION=1 tools/heavy.sh pnpm --filter @ngfw/api exec vitest run -c vitest.e2e.config.ts test/e2e/nat46.e2e.test.ts
```

Final outcome: exit0,4passed/0skipped, testfile3.711s, total28.94s. Slot9 database/role removed,7 prefixed Valkey keys deleted. Separate fixture ESLint returned0 (baseline Node MODULE_TYPELESS_PACKAGE_JSON warning only; no lint findings). `git diff --check` clean; `tools/ci.sh check --base af83737b2` passed11s with gitleaks no leaks. Full quick gate/aggregate292-suite rerun is manager-owned and not claimed here.

Dependency install used `tools/heavy.sh pnpm install --prefer-offline --frozen-lockfile`. Reused identical product-source package dist outputs by copying into local ignored dist directories. An initial attempt to symlink cross-worktree outputs caused gRPC class-identity mismatch; corrected fixture setup with local copies, never treated setup failure as product regression or PASS. Real pre-fix schema response and final passing run are retained in evidence. No secrets/tokens logged.
