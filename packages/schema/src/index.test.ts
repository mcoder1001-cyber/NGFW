import { describe, expect, it } from 'vitest';
import { RootConfig, ROOT_KEYS } from './index.js';

describe('RootConfig', () => {
  it('lists the 13 top-level keys in the documented order (docs/04-api-datamodel.md)', () => {
    expect(ROOT_KEYS).toEqual([
      'system',
      'dataplane',
      'interfaces',
      'vrfs',
      'routing',
      'nat',
      'objects',
      'acl',
      'vpn',
      'tunnels',
      'services',
      'ha',
      'management',
    ]);
  });

  it('accepts an empty document and fills every domain with its default', () => {
    const r = RootConfig.parse({});
    expect(Object.keys(r)).toEqual([...ROOT_KEYS]);
    // Domains may fill nested defaults (D-017 prefault) — assert shape, not emptiness.
    for (const key of ROOT_KEYS) expect(r[key]).toBeTypeOf('object');
  });

  it('accepts a partial document and keeps the given domains', () => {
    const r = RootConfig.parse({ system: { hostname: 'vrx-a' } });
    expect(r.system).toMatchObject({ hostname: 'vrx-a' });
    expect(r.vrfs).toBeTypeOf('object');
  });

  it('rejects unknown top-level keys (strict root)', () => {
    expect(RootConfig.safeParse({ bogus: 1 }).success).toBe(false);
    expect(RootConfig.safeParse({ tenants: {} }).success).toBe(false);
  });

  it('rejects a non-object domain value', () => {
    expect(RootConfig.safeParse({ system: 'x' }).success).toBe(false);
    expect(RootConfig.safeParse({ nat: null }).success).toBe(false);
  });
});
