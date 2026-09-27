import { describe, expect, it } from 'vitest';
import { RootConfig, type RootConfigInput } from '../../index.js';

const doc = (multicast: unknown): RootConfigInput => ({ routing: { multicast } } as RootConfigInput);

describe('F-igmp-mfib schema', () => {
  it('routing.multicast is absent by default', () => {
    expect(RootConfig.parse({}).routing.multicast).toBeUndefined();
  });

  it('accepts a host join with sources and applies defaults', () => {
    const c = RootConfig.parse(
      doc({ igmp: { interfaces: { eth0: { mode: 'host', joins: [{ group: '239.1.1.1', sources: ['10.0.0.5'] }] } } } }),
    );
    const mc = c.routing.multicast!;
    expect(mc.igmp.ssmRanges).toEqual(['232.0.0.0/8']);
    expect(mc.igmp.interfaces.eth0!.joins[0]!.sources).toEqual(['10.0.0.5']);
  });

  it('rejects a join with no sources (VPP is INCLUDE-only)', () => {
    const r = RootConfig.safeParse(doc({ igmp: { interfaces: { eth0: { mode: 'host', joins: [{ group: '239.1.1.1', sources: [] }] } } } }));
    expect(r.success).toBe(false);
  });

  it('accepts a static (S,G) mroute with one accept and forwards', () => {
    const r = RootConfig.safeParse(
      doc({
        mroutes: [
          {
            group: '239.2.2.2',
            source: '10.0.0.9',
            paths: [
              { interface: 'eth0', flags: 'accept' },
              { interface: 'eth1', flags: 'forward' },
            ],
          },
        ],
      }),
    );
    expect(r.success).toBe(true);
  });

  it('rejects an mroute with two accept interfaces', () => {
    const r = RootConfig.safeParse(
      doc({
        mroutes: [
          {
            group: '239.2.2.2',
            paths: [
              { interface: 'eth0', flags: 'accept' },
              { interface: 'eth1', flags: 'accept' },
            ],
          },
        ],
      }),
    );
    expect(r.success).toBe(false);
  });

  it('rejects an accept interface that also forwards', () => {
    const r = RootConfig.safeParse(
      doc({
        mroutes: [
          {
            group: '239.2.2.2',
            paths: [
              { interface: 'eth0', flags: 'accept' },
              { interface: 'eth0', flags: 'forward' },
            ],
          },
        ],
      }),
    );
    expect(r.success).toBe(false);
  });
});
