import { describe, expect, it } from 'vitest';
import {
  RootConfig,
  SetupInputSchema,
  buildSetup,
  setupDiff,
  setupPool,
  setupHostName,
  validateConfig,
} from './index.js';
const base = () =>
  RootConfig.parse({ interfaces: { wan0: {}, lan0: {}, other0: { description: 'preserve' } } });
const input = SetupInputSchema.parse({
  language: 'en',
  timezone: 'UTC',
  ntp: ['pool.ntp.org'],
  hostname: 'router',
  wan: 'wan0',
  wanMode: 'dhcp',
  lan: 'lan0',
  lanAddress: '192.168.40.1/24',
  dhcp: true,
});
const at = '2026-10-04T00:00:00Z';
describe('setup builder', () => {
  it('builds schema and semantic valid safe defaults without mutating source', () => {
    const b = base();
    const d = buildSetup(b, input, at);
    expect(validateConfig(d)).toEqual({ ok: true, config: d });
    expect(b.system.setup.completed).toBe(false);
    expect(d.interfaces.other0).toEqual(b.interfaces.other0);
    expect(d.nat.inside).toEqual(['lan0']);
    expect(d.nat.outside).toEqual(['wan0']);
    expect(d.acl.lists['setup-lan-out']!.rules.at(-1)!.action).toBe('reflect');
    expect(d.acl.lists['setup-wan-in']!.rules.at(-1)!.action).toBe('deny');
    expect(d.acl.hostSettings!.antiLockout).toMatchObject({
      enabled: true,
      interfaces: [setupHostName('lan0')],
      sources: ['192.168.40.0/24'],
    });
  });
  it('static WAN creates the default route and a WAN-address NAT pool', () => {
    const d = buildSetup(
      base(),
      {
        ...input,
        wanMode: 'static',
        wanAddress: '203.0.113.2/24',
        wanGateway: '203.0.113.1',
        dhcp: false,
      },
      at,
    );
    expect(validateConfig(d)).toEqual({ ok: true, config: d });
    expect(d.interfaces.wan0!.dhcpClient).toBeUndefined();
    expect(d.interfaces.wan0!.ipv4).toEqual(['203.0.113.2/24']);
    expect(d.routing.static[0]).toMatchObject({
      prefix: '0.0.0.0/0',
      nextHops: [{ address: '203.0.113.1', interface: 'wan0' }],
    });
    expect(d.services.dhcp.servers['setup-lan']!.enabled).toBe(false);
  });
  it('shows exact stable diff and protects completion from accidental rerun', () => {
    const b = base();
    const d = buildSetup(b, input, at);
    expect(setupDiff(b, input, at)).toContainEqual({
      op: 'replace',
      pointer: '/system/setup/completed',
      from: false,
      to: true,
    });
    expect(() => buildSetup(d, input, at)).toThrow('rerun');
    expect(buildSetup(d, { ...input, rerun: true }, at)).toEqual(d);
  });
  it('a different-interface rerun removes old wizard attachments and preserves unrelated policies', () => {
    const b = RootConfig.parse({
      interfaces: { wan0: {}, lan0: {}, wan1: {}, lan1: {}, other0: {} },
    });
    const d = buildSetup(b, input, at);
    d.acl.lists.other = { tags: [], rules: [] };
    d.acl.attachments.push({
      list: 'other',
      target: { kind: 'interface', interface: 'other0' },
      direction: 'in',
      sequence: 20,
      enabled: true,
    });
    const rerun = buildSetup(
      d,
      { ...input, wan: 'wan1', lan: 'lan1', lanAddress: '192.168.41.1/24', rerun: true },
      at,
    );
    expect(
      rerun.acl.attachments.filter((a) => a.list.startsWith('setup-')).map((a) => a.target),
    ).toEqual([
      { kind: 'interface', interface: 'lan1' },
      { kind: 'interface', interface: 'wan1' },
    ]);
    expect(rerun.acl.attachments.some((a) => a.list === 'other')).toBe(true);
    expect(rerun.interfaces.lan0!.lcp!.hostIfName).not.toBe(rerun.interfaces.lan1!.lcp!.hostIfName);
    expect(validateConfig(rerun)).toEqual({ ok: true, config: rerun });
  });
  it('requires distinct existing interfaces and all WAN credentials', () => {
    expect(SetupInputSchema.safeParse({ ...input, wan: input.lan }).success).toBe(false);
    expect(SetupInputSchema.safeParse({ ...input, wanMode: 'static' }).success).toBe(false);
    expect(SetupInputSchema.safeParse({ ...input, wanMode: 'pppoe' }).success).toBe(false);
    expect(() => buildSetup(base(), { ...input, wan: 'missing' }, at)).toThrow('existing');
  });
  it('excludes network broadcast and gateway from suggested pools', () => {
    expect(setupPool('192.168.40.1/24')).toEqual({
      subnet: '192.168.40.0/24',
      gateway: '192.168.40.1',
      start: '192.168.40.2',
      end: '192.168.40.201',
    });
    expect(setupPool('192.168.40.6/29').end).toBe('192.168.40.5');
    expect(() => setupPool('192.168.40.0/24')).toThrow('network');
  });
  it('replaces input allow attachments and scopes host management to LAN', () => {
    const b = base();
    b.acl.host.open = { tags: [], rules: [] };
    b.acl.hostAttachments = [{ list: 'open', chain: 'input', priority: 0, enabled: true }];
    expect(buildSetup(b, input, at).acl.hostAttachments).toEqual([
      { list: 'setup-input', chain: 'input', priority: 0, enabled: true },
    ]);
  });
});

