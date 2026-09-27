import { describe, expect, it } from 'vitest';
import { RootConfig } from '../index.js';
import { validateSemantics } from './index.js';

const interfaces = {
  'host-w4l0': { ipv4: ['10.4.1.1/24'] },
  'host-w4w0': { ipv4: ['10.4.2.1/24'] },
};
const doc = (nat: unknown) => RootConfig.parse({ interfaces, nat });
const run = (nat: unknown) =>
  validateSemantics(doc(nat), ['nat', 'interfaces']).filter((i) => i.pointer.startsWith('/nat'));
const at = (nat: unknown) => run(nat).map((i) => i.pointer);

const domain = (over: Record<string, unknown>) => ({
  name: 'd1',
  mode: 'map-e',
  ipv4Prefix: '192.0.2.0/24',
  ipv6Prefix: '2001:db8::/40',
  ipv6Source: '2001:db8:ffff::1/128',
  ...over,
});

describe('nat.det44-map-dslite-cnat-cnat-snat-address', () => {
  it('refuses a policy without SNAT addresses, pointing at addresses', () => {
    expect(at({ cnat: { snat: { policy: 'interface' } } })).toEqual(['/nat/cnat/snat/addresses']);
  });
  it('refuses excluded prefixes without addresses and accepts an address interface', () => {
    expect(at({ cnat: { snat: { excludePrefixes: ['10.0.0.0/8'] } } })).toEqual(['/nat/cnat/snat/addresses']);
    expect(at({ cnat: { snat: { policy: 'interface', addresses: { interface: 'host-w4w0' } } } })).toEqual([]);
  });
});

describe('nat.det44-map-dslite-cnat-cnat-nat44-interface', () => {
  const cnat = {
    snat: { policy: 'interface', addresses: { ipv4: '10.4.2.1' }, interfaces: [{ interface: 'host-w4w0', table: 'include-v4' }] },
  };
  it('refuses a CNAT policy interface that is a NAT44-ED interface', () => {
    expect(at({ outside: ['host-w4w0'], cnat })).toEqual(['/nat/cnat/snat/interfaces/0/interface']);
  });
  it('allows it when NAT44 is ei or disabled', () => {
    expect(at({ mode: 'ei', outside: ['host-w4w0'], cnat })).toEqual([]);
    expect(at({ enabled: false, outside: ['host-w4w0'], cnat })).toEqual([]);
  });
});

describe('MAP domain rules', () => {
  it('refuses the reserved nat46- prefix', () => {
    expect(at({ map: { domains: [domain({ name: 'nat46-x' })] } })).toEqual(['/nat/map/domains/0/name']);
  });
  it('refuses an lw4o6 domain without rules and accepts one with a rule', () => {
    expect(at({ map: { domains: [domain({ mode: 'lw4o6' })] } })).toEqual(['/nat/map/domains/0/rules']);
    expect(
      at({ map: { domains: [domain({ mode: 'lw4o6', psidLength: 6, rules: [{ psid: 1, ipv6Destination: '2001:db8::5' }] })] } }),
    ).toEqual([]);
  });
});

describe('nat.pnat', () => {
  const binding = { name: 'b1', match: { proto: 'udp', dst: '10.4.2.10', dport: 53 }, rewrite: { dst: '10.4.1.10' } };
  it('is absent by default and accepts a binding with an attachment', () => {
    expect(RootConfig.parse({}).nat.pnat).toBeUndefined();
    expect(at({ pnat: { bindings: [binding], attachments: [{ binding: 'b1', interface: 'host-w4w0', point: 'input' }] } })).toEqual([]);
  });
  it('refuses an attachment on an unknown interface (semantic)', () => {
    expect(at({ pnat: { bindings: [binding], attachments: [{ binding: 'b1', interface: 'nope0', point: 'input' }] } })).toEqual([
      '/nat/pnat/attachments/0/interface',
    ]);
  });
  const issues = (pnat: unknown) => {
    const r = RootConfig.safeParse({ nat: { pnat } });
    return r.success ? [] : r.error.issues.map((i) => i.path.join('/'));
  };
  it('refines: empty match/rewrite, ports without tcp/udp, duplicate match, unknown binding, mixed masks', () => {
    expect(issues({ bindings: [{ name: 'x', match: {}, rewrite: {} }] })).toEqual([
      'nat/pnat/bindings/0/match',
      'nat/pnat/bindings/0/rewrite',
    ]);
    expect(issues({ bindings: [{ name: 'x', match: { dport: 80 }, rewrite: { dst: '10.0.0.1' } }] })).toEqual([
      'nat/pnat/bindings/0/match/proto',
    ]);
    expect(issues({ bindings: [binding, { ...binding, name: 'b2' }] })).toEqual(['nat/pnat/bindings/1/match']);
    expect(issues({ bindings: [binding], attachments: [{ binding: 'zz', interface: 'e0', point: 'input' }] })).toEqual([
      'nat/pnat/attachments/0/binding',
    ]);
    const other = { name: 'b2', match: { src: '10.4.1.5' }, rewrite: { src: '10.4.2.5' } };
    expect(
      issues({
        bindings: [binding, other],
        attachments: [
          { binding: 'b1', interface: 'e0', point: 'input' },
          { binding: 'b2', interface: 'e0', point: 'input' },
        ],
      }),
    ).toEqual(['nat/pnat/attachments/1/binding']);
  });
});

describe('MAP parameter defaults follow VPP (Q3)', () => {
  it('securityCheck.enabled and trafficClass.copy default to true', () => {
    const p = RootConfig.parse({}).nat.map.parameters;
    expect(p.securityCheck.enabled).toBe(true);
    expect(p.trafficClass.copy).toBe(true);
  });
});
