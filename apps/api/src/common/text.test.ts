import { describe, expect, it } from 'vitest';
import { firstUnsafe, newUnsafeTextIssues, safeText, unsafeTextIssues, visible } from './text.js';
import { openapi } from './zod.js';

describe('safe text (TD-2 #4, D-049)', () => {
  const bad = {
    NUL: '\u0000',
    BEL: '\u0007',
    LF: '\n',
    CR: '\r',
    ESC: '\u001b',
    DEL: '\u007f',
    CSI: '\u009b',
    OSC: '\u009d',
    LS: '\u2028',
    PS: '\u2029',
    LRE: '\u202a',
    RLO: '\u202e',
    LRI: '\u2066',
    PDI: '\u2069',
  };

  it.each(Object.entries(bad))('single-line text rejects %s', (_n, c) => {
    expect(firstUnsafe(`ok ${c} ok`)).toMatch(/^U\+[0-9A-F]{4}$/);
    expect(safeText(100).safeParse(`x${c}y`).success).toBe(false);
  });

  it('accepts ordinary text, TAB, Persian and LRM/RLM', () => {
    for (const s of ['release 42: mtu 9000', 'a\tb', 'پیکربندی شبکه', 'x‎y‏z', '']) {
      expect(firstUnsafe(s)).toBeUndefined();
      expect(safeText(100).safeParse(s).success).toBe(true);
    }
  });

  it('the rule is a pattern in the OpenAPI schema', () => {
    const js = openapi(safeText(1024)) as { pattern?: string; maxLength?: number };
    expect(js.maxLength).toBe(1024);
    expect(js.pattern).toBeDefined();
    expect(new RegExp(js.pattern!, 'u').test('ok')).toBe(true);
    expect(new RegExp(js.pattern!, 'u').test('\u001b[2K')).toBe(false);
  });

  it('documents: values and member names, LF allowed in values, pointers exact', () => {
    const issues = unsafeTextIssues({
      a: { description: 'x\u001b[1Ay', banner: 'l1\nl2' },
      list: ['ok', 'b\u202ed'],
      ['k\u0007']: { deep: '\u001b' },
    });
    expect(issues.map((i) => i.pointer)).toEqual(['/a/description', '/list/1', '/k\\u{7}']);
    expect(issues.every((i) => i.rule === 'api.safe-text')).toBe(true);
    expect(unsafeTextIssues({ motd: 'a\n\tb', n: 1, t: true, z: null })).toEqual([]);
  });

  it('visible() never returns a raw control character', () => {
    expect(visible('a\u001b]52;c;x\u0007\n')).toBe('a\\u{1b}]52;c;x\\u{7}\\u{a}');
  });

  it('review L2: legacy unsafe data does not let an edit put ANOTHER unsafe value at the same pointer', () => {
    const base = { a: { description: 'old\u001b[1m' } };
    expect(newUnsafeTextIssues(base, base)).toEqual([]);
    expect(newUnsafeTextIssues(base, { a: { description: 'new\u001b]0;x\u0007' } })).toEqual([
      expect.objectContaining({ pointer: '/a/description' }),
    ]);
    expect(newUnsafeTextIssues(base, { a: { description: 'clean' } })).toEqual([]);
  });
});
