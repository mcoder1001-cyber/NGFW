import { createCipheriv, createDecipheriv, createHash, randomBytes, scrypt } from 'node:crypto';
import { promisify } from 'node:util';
import { gzipSync, gunzipSync } from 'node:zlib';
import { z } from 'zod';
import { problems } from '../../common/problem.js';

const derive = promisify(scrypt);
const MAGIC = Buffer.from('NGFWBKP1');
export const MAX_ARCHIVE = 32 * 1024 * 1024;
export const Archive = z.strictObject({
  manifest: z.strictObject({
    format: z.literal(1),
    schemaVersion: z.string().regex(/^1\.\d+$/),
    revision: z.number().int().nullable(),
    hash: z.string().regex(/^[a-f0-9]{64}$/),
    createdAt: z.string().datetime(),
    createdBy: z.string().max(128),
  }),
  running: z.record(z.string(), z.unknown()),
  revisions: z.array(z.record(z.string(), z.unknown())).max(1000),
  secrets: z
    .array(
      z.strictObject({
        ref: z.string().max(256),
        value: z.string().max(1024 * 1024),
        version: z.number().int().positive(),
      }),
    )
    .max(4096),
  audit: z.array(z.record(z.string(), z.unknown())).max(10000),
});
export type BackupArchive = z.infer<typeof Archive>;
export const hashDocument = (doc: unknown): string =>
  createHash('sha256').update(JSON.stringify(doc)).digest('hex');
function password(passphrase: string): void {
  if (typeof passphrase !== 'string' || passphrase.length < 12 || passphrase.length > 1024)
    throw problems.badRequest('passphrase must contain 12–1024 characters', [
      { pointer: '/passphrase', message: 'invalid passphrase length' },
    ]);
}
/** Fixed KDF parameters and authenticated header: an untrusted archive cannot request arbitrary CPU/memory. */
export async function encryptArchive(archive: BackupArchive, passphrase: string): Promise<Buffer> {
  password(passphrase);
  const raw = Buffer.from(JSON.stringify(Archive.parse(archive)));
  if (raw.length > MAX_ARCHIVE) {
    raw.fill(0);
    throw problems.badRequest('backup exceeds the expanded archive limit');
  }
  const data = gzipSync(raw, { level: 6 });
  raw.fill(0);
  if (data.length > MAX_ARCHIVE - 64) throw problems.badRequest('backup exceeds the archive limit');
  const salt = randomBytes(16),
    iv = randomBytes(12);
  const key = (await derive(passphrase, salt, 32)) as Buffer;
  try {
    const header = Buffer.concat([MAGIC, salt, iv]);
    const cipher = createCipheriv('aes-256-gcm', key, iv);
    cipher.setAAD(header);
    const ciphertext = Buffer.concat([cipher.update(data), cipher.final()]);
    return Buffer.concat([header, cipher.getAuthTag(), ciphertext]);
  } finally {
    key.fill(0);
    data.fill(0);
  }
}
export async function decryptArchive(bytes: Buffer, passphrase: string): Promise<BackupArchive> {
  password(passphrase);
  if (bytes.length < 52 || bytes.length > MAX_ARCHIVE || !bytes.subarray(0, 8).equals(MAGIC))
    throw problems.badRequest('invalid or unsupported backup archive');
  const key = (await derive(passphrase, bytes.subarray(8, 24), 32)) as Buffer;
  let plain: Buffer | undefined;
  try {
    const d = createDecipheriv('aes-256-gcm', key, bytes.subarray(24, 36));
    d.setAAD(bytes.subarray(0, 36));
    d.setAuthTag(bytes.subarray(36, 52));
    plain = Buffer.concat([d.update(bytes.subarray(52)), d.final()]);
    const json = gunzipSync(plain, { maxOutputLength: MAX_ARCHIVE });
    try {
      const archive = Archive.parse(JSON.parse(json.toString('utf8')));
      if (hashDocument(archive.running) !== archive.manifest.hash) throw new Error('hash');
      return archive;
    } finally {
      json.fill(0);
    }
  } catch {
    throw problems.badRequest('backup authentication or format validation failed');
  } finally {
    key.fill(0);
    plain?.fill(0);
  }
}
