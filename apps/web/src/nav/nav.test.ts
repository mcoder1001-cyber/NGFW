import { ROOT_KEYS } from '@ngfw/schema';
import { describe, expect, it } from 'vitest';
import { domains } from '../schema/registry';
import { buildNav, DOMAIN_GROUP, NAV_GROUPS, domainPath } from './nav';

describe('navigation from the schema (vdom.md guardrail 4)', () => {
  it('places every root key exactly once, in x-vrx-ui.order, with dashboard first and dev last', () => {
    const nav = buildNav(domains);
    const domainItems = nav.flatMap((g) => g.items).filter((i) => i.domain);
    expect([...domainItems.map((i) => i.domain)].sort()).toEqual([...ROOT_KEYS].sort());
    expect(new Set(domainItems.map((i) => i.domain)).size).toBe(ROOT_KEYS.length);
    // inside each group the schema order (x-vrx-ui.order) is kept
    const rank = new Map(domains.map((d, i) => [d.key, i]));
    for (const g of nav) {
      const ranks = g.items.filter((i) => i.domain).map((i) => rank.get(i.domain!)!);
      expect(ranks, g.id).toEqual([...ranks].sort((a, b) => a - b));
    }
    expect(nav[0]!.id).toBe('dashboard');
    expect(nav.at(-1)!.id).toBe('dev');
    expect(nav.map((g) => g.id)).toEqual(NAV_GROUPS.filter((g) => nav.some((n) => n.id === g)));
  });

  it('maps every root key to a group and builds stable paths', () => {
    for (const key of ROOT_KEYS) expect(DOMAIN_GROUP[key], key).toBeDefined();
    expect(domainPath('interfaces')).toBe('/interfaces');
    expect(domainPath('vrfs')).toBe('/routing/vrfs');
    expect(domainPath('acl')).toBe('/firewall/acl');
    expect(domainPath('management')).toBe('/system/management');
  });

  it('marks unbuilt screens unavailable and only the dashboard/dev demos available', () => {
    const nav = buildNav(domains);
    const available = nav.flatMap((g) => g.items).filter((i) => i.available).map((i) => i.id);
    expect(available).toEqual(['dashboard', 'dev-schema-form', 'dev-data-grid', 'dev-stream']);
  });
});
