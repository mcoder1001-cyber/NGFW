import { describe, expect, it } from 'vitest';
import { IsisSchema, RipSchema, RipngSchema } from './routing.js';

describe('IS-IS and RIP additive contracts', () => {
  it('keeps both IS-IS families enabled by default', () => {
    const result = IsisSchema.parse({
      net: '49.0001.1921.6800.1001.00',
      interfaces: { loop0: {} },
    });
    expect(result.interfaces.loop0).toMatchObject({ ipv4: true, ipv6: true });
    expect(IsisSchema.safeParse({ ...result, areaPasswordRef: 'password/area-key' }).success).toBe(
      true,
    );
    expect(IsisSchema.safeParse({ ...result, domainPasswordRef: 'plaintext' }).success).toBe(false);
  });
  it('accepts version 2 only and enforces separate network families', () => {
    expect(RipSchema.parse({}).version).toBe(2);
    expect(RipSchema.safeParse({ version: 1 }).success).toBe(false);
    expect(RipSchema.safeParse({ networks: ['2001:db8::/64'] }).success).toBe(false);
    expect(RipngSchema.safeParse({ networks: ['10.0.0.0/8'] }).success).toBe(false);
    expect(RipngSchema.parse({ networks: ['2001:db8::/64'] }).networks).toEqual(['2001:db8::/64']);
  });
});
