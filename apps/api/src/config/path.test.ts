import { describe, expect, it } from 'vitest';
import type { ProblemError } from '../common/problem.js';
import { pointerFromUrl } from './path.js';

const P = '/api/v1/config';

describe('pointerFromUrl', () => {
  it('maps the URL path to a JSON pointer, first segment a root key', () => {
    expect(pointerFromUrl('/api/v1/config/interfaces/loop0/mtu', P)).toBe('/interfaces/loop0/mtu');
    expect(pointerFromUrl('/api/v1/config/system?x=1', P)).toBe('/system');
    expect(pointerFromUrl('/api/v1/config/', P)).toBe('');
  });

  it('accepts ~1 and %2F for a slash inside a name', () => {
    expect(pointerFromUrl('/api/v1/config/interfaces/TenGigabitEthernet0~10~10', P)).toBe(
      '/interfaces/TenGigabitEthernet0~10~10',
    );
    expect(pointerFromUrl('/api/v1/config/interfaces/TenGigabitEthernet0%2F0%2F0/mtu', P)).toBe(
      '/interfaces/TenGigabitEthernet0~10~10/mtu',
    );
  });

  it('404 for a non-domain, 400 for broken escapes/encoding', () => {
    const status = (url: string) => {
      try {
        pointerFromUrl(url, P);
        return 0;
      } catch (e) {
        return (e as ProblemError).getStatus();
      }
    };
    expect(status('/api/v1/config/bogus/x')).toBe(404);
    expect(status('/api/v1/config/interfaces/a~2b')).toBe(400);
    expect(status('/api/v1/config/interfaces/%E0%A4%A')).toBe(400);
  });
});

describe('pointerFromUrl — control characters (TD-2 #4, D-049)', () => {
  it.each([
    ['ESC', '%1B%5B2K'],
    ['CR', 'a%0Db'],
    ['C1 CSI', '%C2%9B31m'],
    ['RLO', 'x%E2%80%AEy'],
    ['FSI', 'x%E2%81%A8y'],
  ])('%s in a segment → 400 with a pointer', (_n, seg) => {
    try {
      pointerFromUrl(`/api/v1/config/routing/bgp/neighbors/${seg}`, P);
      expect.unreachable();
    } catch (e) {
      const p = (e as ProblemError).body();
      expect(p.status).toBe(400);
      expect(p.errors?.[0]?.pointer).toMatch(/^\/routing\/bgp\/neighbors\/.*\\u\{/);
      expect(JSON.stringify(p)).not.toMatch(/\\u001b|\\r|\u009b|\u202e|\u2068/);
    }
  });
});
