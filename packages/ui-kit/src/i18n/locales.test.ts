import { describe, expect, it } from 'vitest';
import { en } from './locales/en.js';
import { fa } from './locales/fa.js';

function leaves(o: Record<string, unknown>, prefix = ''): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(o)) {
    const key = prefix ? `${prefix}.${k}` : k;
    if (typeof v === 'string') out[key] = v;
    else Object.assign(out, leaves(v as Record<string, unknown>, key));
  }
  return out;
}

describe('ui-kit locales', () => {
  it('fa has exactly the keys of en, all non-empty, with the same interpolation variables', () => {
    const e = leaves(en);
    const f = leaves(fa);
    expect(Object.keys(f).sort()).toEqual(Object.keys(e).sort());
    for (const [k, v] of Object.entries(f)) {
      expect(v.trim(), k).not.toBe('');
      const vars = (s: string) => [...s.matchAll(/\{\{(\w+)\}\}/g)].map((m) => m[1]).sort();
      expect(vars(v), k).toEqual(vars(e[k]!));
    }
  });
});
