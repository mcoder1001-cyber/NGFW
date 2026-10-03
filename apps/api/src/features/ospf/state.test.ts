import { RoutingStateResponse } from '@ngfw/proto';
import { describe, expect, it } from 'vitest';
import { OspfStateOut } from './dto.js';
import { OSPF_NEIGHBORS_READER, ospfStateOut } from './state.js';
const at = new Date('2026-10-03T12:00:00Z');
const observed = (raw = '{}') =>
  RoutingStateResponse.fromPartial({
    frrRunning: true,
    retrievedAt: at,
    readers: { [OSPF_NEIGHBORS_READER]: raw },
  });
const neighbor = {
  nbrState: 'Full/DR',
  ifaceAddress: '10.0.0.2',
  ifaceName: 'eth0:10.0.0.1',
  nbrPriority: 1,
};
describe('bounded public OSPF observations', () => {
  it('projects current and older FRR shapes across VRFs without raw diagnostics or extra reader data', () => {
    const response = observed(
      JSON.stringify({
        red: {
          neighbors: {
            '2.2.2.2': {
              state: '2-Way/DROther',
              address: '10.1.0.2',
              ifaceName: 'eth1:10.1.0.1',
              priority: 0,
            },
          },
        },
        default: { neighbors: { '1.1.1.1': [{ ...neighbor, password: 'NGFW_TEST_PSK_READER' }] } },
      }),
    );
    response.error = 'NGFW_TEST_PSK_PROVIDER';
    response.readers['other'] = 'NGFW_TEST_PSK_UNREQUESTED';
    const output = OspfStateOut.parse(ospfStateOut(response));
    expect(output.neighbors).toEqual([
      {
        vrf: 'default',
        routerId: '1.1.1.1',
        address: '10.0.0.2',
        interface: 'eth0',
        state: 'Full/DR',
        priority: 1,
      },
      {
        vrf: 'red',
        routerId: '2.2.2.2',
        address: '10.1.0.2',
        interface: 'eth1',
        state: '2-Way/DROther',
        priority: 0,
      },
    ]);
    expect(output).toMatchObject({
      retrievedAt: at.toISOString(),
      unavailable: null,
      warning: 'routing-observation-partial',
      truncated: false,
    });
    expect(JSON.stringify(output)).not.toContain('NGFW_TEST_PSK');
  });
  it('preserves FRR point-to-point Full/- adjacency state', () => {
    const output = ospfStateOut(
      observed(
        JSON.stringify({
          neighbors: {
            '1.1.1.1': [{ ...neighbor, nbrState: 'Full/-' }],
          },
        }),
      ),
    );
    expect(output.neighbors[0]?.state).toBe('Full/-');
    expect(output.unavailable).toBeNull();
    expect(output.warning).toBeNull();
  });
  it('skips bounded NBMA Attempt placeholder rows while preserving healthy neighbors and reporting partial observation', () => {
    const output = ospfStateOut(
      observed(
        JSON.stringify({
          default: {
            neighbors: {
              neighbor: [
                {
                  nbrState: 'Attempt/-',
                  ifaceAddress: '10.0.0.3',
                  ifaceName: 'eth0:10.0.0.1',
                  nbrPriority: 0,
                },
              ],
              '1.1.1.1': [neighbor],
            },
          },
        }),
      ),
    );
    expect(output.neighbors).toHaveLength(1);
    expect(output.neighbors[0]?.routerId).toBe('1.1.1.1');
    expect(output.unavailable).toBeNull();
    expect(output.warning).toBe('routing-observation-partial');
  });
  it('reports an all-placeholder observation as partial and still applies its work bound', () => {
    expect(
      ospfStateOut(observed(JSON.stringify({ neighbors: { neighbor: [neighbor] } }))),
    ).toMatchObject({
      neighbors: [],
      unavailable: null,
      warning: 'routing-observation-partial',
    });
    expect(
      ospfStateOut(
        observed(
          JSON.stringify({
            neighbors: {
              neighbor: Array.from({ length: 2001 }, () => neighbor),
            },
          }),
        ),
      ).unavailable,
    ).toBe('reader-limit-exceeded');
  });
  it('supports a single default instance and preserves unknown facts', () => {
    const state = ospfStateOut(
      observed(
        JSON.stringify({
          neighbors: {
            '1.1.1.1': [
              {
                state: 'NGFW_TEST_PSK_STATE',
                ifaceName: '\u001bunsafe',
                priority: 999,
                address: 'invalid',
              },
            ],
          },
        }),
      ),
    );
    expect(state.neighbors[0]).toEqual({
      vrf: 'default',
      routerId: '1.1.1.1',
      state: 'Unknown',
      interface: null,
      priority: null,
      address: null,
    });
    expect(JSON.stringify(state)).not.toContain('NGFW_TEST_PSK');
  });
  it('distinguishes observed empty inventory from missing readers or stopped FRR', () => {
    expect(ospfStateOut(observed())).toMatchObject({ unavailable: null, neighbors: [] });
    const missing = observed();
    missing.readers = {};
    expect(ospfStateOut(missing).unavailable).toBe('reader-unavailable');
    const stopped = observed('malformed');
    stopped.frrRunning = false;
    expect(ospfStateOut(stopped)).toMatchObject({ unavailable: 'frr-unavailable', neighbors: [] });
  });
  it.each([
    '',
    'not-json',
    'null',
    '[]',
    '{"default":{"neighbors":null}}',
    '{"neighbors":{"invalid":[{}]}}',
    '{"neighbors":{"1.1.1.1":[null]}}',
  ])('marks malformed reader %s unavailable rather than healthy empty', (raw) => {
    expect(ospfStateOut(observed(raw))).toMatchObject({
      unavailable: 'reader-invalid',
      neighbors: [],
      truncated: false,
    });
  });
  it('sorts before bounding output to 100 rows and reports truncation', () => {
    const rows = Array.from({ length: 101 }, (_, i) => ({
      ...neighbor,
      ifaceName: `eth${String(100 - i).padStart(3, '0')}:10.0.0.1`,
    }));
    const output = ospfStateOut(observed(JSON.stringify({ neighbors: { '1.1.1.1': rows } })));
    expect(output.neighbors).toHaveLength(100);
    expect(output.truncated).toBe(true);
    expect(output.neighbors[0]?.interface).toBe('eth000');
    expect(output.neighbors.at(-1)?.interface).toBe('eth099');
  });
  it('rejects excessive input bytes or inspection work with no partial healthy inventory', () => {
    expect(ospfStateOut(observed(' '.repeat(1024 * 1024 + 1))).unavailable).toBe(
      'reader-limit-exceeded',
    );
    expect(
      ospfStateOut(
        observed(
          JSON.stringify({
            neighbors: { '1.1.1.1': Array.from({ length: 2001 }, () => neighbor) },
          }),
        ),
      ).unavailable,
    ).toBe('reader-limit-exceeded');
    const instances = Object.fromEntries(
      Array.from({ length: 129 }, (_, i) => [`vrf${i}`, { neighbors: {} }]),
    );
    expect(ospfStateOut(observed(JSON.stringify(instances))).unavailable).toBe(
      'reader-limit-exceeded',
    );
  });
  it('leaves missing or invalid timestamps unknown', () => {
    const response = observed();
    response.retrievedAt = undefined;
    expect(ospfStateOut(response).retrievedAt).toBeNull();
    response.retrievedAt = new Date('invalid');
    expect(ospfStateOut(response).retrievedAt).toBeNull();
  });
});
