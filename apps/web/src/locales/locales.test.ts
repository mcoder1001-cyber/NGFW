import { readdirSync, readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

// Review P07a L2: en/fa key parity for every web namespace (ui-kit has its own test).
const here = dirname(fileURLToPath(import.meta.url));
const load = (lang: string, ns: string) => JSON.parse(readFileSync(join(here, lang, `${ns}.json`), 'utf8')) as Record<string, unknown>;

function leaves(o: Record<string, unknown>, prefix = ''): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(o)) {
    const key = prefix ? `${prefix}.${k}` : k;
    if (typeof v === 'string') out[key] = v;
    else Object.assign(out, leaves(v as Record<string, unknown>, key));
  }
  return out;
}

const vars = (s: string) => [...s.matchAll(/\{\{(\w+)\}\}/g)].map((m) => m[1]).sort();

describe('web locales', () => {
  const namespaces = readdirSync(join(here, 'en')).filter((f) => f.endsWith('.json')).map((f) => f.slice(0, -5));

  it('en and fa ship the same namespace files', () => {
    expect(readdirSync(join(here, 'fa')).filter((f) => f.endsWith('.json')).sort()).toEqual(namespaces.map((n) => `${n}.json`).sort());
    expect(namespaces.length).toBeGreaterThan(0);
  });

  it.each(namespaces)('%s: fa has exactly the keys of en, all non-empty, with the same interpolation variables', (ns) => {
    const e = leaves(load('en', ns));
    const f = leaves(load('fa', ns));
    expect(Object.keys(f).sort()).toEqual(Object.keys(e).sort());
    for (const [k, v] of Object.entries(f)) {
      expect(v.trim(), k).not.toBe('');
      expect(vars(v), k).toEqual(vars(e[k]!));
    }
  });
});
