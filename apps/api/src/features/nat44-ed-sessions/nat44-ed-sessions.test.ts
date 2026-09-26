import type { NatPoolUsage } from '@ngfw/proto';
import { describe, expect, it } from 'vitest';
import type { ProblemError } from '../../common/problem.js';
import { SessionsQuery } from './dto.js';
import { fakeSession } from './fake.js';
import { filterOf, killOutcome, mergePools, PORTS_PER_ADDRESS, sessionJson } from './service.js';

const usage = (p: Partial<NatPoolUsage>): NatPoolUsage => ({
  firstAddress: '',
  lastAddress: '',
  interface: '',
  vrf: '',
  twiceNat: false,
  addresses: 0,
  sessions: '0',
  ...p,
});

describe('mergePools', () => {
  it('names the agent pools from the running configuration by identity and flags drift both ways', () => {
    const agent = [
      usage({
        firstAddress: '10.4.2.100',
        lastAddress: '10.4.2.103',
        vrf: 'default',
        addresses: 4,
        sessions: '2000',
      }),
      usage({
        firstAddress: '10.4.2.120',
        lastAddress: '10.4.2.120',
        vrf: 'default',
        twiceNat: true,
        addresses: 1,
      }),
      usage({
        interface: 'host-w4w0',
        firstAddress: '10.4.2.1',
        lastAddress: '10.4.2.1',
        addresses: 1,
        sessions: '3',
      }),
      usage({ firstAddress: '10.4.9.1', lastAddress: '10.4.9.1', vrf: 'default', addresses: 1 }), // not configured
    ];
    const running = {
      pools: [
        { name: 'out', range: '10.4.2.100-10.4.2.103' },
        { name: 'tn', range: '10.4.2.120', twiceNat: true },
        { name: 'wan', interface: 'host-w4w0' },
        { name: 'cust', range: '10.4.3.1-10.4.3.4', vrf: 'cust' }, // not applied
      ],
    };
    const out = mergePools(agent, running);
    expect(
      out.map((p) => [p.name, p.kind, p.range ?? p.interface, p.applied, p.configured]),
    ).toEqual([
      ['out', 'range', '10.4.2.100-10.4.2.103', true, true],
      ['tn', 'range', '10.4.2.120', true, true],
      ['wan', 'interface', 'host-w4w0', true, true],
      [null, 'range', '10.4.9.1', true, false],
      ['cust', 'range', '10.4.3.1-10.4.3.4', false, true],
    ]);
    expect(out[0]?.utilisation).toBeCloseTo(2000 / (4 * PORTS_PER_ADDRESS));
    expect(out[4]).toMatchObject({ addresses: 4, sessions: 0, utilisation: 0, vrf: 'cust' });
  });

  it('keeps the twice-NAT class and the VRF apart and caps utilisation at 1', () => {
    const out = mergePools(
      [
        usage({
          firstAddress: '10.4.2.1',
          lastAddress: '10.4.2.1',
          vrf: 'default',
          addresses: 1,
          sessions: '999999',
        }),
      ],
      { pools: [{ name: 'tn', range: '10.4.2.1', twiceNat: true }] },
    );
    expect(out.map((p) => p.name)).toEqual([null, 'tn']);
    expect(out[0]?.utilisation).toBe(1);
    expect(mergePools([], undefined)).toEqual([]);
  });
});

describe('sessions and kill mapping', () => {
  it('turns the query into the agent filter and the rows into numbers', () => {
    const q = SessionsQuery.parse({
      page: '3',
      pageSize: '50',
      inside: '10.4.1.2',
      port: '80',
      protocol: 'TCP',
      vrf: 'default',
    });
    expect(q).toMatchObject({ page: 3, pageSize: 50, port: 80 });
    expect(filterOf(q)).toEqual({
      insideAddress: '10.4.1.2',
      port: 80,
      protocol: 'tcp',
      vrf: 'default',
    });
    expect(SessionsQuery.safeParse({ pageSize: '257' }).success).toBe(false); // review H1: ≤ 256 hosts per call
    expect(SessionsQuery.safeParse({ pageSize: '256' }).success).toBe(true);
    expect(SessionsQuery.safeParse({ inside: '10.4.1' }).success).toBe(false);
    expect(
      sessionJson(fakeSession({ bytes: '4096', packets: '7', idleSeconds: '3' })),
    ).toMatchObject({
      bytes: 4096,
      packets: 7,
      idleSeconds: 3,
    });
  });

  it('maps the done exit codes to 200 / 404 / 502', () => {
    expect(killOutcome({ summary: 'session deleted', exitCode: 0, stats: { a: 'b' } })).toEqual({
      deleted: true,
      summary: 'session deleted',
      stats: { a: 'b' },
    });
    const status = (code: number) => {
      try {
        killOutcome({ summary: 'x', exitCode: code, stats: {} });
      } catch (e) {
        return (e as ProblemError).getStatus();
      }
      return 200;
    };
    expect([status(1), status(2)]).toEqual([404, 502]);
  });
});
