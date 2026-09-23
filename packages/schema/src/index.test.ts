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

  it('accepts an empty document and fills every domain with its (nested) defaults', () => {
    const r = RootConfig.parse({});
    expect(Object.keys(r)).toEqual([...ROOT_KEYS]);
    for (const key of ROOT_KEYS) expect(r[key]).toBeTypeOf('object');
    // prefault runs the domain schema on {} so nested defaults are filled (D-017)
    expect(r.system.hostname).toBe('vrx');
    expect(r.system.timezone).toBe('UTC');
    expect(r.system.ntp).toEqual({ enabled: true, servers: [], vrf: 'default' });
    expect(r.management.users).toEqual([]);
    expect(r.management.aaa.order).toEqual(['local']);
    expect(r.interfaces).toEqual({});
    expect(r.vrfs).toEqual({});
    expect(r.routing.static).toEqual([]);
  });

  it('accepts a partial document and keeps the given domains', () => {
    const r = RootConfig.parse({ system: { hostname: 'vrx-a' } });
    expect(r.system.hostname).toBe('vrx-a');
    expect(r.system.timezone).toBe('UTC');
    expect(r.vrfs).toEqual({});
  });

  it('is idempotent: parsing its own output yields the same document', () => {
    const once = RootConfig.parse({ interfaces: { loop0: { ipv4: ['10.255.0.1/32'] } } });
    expect(RootConfig.parse(once)).toEqual(once);
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
