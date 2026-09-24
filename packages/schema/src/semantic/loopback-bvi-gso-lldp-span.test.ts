import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import {
  InterfaceSchema,
  nsimWheelSlots,
  NSIM_WHEEL_SLOTS_MAX,
  RootConfig,
  SubinterfaceSchema,
  type RootConfigInput,
} from '../index.js';
import { validateSemantics } from './index.js';
import { loopbackBviGsoLldpSpanValidators } from './loopback-bvi-gso-lldp-span.js';

/**
 * F-loopback-bvi-gso-lldp-span semantic rules. The valid corpus document is
 * `packages/proto/test/fixtures/loopback-bvi-gso-lldp-span-full.json` (the schema examples directory only admits
 * the P02 groups' prefixes — examples.test.ts; same as F-bridge-l2 Q4).
 */
const fixture = JSON.parse(
  readFileSync(
    new URL('../../../proto/test/fixtures/loopback-bvi-gso-lldp-span-full.json', import.meta.url),
    'utf8',
  ),
) as RootConfigInput;

const run = (name: string, doc: RootConfigInput) => {
  const v = loopbackBviGsoLldpSpanValidators.find((x) => x.name === name);
  if (v === undefined) throw new Error(`no validator ${name}`);
  return v.validate(RootConfig.parse(doc));
};
const pointers = (name: string, doc: RootConfigInput) => run(name, doc).map((i) => i.pointer);

type Doc = RootConfigInput & {
  interfaces: Record<string, Record<string, unknown>>;
  services: Record<string, unknown>;
};

/** A small valid base: a loopback BVI with GSO, a mirrored port, a monitor port and two nsim ports. */
function base(): Doc {
  return {
    interfaces: {
      loop7001: { enabled: true, gso: true },
      'GigabitEthernet0/0/0': {
        mirror: [{ destination: 'GigabitEthernet0/0/1', direction: 'both', level: 'device' }],
      },
      'GigabitEthernet0/0/1': { subinterfaces: { '10': { vlanId: 10 } } },
      'GigabitEthernet0/0/2': {},
      'GigabitEthernet0/0/3': {},
    },
    services: {
      lldp: { enabled: true, interfaces: [{ interface: 'GigabitEthernet0/0/0' }] },
      nsim: {
        delayMs: 20,
        bandwidthMbps: 100,
        crossConnect: { a: 'GigabitEthernet0/0/2', b: 'GigabitEthernet0/0/3' },
      },
    },
  };
}

describe('loopback-bvi-gso-lldp-span corpus', () => {
  it('the full fixture is schema- and semantically valid', () => {
    expect(validateSemantics(RootConfig.parse(fixture))).toEqual([]);
  });
  it('the base document is valid for every rule', () => {
    expect(validateSemantics(RootConfig.parse(base()))).toEqual([]);
  });
  it('validator names carry the owning domain and the slug', () => {
    for (const v of loopbackBviGsoLldpSpanValidators)
      expect(v.name).toMatch(/^(interfaces|services)\.loopback-bvi-gso-lldp-span-/);
  });
  it('defaults: a session mirrors both directions at the device level; nsim packet size 1500, no loss', () => {
    const c = RootConfig.parse({
      interfaces: { loop1: { mirror: [{ destination: 'loop2' }] }, loop2: {} },
      services: { nsim: { delayMs: 1, bandwidthMbps: 1 } },
    });
    expect(c.interfaces['loop1']?.mirror).toEqual([
      { destination: 'loop2', direction: 'both', level: 'device' },
    ]);
    expect(c.interfaces['loop1']?.gso).toBeUndefined();
    expect(c.services.nsim).toMatchObject({
      packetSize: 1500,
      dropFraction: 0,
      outputInterfaces: [],
    });
  });
});

describe('D-105: loop16000–loop16383 are reserved', () => {
  const rule = 'interfaces.loopback-bvi-gso-lldp-span-reserved-loopback';
  it.each(['loop16000', 'loop16200', 'loop16383'])('%s is refused at its key', (name) => {
    const d = base();
    d.interfaces[name] = { enabled: true };
    expect(pointers(rule, d)).toEqual([`/interfaces/${name}`]);
  });
  it.each(['loop15999', 'loop16384', 'loop0', 'loop7001'])('%s is allowed', (name) => {
    const d = base();
    d.interfaces[name] = {};
    expect(pointers(rule, d)).toEqual([]);
  });
});

