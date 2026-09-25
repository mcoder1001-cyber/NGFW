import { describe, expect, it } from 'vitest';
import type { AgentClient } from '../../agent/agent.client.js';
import { annotateFrr } from './routes.js';

const row = (prefix: string, source: string) => ({
  vrf: 'default',
  prefix,
  source,
  origin: source,
});

function fakeAgent(fail = false) {
  const calls: unknown[] = [];
  const agent = {
    routingState: (req: unknown) => {
      calls.push(req);
      if (fail) return Promise.reject(new Error('agent busy'));
      return Promise.resolve({
        rib: [
          { prefix: '10.8.64.0/25', protocol: 'bgp', selected: true },
          { prefix: '10.8.200.0/24', protocol: 'static', selected: true },
        ],
      });
    },
  } as unknown as AgentClient;
  return { agent, calls };
}

describe('annotateFrr (review M3)', () => {
  it('marks linux-nl routes as frr without asking the agent when no proto is given', async () => {
    const { agent, calls } = fakeAgent(true);
    const items = [row('10.8.64.0/25', 'lcp-rt-dynamic'), row('10.8.1.0/24', 'interface')];
    const out = await annotateFrr(agent, items, undefined);
    expect(calls).toHaveLength(0);
    expect(out.map((i) => i.origin)).toEqual(['frr', 'interface']);
    expect(out[0]).not.toHaveProperty('proto');
  });

  it('with proto: one RIB lookup per VRF page, only that protocol kept', async () => {
    const { agent, calls } = fakeAgent();
    const out = await annotateFrr(
      agent,
      [row('10.8.64.0/25', 'lcp-rt-dynamic'), row('10.8.200.0/24', 'lcp-rt-dynamic')],
      'bgp',
    );
    expect(calls).toEqual([
      { readers: [], ribPrefixes: ['10.8.64.0/25', '10.8.200.0/24'], ribVrf: '' },
    ]);
    expect(out).toEqual([
      expect.objectContaining({ prefix: '10.8.64.0/25', proto: 'bgp', origin: 'frr' }),
    ]);
  });

  it('with proto: the agent error is the answer (no silently empty page)', async () => {
    const { agent } = fakeAgent(true);
    await expect(
      annotateFrr(agent, [row('10.8.64.0/25', 'lcp-rt-dynamic')], 'bgp'),
    ).rejects.toThrow('agent busy');
  });
});
