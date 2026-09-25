import { describe, expect, it } from 'vitest';
import { RootConfig } from '../../index.js';
import { validateConfig } from '../../validate.js';
import { LbSchema } from './lb.js';

/** F-lb: `services.lb` (VPP lb plugin) — shape, defaults and every intra-`lb` rule, with the pointer each one reports. */

const IF = 'TenGigabitEthernet0/0/1';
const base = { interfaces: { [IF]: { ipv4: ['10.2.1.1/24'], ipv6: ['2001:db8:2::1/64'] } } };
const doc = (lb: Record<string, unknown>) => ({ ...base, services: { lb } });
const gre = (extra: Record<string, unknown> = {}) => ({
  prefix: '10.2.250.1/32',
  protocol: 'tcp',
  port: 80,
  encap: 'gre4',
  servers: [{ address: '10.2.2.10' }],
  ...extra,
});

/** Pointers of the schema-tier issues of a document (empty when valid). */
function pointers(d: unknown): string[] {
  const r = validateConfig(d);
  return r.ok ? [] : r.issues.map((i) => i.pointer);
}

function messages(d: unknown): string[] {
  const r = validateConfig(d);
  return r.ok ? [] : r.issues.map((i) => `${i.pointer}: ${i.message}`);
}

describe('services.lb schema', () => {
  it('is optional: absent means no load balancer', () => {
    expect(RootConfig.parse({}).services.lb).toBeUndefined();
  });

  it('fills the defaults of a VIP and a server', () => {
    const lb = LbSchema.parse({
      vips: {
        web: { prefix: '10.2.250.1/32', encap: 'gre4', servers: [{ address: '10.2.2.10' }] },
      },
    });
    expect(lb.vips['web']).toEqual({
      prefix: '10.2.250.1/32',
      protocol: 'any',
      encap: 'gre4',
      newFlowsTableLength: 1024,
      srcIpSticky: false,
      servers: [{ address: '10.2.2.10', flushOnDelete: false }],
    });
    expect(lb.natInterfaces).toEqual([]);
  });

  it('accepts GRE4, GRE6 (IPv4 VIP), L3DSR and NAT4 VIPs with their prerequisites', () => {
    expect(
      messages(
        doc({
          settings: {
            ip4Source: '10.2.1.1',
            ip6Source: '2001:db8:2::1',
            flowBuckets: 2048,
            flowTimeoutSec: 60,
          },
          vips: {
            web: gre({ newFlowsTableLength: 4096, srcIpSticky: true }),
            v6srv: {
              prefix: '10.2.250.2/32',
              encap: 'gre6',
              servers: [{ address: '2001:db8:2::10', flushOnDelete: true }],
            },
            dsr: {
              prefix: '10.2.250.3/32',
              encap: 'l3dsr',
              dscp: 10,
              servers: [{ address: '10.2.2.11' }],
            },
            nat: {
              prefix: '10.2.250.4/32',
              protocol: 'udp',
              port: 53,
              encap: 'nat4',
              srvType: 'clusterip',
              targetPort: 5353,
              servers: [{ address: '10.2.2.12' }],
            },
            v6: {
              prefix: '2001:db8:250::1/128',
              protocol: 'tcp',
              port: 443,
              encap: 'gre4',
              servers: [{ address: '10.2.2.13' }],
            },
          },
          natInterfaces: [{ interface: IF, family: 'ip4' }],
        }),
      ),
    ).toEqual([]);
  });

  it('GRE4 VIP with an IPv6 server → pointer at the server address (acceptance)', () => {
    const r = validateConfig(
      doc({ vips: { web: gre({ servers: [{ address: '2001:db8:2::10' }] }) } }),
    );
    expect(r.ok).toBe(false);
    expect(r.ok ? [] : r.issues).toEqual([
      {
        pointer: '/services/lb/vips/web/servers/0/address',
        message: 'encap gre4 needs IPv4 application servers, 2001:db8:2::10 is IPv6',
      },
    ]);
    expect(r.ok ? undefined : r.tier).toBe('schema');
  });

  it('GRE6 needs IPv6 servers; NAT6 an IPv6 VIP and IPv6 servers', () => {
    expect(
      pointers(
        doc({
          vips: {
            a: { prefix: '10.2.250.1/32', encap: 'gre6', servers: [{ address: '10.2.2.10' }] },
          },
        }),
      ),
    ).toEqual(['/services/lb/vips/a/servers/0/address']);
    expect(
      pointers(
        doc({
          vips: {
            a: {
              prefix: '10.2.250.1/32',
              protocol: 'tcp',
              port: 80,
              encap: 'nat6',
              targetPort: 8080,
            },
          },
          natInterfaces: [{ interface: IF, family: 'ip6' }],
        }),
      ),
    ).toEqual(['/services/lb/vips/a/encap']);
  });

  it('L3DSR and NAT4 need an IPv4 VIP', () => {
    expect(
      pointers(doc({ vips: { a: { prefix: '2001:db8:250::1/128', encap: 'l3dsr' } } })),
    ).toEqual(['/services/lb/vips/a/encap']);
    expect(
      pointers(
        doc({
          vips: {
            a: {
              prefix: '2001:db8:250::1/128',
              protocol: 'tcp',
              port: 80,
              encap: 'nat4',
              targetPort: 80,
            },
          },
          natInterfaces: [{ interface: IF, family: 'ip4' }],
        }),
      ),
    ).toEqual(['/services/lb/vips/a/encap']);
  });

  it('a port needs tcp/udp and tcp/udp need a port', () => {
    expect(pointers(doc({ vips: { a: gre({ protocol: 'any' }) } }))).toEqual([
      '/services/lb/vips/a/port',
    ]);
    expect(pointers(doc({ vips: { a: gre({ port: undefined }) } }))).toEqual([
      '/services/lb/vips/a/port',
    ]);
  });

  it('table length and sticky buckets are powers of two', () => {
    expect(pointers(doc({ vips: { a: gre({ newFlowsTableLength: 1000 }) } }))).toEqual([
      '/services/lb/vips/a/newFlowsTableLength',
    ]);
    expect(pointers(doc({ settings: { flowBuckets: 1000 } }))).toEqual([
      '/services/lb/settings/flowBuckets',
    ]);
  });

  it('dscp is l3dsr-only; srvType/targetPort/nodePort are nat-only; nodePort needs nodeport', () => {
    expect(pointers(doc({ vips: { a: gre({ dscp: 10 }) } }))).toEqual(['/services/lb/vips/a/dscp']);
    expect(
      pointers(
        doc({ vips: { a: gre({ srvType: 'clusterip', targetPort: 80, nodePort: 30080 }) } }),
      ),
    ).toEqual([
      '/services/lb/vips/a/nodePort',
      '/services/lb/vips/a/srvType',
      '/services/lb/vips/a/targetPort',
    ]);
    expect(
      pointers(
        doc({
          vips: {
            a: {
              prefix: '10.2.250.4/32',
              protocol: 'tcp',
              port: 80,
              encap: 'nat4',
              targetPort: 80,
              nodePort: 30080,
            },
          },
          natInterfaces: [{ interface: IF, family: 'ip4' }],
        }),
      ),
    ).toEqual(['/services/lb/vips/a/nodePort']);
  });

  it('a NAT VIP needs tcp/udp + port, a target port and a natInterfaces entry of its family', () => {
    expect(
      pointers(
        doc({
          vips: { a: { prefix: '10.2.250.4/32', encap: 'nat4' } },
          natInterfaces: [{ interface: IF, family: 'ip4' }],
        }),
      ),
    ).toEqual(['/services/lb/vips/a/protocol', '/services/lb/vips/a/targetPort']);
    expect(
      pointers(
        doc({
          vips: {
            a: {
              prefix: '10.2.250.4/32',
              protocol: 'tcp',
              port: 80,
              encap: 'nat4',
              targetPort: 80,
            },
          },
          natInterfaces: [{ interface: IF, family: 'ip6' }],
        }),
      ),
    ).toEqual(['/services/lb/vips/a/encap']);
  });

  it('(prefix, protocol, port) is unique; host bits of the prefix are refused', () => {
    expect(pointers(doc({ vips: { a: gre(), b: gre() } }))).toEqual(['/services/lb/vips/b/prefix']);
    expect(pointers(doc({ vips: { a: gre({ prefix: '10.2.250.1/24' }) } }))).toEqual([
      '/services/lb/vips/a/prefix',
    ]);
    // same prefix, other port: allowed
    expect(pointers(doc({ vips: { a: gre(), b: gre({ port: 443 }) } }))).toEqual([]);
  });

  it("VPP's per-prefix rules: all-port xor per-port VIPs; one encapsulation per prefix", () => {
    expect(
      pointers(doc({ vips: { a: gre(), b: gre({ protocol: 'any', port: undefined }) } })),
    ).toEqual(['/services/lb/vips/b/port']);
    expect(pointers(doc({ vips: { a: gre(), b: gre({ port: 443, encap: 'l3dsr' }) } }))).toEqual([
      '/services/lb/vips/b/encap',
    ]);
  });

  it('servers are unique per VIP; NAT VIPs never share an (AS address, target port) pair (V20 SNAT key)', () => {
    expect(
      pointers(
        doc({
          vips: { a: gre({ servers: [{ address: '10.2.2.10' }, { address: '10.2.2.10' }] }) },
        }),
      ),
    ).toEqual(['/services/lb/vips/a/servers/1/address']);
    const nat = (prefix: string) => ({
      prefix,
      protocol: 'tcp',
      port: 80,
      encap: 'nat4',
      targetPort: 8080,
      servers: [{ address: '10.2.2.12' }],
    });
    expect(
      pointers(
        doc({
          vips: { a: nat('10.2.250.4/32'), b: nat('10.2.250.5/32') },
          natInterfaces: [{ interface: IF, family: 'ip4' }],
        }),
      ),
    ).toEqual(['/services/lb/vips/b/servers/0/address']);
  });

  it('duplicate NAT interfaces and unknown keys are refused', () => {
    expect(
      pointers(
        doc({
          natInterfaces: [
            { interface: IF, family: 'ip4' },
            { interface: IF, family: 'ip4' },
          ],
        }),
      ),
    ).toEqual(['/services/lb/natInterfaces/1']);
    expect(pointers(doc({ vips: { a: gre({ weight: 2 }) } }))).toEqual([
      '/services/lb/vips/a/weight',
    ]);
  });
});

describe('services.lb semantics', () => {
  it('services.lb-nat-interface-exists', () => {
    const r = validateConfig(
      doc({ natInterfaces: [{ interface: 'TenGigabitEthernet0/0/9', family: 'ip4' }] }),
    );
    expect(r.ok ? [] : r.issues).toEqual([
      {
        pointer: '/services/lb/natInterfaces/0/interface',
        message: "interface 'TenGigabitEthernet0/0/9' does not exist",
      },
    ]);
    expect(r.ok ? undefined : r.tier).toBe('semantic');
  });
});