describe('GSO', () => {
  it('only parent interfaces carry gso: a sub-interface key is refused by the strict schema', () => {
    expect(InterfaceSchema.safeParse({ gso: true }).success).toBe(true);
    const sub = SubinterfaceSchema.safeParse({ vlanId: 10, gso: true });
    expect(sub.success).toBe(false);
    const doc = base();
    const bad: Record<string, unknown> = { subinterfaces: { '10': { vlanId: 10, gso: true } } };
    doc.interfaces['GigabitEthernet0/0/1'] = bad;
    const r = RootConfig.safeParse(doc);
    expect(r.success).toBe(false);
    expect(r.error?.issues.map((i) => i.path.join('/'))).toContain(
      'interfaces/GigabitEthernet0/0/1/subinterfaces/10',
    );
  });
  it('local0 has no output path', () => {
    const d = base();
    d.interfaces['local0'] = { gso: true };
    expect(pointers('interfaces.loopback-bvi-gso-lldp-span-gso-interface', d)).toEqual([
      '/interfaces/local0/gso',
    ]);
    d.interfaces['local0'] = { gso: false };
    expect(pointers('interfaces.loopback-bvi-gso-lldp-span-gso-interface', d)).toEqual([]);
  });
});

describe('mirror sessions', () => {
  const dest = 'interfaces.loopback-bvi-gso-lldp-span-mirror-destination';
  it('destination equal to the source is refused at the destination (acceptance: 400 + pointer)', () => {
    const d = base();
    d.interfaces['GigabitEthernet0/0/0'] = { mirror: [{ destination: 'GigabitEthernet0/0/0' }] };
    expect(run(dest, d)).toEqual([
      {
        pointer: '/interfaces/GigabitEthernet0~10~10/mirror/0/destination',
        message: 'a mirror session cannot copy GigabitEthernet0/0/0 to itself',
      },
    ]);
    expect(validateSemantics(RootConfig.parse(d)).map((i) => i.pointer)).toContain(
      '/interfaces/GigabitEthernet0~10~10/mirror/0/destination',
    );
  });
  it('the destination must exist: an interface, a sub-interface or a tunnel of the document', () => {
    const d = base();
    d.interfaces['GigabitEthernet0/0/0'] = {
      mirror: [
        { destination: 'GigabitEthernet0/0/9' },
        { destination: 'GigabitEthernet0/0/1.10' },
        { destination: 'gre7' },
        { destination: 'local0' },
      ],
    };
    expect(pointers(dest, d)).toEqual([
      '/interfaces/GigabitEthernet0~10~10/mirror/0/destination',
      '/interfaces/GigabitEthernet0~10~10/mirror/2/destination',
      '/interfaces/GigabitEthernet0~10~10/mirror/3/destination',
    ]);
    // ERSPAN: a GRE tunnel of type erspan in `tunnels` is a valid destination
    Object.assign(d, {
      tunnels: {
        gre: {
          collector: {
            instance: 7,
            type: 'erspan',
            src: '10.0.0.1',
            dst: '10.0.0.2',
            sessionId: 1,
          },
        },
      },
    });
    expect(pointers(dest, d)).toEqual([
      '/interfaces/GigabitEthernet0~10~10/mirror/0/destination',
      '/interfaces/GigabitEthernet0~10~10/mirror/3/destination',
    ]);
  });
  it('a destination that is itself mirrored is refused (no loops)', () => {
    const d = base();
    d.interfaces['GigabitEthernet0/0/1'] = { mirror: [{ destination: 'GigabitEthernet0/0/0' }] };
    expect(pointers('interfaces.loopback-bvi-gso-lldp-span-mirror-loop', d)).toEqual([
      '/interfaces/GigabitEthernet0~10~10/mirror/0/destination',
      '/interfaces/GigabitEthernet0~10~11/mirror/0/destination',
    ]);
    // a chain a → b, b → c is refused at a (b is a source)
    const c = base();
    c.interfaces['GigabitEthernet0/0/1'] = { mirror: [{ destination: 'GigabitEthernet0/0/2' }] };
    expect(pointers('interfaces.loopback-bvi-gso-lldp-span-mirror-loop', c)).toEqual([
      '/interfaces/GigabitEthernet0~10~10/mirror/0/destination',
    ]);
  });
  it('one session per destination and level', () => {
    const d = base();
    d.interfaces['GigabitEthernet0/0/0'] = {
      mirror: [
        { destination: 'GigabitEthernet0/0/1', direction: 'rx' },
        { destination: 'GigabitEthernet0/0/1', direction: 'tx', level: 'l2' },
        { destination: 'GigabitEthernet0/0/1', direction: 'tx' },
      ],
    };
    expect(pointers('interfaces.loopback-bvi-gso-lldp-span-mirror-duplicate', d)).toEqual([
      '/interfaces/GigabitEthernet0~10~10/mirror/2',
    ]);
  });
  it('the schema bounds the session list and the enums', () => {
    const many = Array.from({ length: 9 }, (_, i) => ({ destination: `loop${i + 1}` }));
    expect(InterfaceSchema.safeParse({ mirror: many }).success).toBe(false);
    expect(
      InterfaceSchema.safeParse({ mirror: [{ destination: 'loop1', direction: 'in' }] }).success,
    ).toBe(false);
    expect(
      InterfaceSchema.safeParse({ mirror: [{ destination: 'loop1', level: 'l3' }] }).success,
    ).toBe(false);
  });
});

