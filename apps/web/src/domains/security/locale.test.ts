import en from '../../locales/en/security.json';
import fa from '../../locales/fa/security.json';
import { describe, expect, it } from 'vitest';

// F-security-page: the security namespace ships the same keys in en and fa, and fa is actually translated
// (the same check the management namespace has).
function leaves(o: Record<string, unknown>, prefix = ''): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(o)) {
    const key = prefix ? `${prefix}.${k}` : k;
    if (typeof v === 'string') out[key] = v;
    else Object.assign(out, leaves(v as Record<string, unknown>, key));
  }
  return out;
}

/** A sentence (three or more words) left in English: technical terms alone (SSH, API, IKE/EAP) are fine. */
const englishSentence = (s: string) => /[A-Za-z]+\s+[A-Za-z]+\s+[A-Za-z]+/.test(s.replace(/\{\{\w+\}\}/g, ''));

describe('security locales', () => {
  const e = leaves(en);
  const f = leaves(fa);
  it('en and fa have identical key sets', () => {
    expect(Object.keys(f).sort()).toEqual(Object.keys(e).sort());
  });
  it('fa leaves no English sentences behind', () => {
    expect(
      Object.entries(f)
        .filter(([, v]) => englishSentence(v))
        .map(([k]) => k),
    ).toEqual([]);
  });
});
