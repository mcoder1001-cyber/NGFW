import {
  Inject,
  Injectable,
  type OnApplicationBootstrap,
  type OnModuleDestroy,
} from '@nestjs/common';
import { ModuleRef } from '@nestjs/core';
import { RootConfig } from '@ngfw/schema';
import { desc, eq } from 'drizzle-orm';
import { createHash } from 'node:crypto';
import { mkdir, readdir, lstat, realpath, unlink, writeFile } from 'node:fs/promises';
import { request } from 'node:https';
import { posix } from 'node:path';
import { Client } from 'ssh2';
import { DB, type Db } from '../../db/db.js';
import { backupRun, secret } from '../../db/schema.js';
import { DatastoreService } from '../../datastore/datastore.service.js';
import { SecretsService } from '../../secrets/secrets.service.js';
import { AuditService } from '../../audit/audit.service.js';
import { SystemEventsService } from '../../audit/system-events.service.js';
import { BackupRestoreService } from './backup-restore.service.js';

export function cronMatches(cron: string, now: Date): boolean {
  const limits = [59, 23, 31, 12, 6],
    values = [
      now.getUTCMinutes(),
      now.getUTCHours(),
      now.getUTCDate(),
      now.getUTCMonth() + 1,
      now.getUTCDay(),
    ];
  const fields = cron.split(' ');
  if (fields.length !== 5) return false;
  return fields.every(
    (field, i) =>
      field === '*' ||
      (/^\*\/[1-9][0-9]*$/.test(field)
        ? values[i]! % Number(field.slice(2)) === 0
        : /^\d+$/.test(field) && Number(field) <= limits[i]! && values[i] === Number(field)),
  );
}
const filePattern = /^ngfw-backup-\d{13}-[a-f0-9]{8}\.ngfwbackup$/;
type BackupConfig = ReturnType<typeof RootConfig.parse>['management']['backup'];
type Target = NonNullable<BackupConfig['target']>;
/** Scheduled exports run once per UTC minute, only from committed config, one bounded transfer at a time. */
@Injectable()
export class BackupScheduleService implements OnApplicationBootstrap, OnModuleDestroy {
  readonly runs: {
    at: string;
    result: 'success' | 'failure';
    filename?: string;
    error?: string;
  }[] = [];
  private timer: NodeJS.Timeout | undefined;
  private busy = false;
  private minute = '';
  constructor(
    @Inject(DB) private readonly db: Db,
    private readonly ds: DatastoreService,
    private readonly modules: ModuleRef,
    private readonly backup: BackupRestoreService,
    private readonly events: SystemEventsService,
  ) {}
  onApplicationBootstrap(): void {
    this.timer = setInterval(() => {
      void this.tick().catch(() => undefined);
    }, 15000);
    this.timer.unref();
  }
  onModuleDestroy(): void {
    if (this.timer) clearInterval(this.timer);
  }
  async history() {
    const rows = await this.db.select().from(backupRun).orderBy(desc(backupRun.at)).limit(100);
    return rows.map((row) => ({
      at: row.at.toISOString(),
      result: row.result,
      ...(row.filename ? { filename: row.filename } : {}),
      ...(row.error ? { error: row.error } : {}),
    }));
  }
  private async credential(ref: string): Promise<string> {
    const [row] = await this.db.select().from(secret).where(eq(secret.ref, ref));
    if (!row) throw new Error('credential unavailable');
    return this.modules.get(SecretsService, { strict: false }).decrypt(row.ciphertext, ref);
  }
  async tick(now = new Date()): Promise<void> {
    const minute = now.toISOString().slice(0, 16);
    if (this.busy || minute === this.minute) return;
    const running = await this.ds.getRunning();
    const config = RootConfig.parse(running.doc).management.backup;
    if (
      !config.enabled ||
      !config.target ||
      !config.passphraseRef ||
      !cronMatches(config.schedule, now)
    )
      return;
    const claim = await this.db
      .insert(backupRun)
      .values({ minute, at: now, result: 'running' })
      .onConflictDoNothing()
      .returning({ minute: backupRun.minute });
    if (claim.length === 0) return;
    this.minute = minute;
    this.busy = true;
    const audit = this.modules.get(AuditService, { strict: false });
    const entry = {
      userId: null,
      username: 'scheduled-backup',
      sourceIp: null,
      action: 'scheduled-backup.export',
      resource: `backup/${config.target.type}`,
      result: 'failure' as const,
      status: 0,
    };
    let auditId: number | undefined;
    try {
      auditId = await audit.begin(entry);
      const bytes = await this.backup.backup(
        { id: 0, username: 'scheduled-backup', role: 'admin', via: 'jwt' },
        await this.credential(config.passphraseRef),
        config.revisions,
      );
      const filename = `ngfw-backup-${now.getTime()}-${createHash('sha256').update(bytes).digest('hex').slice(0, 8)}.ngfwbackup`;
      await this.export(config.target, filename, bytes, config.retention);
      await audit.finish(auditId, {
        ...entry,
        result: 'success',
        status: 200,
        after: { filename },
      });
      this.runs.unshift({ at: now.toISOString(), result: 'success', filename });
      await this.db
        .update(backupRun)
        .set({ result: 'success', filename })
        .where(eq(backupRun.minute, minute));
      await this.events.record('info', 'backup', 'BACKUP_EXPORTED', 'scheduled export completed', {
        filename,
      });
    } catch {
      if (auditId !== undefined)
        await audit.finish(auditId, { ...entry, result: 'failure', status: 503 });
      this.runs.unshift({
        at: now.toISOString(),
        result: 'failure',
        error: 'scheduled export failed',
      });
      await this.db
        .update(backupRun)
        .set({ result: 'failure', error: 'scheduled export failed' })
        .where(eq(backupRun.minute, minute));
      await this.events.record('warning', 'backup', 'BACKUP_FAILED', 'scheduled export failed', {});
    } finally {
      this.runs.splice(100);
      this.busy = false;
    }
  }
  async export(target: Target, filename: string, bytes: Buffer, retention: number): Promise<void> {
    if (!filePattern.test(filename)) throw new Error('invalid backup filename');
    if (target.type === 'local') {
      await mkdir(target.path, { recursive: true, mode: 0o700 });
      if ((await realpath(target.path)) !== target.path) throw new Error('backup target symlink');
      await writeFile(`${target.path}/${filename}`, bytes, { flag: 'wx', mode: 0o600 });
      const files = (await readdir(target.path))
        .filter((name) => filePattern.test(name))
        .sort()
        .reverse();
      for (const file of files.slice(retention)) {
        const path = `${target.path}/${file}`;
        if ((await lstat(path)).isFile()) await unlink(path);
      }
      return;
    }
    if (target.type === 'https') {
      const token = target.credentialRef ? await this.credential(target.credentialRef) : undefined;
      await new Promise<void>((resolve, reject) => {
        const req = request(
          target.path,
          {
            method: 'PUT',
            timeout: 60000,
            signal: AbortSignal.timeout(60000),
            headers: {
              'content-type': 'application/octet-stream',
              'content-length': bytes.length,
              'x-ngfw-backup-filename': filename,
              'x-ngfw-backup-retention': String(retention),
              ...(token ? { authorization: `Bearer ${token}` } : {}),
            },
          },
          (res) => {
            res.resume();
            res.on('end', () =>
              res.statusCode && res.statusCode >= 200 && res.statusCode < 300
                ? resolve()
                : reject(new Error('HTTPS refused export')),
            );
          },
        );
        req.on('timeout', () => req.destroy(new Error('export timeout')));
        req.on('error', reject);
        req.end(bytes);
      });
      return;
    }
    const credential = await this.credential(target.credentialRef!);
    await new Promise<void>((resolve, reject) => {
      const client = new Client();
      const timer = setTimeout(() => {
        client.destroy();
        reject(new Error('SFTP timeout'));
      }, 60000);
      const finish = (error?: Error) => {
        clearTimeout(timer);
        client.end();
        if (error) reject(error);
        else resolve();
      };
      client.once('error', finish);
      client.once('ready', () =>
        client.sftp((err, sftp) => {
          if (err) return finish(err);
          const path = posix.join(target.path, filename);
          sftp.writeFile(path, bytes, { mode: 0o600, flag: 'wx' }, (error) => {
            if (error) return finish(error);
            sftp.readdir(target.path, (readError, list) => {
              if (readError) return finish(readError);
              const prune = list
                .filter((entry) => filePattern.test(entry.filename) && entry.attrs.isFile())
                .map((entry) => entry.filename)
                .sort()
                .reverse()
                .slice(retention);
              const next = () => {
                const name = prune.pop();
                if (!name) return finish();
                sftp.unlink(posix.join(target.path, name), (removeError) =>
                  removeError ? finish(removeError) : next(),
                );
              };
              next();
            });
          });
        }),
      );
      client.connect({
        host: target.host!,
        port: target.port ?? 22,
        username: target.username!,
        readyTimeout: 15000,
        hostHash: 'sha256',
        hostVerifier: (key: string) => key === target.hostKeySha256,
        ...(target.credentialRef!.startsWith('key/')
          ? { privateKey: credential }
          : { password: credential }),
      });
    });
  }
}
