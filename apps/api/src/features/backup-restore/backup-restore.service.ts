import { Inject, Injectable } from '@nestjs/common';
import { ModuleRef } from '@nestjs/core';
import { RootConfig, SECRET_KINDS, redactSecrets } from '@ngfw/schema';
import { and, desc, eq, gte, or, sql } from 'drizzle-orm';
import { AgentClient } from '../../agent/agent.client.js';
import { DatastoreService } from '../../datastore/datastore.service.js';
import { secretRefs, withoutPasswordHashes } from '../../datastore/documents.js';
import { DB, type Db } from '../../db/db.js';
import { auditLog, configRevision, secretVersion } from '../../db/schema.js';
import { SecretsService } from '../../secrets/secrets.service.js';
import type { Principal } from '../../common/principal.js';
import { problems } from '../../common/problem.js';
import { decryptArchive, encryptArchive, hashDocument, MAX_ARCHIVE } from './archive.js';

/** No credentials or account table snapshots are returned outside encrypted archives. */
@Injectable()
export class BackupRestoreService {
  private busy = false;
  constructor(
    @Inject(DB) private readonly db: Db,
    private readonly ds: DatastoreService,
    private readonly modules: ModuleRef,
    private readonly agent: AgentClient,
  ) {}
  private secrets(): SecretsService {
    return this.modules.get(SecretsService, { strict: false });
  }
  private async exclusive<T>(f: () => Promise<T>): Promise<T> {
    if (this.busy) throw problems.conflict('backup-busy', 'another archive operation is running');
    this.busy = true;
    try {
      return await f();
    } finally {
      this.busy = false;
    }
  }
  backup(user: Principal, passphrase: string, revisions = 100): Promise<Buffer> {
    return this.exclusive(async () => {
      // One repeatable-read snapshot pins document and its secret versions consistently.
      const archive = await this.db.transaction(
        async (tx) => {
          // Preflight cumulative uncompressed snapshot bytes before fetching JSON documents into Node.
          const revisionSizes = await tx
            .select({
              id: configRevision.id,
              size: sql<number>`octet_length(row_to_json(${configRevision})::text)`,
            })
            .from(configRevision)
            .orderBy(desc(configRevision.id))
            .limit(revisions);
          const auditSizes = await tx
            .select({
              id: auditLog.id,
              size: sql<number>`octet_length(row_to_json(${auditLog})::text)`,
            })
            .from(auditLog)
            .where(gte(auditLog.ts, new Date(Date.now() - 30 * 86400000)))
            .orderBy(desc(auditLog.id))
            .limit(10000);
          let budget =
            revisionSizes.reduce((total, row) => total + Number(row.size), 0) +
            Number(revisionSizes[0]?.size ?? 0) +
            auditSizes.reduce((total, row) => total + Number(row.size), 0) +
            65536;
          if (budget > MAX_ARCHIVE)
            throw problems.badRequest(
              'selected revision/audit history exceeds the expanded archive limit; request fewer revisions',
            );
          const rows = await tx
            .select()
            .from(configRevision)
            .orderBy(desc(configRevision.id))
            .limit(revisions);
          const current = rows[0];
          const running = (current?.payload ?? RootConfig.parse({})) as Record<string, unknown>;
          const needed = new Map<string, number>();
          for (const row of rows)
            for (const [ref, version] of Object.entries(row.secretVersions ?? {}))
              needed.set(`${ref}@${version}`, version);
          if (needed.size > 4096)
            throw problems.badRequest(
              'selected snapshot has too many secret versions; request fewer revisions',
            );
          const wanted = or(
            ...[...needed.keys()].map((key) => {
              const at = key.lastIndexOf('@');
              return and(
                eq(secretVersion.ref, key.slice(0, at)),
                eq(secretVersion.version, Number(key.slice(at + 1))),
              );
            }),
          );
          const secretSizes =
            needed.size === 0
              ? []
              : await tx
                  .select({
                    ref: secretVersion.ref,
                    version: secretVersion.version,
                    size: sql<number>`octet_length(${secretVersion.ciphertext})`,
                  })
                  .from(secretVersion)
                  .where(wanted);
          if (secretSizes.length !== needed.size)
            throw problems.unavailable('a pinned backup secret version is unavailable');
          // JSON escaping can expand secret bytes sixfold; refuse before loading ciphertext/plaintext.
          budget += secretSizes
            .filter((row) => needed.has(`${row.ref}@${row.version}`))
            .reduce((total, row) => total + 6 * Number(row.size) + 1024, 0);
          if (budget > MAX_ARCHIVE)
            throw problems.badRequest(
              'selected secret history exceeds the expanded archive limit; request fewer revisions',
            );
          const secretRows =
            needed.size === 0 ? [] : await tx.select().from(secretVersion).where(wanted);
          const stored = new Map(secretRows.map((row) => [`${row.ref}@${row.version}`, row]));
          const values = [];
          for (const key of needed.keys()) {
            const exact = stored.get(key);
            if (!exact) throw problems.unavailable('a pinned backup secret version is unavailable');
            values.push({
              ref: exact.ref,
              version: exact.version,
              value: this.secrets().decrypt(exact.ciphertext, exact.ref),
            });
          }
          const audit = await tx
            .select()
            .from(auditLog)
            .where(gte(auditLog.ts, new Date(Date.now() - 30 * 86400000)))
            .orderBy(desc(auditLog.id))
            .limit(10000);
          return {
            manifest: {
              format: 1 as const,
              schemaVersion: '1.0',
              revision: current?.id ?? null,
              hash: hashDocument(running),
              createdAt: new Date().toISOString(),
              createdBy: user.username,
            },
            running,
            revisions: rows.map((r) => ({ ...r, createdAt: r.createdAt.toISOString() })),
            secrets: values,
            audit: audit.map((r) => ({ ...r, ts: r.ts.toISOString() })),
          };
        },
        { isolationLevel: 'repeatable read', accessMode: 'read only' },
      );
      return encryptArchive(archive, passphrase);
    });
  }
  restore(user: Principal, bytes: Buffer, passphrase: string) {
    return this.exclusive(async () => {
      const archive = await decryptArchive(bytes, passphrase);
      const clean = withoutPasswordHashes(archive.running).doc;
      const doc = RootConfig.parse(clean);
      const refs = secretRefs(doc);
      const rows = [];
      for (const ref of new Set(refs.map((r) => r.ref))) {
        const kind = ref.split('/')[0];
        if (!(SECRET_KINDS as readonly string[]).includes(kind!))
          throw problems.badRequest('unsupported secret kind');
        const pins = archive.revisions.find((r) => r['id'] === archive.manifest.revision)?.[
          'secretVersions'
        ] as Record<string, number> | undefined;
        const value = archive.secrets.find(
          (s) => s.ref === ref && (pins?.[ref] === undefined || s.version === pins[ref]),
        );
        if (!value) throw problems.badRequest('backup lacks a referenced secret');
        rows.push({ ref, ciphertext: this.secrets().encrypt(value.value, ref) });
      }
      await this.ds.importCandidate(user, doc, rows);
      return { staged: true, diff: await this.ds.diff() };
    });
  }
  async support(): Promise<Buffer> {
    const running = await this.ds.getRunning();
    const rows = await this.db
      .select({
        ts: auditLog.ts,
        action: auditLog.action,
        resource: auditLog.resource,
        result: auditLog.result,
      })
      .from(auditLog)
      .orderBy(desc(auditLog.id))
      .limit(1000);
    const health = await this.agent.health();
    const retrieve = await this.agent.retrieve();
    const host = await this.agent.runAction({
      supportBundle: { sinceSec: 86400, auditRows: 1000 },
    });
    if (!host.done || host.done.exitCode !== 0 || host.lines.join('').length > 1024 * 1024)
      throw problems.unavailable('support collector failed');
    const document = JSON.stringify({
      format: 1,
      config: redactSecrets(running.doc),
      audit: rows,
      health,
      agent: {
        subsystems: retrieve.subsystems,
        owner: retrieve.owner,
        retrievedAt: retrieve.retrievedAt,
        interfaceCount: Object.keys(retrieve.desiredState?.interfaces ?? {}).length,
      },
      host: host.lines,
      version: '0.1.0',
    });
    // Fail closed on private material even if a producer forgot a schema secret flag.
    assertSupportSafe(document);
    return Buffer.from(document);
  }
}

/** Final scan includes JSON-quoted keys and serialized collector strings; never exports a suspect bundle. */
export function assertSupportSafe(document: string): void {
  const normalized = document.replace(/\\"/g, '"');
  if (
    /-----BEGIN [A-Z ]*PRIVATE KEY|\$(?:argon2|2[aby])\$|(?:passwordHash|privateKey|password|psk|token|secret|authorization)["']?\s*[=:]\s*["']?[^\s"'}]+/i.test(
      normalized,
    )
  )
    throw problems.unavailable('support collector returned sensitive material');
}
