import { describe, expect, it } from 'vitest';
import {
  escapePointerSegment,
  jsonPointer,
  parsePointer,
  unescapePointerSegment,
} from './pointer.js';

describe('jsonPointer (RFC 6901)', () => {
  it('escapes ~ and / in VPP interface names', () => {
    expect(jsonPointer('interfaces', 'TenGigabitEthernet0/0/0', 'mtu')).toBe(
      '/interfaces/TenGigabitEthernet0~10~10/mtu',
    );
    expect(escapePointerSegment('a~b/c')).toBe('a~0b~1c');
    expect(unescapePointerSegment('a~0b~1c')).toBe('a~b/c');
    expect(unescapePointerSegment('~01')).toBe('~1');
  });

  it('accepts numeric segments (array indices)', () => {
    expect(jsonPointer('routing', 'static', 0, 'nextHops', 1)).toBe('/routing/static/0/nextHops/1');
  });

  it('returns the whole-document pointer for no segments', () => {
    expect(jsonPointer()).toBe('');
    expect(parsePointer('')).toEqual([]);
  });

  it('round-trips through parsePointer', () => {
    const segments = ['interfaces', 'TenGigabitEthernet0/0/0', 'sub~1', ''];
    expect(parsePointer(jsonPointer(...segments))).toEqual(segments);
  });

  it('rejects pointers that do not start with /', () => {
    expect(() => parsePointer('interfaces')).toThrow(/invalid JSON pointer/);
  });
});
