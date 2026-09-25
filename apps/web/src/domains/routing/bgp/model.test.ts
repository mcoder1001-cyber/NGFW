import { describe, expect, it } from 'vitest';
import en from '../../../locales/en/bgp.json';
import fa from '../../../locales/fa/bgp.json';
import {
  bgpGlobalSchema,
  bgpRecordItemSchema,
  formatUptime,
  isAddress,
  lcpSchema,
  neighborRows,
  policyItemSchema,
  stateChip,
  toggleRedistribute,
  type BgpConfig,
} from './model';

const keys = (o: object, p = ''): string[] =>
  Object.entries(o).flatMap(([k, v]) =>
    v !== null && typeof v === 'object' ? keys(v as object, `${p}${k}.`) : [`${p}${k}`],
  );

describe('bgp model', () => {
  it('takes every form from the one schema', () => {
    const g = bgpGlobalSchema().properties as Record<string, unknown>;
    expect(Object.keys(g)).toEqual(
      expect.arrayContaining(['asn', 'routerId', 'networks', 'ebgpRequiresPolicy']),
    );
    expect(g).not.toHaveProperty('neighbors');
    expect(g).not.toHaveProperty('peerGroups');
    expect(g).not.toHaveProperty('redistribute');
    expect(Object.keys(bgpRecordItemSchema('neighbors').properties ?? {})).toEqual(
      expect.arrayContaining(['remoteAs', 'peerGroup', 'passwordRef', 'afi']),
    );
    expect(Object.keys(bgpRecordItemSchema('peerGroups').properties ?? {})).not.toContain(
      'peerGroup',
    );
    expect(Object.keys(policyItemSchema('prefixLists').properties ?? {})).toEqual([
      'description',
      'family',
      'rules',
    ]);
    expect(Object.keys(policyItemSchema('routeMaps').properties ?? {})).toEqual([
      'description',
      'entries',
    ]);
    expect(Object.keys(lcpSchema().properties ?? {})).toEqual([
      'hostIfName',
      'hostIfType',
      'netns',
    ]);
  });

  it('joins configured neighbours with their live session (peer-group inheritance, case-insensitive addresses)', () => {
    const bgp = {
      asn: 65000,
      peerGroups: { pg: { remoteAs: 65001, afi: { ipv4Unicast: { enabled: true } } } },
      neighbors: {
        '10.0.0.2': { peerGroup: 'pg', description: 'b' },
        '2001:DB8::1': { remoteAs: 65002, shutdown: true, afi: { ipv6Unicast: { enabled: true } } },
      },
    } as unknown as BgpConfig;
    const rows = neighborRows(bgp, [
      {
        address: '10.0.0.2',
        state: 'Established',
        uptimeSec: 90061,
        prefixesReceived: 50,
        prefixesSent: 1,
        flaps: 2,
      },
      {
        address: '2001:db8::1',
        state: 'Idle (Admin)',
        uptimeSec: 0,
        prefixesReceived: 0,
        prefixesSent: 0,
        flaps: 0,
      },
    ]);
    expect(
      rows.map((r) => [r.address, r.remoteAs, r.families, r.state, r.prefixesReceived]),
    ).toEqual([
      ['10.0.0.2', '65001', 'IPv4', 'Established', 50],
      ['2001:DB8::1', '65002', 'IPv6', 'Idle (Admin)', 0],
    ]);
    expect(stateChip(rows[0]!.state, rows[0]!.shutdown)).toBe('up');
    expect(stateChip(rows[1]!.state, rows[1]!.shutdown)).toBe('adminDown');
    expect(stateChip('Active', false)).toBe('down');
    expect(stateChip('', false)).toBe('degraded');
    expect(formatUptime(90061)).toBe('1d 01:01:01');
    expect(formatUptime(0)).toBe('');
  });

  it('toggles redistribution sources and checks neighbour keys', () => {
    expect(toggleRedistribute(undefined, 'connected', true)).toEqual({ connected: {} });
    expect(toggleRedistribute({ static: { routeMap: 'rm' } }, 'static', true)).toEqual({
      static: { routeMap: 'rm' },
    });
    expect(toggleRedistribute({ static: {}, connected: {} }, 'static', false)).toEqual({
      connected: {},
    });
    for (const ok of ['10.0.0.1', '2001:db8::1', '::1']) expect(isAddress(ok), ok).toBe(true);
    for (const bad of ['10.0.0', 'x', '10.0.0.256', 'loop0'])
      expect(isAddress(bad), bad).toBe(false);
  });

  it('has identical en and fa key sets', () => {
    expect(keys(fa).sort()).toEqual(keys(en).sort());
  });
});
