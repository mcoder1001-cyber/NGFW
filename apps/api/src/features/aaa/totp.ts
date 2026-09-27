import { createHmac, randomBytes, timingSafeEqual } from 'node:crypto';

/**
 * F-aaa: TOTP (RFC 6238, HMAC-SHA1, 6 digits, 30 s) implemented with node crypto — no external library. Used for the
 * second factor. The shared secret is base32 (RFC 4648) so it fits an otpauth:// URI for authenticator apps; it is
 * stored encrypted (secret store) by the caller, never in clear.
 */

const B32 = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567';

/** A new 160-bit base32 secret for enrolment. */
export function generateSecret(): string {
  const bytes = randomBytes(20);
  let bits = '';
  for (const b of bytes) bits += b.toString(2).padStart(8, '0');
  let out = '';
  for (let i = 0; i + 5 <= bits.length; i += 5) out += B32[parseInt(bits.slice(i, i + 5), 2)];
  return out;
}

function base32Decode(s: string): Buffer {
  const clean = s.toUpperCase().replace(/=+$/, '').replace(/\s+/g, '');
  let bits = '';
  for (const c of clean) {
    const idx = B32.indexOf(c);
    if (idx < 0) throw new Error('bad base32 secret');
    bits += idx.toString(2).padStart(5, '0');
  }
  const bytes: number[] = [];
  for (let i = 0; i + 8 <= bits.length; i += 8) bytes.push(parseInt(bits.slice(i, i + 8), 2));
  return Buffer.from(bytes);
}

/** The 6-digit code for `secret` at time `atMs` (default now), for time step `stepMs` (default 30 s). */
export function totpCode(secret: string, atMs: number = Date.now(), stepMs = 30_000): string {
  const counter = Math.floor(atMs / stepMs);
  const buf = Buffer.alloc(8);
  buf.writeBigUInt64BE(BigInt(counter));
  const hmac = createHmac('sha1', base32Decode(secret)).update(buf).digest();
  const offset = (hmac[hmac.length - 1] ?? 0) & 0x0f;
  const bin =
    (((hmac[offset] ?? 0) & 0x7f) << 24) |
    (((hmac[offset + 1] ?? 0) & 0xff) << 16) |
    (((hmac[offset + 2] ?? 0) & 0xff) << 8) |
    ((hmac[offset + 3] ?? 0) & 0xff);
  return (bin % 1_000_000).toString().padStart(6, '0');
}

/** Verify `code` against `secret`, accepting the current step and one step either side (clock skew). */
export function verifyTotp(
  secret: string,
  code: string,
  atMs: number = Date.now(),
  stepMs = 30_000,
): boolean {
  const trimmed = code.trim();
  if (!/^[0-9]{6}$/.test(trimmed)) return false;
  for (const d of [-1, 0, 1]) {
    const expected = totpCode(secret, atMs + d * stepMs, stepMs);
    const a = Buffer.from(expected);
    const b = Buffer.from(trimmed);
    if (a.length === b.length && timingSafeEqual(a, b)) return true;
  }
  return false;
}

/** The otpauth:// URI an authenticator app scans. */
export function otpauthUri(secret: string, account: string, issuer: string): string {
  const label = encodeURIComponent(`${issuer}:${account}`);
  const params = new URLSearchParams({
    secret,
    issuer,
    algorithm: 'SHA1',
    digits: '6',
    period: '30',
  });
  return `otpauth://totp/${label}?${params.toString()}`;
}

/**
 * N single-use recovery codes (returned once at enrolment; stored hashed by the caller).
 *
 * 80 bits each: only a sha256 hash is kept, so the codes must stay out of reach of an offline search if that hash
 * ever leaks — which a short code would not be. They are the fallback for a lost authenticator, so they are as
 * powerful as the second factor itself.
 */
export function recoveryCodes(n = 10): string[] {
  const out: string[] = [];
  for (let i = 0; i < n; i++) out.push(randomBytes(10).toString('hex'));
  return out;
}
