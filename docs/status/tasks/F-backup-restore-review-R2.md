# F-backup-restore — independent R2 security closure

Reviewed source: local `5eb2378dff8deadada4d7080e8b5ef8a01f4339e`, tree `eea361b241567947405742458d6817e39ca133b8`, matching remote PR188 source `5e0bd3f4ab49f2d615d7b71d7cc4d660818ec5d7`. Worktree `/root/ngfw-wt/f-backup-security-review-20261005`; branch `codex/f-backup-security-final-review-20261005`. Reviewer did not write product code or modify the gate. Scope R2 only; no R4/R7/R8 verdict implied.

## Findings and closure

**Prior MAJOR closed:** `apps/api/src/audit/audit.interceptor.ts` now registers backup, restore, support, upgrade/upload and template mutation routes for fail-closed write-ahead audit. Independently executed real PostgreSQL test confirms failed audit.begin returns503 before candidate or inactive secret-version mutation; failed upgrade audit returns503 with no Action RPC. Scheduled exports likewise begin audit before backup/decryption/export; an additional independent realDB probe confirms audit refusal invokes neither backup nor exporter, records failure durably, and produces no output file.

**Prior template concern closed:** `ConfigTemplateSchema.patchJson` recursively rejects prototype keys, controls and inline credential keys, including passwordHash. Renderer preserves JSON types and rejects control/prototype injection. Current stored templates therefore cannot carry the initially observed inline hash example; template DTOs operate on the finalized scalar contract.

No unresolved R2 BLOCKER, MAJOR or MINOR findings at the reviewed source.

## Security assessment

Inspected AES-256-GCM header AAD, random nonce/salt, fixed scrypt parameters, 32MiB archive/decompression limit, SQL-side cumulative snapshot-size preflight and exact pinned-version query. Restored secrets are authenticated, re-encrypted under destination key, kept inactive until normal commit, and discarded with candidate pins. Current user identities/roles/hashes are retained through snapshot restoration. No account/API-key/session snapshot restoration added.

Controller-wide admin guard and global audit inspected; independently tested unauthorized binary upload returns401 without creating a file. Upload uses exclusive create, 0600, bounded streaming and fixed filename/path controls. UI uses same-origin requests, existing session refresh, replayable Blob/string bodies, no upload clone/tee, cleared passphrases and no browser credential persistence.

SFTP requires pinned SHA256 host key; HTTPS keeps native certificate verification without redirect handling. Upgrade operation is an enum, argv fixed, path direct under /data/updates with regular-file and symlink checks; root handoff helper checks owner/mode/size and never shells out. Fixed collector exports bounded facts; final support regex rejects quoted/serialized credential keys, private keys and password hashes before download. No plaintext credential logging or changed-file secret detection found.

## Commands actually run and output

Frozen installation and schema/proto/yang builds completed in reviewer worktree. Source remained unchanged (`git status --short` empty before report writes).

```text
pnpm --filter @ngfw/api exec vitest run src/features/backup-restore/archive.test.ts src/features/backup-restore/templates.test.ts src/auth/route-guard.test.ts
 ✓ src/features/backup-restore/archive.test.ts (2 tests)
 ✓ src/features/backup-restore/templates.test.ts (3 tests)
 ✓ src/auth/route-guard.test.ts (8 tests)
 Test Files 3 passed (3)
 Tests 13 passed (13)

NGFW_TEST_PREFIX=w28 NGFW_VALKEY_DB=10 pnpm --filter @ngfw/api exec vitest run --config vitest.e2e.config.ts test/e2e/backup-restore.e2e.test.ts
 check ngfw_w28 as ngfw_w28 · PostgreSQL18.6
 FBR recovered document SHA256 ad27929269906644d14d795f096a6dd441780cbe241acf04821e88df696974e4; inactive secret preserved before commit
 Test Files 1 passed (1)
 Tests 5 passed (5)
 e2e teardown: deleted4 Valkey keys ngfw:w28:e2e:* in db10
 drop database ngfw_w28; drop role ngfw_w28
 ok nothing named ngfw_w28 / ngfw_w28 remains

NGFW_TEST_PREFIX=w28 NGFW_VALKEY_DB=10 pnpm --filter @ngfw/api exec vitest run --config /tmp/ngfw-fbr-r2-final/vitest.config.ts
 ✓ R2 independent security probes > scheduled audit refusal blocks decrypt/export and durable outcome records failure; unauthorized binary upload creates no file
 Test Files 1 passed (1)
 Tests 1 passed (1)
 e2e teardown: deleted4 Valkey keys ngfw:w28:e2e:* in db10
 drop database ngfw_w28; drop role ngfw_w28
 ok nothing named ngfw_w28 / ngfw_w28 remains

(cd apps/agent && go test ./internal/actions/backup-restore)
 ok ngfw/agent/internal/actions/backup-restore 0.032s

gitleaks detect --no-git -s . --redact --exit-code 0 --report-path /tmp/backup-r2-final-gitleaks.json
 Python intersection with git diff --name-only 99e060da 5eb2378df:
 Changed-file gitleaks hits: []
```

