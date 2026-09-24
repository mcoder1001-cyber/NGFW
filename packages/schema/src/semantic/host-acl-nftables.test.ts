import { describe, expect, it } from 'vitest';
import { RootConfig } from '../index.js';
import { validateSemantics } from './index.js';

const withSettings = (hostSettings: unknown): unknown => ({ acl: { hostSettings } });

describe('acl.hostSettings (F-host-acl-nftables)', () => {
  it('is optional; when present every field has its default', () => {
    expect(RootConfig.parse({}).acl.hostSettings).toBeUndefined();
    expect(RootConfig.parse(withSettings({})).acl.hostSettings).toEqual({
      defaultInput: 'accept',
      allowIcmp: true,
      antiLockout: { enabled: true, sources: [], interfaces: [], ports: [22, 443] },
    });
  });

  it('rejects bad values at the schema tier', () => {
    for (const bad of [
      { defaultInput: 'reject' },
      { antiLockout: { ports: [] } },
      { antiLockout: { ports: [0] } },
      { antiLockout: { sources: ['10.0.0.0/33'] } },
      { antiLockout: { interfaces: ['eth0"; flush ruleset'] } },
      { antiLockout: { interfaces: ['a-name-longer-than-15'] } },
      { antiLockout: { interfaces: ['eth0\n}'] } },
      { unknown: true },
    ]) {
      expect(RootConfig.safeParse(withSettings(bad)).success, JSON.stringify(bad)).toBe(false);
    }
  });

  it('acl.host-settings reports duplicates with pointers (prefixes compare canonically)', () => {
    const config = RootConfig.parse(
      withSettings({
        antiLockout: {
          sources: ['10.0.0.0/24', '10.0.0.1/24', '2001:DB8::/32', '2001:db8::/32'],
          interfaces: ['ens192', 'ens192'],
          ports: [22, 443, 22],
        },
      }),
    );
    const issues = validateSemantics(config).filter((i) =>
      i.pointer.startsWith('/acl/hostSettings'),
    );
    expect(issues.map((i) => i.pointer).sort()).toEqual([
      '/acl/hostSettings/antiLockout/interfaces/1',
      '/acl/hostSettings/antiLockout/ports/2',
      '/acl/hostSettings/antiLockout/sources/1',
      '/acl/hostSettings/antiLockout/sources/3',
    ]);
  });

  it('a clean document has no findings', () => {
    const config = RootConfig.parse(
      withSettings({
        antiLockout: { sources: ['10.0.0.0/24', '2001:db8::/32'], interfaces: ['ens192'] },
      }),
    );
    expect(validateSemantics(config)).toEqual([]);
  });
});