describe('PPPoE setup logical WAN', () => {
  const pppoe = SetupInputSchema.parse({
    ...input,
    wanMode: 'pppoe',
    wanPppoe: { username: 'isp-user', passwordRef: 'password/isp' },
  });
  it('creates an explicit logical WAN and keeps NAT/filtering off the physical parent', () => {
    const original = base();
    const doc = buildSetup(original, pppoe, at);
    expect(validateConfig(doc)).toEqual({ ok: true, config: doc });
    expect(doc.interfaces['setup-pppoe']).toMatchObject({
      mtu: 1492,
      pppoe: {
        parent: 'wan0',
        username: 'isp-user',
        passwordRef: 'password/isp',
        defaultRoute: true,
      },
    });
    expect(doc.interfaces.wan0?.pppoe).toBeUndefined();
    expect(doc.interfaces.wan0?.ipv4).toEqual([]);
    expect(doc.nat.outside).toEqual(['setup-pppoe']);
    expect(doc.nat.pools).toEqual([
      { name: 'setup-wan', interface: 'setup-pppoe', twiceNat: false },
    ]);
    expect(doc.acl.attachments.find((a) => a.list === 'setup-wan-in')?.target).toEqual({
      kind: 'interface',
      interface: 'setup-pppoe',
    });
    expect(doc.routing.static).toEqual([]);
    expect(original.interfaces['setup-pppoe']).toBeUndefined();
  });
  it('accepts credentials only as existing reference syntax in PPPoE mode', () => {
    expect(SetupInputSchema.safeParse({ ...input, wanMode: 'pppoe' }).success).toBe(false);
    expect(
      SetupInputSchema.safeParse({
        ...pppoe,
        wanPppoe: { username: 'isp', passwordRef: 'plaintext-password' },
      }).success,
    ).toBe(false);
    expect(SetupInputSchema.safeParse({ ...input, wanPppoe: pppoe.wanPppoe }).success).toBe(false);
  });
  it('refuses logical name collisions and incompatible physical parent ownership', () => {
    const collision = base();
    collision.interfaces['setup-pppoe'] = collision.interfaces.other0!;
    expect(() => buildSetup(collision, pppoe, at)).toThrow('wizard-owned');
    const busy = base();
    busy.interfaces.wan0!.lcp = { hostIfName: 'host-wan', hostIfType: 'tap' };
    expect(() => buildSetup(busy, pppoe, at)).toThrow('exclusive physical');
  });
  it('reruns PPPoE safely and removes only its own unused logical WAN on DHCP/static migration', () => {
    const first = buildSetup(base(), pppoe, at);
    const second = buildSetup(
      first,
      { ...pppoe, rerun: true, wanPppoe: { username: 'new', passwordRef: 'password/new' } },
      at,
    );
    expect(second.interfaces['setup-pppoe']?.pppoe?.username).toBe('new');
    const dhcp = buildSetup(second, { ...input, rerun: true }, at);
    expect(dhcp.interfaces['setup-pppoe']).toBeUndefined();
    expect(dhcp.nat.outside).toEqual(['wan0']);
    expect(validateConfig(dhcp)).toEqual({ ok: true, config: dhcp });
    first.interfaces['setup-pppoe']!.ipv4 = ['192.0.2.1/24'];
    expect(() => buildSetup(first, { ...input, rerun: true }, at)).toThrow(
      'unchanged wizard-owned',
    );
  });
  it('refuses cleanup when an unrelated route still refers to the logical WAN', () => {
    const doc = buildSetup(base(), pppoe, at);
    doc.routing.static.push({
      prefix: '198.51.100.0/24',
      vrf: 'default',
      nextHops: [{ interface: 'setup-pppoe', weight: 1 }],
      distance: 1,
      blackhole: false,
    });
    expect(() => buildSetup(doc, { ...input, rerun: true }, at)).toThrow('unrelated references');
  });
});
