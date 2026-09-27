import { describe, expect, it } from 'vitest';
import { RootConfig, type RootConfigInput } from '../../index.js';

const doc = (ldp: unknown): RootConfigInput => ({ routing: { mpls: { ldp } } } as RootConfigInput);

describe('F-mpls-ldp schema', () => {
  it('routing.mpls.ldp is absent by default', () => {
    expect(RootConfig.parse({}).routing.mpls).toBeUndefined();
  });

  it('accepts a minimal LDP config', () => {
    const c = RootConfig.parse(
      doc({ routerId: '10.0.0.1', transportAddress: '10.0.0.1', interfaces: ['eth0'] }),
    );
    expect(c.routing.mpls!.ldp!.routerId).toBe('10.0.0.1');
    expect(c.routing.mpls!.ldp!.neighbors).toEqual({});
  });

  it('accepts a password reference and a label range', () => {
    const r = RootConfig.safeParse(
      doc({
        routerId: '10.0.0.1',
        transportAddress: '10.0.0.1',
        interfaces: ['eth0'],
        neighbors: { '10.0.0.2': { passwordRef: 'password/ldp' } },
        labelRange: { min: 16000, max: 24000 },
      }),
    );
    expect(r.success).toBe(true);
  });

  it('rejects an inline secret in passwordRef', () => {
    const r = RootConfig.safeParse(
      doc({ routerId: '10.0.0.1', transportAddress: '10.0.0.1', interfaces: ['eth0'], neighbors: { '10.0.0.2': { passwordRef: 'hunter2' } } }),
    );
    expect(r.success).toBe(false);
  });

  it('rejects a non-IPv4 router-id and an empty interface list', () => {
    expect(RootConfig.safeParse(doc({ routerId: '2001:db8::1', transportAddress: '10.0.0.1', interfaces: ['eth0'] })).success).toBe(false);
    expect(RootConfig.safeParse(doc({ routerId: '10.0.0.1', transportAddress: '10.0.0.1', interfaces: [] })).success).toBe(false);
  });

  it('rejects an inverted label range', () => {
    const r = RootConfig.safeParse(
      doc({ routerId: '10.0.0.1', transportAddress: '10.0.0.1', interfaces: ['eth0'], labelRange: { min: 24000, max: 16000 } }),
    );
    expect(r.success).toBe(false);
  });
});
