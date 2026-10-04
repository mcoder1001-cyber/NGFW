import { describe, expect, it, vi } from 'vitest';
import type { RoutingStateResponse } from '@ngfw/proto';
import type { AgentClient } from '../../agent/agent.client.js';
import { IsisRipController } from './isis-rip.controller.js';
import { observed } from './state.js';
const response = (readers: Record<string, string>, running = true) =>
  ({ frrRunning: running, readers }) as RoutingStateResponse;
describe('bounded IS-IS and RIP observations', () => {
  it('projects public adjacency fields without daemon secrets', () => {
    const result = observed(
      response({
        isisNeighbors: JSON.stringify({
          areas: [
            {
              area: 'ngfw',
              circuits: [
                {
                  adj: '0000.0000.0002',
                  interface: 'eth0',
                  level: 2,
                  state: 'Up',
                  password: 'hidden',
                },
              ],
            },
          ],
        }),
      }),
      'isisNeighbors',
      0,
      100,
    );
    expect(result.unavailable).toBeNull();
    expect(result.rows).toEqual([
      {
        vrf: 'default',
        area: 'ngfw',
        systemId: '0000.0000.0002',
        interface: 'eth0',
        level: 2,
        state: 'Up',
      },
    ]);
  });
  it.each(['{}', '{"areas":[]}'])('accepts legitimate empty IS-IS %s', (raw) => {
    expect(
      observed(response({ isisNeighbors: raw }), 'isisNeighbors', 0, 100).unavailable,
    ).toBeNull();
  });
  it.each([
    '',
    '{"foo":1}',
    '{"areas":null}',
    '{"vrfs":[{"vrf":"default"}]}',
    '{"areas":[{"circuits":[{"adj":"peer"}]}]}',
  ])('rejects malformed adjacency %s', (raw) => {
    expect(observed(response({ isisNeighbors: raw }), 'isisNeighbors', 0, 100).unavailable).toBe(
      'reader-invalid',
    );
  });
  it('reports unavailable and oversized readers truthfully', () => {
    expect(observed(response({}, false), 'ripStatus', 0, 100).unavailable).toBe('frr-unavailable');
    expect(observed(response({}), 'ripStatus', 0, 100).unavailable).toBe('reader-unavailable');
    expect(
      observed(response({ ripStatus: 'x'.repeat(1024 * 1024 + 1) }), 'ripStatus', 0, 100)
        .unavailable,
    ).toBe('reader-limit-exceeded');
  });
  it('bounds peers and states the default VRF scope', () => {
    const result = observed(
      response({
        ripngStatus: JSON.stringify({
          vrf: 'default',
          protocol: 'ripng',
          peers: Array.from({ length: 105 }, (_, i) => ({
            address: `fe80::${i}`,
            badPackets: 0,
            badRoutes: 1,
            distance: 120,
            lastUpdate: '00:00:12',
          })),
        }),
      }),
      'ripngStatus',
      5,
      10,
    );
    expect(result.scope).toBe('default-vrf');
    expect(result.total).toBe(105);
    expect(result.rows).toHaveLength(10);
  });
  it('queries only fixed readers and validates before the RPC', async () => {
    const routingState = vi.fn().mockResolvedValue(response({}));
    const controller = new IsisRipController({ routingState } as unknown as AgentClient);
    await controller.peers('ng');
    expect(routingState.mock.lastCall?.[0]).toEqual({
      readers: ['ripngStatus'],
      ribPrefixes: [],
      ribVrf: '',
    });
    await expect(controller.database('-1', '1')).rejects.toMatchObject({ status: 400 });
    expect(() => controller.peers('1')).toThrow();
    expect(routingState).toHaveBeenCalledTimes(1);
  });
  it('pages FRR routes without identifying them as VPP forwarding', () => {
    const result = observed(
      response({
        ripRoutes:
          '{"default":{"192.0.2.0/24":[{"protocol":"rip","metric":2,"password":"hidden"}]}}',
      }),
      'ripRoutes',
      0,
      100,
    );
    expect(result.rows).toEqual([{ protocol: 'rip', metric: 2, prefix: '192.0.2.0/24' }]);
  });
});
