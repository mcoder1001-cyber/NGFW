import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { FakeAgent } from '../../testing/fake-agent.js';
import { testEnv } from '../../testing/fixtures.js';
import { describe, expect, it } from 'vitest';
import { TunnelStateTunnel, type TunnelStateResponse } from '@ngfw/proto';
import { AgentClient } from '../../agent/agent.client.js';
import { TunnelsController, TunnelsStateOut, toTunnelsState } from './tunnels.controller.js';

const sample = (): TunnelStateResponse => ({
  owner: 'w1',
  retrievedAt: new Date('2026-10-04T00:00:00Z'),
  tunnels: [
    TunnelStateTunnel.fromPartial({
      name: 'gpe-config',
      kind: 'vxlanGpe',
      interface: 'vxlan_gpe_tunnel42',
      src: '2001:db8::1',
      dst: '2001:db8::2',
      ipv4TableId: 0,
      ipv6TableId: 17,
      notes: ['Counters unavailable'],
    }),
  ],
});
describe('tunnels state contract', () => {
  it('keeps zero FIB distinct from unavailable counters and endpoints', () => {
    const out = toTunnelsState(sample());
    expect(TunnelsStateOut.parse(out)).toEqual(out);
    expect(out.items[0]).toMatchObject({
      name: 'gpe-config',
      interface: 'vxlan_gpe_tunnel42',
      ipv4TableId: 0,
      ipv6TableId: 17,
      underlayTableId: null,
      counters: null,
    });
    expect(
      toTunnelsState({
        owner: 'w1',
        retrievedAt: undefined,
        tunnels: [TunnelStateTunnel.fromPartial({ kind: 'ipip', deviceClass: 'ip6ip-6rd' })],
      }).items[0]?.src,
    ).toBeNull();
  });
  it('serializes uint64 counters without precision loss', () => {
    const r = sample();
    r.tunnels[0]!.counters = {
      name: 'x',
      swIfIndex: 0,
      rxPackets: '18446744073709551615',
      rxBytes: '0',
      txPackets: '0',
      txBytes: '0',
      drops: '0',
      errors: '0',
      punts: '0',
      rxMisses: '0',
    };
    expect(toTunnelsState(r).items[0]?.counters?.rxPackets).toBe('18446744073709551615');
  });
  it('controller consumes the owner-filtered agent RPC and propagates unavailability', async () => {
    const c = new TunnelsController({ tunnelState: async () => sample() } as AgentClient);
    expect((await c.state()).items[0]?.name).toBe('gpe-config');
    const failed = new TunnelsController({
      tunnelState: async () => {
        throw new Error('unavailable');
      },
    } as unknown as AgentClient);
    await expect(failed.state()).rejects.toThrow('unavailable');
  });
  it('client sends the configured owner over gRPC and returns actual allocated names', async () => {
    const dir = mkdtempSync(join(tmpdir(), 'tunnels-'));
    const socket = join(dir, 'agent.sock');
    const fake = new FakeAgent({ owner: 'w1' });
    await fake.start(socket);
    fake.reset({
      tunnels: {
        vxlanGpe: { 'configured-gpe': { src: '10.0.0.1', dst: '10.0.0.2', vni: 15 } },
        pppoe: {
          subscriber: { sessionId: 1, clientIp: '10.0.0.3', clientMac: '02:00:00:00:00:01' },
        },
      },
    });
    const agent = new AgentClient(testEnv({ NGFW_AGENT_SOCKET: socket, NGFW_AGENT_OWNER: 'w1' }));
    try {
      const r = await agent.tunnelState();
      expect(r.tunnels.map((t) => t.name)).toContain('configured-gpe');
      expect(fake.calls.find((c) => c.method === 'TunnelState')?.request).toEqual({ owner: 'w1' });
      fake.failAllWith = 14;
      await expect(agent.tunnelState()).rejects.toThrow();
    } finally {
      agent.close();
      await fake.stop();
      rmSync(dir, { recursive: true, force: true });
    }
  });
});
