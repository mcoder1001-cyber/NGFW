import { ROOT_KEYS } from '@ngfw/schema';
import { describe, expect, it } from 'vitest';
import { domains } from '../schema/registry';
import { buildNav, currentNavPath, DOMAIN_GROUP, NAV_GROUPS, domainPath } from './nav';

describe('navigation from the schema (vdom.md guardrail 4)', () => {
  it('places every root key exactly once, in x-vrx-ui.order, with dashboard first and dev last', () => {
    const nav = buildNav(domains, { devRoutes: true });
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

  it('marks unbuilt screens unavailable; dashboard, users, revisions (P07b) and dev demos are available', () => {
    const nav = buildNav(domains, { devRoutes: true });
    const available = nav.flatMap((g) => g.items).filter((i) => i.available).map((i) => i.id);
    expect(available).toEqual([
      'dashboard',
      'interfaces',
      // Feature items, in navigation order (group, then schema order, then non-domain items); one line under the feature's anchor.
      // wave-A: F-bonding
      'bonds',
      // wave-A: F-bridge-l2
      'bridging',
      // wave-A: F-loopback-bvi-gso-lldp-span
      'lldp',
      'mirroring',
      // wave-A: F-vrf-static-ecmp
      'vrfs',
      'routing',
      // wave-A: F-neighbors-ra
      // wave-A: F-rpf-adl-pbr
      // wave-A: P12
      // wave-A: F-nat44-ed-sessions
      // wave-A: F-object-model
      // wave-A: F-acl
      // wave-A: F-host-acl-nftables
      // wave-BC: F-ospf
      // wave-BC: F-isis-rip
      // wave-BC: F-bfd-redistribution
      // wave-BC: F-mpls-srmpls
      // wave-BC: F-igmp-mfib
      // wave-BC: F-capture-trace
      // wave-BC: F-tunnels
      // wave-BC: F-vrrp-config-sync
      // wave-BC: F-srv6
      // wave-BC: F-lisp
      'vpn', // F-lisp: vpn group, after the interfaces/routing/firewall items above
      // wave-A: P11
      // wave-A: F-wireguard
      // wave-A: F-kea-dhcp-relay
      // wave-A: F-unbound-chrony-syslog
      // wave-BC: F-ipfix-sflow (unanchored)
      // F-qos-flat (unanchored)
      'services',
      'users',
      'revisions',
      // Non-domain system items, one per S5 task (wave-BC-numbers.md S5 pack) + WEB-2:
      // wave-BC: F-restconf-yang
      // wave-BC: F-aaa
      // wave-BC: F-backup-restore
      // wave-BC: P10
      // wave-BC: P14
      // wave-BC: F-ab-upgrade
      // wave-BC: F-images
      // wave-BC: F-hardening-lite
      // wave-BC: F-licensing (unanchored)
      'licensing',
      'nsim', // F-loopback-bvi-gso-lldp-span: the Tools group comes after System (no anchor there)
      // web: WEB-2
      'dev-schema-form',
      'dev-data-grid',
      'dev-stream',
    ]);
  });

  it('has no Developer group and no /dev entries when dev routes are off (production builds, review M1)', () => {
    const nav = buildNav(domains, { devRoutes: false });
    expect(nav.map((g) => g.id)).not.toContain('dev');
    expect(nav.flatMap((g) => g.items).filter((i) => i.path.startsWith('/dev'))).toEqual([]);
    expect(nav.at(-1)!.id).toBe('tools');
  });

  it('marks exactly one item current, the most specific one (review L4)', () => {
    const nav = buildNav(domains, { devRoutes: false });
    expect(currentNavPath(nav, '/')).toBe('/');
    expect(currentNavPath(nav, '/vpn/tunnels')).toBe('/vpn/tunnels');
    expect(currentNavPath(nav, '/vpn')).toBe('/vpn');
    expect(currentNavPath(nav, '/system/users')).toBe('/system/users');
    expect(currentNavPath(nav, '/system/users/admin')).toBe('/system/users');
    expect(currentNavPath(nav, '/routing/vrfs')).toBe('/routing/vrfs');
    expect(currentNavPath(nav, '/nope')).toBeUndefined();
  });
});
