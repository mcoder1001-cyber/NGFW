import { describe, expect, it } from 'vitest';
import { createInstance } from 'i18next';
import { productEngineLabels, productWording } from './product-wording.js';
import { createSchemaText } from '../schema-form/text.js';

describe('product presentation wording', () => {
  it('names services in both languages, including daemon diagnostics and paths', () => {
    const diagnostic =
      'FRRouting failed to reload /etc/frr/frr.conf; VPP disconnected from /run/vpp/api.sock; strongSwan (kernel-vpp) is unavailable';
    for (const language of ['en', 'fa']) {
      const shown = productWording(diagnostic, language);
      expect(shown).not.toMatch(/frr|vpp|strongswan/i);
      expect(shown).toContain(language === 'fa' ? 'سرویس مسیریابی' : 'routing service');
      expect(shown).not.toContain('/etc/');
    }
    expect(diagnostic).toContain('/etc/frr/frr.conf');
    expect(productWording('VPPX on eth0')).toBe('VPPX on eth0');
    expect(productWording('MTU 9000 on eth0: permission denied')).toBe(
      'MTU 9000 on eth0: permission denied',
    );
  });

  it('translates schema fallback text and engine labels without changing wire values', async () => {
    const i18n = createInstance();
    await i18n.init({ lng: 'fa', resources: {} });
    const text = createSchemaText(i18n, undefined);
    expect(text.title('route', 'Program via FRR')).not.toMatch(/FRR/);
    expect(text.help('engine', 'strongSwan (kernel-vpp)', undefined)).not.toMatch(
      /strongswan|vpp/i,
    );
    expect(text.group('', 'frr-linuxcp')).toBe('مسیریابی');
    const values = ['vpp', 'strongswan', 'vpp-ikev2'];
    expect(values.map((value) => text.enumLabels('engine')?.[value])).toEqual([
      'صفحهٔ داده',
      'سرویس IPsec',
      'IKEv2 داخلی',
    ]);
    expect(Object.keys(productEngineLabels())).toEqual(values);
  });
});
