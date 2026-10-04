import { describe, expect, it } from 'vitest';
import { RipSchema, RipngSchema } from './domains/routing.js';

describe('RIPv2 authentication contract', () => {
  const interfaces = (auth: unknown) => ({ loop0: { auth } });
  it('accepts an MD5 password reference and explicit unauthenticated mode', () => {
    expect(
      RipSchema.parse({
        interfaces: interfaces({ type: 'md5', keyId: 255, keyRef: 'password/rip' }),
      }).interfaces.loop0?.auth?.keyRef,
    ).toBe('password/rip');
    expect(
      RipSchema.parse({ interfaces: interfaces({ type: 'none' }) }).interfaces.loop0?.auth?.type,
    ).toBe('none');
  });
  it.each([
    { type: 'md5', keyRef: 'password/rip' },
    { type: 'md5', keyId: 0, keyRef: 'password/rip' },
    { type: 'md5', keyId: 256, keyRef: 'password/rip' },
    { type: 'md5', keyId: 1, keyRef: 'psk/rip' },
    { type: 'none', keyRef: 'password/rip' },
    { type: 'text', keyRef: 'password/rip' },
  ])('rejects inconsistent or unsupported authentication %j', (auth) => {
    expect(RipSchema.safeParse({ interfaces: interfaces(auth) }).success).toBe(false);
  });
  it('rejects RIPv2 authentication on RIPng', () => {
    expect(
      RipngSchema.safeParse({
        interfaces: interfaces({ type: 'md5', keyId: 1, keyRef: 'password/rip' }),
      }).success,
    ).toBe(false);
  });
});
