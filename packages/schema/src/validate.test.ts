import { describe, expect, it } from 'vitest';
import { RootConfig } from './index.js';
import { pointerIssues, validateConfig } from './validate.js';

const ADMIN = { username: 'admin', role: 'admin', passwordHash: '$vrx-test$VRX_TEST_HASH_admin' };

describe('validateConfig', () => {
  it('returns the parsed config (defaults filled) for a valid document', () => {
    const result = validateConfig({ management: { users: [ADMIN] } });
    expect(result.ok).toBe(true);
    if (result.ok) expect(result.config.system.timezone).toBe('UTC');
    expect(validateConfig({}).ok).toBe(true); // D-048: the empty document is valid in both tiers
  });

  it('reports schema failures with escaped RFC 6901 pointers, sorted', () => {
    const result = validateConfig({
      interfaces: { 'TenGigabitEthernet0/0/0': { mtu: 10, bogus: true, 'x/y': 1 } },
      system: { hostname: '-bad' },
    });
    expect(result).toMatchObject({ ok: false, tier: 'schema' });
    if (!result.ok) {
      expect(result.issues.map((i) => i.pointer)).toEqual([
        '/interfaces/TenGigabitEthernet0~10~10/bogus',
        '/interfaces/TenGigabitEthernet0~10~10/mtu',
        '/interfaces/TenGigabitEthernet0~10~10/x~1y',
        '/system/hostname',
      ]);
      expect(result.issues[0]?.message).toBe("unknown key 'bogus'");
    }
  });

  it('reports semantic failures after a successful parse', () => {
    const result = validateConfig({
      interfaces: { loop0: { vrf: 'nope' } },
      management: { users: [ADMIN] },
    });
    expect(result).toEqual({
      ok: false,
      tier: 'semantic',
      issues: [{ pointer: '/interfaces/loop0/vrf', message: "VRF 'nope' does not exist" }],
    });
  });

  it('never throws on garbage input', () => {
    expect(validateConfig(null).ok).toBe(false);
    expect(validateConfig('x').ok).toBe(false);
    expect(validateConfig([]).ok).toBe(false);
  });
});

describe('pointerIssues', () => {
  it('maps array indices and record keys', () => {
    const error = RootConfig.safeParse({
      routing: { static: [{ prefix: '10.0.0.1/24', nextHops: [{}] }] },
    }).error!;
    expect(pointerIssues(error).map((i) => i.pointer)).toEqual([
      '/routing/static/0/nextHops/0/address',
      '/routing/static/0/prefix',
    ]);
  });
});
