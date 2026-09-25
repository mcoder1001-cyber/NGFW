import { Inject, Injectable } from '@nestjs/common';
import { SECRET_KINDS, type SecretKind } from '@ngfw/schema';
import { asc, eq, max } from 'drizzle-orm';
import { createCipheriv, createDecipheriv, randomBytes } from 'node:crypto';
import { chmodSync, existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { dirname } from 'node:path';
import { ENV, type Env } from '../config.js';
import { problems } from '../common/problem.js';
import { DB, type Db } from '../db/db.js';
import {
  configCandidate,
  configPending,
  configRevision,
  secret,
  secretVersion,
} from '../db/schema.js';
import { SystemEventsService } from '../audit/system-events.service.js';
import { CommitService } from '../commit/commit.service.js';
import { secretRefs } from '../datastore/documents.js';
import { desc } from 'drizzle-orm';

export interface SecretMeta {
  ref: string;
  kind: SecretKind;
  name: string;
  version: number;
  createdAt: Date;
}

/**
 * Secret store (00-CONTEXT rule 10, D-051): PSKs, keys, passphrases encrypted at rest with AES-256-GCM under a master
 * key file (VRX_SECRET_KEY_FILE, 0600, created on first use). The config document only holds `<kind>/<name>`
 * references; values are write-only through the API and never logged. The agent resolves references through its own
 * channel (out of P06 scope).
 */
@Injectable()
export class SecretsService {
  private key: Buffer | undefined;

  constructor(
    @Inject(DB) private readonly db: Db,
    @Inject(ENV) private readonly env: Env,
    private readonly events: SystemEventsService,
    private readonly commits: CommitService,
  ) {}

  private masterKey(): Buffer {
    if (this.key) return this.key;
    const file = this.env.VRX_SECRET_KEY_FILE;
    if (!existsSync(file)) {
      mkdirSync(dirname(file), { recursive: true, mode: 0o700 });
      writeFileSync(file, randomBytes(32), { mode: 0o600, flag: 'wx' });
    }
    chmodSync(file, 0o600);
    const key = readFileSync(file);
    if (key.length !== 32)
      throw problems.unavailable('the secret store master key is invalid (expected 32 bytes)');
    this.key = key;
    return key;
  }

  /** AES-256-GCM, fresh 96-bit IV; the reference is bound as AAD so ciphertexts cannot be swapped between refs. */
  encrypt(plain: string, ref: string): string {
    const iv = randomBytes(12);
    const c = createCipheriv('aes-256-gcm', this.masterKey(), iv);
    c.setAAD(Buffer.from(ref, 'utf8'));
    const ct = Buffer.concat([c.update(plain, 'utf8'), c.final()]);
    return Buffer.concat([iv, c.getAuthTag(), ct]).toString('base64');
  }

  decrypt(blob: string, ref: string): string {
    const raw = Buffer.from(blob, 'base64');
    const d = createDecipheriv('aes-256-gcm', this.masterKey(), raw.subarray(0, 12));
    d.setAAD(Buffer.from(ref, 'utf8'));
    d.setAuthTag(raw.subarray(12, 28));
    return Buffer.concat([d.update(raw.subarray(28)), d.final()]).toString('utf8');
  }

  async list(): Promise<SecretMeta[]> {
    const rows = await this.db
      .select({
        ref: secret.ref,
        kind: secret.kind,
        version: secret.version,
        createdAt: secret.createdAt,
      })
      .from(secret)
      .orderBy(asc(secret.ref));
    return rows.map((r) => ({
      ...r,
      kind: r.kind as SecretKind,
      name: r.ref.slice(r.kind.length + 1),
    }));
  }

  /**
   * Create a secret, or with `replace` store a NEW VERSION of an existing one (review M2). Every value is kept in
   * `secret_version`; revisions pin the versions they were committed with, and a rollback re-activates them.
   * Without `replace`, an existing ref is 409 — a live credential is never overwritten by accident.
   */
  async put(
    kind: SecretKind,
    name: string,
    value: string,
    opts: { replace: boolean; userId: number | null },
  ): Promise<{ ref: string; created: boolean; version: number }> {
    if (!(SECRET_KINDS as readonly string[]).includes(kind))
      throw problems.badRequest(`unknown secret kind '${kind}'`);
    const ref = `${kind}/${name}`;
    const ciphertext = this.encrypt(value, ref);
    const out = await this.db.transaction(async (tx) => {
      const [cur] = await tx
        .select({ version: secret.version })
        .from(secret)
        .where(eq(secret.ref, ref))
        .for('update');
      if (cur !== undefined && !opts.replace) {
        throw problems.conflict(
          'secret-exists',
          `secret '${ref}' exists; send ?replace=true to store a new version (the old one stays available for rollback)`,
        );
      }
      const [top] = await tx
        .select({ v: max(secretVersion.version) })
        .from(secretVersion)
        .where(eq(secretVersion.ref, ref));
      const version = (top?.v ?? 0) + 1;
      await tx.insert(secretVersion).values({ ref, version, ciphertext, createdBy: opts.userId });
      if (cur === undefined) {
        await tx.insert(secret).values({ kind, ref, ciphertext, version });
        return { ref, created: true, version };
      }
      await tx.update(secret).set({ ciphertext, version }).where(eq(secret.ref, ref));
      return { ref, created: false, version };
    });
    await this.events.record(
      'info',
      'secrets',
      out.created ? 'SECRET_CREATED' : 'SECRET_REPLACED',
      `${ref} → version ${out.version}`,
      { ref, version: out.version },
    );
    return out;
  }

  /**
   * Delete a secret unless running, the candidate or a pending commit still references it (409). TD-10a (review
   * 2.3f): the reference check and the delete are ONE transaction inside the commit lock (no promote can land between
   * the reads and the delete, in this or another API process), with the candidate and pending singleton rows locked
   * FOR UPDATE (no candidate edit slips in either); secret and versions go together. Review M2: like a commit, it
   * does not queue behind one — 409 `commit-busy` after the lock wait (`commits.userExclusive`).
   */
  async delete(kind: string, name: string): Promise<void> {
    const ref = `${kind}/${name}`;
    await this.commits.userExclusive(() =>
      this.db.transaction(async (tx) => {
        const [cand] = await tx
          .select({ payload: configCandidate.payload })
          .from(configCandidate)
          .for('update');
        const [pend] = await tx
          .select({ payload: configPending.payload })
          .from(configPending)
          .for('update');
        const [running] = await tx
          .select({ payload: configRevision.payload })
          .from(configRevision)
          .orderBy(desc(configRevision.id))
          .limit(1);
        const users = [
          ...secretRefs(running?.payload ?? {}),
          ...secretRefs(cand?.payload ?? {}),
          ...secretRefs(pend?.payload ?? {}),
        ].filter((r) => r.ref === ref);
        if (users.length > 0) {
          throw problems.conflict(
            'secret-in-use',
            `secret '${ref}' is referenced by the configuration`,
            {
              errors: users.map((u) => ({ pointer: u.pointer, message: `references ${ref}` })),
            },
          );
        }
        const deleted = await tx
          .delete(secret)
          .where(eq(secret.ref, ref))
          .returning({ id: secret.id });
        if (deleted.length === 0) throw problems.notFound(`secret '${ref}' does not exist`);
        await tx.delete(secretVersion).where(eq(secretVersion.ref, ref));
      }),
    );
    await this.events.record('info', 'secrets', 'SECRET_DELETED', ref, { ref });
  }
}
