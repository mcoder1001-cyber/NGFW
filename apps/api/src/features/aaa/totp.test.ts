import { describe, expect, it } from 'vitest';
import { generateSecret, otpauthUri, recoveryCodes, totpCode, verifyTotp } from './totp.js';

describe('TOTP (RFC 6238)', () => {
  it('matches the RFC 6238 SHA1 test vector', () => {
    // RFC 6238 Appendix B: secret "12345678901234567890" = base32 GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ; T=59s → 94287082
    const secret = 'GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ';
    expect(totpCode(secret, 59_000)).toBe('287082');
  });

  it('verifies the current code and tolerates ±1 step, rejects others', () => {
    const s = generateSecret();
    const now = 1_700_000_000_000;
    expect(verifyTotp(s, totpCode(s, now), now)).toBe(true);
    expect(verifyTotp(s, totpCode(s, now - 30_000), now)).toBe(true); // previous step
    expect(verifyTotp(s, totpCode(s, now + 30_000), now)).toBe(true); // next step
    expect(verifyTotp(s, totpCode(s, now + 120_000), now)).toBe(false); // too far
    expect(verifyTotp(s, '000000', now) && totpCode(s, now) !== '000000').toBe(false);
    expect(verifyTotp(s, 'abc', now)).toBe(false);
    expect(verifyTotp(s, '12345', now)).toBe(false);
  });

  it('produces a scannable otpauth URI and distinct recovery codes', () => {
    const s = generateSecret();
    const uri = otpauthUri(s, 'alice', 'vrx');
    expect(uri).toContain('otpauth://totp/vrx%3Aalice?');
    expect(uri).toContain(`secret=${s}`);
    const codes = recoveryCodes(10);
    expect(new Set(codes).size).toBe(10);
  });
});
