import type { LbVipState } from '@ngfw/proto';
import { describe, expect, it } from 'vitest';
import { lbVipsOf, vipJson } from './lb.controller.js';

const state = (over: Partial<LbVipState> = {}): LbVipState => ({
  name: 'web',
  prefix: '10.2.250.1/32',
  protocol: 'tcp',
  port: 80,
  applied: true,
  vppEntries: 1,
  encap: 'gre4',
  dscp: 0,
  targetPort: 0,
  servers: [{ address: '10.2.2.10', inUse: true, inUseSince: 5 }],
  ...over,
});

describe('F-lb state view', () => {
  it('derives the status column from the agent state', () => {
    const cfg = { encap: 'gre4', servers: [{ address: '10.2.2.10' }] };
    expect(vipJson(state(), cfg).status).toBe('active');
    expect(vipJson(state({ applied: false, vppEntries: 0, encap: '' }), cfg).status).toBe(
      'not-applied',
    );
    expect(vipJson(state({ vppEntries: 0 }), cfg).status).toBe('missing');
    expect(
      vipJson(state({ servers: [{ address: '10.2.2.10', inUse: false, inUseSince: 5 }] }), cfg)
        .status,
    ).toBe('no-servers');
  });

  it('marks configured servers; removed copies are never configured', () => {
    const v = vipJson(
      state({
        servers: [
          { address: '10.2.2.10', inUse: true, inUseSince: 5 },
          { address: '10.2.2.10', inUse: false, inUseSince: 1 },
          { address: '10.2.2.99', inUse: true, inUseSince: 5 },
        ],
      }),
      { encap: 'gre4', servers: [{ address: '10.2.2.10' }] },
    );
    expect(v.servers.map((s) => s.configured)).toEqual([true, false, false]);
    expect(v.encap).toBe('gre4');
  });

  it('reads the running VIPs defensively', () => {
    expect(lbVipsOf({}).size).toBe(0);
    expect(lbVipsOf({ services: { lb: { vips: { a: { prefix: 'x' }, b: 3 } } } }).size).toBe(1);
  });
});
