import { describe, expect, it } from 'vitest';
import en from '../../../locales/en/snmp.json';
import fa from '../../../locales/fa/snmp.json';
import { servicesTabs } from '../tabs';

function keys(o: object, prefix = ''): string[] {
  return Object.entries(o).flatMap(([k, v]) =>
    typeof v === 'object' && v !== null ? keys(v as object, `${prefix}${k}.`) : [`${prefix}${k}`],
  );
}

describe('Services → SNMP (F-snmp)', () => {
  it('is registered as a services tab', () => {
    expect(servicesTabs.map((t) => t.id)).toContain('snmp');
  });
  it('has the same keys in en and fa', () => {
    expect(keys(fa).sort()).toEqual(keys(en).sort());
  });
});
