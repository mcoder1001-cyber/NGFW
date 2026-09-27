import { describe, expect, it } from 'vitest';
import { canonicalEntries, canonicalEntry, diffEntries, parseBlockList } from './global-blocking-parse.js';

describe('F-global-blocking block-list parser', () => {
  it('normalises addresses and prefixes, skips comments, reports invalid lines by number', () => {
    const p = parseBlockList(
      [
        '# feed v1',
        '192.0.2.7',
        '198.51.100.9/24   # host bits → 198.51.100.0/24',
        '',
        '2001:DB8:0:0:0:0:0:1',
        '2001:db8:abcd::/48',
        'not-an-ip',
        '10.0.0.0/33',
        'fe80::1%eth0',
      ].join('\n'),
    );
    expect(p.entries).toEqual(['192.0.2.7/32', '198.51.100.0/24', '2001:db8::1/128', '2001:db8:abcd::/48']);
    expect(p.invalid.map((i) => [i.line, i.reason])).toEqual([
      [7, 'not an IPv4 or IPv6 address'],
      [8, 'bad prefix length'],
      [9, 'zone indexes are not allowed'],
    ]);
    expect(p.normalised).toBe(1);
  });

  it('deduplicates and collapses covered prefixes (both families), CRLF files too', () => {
    const p = parseBlockList('10.1.2.3\r\n10.1.0.0/16\r\n10.1.2.3/32\r\n10.2.0.0/16\r\n::/0\r\n2001:db8::1\r\n');
    expect(p.entries).toEqual(['10.1.0.0/16', '10.2.0.0/16', '::/0']);
    expect(p.collapsed).toBe(3);
  });

  it('canonical forms and diffs', () => {
    expect(canonicalEntry('2001:0db8:0000:0000:0001:0000:0000:0001')).toBe('2001:db8::1:0:0:1/128');
    expect(canonicalEntry('1:0:0:2:0:0:0:3')).toBe('1:0:0:2::3/128'); // the longest zero run
    expect(canonicalEntry('x')).toBeUndefined();
    expect(canonicalEntries(['10.0.0.1/8', '10.9.9.9'])).toEqual(['10.0.0.0/8']);
    expect(diffEntries(['a', 'b'], ['b', 'c'])).toEqual({ added: ['c'], removed: ['a'] });
  });

  it('parses 200 000 entries quickly', () => {
    const lines: string[] = [];
    for (let i = 0; i < 200_000; i++) lines.push(`10.${(i >> 16) & 255}.${(i >> 8) & 255}.${i & 255}`);
    const t0 = performance.now();
    const p = parseBlockList(lines.join('\n'));
    const ms = performance.now() - t0;
    expect(p.entries).toHaveLength(200_000);
    expect(ms).toBeLessThan(5_000);
  });
});
