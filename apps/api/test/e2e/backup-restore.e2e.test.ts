import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest';
import { eq, sql } from 'drizzle-orm';
import { randomBytes } from 'node:crypto';
import { mkdtemp, readdir, readFile, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { BackupScheduleService } from '../../src/features/backup-restore/schedule.js';
import {
  decryptArchive,
  encryptArchive,
  hashDocument,
} from '../../src/features/backup-restore/archive.js';
import {
  secret,
  configCandidate,
  configRevision,
  appUser,
  secretVersion,
} from '../../src/db/schema.js';
import { SecretsService } from '../../src/secrets/secrets.service.js';
import { AuditService } from '../../src/audit/audit.service.js';
import { startHarness, type Harness } from '../support/harness.js';

describe('backup recovery on PostgreSQL with the normal commit API', () => {
  let h: Harness;
  let token: string;
  let directory: string;
  const passphrase = 'NGFW_TEST_PSK_FBR_PASSPHRASE';
  beforeAll(async () => {
    h = await startHarness();
    token = await h.login('admin', h.adminPassword);
    directory = await mkdtemp(join(tmpdir(), 'ngfw-fbr-'));
  });
  afterAll(async () => {
    await h?.close();
    if (directory) await rm(directory, { recursive: true, force: true });
  });
  it('full backup authenticates, restores secrets inactive, commit promotes, and wrong password stages nothing', async () => {
    const put = await h.call(token, 'POST', '/api/v1/secrets', {
      kind: 'password',
      name: 'scheduled',
      value: 'NGFW_TEST_PSK_FBR_ORIGINAL',
    });
    expect(put.status).toBe(200);
    const schedule = {
      enabled: false,
      schedule: '0 2 * * *',
      retention: 7,
      revisions: 100,
      passphraseRef: 'password/scheduled',
    };
    expect((await h.call(token, 'PUT', '/api/v1/config/management/backup', schedule)).status).toBe(
      200,
    );
    expect(
      (await h.call(token, 'PUT', '/api/v1/config/system', { hostname: 'fbr-source' })).status,
    ).toBe(200);
    expect((await h.call(token, 'POST', '/api/v1/config/commit')).status).toBe(200);
    const original = (await h.call(token, 'GET', '/api/v1/config')).body;
    const response = await h.app.inject({
      method: 'POST',
      url: '/api/v1/actions/backup',
      headers: { authorization: `Bearer ${token}` },
      payload: { passphrase },
    });
    expect(response.statusCode).toBe(200);
    const bytes = response.rawPayload;
    expect(bytes.includes(Buffer.from('NGFW_TEST_PSK_FBR_ORIGINAL'))).toBe(false);
    const decoded = await decryptArchive(bytes, passphrase);
    expect(decoded.manifest.hash).toBe(hashDocument(original));
    await h.call(token, 'POST', '/api/v1/secrets?replace=true', {
      kind: 'password',
      name: 'scheduled',
      value: 'NGFW_TEST_PSK_FBR_LIVE',
    });
    await h.db.delete(configRevision); // Dedicated test DB only: demonstrate recovery from a wiped datastore.
    const [live] = await h.db.select().from(secret).where(eq(secret.ref, 'password/scheduled'));
    const inactiveBefore = await h.db.select().from(secretVersion);
    vi.spyOn(h.app.get(AuditService), 'begin').mockRejectedValueOnce(
      new Error('audit unavailable'),
    );
    const unaudited = await h.call(token, 'POST', '/api/v1/actions/restore', {
      passphrase,
      archive: bytes.toString('base64'),
    });
    expect(unaudited.status).toBe(503);
    expect(await h.db.select().from(secretVersion)).toEqual(inactiveBefore);
    const [afterAuditRefusal] = await h.db.select().from(configCandidate);
    expect(afterAuditRefusal?.payload).toBeNull();
    const actionsBefore = h.fake.calls.filter((c) => c.method === 'Action').length;
    vi.spyOn(h.app.get(AuditService), 'begin').mockRejectedValueOnce(
      new Error('audit unavailable'),
    );
    expect(
      (await h.call(token, 'POST', '/api/v1/actions/upgrade', { op: 'activate' })).status,
    ).toBe(503);
    expect(h.fake.calls.filter((c) => c.method === 'Action')).toHaveLength(actionsBefore);
    const wrong = await h.call(token, 'POST', '/api/v1/actions/restore', {
      passphrase: 'wrong-passphrase-123',
      archive: bytes.toString('base64'),
    });
    expect(wrong.status).toBe(400);
    const [unstaged] = await h.db.select().from(configCandidate);
    expect(unstaged?.payload).toBeNull();
    expect(
      (
        await h.call(token, 'POST', '/api/v1/actions/restore', {
          passphrase,
          archive: bytes.toString('base64'),
        })
      ).status,
    ).toBe(200);
    const [stillLive] = await h.db
      .select()
      .from(secret)
      .where(eq(secret.ref, 'password/scheduled'));
    expect(stillLive?.ciphertext).toBe(live?.ciphertext);
    expect(stillLive?.version).toBe(live?.version);
    await h.call(token, 'POST', '/api/v1/config/discard');
    const [discarded] = await h.db.select().from(configCandidate);
    expect(discarded?.restoreSecrets).toBeNull();
    await h.call(token, 'POST', '/api/v1/actions/restore', {
      passphrase,
      archive: bytes.toString('base64'),
    });
    expect((await h.call(token, 'POST', '/api/v1/config/commit')).status).toBe(200);
    const [promoted] = await h.db.select().from(secret).where(eq(secret.ref, 'password/scheduled'));
    expect(h.app.get(SecretsService).decrypt(promoted!.ciphertext, 'password/scheduled')).toBe(
      'NGFW_TEST_PSK_FBR_ORIGINAL',
    );
    expect(hashDocument((await h.call(token, 'GET', '/api/v1/config')).body)).toBe(
      hashDocument(original),
    );
    const accountsBefore = await h.db
      .select({
        username: appUser.username,
        role: appUser.role,
        passwordHash: appUser.passwordHash,
        disabled: appUser.disabled,
      })
      .from(appUser);
    const foreign = structuredClone(decoded);
    (foreign.running['management'] as Record<string, unknown>)['users'] = [
      { username: 'foreign-admin', role: 'admin', scope: '*', disabled: false },
    ];
    foreign.manifest.hash = hashDocument(foreign.running);
    const foreignBytes = await encryptArchive(foreign, passphrase);
    expect(
      (
        await h.call(token, 'POST', '/api/v1/actions/restore', {
          passphrase,
          archive: foreignBytes.toString('base64'),
        })
      ).status,
    ).toBe(200);
    expect((await h.call(token, 'POST', '/api/v1/config/validate')).status).toBe(200);
    expect((await h.call(token, 'POST', '/api/v1/config/commit?confirm=60')).body.status).toBe(
      'pending',
    );
    const [beforeConfirm] = await h.db
      .select()
      .from(secret)
      .where(eq(secret.ref, 'password/scheduled'));
    expect(beforeConfirm?.version).toBe(promoted?.version);
    expect((await h.call(token, 'POST', '/api/v1/config/commit/confirm')).status).toBe(200);
    expect(
      await h.db
        .select({
          username: appUser.username,
          role: appUser.role,
          passwordHash: appUser.passwordHash,
          disabled: appUser.disabled,
        })
        .from(appUser),
    ).toEqual(accountsBefore);
    console.log(
      `FBR recovered document SHA256 ${hashDocument(original)}; inactive secret preserved before commit`,
    );
  });
  it('templates stage expected diff and reject prototype or newline parameters', async () => {
    const template = {
      description: 'test',
      parameters: { hostname: { type: 'string', required: true } },
      patch: { system: { hostname: '${hostname}' } },
    };
    expect((await h.call(token, 'PUT', '/api/v1/config-templates/demo', template)).status).toBe(
      200,
    );
    expect((await h.call(token, 'GET', '/api/v1/config-templates/demo')).body).toEqual(template);
    const applied = await h.call(token, 'POST', '/api/v1/config-templates/demo/apply', {
      parameters: { hostname: 'from-template' },
    });
    expect(applied.status).toBe(200);
    expect(applied.body.diff.changes).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ pointer: '/system/hostname', to: 'from-template' }),
      ]),
    );
    expect(
      (
        await h.call(token, 'POST', '/api/v1/config-templates/demo/apply', {
          parameters: { hostname: 'bad\nvalue' },
        })
      ).status,
    ).toBe(400);
    expect(
      (
        await h.call(token, 'PUT', '/api/v1/config-templates/bad', {
          ...template,
          patch: JSON.parse('{"__proto__":{"polluted":true}}'),
        })
      ).status,
    ).toBe(400);
    await h.call(token, 'POST', '/api/v1/config/discard');
  });
  it('scheduled committed local export claims its UTC minute once and logs success durably', async () => {
    const schedule = {
      enabled: true,
      schedule: '* * * * *',
      retention: 2,
      revisions: 100,
      passphraseRef: 'password/scheduled',
      target: { type: 'local', path: directory },
    };
    expect((await h.call(token, 'PUT', '/api/v1/config/management/backup', schedule)).status).toBe(
      200,
    );
    expect((await h.call(token, 'POST', '/api/v1/config/commit')).status).toBe(200);
    const scheduler = h.app.get(BackupScheduleService),
      now = new Date('2026-10-05T03:20:00Z');
    await scheduler.tick(now);
    await scheduler.tick(now);
    const runs = await h.call(token, 'GET', '/api/v1/state/backup');
    expect(runs.status).toBe(200);
    expect(runs.body.runs.filter((r: { at: string }) => r.at === now.toISOString())).toHaveLength(
      1,
    );
    expect(runs.body.runs[0].result).toBe('success');
  });
  it('refuses oversized revision history before selecting JSON document payloads into Node', async () => {
    // Grow only the dedicated fixture DB; SQL generates the data, avoiding a large client payload.
    await h.db.execute(
      sql`insert into config_revision (author_id,comment,parent_id,payload,hash,txn_id,kind,secret_changes,secret_versions) select null,'oversize fixture',null,jsonb_build_object('system',jsonb_build_object('hostname','large'),'filler',repeat('x',1048576)),'fixture',null,'test','[]'::jsonb,'{}'::jsonb from generate_series(1,33)`,
    );
    const reply = await h.app.inject({
      method: 'POST',
      url: '/api/v1/actions/backup',
      headers: { authorization: `Bearer ${token}` },
      payload: { passphrase, revisions: 100 },
    });
    expect(reply.statusCode).toBe(400);
    expect(reply.json().detail).toContain('history exceeds');
    await h.db.delete(configRevision).where(eq(configRevision.comment, 'oversize fixture'));
  });
  it('local retention prunes only our named regular backups', async () => {
    const scheduler = h.app.get(BackupScheduleService);
    const target = { type: 'local' as const, path: directory };
    await writeFile(join(directory, 'keep.txt'), 'user file');
    for (const ts of [1000000000000, 1000000000001, 1000000000002])
      await scheduler.export(target, `ngfw-backup-${ts}-abcdef12.ngfwbackup`, randomBytes(32), 2);
    const files = await readdir(directory);
    expect(files.filter((name) => name.endsWith('.ngfwbackup'))).toHaveLength(2);
    expect(await readFile(join(directory, 'keep.txt'), 'utf8')).toBe('user file');
  });
});
