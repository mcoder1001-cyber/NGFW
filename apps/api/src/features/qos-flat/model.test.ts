import type { QosPolicerStatus } from '@ngfw/proto';
import { describe, expect, it } from 'vitest';
import { joinPolicers, shaperBurstBytes } from './model.js';

const live = (p: Partial<QosPolicerStatus> & { name: string }): QosPolicerStatus => ({
  kind: 'policer',
  index: 0,
  type: '1r2c',
  rateUnit: 'kbps',
  cir: 1,
  eir: 0,
  cb: '1500',
  eb: '0',
  currentBucket: 7,
  currentLimit: 9,
  extendedBucket: 0,
  extendedLimit: 0,
  conform: { packets: '5', bytes: '500' },
  exceed: undefined,
  violate: undefined,
  ...p,
});

describe('shaperBurstBytes (the agent derives the same value, desired/qos.go)', () => {
  it('is ≈ 10 ms of traffic, at least two 1500-byte frames', () => {
    expect(shaperBurstBytes(50_000)).toBe(62_500);
    expect(shaperBurstBytes(1_000_001)).toBe(1_250_002); // rounds up
    expect(shaperBurstBytes(1_000)).toBe(3000);
    expect(shaperBurstBytes(2_400)).toBe(3000);
    expect(shaperBurstBytes(2_401)).toBe(3002);
  });
});

describe('joinPolicers', () => {
  const running = {
    services: {
      qos: {
        policers: {
          b: { cir: 100, cb: 2000, rateUnit: 'pps' },
          a: { cir: 10, cb: 1000, type: '2r3c-rfc2698', eir: 20, eb: 3000 },
        },
        shapers: { up: { rateKbps: 8000 }, dn: { rateKbps: 1000, burstBytes: 4096 } },
        interfaces: {
          loop2: { policer: { input: 'a', output: 'b' } },
          loop1: { policer: { input: 'a' }, shaper: 'up' },
        },
      },
    },
  };

  it('joins VPP and running: attachments, counters as strings, missing and unmanaged items', () => {
    const items = joinPolicers(running, [
      live({ name: 'a', index: 3, cir: 10, cb: '1000', type: '2r3c-rfc2698', eir: 20, eb: '3000' }),
      live({ name: 'shaper:up', kind: 'shaper', index: 4, cir: 8000, cb: '10000' }),
      live({ name: 'zz', index: 9 }),
    ]);
    expect(
      items.map((i) => `${i.kind}:${i.name}:${i.configured ? 'c' : '-'}${i.present ? 'p' : '-'}`),
    ).toEqual(['policer:a:cp', 'policer:b:c-', 'policer:zz:-p', 'shaper:dn:c-', 'shaper:up:cp']);
    const [a, b, zz, dn, up] = items;
    expect(a).toMatchObject({
      vppName: 'a',
      index: 3,
      cb: 1000,
      eb: 3000,
      bucket: { current: 7, limit: 9, extendedCurrent: 0, extendedLimit: 0 },
      conform: { packets: '5', bytes: '500' },
      exceed: { packets: '0', bytes: '0' },
      attachments: [
        { interface: 'loop1', direction: 'input' },
        { interface: 'loop2', direction: 'input' },
      ],
    });
    expect(b).toMatchObject({
      present: false,
      index: null,
      bucket: null,
      rateUnit: 'pps',
      type: '1r2c',
      cir: 100,
      cb: 2000,
      attachments: [{ interface: 'loop2', direction: 'output' }],
    });
    expect(zz).toMatchObject({ configured: false, attachments: [] });
    expect(dn).toMatchObject({
      vppName: 'shaper:dn',
      present: false,
      cir: 1000,
      cb: 4096,
      type: '1r2c',
    });
    expect(up).toMatchObject({
      vppName: 'shaper:up',
      cb: 10000,
      attachments: [{ interface: 'loop1', direction: 'output' }],
    });
  });

  it('a configured shaper VPP lacks carries the derived burst', () => {
    const [up] = joinPolicers({ services: { qos: { shapers: { up: { rateKbps: 8000 } } } } }, []);
    expect(up).toMatchObject({ kind: 'shaper', cb: 10_000, present: false });
  });

  it('an empty running document and no VPP policers give no items', () => {
    expect(joinPolicers({}, [])).toEqual([]);
  });
});
