import { configPointerPath, configUrl } from '@ngfw/api-client';
import { jsonPointer } from '@ngfw/schema';
import { describe, expect, it } from 'vitest';

/**
 * D-UDE-1 (review, fix round 1): `packages/api-client` has no vitest of its own (its "test" script only
 * type-checks a consumer against the built package, `packages/api-client/test/consumer.ts`) — these run here
 * instead, against the real export, since `@ngfw/web` already depends on `@ngfw/api-client` and vitest. No
 * dependency was added to either package.
 */
describe('configPointerPath', () => {
  it('keeps a real "/" between segments (unlike the generated client\'s own path serializer)', () => {
    expect(configPointerPath('/interfaces/eth0/mtu')).toBe('interfaces/eth0/mtu');
  });

  it('a single-segment (domain-root) pointer round-trips exactly as before (no regression for existing call sites)', () => {
    expect(configPointerPath('/interfaces')).toBe('interfaces');
  });

  it('the whole-document pointer has no path suffix', () => {
    expect(configPointerPath('')).toBe('');
    expect(configPointerPath('/')).toBe('');
  });

  it('preserves RFC 6901 escapes literally (they are not further percent-encoded)', () => {
    // /interfaces/TenGigabitEthernet0~10~10/mtu — the pointer already escaped the interface's own "/"
    const pointer = jsonPointer('interfaces', 'TenGigabitEthernet0/0/0', 'mtu');
    expect(pointer).toBe('/interfaces/TenGigabitEthernet0~10~10/mtu');
    expect(configPointerPath(pointer)).toBe('interfaces/TenGigabitEthernet0~10~10/mtu');
  });

  it('a "~" in a name (escaped as ~0 by jsonPointer) survives untouched', () => {
    const pointer = jsonPointer('objects', 'weird~name', 'value');
    expect(pointer).toBe('/objects/weird~0name/value');
    expect(configPointerPath(pointer)).toBe('objects/weird~0name/value');
  });

  it('a literal "%" or non-ASCII segment is percent-encoded for transport, per-segment', () => {
    expect(configPointerPath('/objects/100%/value')).toBe('objects/100%25/value');
    expect(configPointerPath('/objects/دامنه/value')).toBe(`objects/${encodeURIComponent('دامنه')}/value`);
  });

  it('a segment that is itself only digits/letters needs no encoding (readable URLs stay readable)', () => {
    expect(configPointerPath('/interfaces/eth0/subinterfaces/100')).toBe('interfaces/eth0/subinterfaces/100');
  });
});

describe('configUrl', () => {
  it('builds the candidate and running/write URLs from the same pointer', () => {
    const pointer = '/interfaces/eth0/mtu';
    expect(configUrl('/api/v1/config/candidate', pointer)).toBe('/api/v1/config/candidate/interfaces/eth0/mtu');
    expect(configUrl('/api/v1/config', pointer)).toBe('/api/v1/config/interfaces/eth0/mtu');
  });

  it('the root pointer resolves to the prefix alone (the whole-document routes)', () => {
    expect(configUrl('/api/v1/config', '')).toBe('/api/v1/config');
    expect(configUrl('/api/v1/config/candidate', '/')).toBe('/api/v1/config/candidate');
  });
});
