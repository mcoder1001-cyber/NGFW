import { createHash, createPublicKey, verify, type KeyObject } from 'node:crypto';
import { z } from 'zod';

/**
 * `.vrxlic` licence file (F-licensing): one JSON object `{format, license, signature}`. `signature` is a detached
 * Ed25519 signature (base64) over the canonical JSON of `license` (keys sorted recursively, no whitespace). The same
 * canonicalisation lives in `tools/license/vrx-license.mjs` (dependency-free CLI); `cli.test.ts` proves both agree.
 */
export const LICENSE_FORMAT = 'vrxlic/1';

export const LicenseSchema = z.strictObject({
  version: z.literal(1),
  licenseId: z.string().min(1).max(128),
  customer: z.string().min(1).max(256),
  issuedAt: z.iso.datetime({ offset: true }),
  notBefore: z.iso.datetime({ offset: true }),
  expiresAt: z.iso.datetime({ offset: true }),
  binding: z
    .strictObject({
      machineIdHash: z
        .string()
        .regex(/^[0-9a-f]{64}$/)
        .optional(),
      serial: z.string().min(1).max(256).optional(),
    })
    .default({}),
  entitlements: z.strictObject({
    features: z.array(z.string().min(1).max(64)).max(256),
    limits: z.record(z.string().min(1).max(64), z.number().int().min(0)).default({}),
  }),
});
export type License = z.output<typeof LicenseSchema>;

const FileSchema = z.strictObject({
  format: z.literal(LICENSE_FORMAT),
  license: z.unknown(),
  signature: z.string().regex(/^[A-Za-z0-9+/]+={0,2}$/),
});

/** Deterministic JSON: object keys sorted recursively, no insignificant whitespace. */
export function canonicalJson(value: unknown): string {
  if (Array.isArray(value)) return `[${value.map(canonicalJson).join(',')}]`;
  if (value !== null && typeof value === 'object') {
    const o = value as Record<string, unknown>;
    return `{${Object.keys(o)
      .filter((k) => o[k] !== undefined)
      .sort()
      .map((k) => `${JSON.stringify(k)}:${canonicalJson(o[k])}`)
      .join(',')}}`;
  }
  return JSON.stringify(value);
}

export class LicenseFormatError extends Error {
  constructor(
    readonly code: 'malformed' | 'signature',
    message: string,
  ) {
    super(message);
    this.name = 'LicenseFormatError';
  }
}

export function publicKeyFrom(pem: string): KeyObject {
  const key = createPublicKey(pem);
  if (key.asymmetricKeyType !== 'ed25519') throw new Error('licence public key is not Ed25519');
  return key;
}

/**
 * Parse a `.vrxlic` file and verify its signature against any of `keys`. Throws LicenseFormatError on a malformed
 * file or a bad signature (tampered byte, wrong key). Time and host binding are checked by `evaluate()`.
 */
export function parseAndVerify(text: string, keys: readonly KeyObject[]): License {
  let raw: unknown;
  try {
    raw = JSON.parse(text);
  } catch {
    throw new LicenseFormatError('malformed', 'licence file is not JSON');
  }
  const file = FileSchema.safeParse(raw);
  if (!file.success) throw new LicenseFormatError('malformed', 'not a vrxlic/1 licence file');
  const payload = Buffer.from(canonicalJson(file.data.license), 'utf8');
  const sig = Buffer.from(file.data.signature, 'base64');
  const ok = sig.length === 64 && keys.some((k) => verify(null, payload, k, sig));
  if (!ok) throw new LicenseFormatError('signature', 'licence signature is invalid');
  const lic = LicenseSchema.safeParse(file.data.license);
  if (!lic.success) throw new LicenseFormatError('malformed', 'licence content is invalid');
  return lic.data;
}

export const GRACE_DAYS = 30;
const DAY_MS = 86_400_000;

export type LicenseStatus = 'community' | 'valid' | 'grace' | 'expired' | 'invalid';

export interface HostIdentity {
  /** sha256 hex of /etc/machine-id (undefined when unreadable). */
  machineIdHash?: string | undefined;
  /** DMI product serial (or the VRX_LICENSE_SERIAL override). */
  serial?: string | undefined;
}

export interface Evaluation {
  status: LicenseStatus;
  /** Why the licence is invalid (not-yet-valid, binding mismatch). */
  reason?: string;
  /** Days until expiry (valid), until the end of grace (grace); 0 otherwise. */
  daysLeft: number;
}

export function machineIdHash(machineId: string): string {
  return createHash('sha256').update(machineId.trim(), 'utf8').digest('hex');
}

/** Time window + host binding of a signature-verified licence. */
export function evaluate(lic: License, now: Date, host: HostIdentity): Evaluation {
  const t = now.getTime();
  const b = lic.binding;
  if (b.machineIdHash !== undefined && b.machineIdHash !== host.machineIdHash)
    return {
      status: 'invalid',
      reason: 'machine-id binding does not match this host',
      daysLeft: 0,
    };
  if (b.serial !== undefined && b.serial !== host.serial)
    return { status: 'invalid', reason: 'serial binding does not match this host', daysLeft: 0 };
  if (t < Date.parse(lic.notBefore))
    return { status: 'invalid', reason: 'licence is not valid yet', daysLeft: 0 };
  const exp = Date.parse(lic.expiresAt);
  if (t <= exp) return { status: 'valid', daysLeft: Math.ceil((exp - t) / DAY_MS) };
  const graceEnd = exp + GRACE_DAYS * DAY_MS;
  if (t <= graceEnd) return { status: 'grace', daysLeft: Math.ceil((graceEnd - t) / DAY_MS) };
  return { status: 'expired', daysLeft: 0 };
}