The original e2e uses real PostgreSQL and Valkey with in-process fake agent. No actual appliance upgrade, root systemd mutation, VPP mutation or live SFTP/HTTPS network fixture was executed by this reviewer. Full mandatory quick gate and other panels remain manager requirements.

[other:R7] Source UI status tail still describes DTO gaps already fixed; reconcile final docs after author follow-up. This is not an R2 finding or grade.

Verdict: **APPROVE (R2)** for the exact reviewed product tree; mandatory other review/gate results are separate.

## Independent probe recovery source

Save the following as `/tmp/ngfw-fbr-r2-final/security.e2e.test.ts`; link its node_modules to the reviewer apps/api/node_modules. Vitest config uses the existing global setup, swc plugin, external include, node environment and 30s/60s test/hook limits. Slotw28 must be reserved before rerunning, never reuse another worker's database.

```ts
import { describe, it, expect, vi } from 'vitest';
import { mkdtemp, readdir, rm } from 'node:fs/promises';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
import { startHarness } from '/root/ngfw-wt/f-backup-security-review-20261005/apps/api/test/support/harness.ts';
import { BackupScheduleService } from '/root/ngfw-wt/f-backup-security-review-20261005/apps/api/src/features/backup-restore/schedule.ts';
import { AuditService } from '/root/ngfw-wt/f-backup-security-review-20261005/apps/api/src/audit/audit.service.ts';
import { BackupRestoreService } from '/root/ngfw-wt/f-backup-security-review-20261005/apps/api/src/features/backup-restore/backup-restore.service.ts';
describe('R2 independent security probes', () => {
  it('scheduled audit refusal blocks decrypt/export and durable outcome records failure; unauthorized binary upload creates no file', async () => {
    const h = await startHarness();
    const directory = await mkdtemp(join(tmpdir(), 'ngfw-r2-upload-'));
    const previous = process.env.NGFW_UPDATES_DIR;
    process.env.NGFW_UPDATES_DIR = directory;
    try {
      const token = await h.login('admin', h.adminPassword);
      await h.call(token, 'POST', '/api/v1/secrets', {kind:'password',name:'r2pass',value:'NGFW_TEST_PSK_FBR_R2'});
      expect((await h.call(token, 'PUT', '/api/v1/config/management/backup', {enabled:true,schedule:'* * * * *', retention:2,revisions:10,passphraseRef:'password/r2pass',target:{type:'local',path:directory}})).status).toBe(200);
      expect((await h.call(token, 'POST', '/api/v1/config/commit')).status).toBe(200);
      const scheduler = h.app.get(BackupScheduleService);
      const exporter = vi.spyOn(scheduler, 'export');
      const backup = vi.spyOn(h.app.get(BackupRestoreService), 'backup');
      vi.spyOn(h.app.get(AuditService), 'begin').mockRejectedValueOnce(new Error('review-induced audit refusal'));
      await scheduler.tick(new Date('2026-10-05T06:11:00Z'));
      expect(exporter).not.toHaveBeenCalled();
      expect(backup).not.toHaveBeenCalled();
      expect((await scheduler.history())[0].result).toBe('failure');
      const upload = await h.app.inject({method:'POST',url:'/api/v1/actions/upgrade-upload',headers:{'content-type':'application/vnd.ngfw.update','x-ngfw-filename':'ngfw-update-1.2.3.tar'},payload:Buffer.from('review bytes')});
      expect(upload.statusCode).toBe(401);
      expect(await readdir(directory)).toEqual([]);
    } finally {
      vi.restoreAllMocks();
      if (previous === undefined) delete process.env.NGFW_UPDATES_DIR; else process.env.NGFW_UPDATES_DIR=previous;
      await h.close();
      await rm(directory,{recursive:true,force:true});
    }
  });
});
```

```ts
import { defineConfig } from 'vitest/config';
import swc from 'unplugin-swc';
export default defineConfig({test:{include:['/tmp/ngfw-fbr-r2-final/security.e2e.test.ts'],environment:'node',globalSetup:['/root/ngfw-wt/f-backup-security-review-20261005/apps/api/test/support/global-setup.ts'],fileParallelism:false,testTimeout:30000,hookTimeout:60000},plugins:[swc.vite({module:{type:'es6'}})]});
```