describe('nsim', () => {
  const range = 'services.loopback-bvi-gso-lldp-span-nsim-range';
  const ifs = 'services.loopback-bvi-gso-lldp-span-nsim-interfaces';
  it('VPP ranges are schema bounds: delay and bandwidth > 0, packet size 64–9000, fraction 0–1', () => {
    for (const bad of [
      { delayMs: 0, bandwidthMbps: 1 },
      { delayMs: 1, bandwidthMbps: 0 },
      { delayMs: 1, bandwidthMbps: 1, packetSize: 63 },
      { delayMs: 1, bandwidthMbps: 1, packetSize: 9001 },
      { delayMs: 1, bandwidthMbps: 1, dropFraction: 1.5 },
      { delayMs: 10001, bandwidthMbps: 1 },
    ]) {
      expect(RootConfig.safeParse({ services: { nsim: bad } }).success, JSON.stringify(bad)).toBe(
        false,
      );
    }
  });
  it('the scheduler wheel is bounded (delay × bandwidth / packet size)', () => {
    expect(nsimWheelSlots(20, 100, 1500)).toBe(167);
    const d = base();
    d.services['nsim'] = { delayMs: 10000, bandwidthMbps: 100000, packetSize: 64 };
    expect(nsimWheelSlots(10000, 100000, 64)).toBeGreaterThan(NSIM_WHEEL_SLOTS_MAX);
    expect(pointers(range, d)).toEqual(['/services/nsim/delayMs']);
    d.services['nsim'] = { delayMs: 100, bandwidthMbps: 1000, packetSize: 1500 };
    expect(pointers(range, d)).toEqual([]);
  });
  it('a drop fraction must be expressible as packets_per_drop (u32)', () => {
    const d = base();
    d.services['nsim'] = { delayMs: 1, bandwidthMbps: 1, dropFraction: 1e-12 };
    expect(pointers(range, d)).toEqual(['/services/nsim/dropFraction']);
  });
  it('cross-connect and output interfaces exist, are hardware interfaces, differ and are unique', () => {
    const d = base();
    d.services['nsim'] = {
      delayMs: 1,
      bandwidthMbps: 1,
      crossConnect: { a: 'GigabitEthernet0/0/2', b: 'GigabitEthernet0/0/2' },
      outputInterfaces: ['GigabitEthernet0/0/9', 'GigabitEthernet0/0/1.10', 'loop7001', 'loop7001'],
    };
    expect(pointers(ifs, d)).toEqual([
      '/services/nsim/crossConnect/b',
      '/services/nsim/outputInterfaces/0',
      '/services/nsim/outputInterfaces/1',
      '/services/nsim/outputInterfaces/3',
    ]);
  });
});

describe('LLDP (the existing services.interface-references rule)', () => {
  it('an LLDP interface must exist', () => {
    const d = base();
    d.services['lldp'] = { enabled: true, interfaces: [{ interface: 'GigabitEthernet0/0/7' }] };
    expect(
      validateSemantics(RootConfig.parse(d)).filter((i) => i.pointer.startsWith('/services/lldp')),
    ).toEqual([
      {
        pointer: '/services/lldp/interfaces/0/interface',
        message: "interface 'GigabitEthernet0/0/7' does not exist",
      },
    ]);
  });
  it('a loopback of the document is a valid LLDP interface', () => {
    const d = base();
    d.services['lldp'] = {
      enabled: true,
      interfaces: [{ interface: 'loop7001', portDescription: 'bvi' }],
    };
    expect(validateSemantics(RootConfig.parse(d))).toEqual([]);
  });
});
