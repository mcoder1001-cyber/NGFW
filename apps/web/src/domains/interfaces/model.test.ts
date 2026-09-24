import { describe, expect, it } from 'vitest';
import { adminStatus, createMergePatch, interfaceFormSchema, linkStatus, subinterfaceSchema, type LiveState } from './model';
import { foldRates, stepRates } from './rates';

describe('interfaces model', () => {
  it('form schema is the generated interface item without subinterfaces; sub-interface schema has vlanId', () => {
    const f = interfaceFormSchema();
    expect(Object.keys(f.properties ?? {})).toEqual(expect.arrayContaining(['enabled', 'mtu', 'ipv4', 'ipv6', 'vrf', 'description']));
    expect(Object.keys(f.properties ?? {})).not.toContain('subinterfaces');
    expect(Object.keys(subinterfaceSchema().properties ?? {})).toEqual(expect.arrayContaining(['vlanId', 'enabled', 'ipv4']));
  });

  it('createMergePatch removes cleared members with null, recurses into objects, replaces arrays', () => {
    const from = { enabled: true, mtu: 1400, ipv4: ['10.1.1.1/24'], dhcpClient: { hostname: 'a', setBroadcastFlag: false } };
    const to = { enabled: true, ipv4: ['10.1.1.1/24', '10.1.3.1/24'], dhcpClient: { setBroadcastFlag: true } };
    expect(createMergePatch(from, to)).toEqual({ mtu: null, ipv4: ['10.1.1.1/24', '10.1.3.1/24'], dhcpClient: { hostname: null, setBroadcastFlag: true } });
    expect(createMergePatch(from, from)).toEqual({});
    expect(createMergePatch(undefined, { enabled: false })).toEqual({ enabled: false });
  });

  it('status chips: admin down is adminDown (not a failure), admin up + link down is down', () => {
    const s = (adminUp: boolean, linkUp: boolean) => ({ adminUp, linkUp }) as LiveState;
    expect([adminStatus(s(true, true)), linkStatus(s(true, true))]).toEqual(['up', 'up']);
    expect([adminStatus(s(true, false)), linkStatus(s(true, false))]).toEqual(['up', 'down']);
    expect([adminStatus(s(false, false)), linkStatus(s(false, true))]).toEqual(['adminDown', 'adminDown']);
    expect(linkStatus(null)).toBeUndefined();
  });
});

describe('rates from iface.counters', () => {
  const c = (rxP: number, rxB: number) => ({ name: 'host-w1l0', rxPackets: String(rxP), rxBytes: String(rxB), txPackets: '0', txBytes: '0' });

  it('bits/s and packets/s from consecutive absolute samples; history for the sparkline', () => {
    const last = new Map();
    let r = foldRates(last, new Map(), { ts: '2026-09-24T00:00:00.000Z', interfaces: [c(100, 10_000)] }, 0);
    expect(r.size).toBe(0); // one sample is no rate
    r = foldRates(last, r, { ts: '2026-09-24T00:00:02.000Z', interfaces: [c(300, 30_000)] }, 0);
    expect(r.get('host-w1l0')).toMatchObject({ rxPps: 100, rxBps: 80_000, txPps: 0, history: [100] });
    r = foldRates(last, r, { ts: '2026-09-24T00:00:03.000Z', interfaces: [c(310, 31_000)] }, 0);
    expect(r.get('host-w1l0')?.history).toEqual([100, 10]);
  });

  it('a counter that went backwards (interface re-created) yields no rate', () => {
    expect(stepRates({ at: 0, rxB: 10, txB: 0, rxP: 5, txP: 0 }, { at: 1000, rxB: 1, txB: 0, rxP: 1, txP: 0 })).toBeUndefined();
  });
});
