import { Inject, Injectable } from '@nestjs/common';
import { SECRET_KINDS, type SecretKind } from '@ngfw/schema';
import { asc, eq } from 'drizzle-orm';
import { createCipheriv, createDecipheriv, randomBytes } from 'node:crypto';
import { chmodSync, existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { dirname } from 'node:path';
import { ENV, type Env } from '../config.js';
import { problems } from '../common/problem.js';
import { DB, type Db } from '../db/db.js';
import { configCandidate, configRevision, secret } from '../db/schema.js';
import { secretRefs } from '../datastore/documents.js';
import { desc } from 'drizzle-orm';

export interface SecretMeta {
  ref: string;
  kind: SecretKind;
  name: string;
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

  encrypt(plain: string): string {
    const iv = randomBytes(12);
    const c = createCipheriv('aes-256-gcm', this.masterKey(), iv);
    const ct = Buffer.concat([c.update(plain, 'utf8'), c.final()]);
    return Buffer.concat([iv, c.getAuthTag(), ct]).toString('base64');
  }

  decrypt(blob: string): string {
    const raw = Buffer.from(blob, 'base64');
    const d = createDecipheriv('aes-256-gcm', this.masterKey(), raw.subarray(0, 12));
    d.setAuthTag(raw.subarray(12, 28));
    return Buffer.concat([d.update(raw.subarray(28)), d.final()]).toString('utf8');
  }

  async list(): Promise<SecretMeta[]> {
    const rows = await this.db
      .select({ ref: secret.ref, kind: secret.kind, createdAt: secret.createdAt })
      .from(secret)
      .orderBy(asc(secret.ref));
    return rows.map((r) => ({
      ...r,
      kind: r.kind as SecretKind,
      name: r.ref.slice(r.kind.length + 1),
    }));
  }

  /** Create or replace a secret; returns its reference. */
  async put(
    kind: SecretKind,
    name: string,
    value: string,
  ): Promise<{ ref: string; created: boolean }> {
    if (!(SECRET_KINDS as readonly string[]).includes(kind))
      throw problems.badRequest(`unknown secret kind '${kind}'`);
    const ref = `${kind}/${name}`;
    const ciphertext = this.encrypt(value);
    const existing = await this.db
      .select({ id: secret.id })
      .from(secret)
      .where(eq(secret.ref, ref));
    if (existing.length > 0) {
      await this.db.update(secret).set({ ciphertext }).where(eq(secret.ref, ref));
      return { ref, created: false };
    }
    await this.db.insert(secret).values({ kind, ref, ciphertext });
    return { ref, created: true };
  }

  /** Delete a secret unless running or the candidate still references it (409). */
  async delete(kind: string, name: string): Promise<void> {
    const ref = `${kind}/${name}`;
    const [running] = await this.db
      .select({ payload: configRevision.payload })
      .from(configRevision)
      .orderBy(desc(configRevision.id))
      .limit(1);
    const [cand] = await this.db.select({ payload: configCandidate.payload }).from(configCandidate);
    const users = [
      ...secretRefs(running?.payload ?? {}),
      ...secretRefs(cand?.payload ?? {}),
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
    const deleted = await this.db
      .delete(secret)
      .where(eq(secret.ref, ref))
      .returning({ id: secret.id });
    if (deleted.length === 0) throw problems.notFound(`secret '${ref}' does not exist`);
  }
}
