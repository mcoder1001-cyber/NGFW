import { describe, expect, it } from 'vitest';
import { rootSchema } from './schema/registry';
import { resultKeyText } from './product-text';

const locales = import.meta.glob('./locales/{en,fa}/*.json', { eager: true, import: 'default' });
const brands = /\b(?:FRRouting|FRR|VPP|strongSwan)\b/i;

function strings(value: unknown): string[] {
  if (typeof value === 'string') return [value];
  if (Array.isArray(value)) return value.flatMap(strings);
  if (value !== null && typeof value === 'object') return Object.values(value).flatMap(strings);
  return [];
}

describe('product-owned presentation text', () => {
  it('preserves result identities and user names while labelling a known service descriptor prefix', () => {
    expect(resultKeyText('nat/pool/p1')).toBe('nat/pool/p1');
    expect(resultKeyText('nat/pool/VPP')).toBe('nat/pool/VPP');
    expect(resultKeyText('frr.config/customer-FRR')).toBe('routing service.config/customer-FRR');
    expect(resultKeyText('strongswan/tunnel-vpp')).toBe('IPsec service/tunnel-vpp');
  });
  it('has no implementation branding in either language resource', () => {
    for (const [path, resource] of Object.entries(locales)) {
      expect(
        strings(resource).filter((text) => brands.test(text)),
        path,
      ).toEqual([]);
    }
  });

  it('adapts every schema label and description, including generic editor fallbacks', () => {
    const metadata: string[] = [];
    function visit(value: unknown) {
      if (value === null || typeof value !== 'object') return;
      for (const [key, child] of Object.entries(value)) {
        if (
          ['title', 'description', 'help', 'group', 'placeholder'].includes(key) &&
          typeof child === 'string'
        )
          metadata.push(child);
        else visit(child);
      }
    }
    visit(rootSchema);
    expect(metadata.filter((text) => brands.test(text))).toEqual([]);
  });
});
