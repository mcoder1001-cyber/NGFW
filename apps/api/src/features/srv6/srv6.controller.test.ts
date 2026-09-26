import { credentials } from '@grpc/grpc-js';
import { DataplaneClient, type Srv6StateResponse } from '@ngfw/proto';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { FakeAgent } from '../../testing/fake-agent.js';
import { setFakeSrv6Counters } from './fake.js';
import { srv6Configured, srv6StateOf, Srv6StateOut, vrfNames } from './srv6.controller.js';

const running = {
  vrfs: { default: { id: 0 }, 'cust-a': { id: 4001 } },
  routing: {
    srv6: {
      encapSource: 'fd00:4::1',
      localSids: {
        'fd00:4:ff::1': { behavior: 'end', psp: true, vrf: 'default' },
        'fd00:4:ff::a': { behavior: 'end.dt4', psp: false, vrf: 'default', lookupVrf: 'cust-a' },
      },
      policies: {
        'fd00:4:bb::1': {
          type: 'default',
          encap: true,
          vrf: 'default',
          sidLists: [{ sids: ['fd00:4:ee::1'], weight: 1 }],
        },
      },
      steering: [
        { type: 'l3', prefix: '10.4.100.0/24', vrf: 'cust-a', bsid: 'fd00:4:bb::1' },
        { type: 'l2', interface: 'host-w4l1', bsid: 'fd00:4:bb::1' },
      ],
    },
  },
};

const agentState: Srv6StateResponse = {
  owner: 'w4',
  retrievedAt: new Date('2026-09-25T10:00:00Z'),
  localSids: [
    {
      sid: 'fd00:4:ff::1',
      behavior: 'end',
      psp: true,
      fibTable: 0,
      interface: '',
      nextHop: '',
      lookupTable: 0,
      goodPackets: '5',
      goodBytes: '640',
      badPackets: '1',
      badBytes: '128',
    },
    {
      sid: 'fd00:4:ff::a',
      behavior: 'end.dt4',
      psp: false,
      fibTable: 0,
      interface: '',
      nextHop: '',
      lookupTable: 4001,
      goodPackets: '0',
      goodBytes: '0',
      badPackets: '0',
      badBytes: '0',
    },
    {
      sid: 'fd00:4:ff::77',
      behavior: 'end.dt6',
      psp: false,
      fibTable: 4999,
      interface: '',
      nextHop: '',
      lookupTable: 0,
      goodPackets: '0',
      goodBytes: '0',
      badPackets: '0',
      badBytes: '0',
    },
  ],
  policies: [
    {
      bsid: 'fd00:4:bb::1',
      type: 'default',
      encap: true,
      fibTable: 0,
      encapSource: 'fd00:4::1',
      sidLists: [{ sids: ['fd00:4:ee::1'], weight: 1 }],
    },
    {
      bsid: 'fd00:4:bb::9',
      type: 'spray',
      encap: false,
      fibTable: 0,
      encapSource: '',
      sidLists: [{ sids: ['fd00:4:ee::9'], weight: 2 }],
    },
  ],
  steering: [
    {
      trafficType: 'ipv4',
      prefix: '10.4.100.0/24',
      fibTable: 4001,
      interface: '',
      bsid: 'fd00:4:bb::1',
    },
    {
      trafficType: 'ipv6',
      prefix: 'fd00:4:100::/48',
      fibTable: 0,
      interface: '',
      bsid: 'fd00:4:bb::9',
    },
    { trafficType: 'l2', prefix: '', fibTable: 0, interface: 'host-w4l1', bsid: 'fd00:4:bb::1' },
  ],
};

describe('F-srv6 API helpers', () => {
  it('maps table ids to the running VRF names (0 = default, unknown = the id)', () => {
    const name = vrfNames(running);
    expect([name(0), name(4001), name(4999)]).toEqual(['default', 'cust-a', '4999']);
    expect(vrfNames({ vrfs: { main: { id: 0 } } })(0)).toBe('main');
  });

  it('collects the configured keys canonically', () => {
    const c = srv6Configured({
      routing: {
        srv6: {
          localSids: { 'FD00:4:FF:0::1': {} },
          policies: {},
          steering: [{ type: 'l3', prefix: 'FD00:4:100::/48', bsid: 'x' }],
        },
      },
    });
    expect([...c.sids]).toEqual(['fd00:4:ff::1']);
    expect([...c.steering]).toEqual(['l3|default|fd00:4:100::/48']);
    expect(srv6Configured({})).toEqual({ sids: new Set(), bsids: new Set(), steering: new Set() });
  });

  it('joins the agent state with the running configuration', () => {
    const out = srv6StateOf(agentState, running);
    expect(Srv6StateOut.parse(out)).toEqual(out);
    expect(out.retrievedAt).toBe('2026-09-25T10:00:00.000Z');
    expect(out.localSids).toEqual([
      {
        sid: 'fd00:4:ff::1',
        behavior: 'end',
        psp: true,
        vrf: 'default',
        table: 0,
        interface: null,
        nextHop: null,
        lookupVrf: null,
        lookupTable: null,
        goodPackets: 5,
        goodBytes: 640,
        badPackets: 1,
        badBytes: 128,
        configured: true,
      },
      expect.objectContaining({
        sid: 'fd00:4:ff::a',
        lookupVrf: 'cust-a',
        lookupTable: 4001,
        configured: true,
      }),
      expect.objectContaining({
        sid: 'fd00:4:ff::77',
        vrf: '4999',
        lookupVrf: 'default',
        lookupTable: 0,
        configured: false,
      }),
    ]);
    expect(out.policies.map((p) => [p.bsid, p.encapSource, p.configured])).toEqual([
      ['fd00:4:bb::1', 'fd00:4::1', true],
      ['fd00:4:bb::9', null, false],
    ]);
    expect(out.steering).toEqual([
      {
        type: 'l3',
        trafficType: 'ipv4',
        prefix: '10.4.100.0/24',
        vrf: 'cust-a',
        table: 4001,
        interface: null,
        bsid: 'fd00:4:bb::1',
        configured: true,
      },
      {
        type: 'l3',
        trafficType: 'ipv6',
        prefix: 'fd00:4:100::/48',
        vrf: 'default',
        table: 0,
        interface: null,
        bsid: 'fd00:4:bb::9',
        configured: false,
      },
      {
        type: 'l2',
        trafficType: 'l2',
        prefix: null,
        vrf: null,
        table: null,
        interface: 'host-w4l1',
        bsid: 'fd00:4:bb::1',
        configured: true,
      },
    ]);
  });
});

describe('F-srv6 fake agent (Srv6State)', () => {
  const dir = mkdtempSync(join(process.env['TMPDIR'] ?? tmpdir(), 'srv6-'));
  const sock = join(dir, 'a.sock');
  const agent = new FakeAgent({ owner: 'w4' });
  let client: DataplaneClient;

  beforeAll(async () => {
    await agent.start(sock);
    client = new DataplaneClient(`unix:${sock}`, credentials.createInsecure());
  });
  afterAll(async () => {
    client.close();
    await agent.stop();
    rmSync(dir, { recursive: true, force: true });
  });

  const ask = (owner: string) =>
    new Promise<Srv6StateResponse>((resolve, reject) =>
      client.srv6State({ owner }, (err, res) => (err ? reject(err) : resolve(res))),
    );

  it('reports what the fake applied, sorted like the agent, with the resolved encap source', async () => {
    agent.reset({
      vrfs: { 'cust-a': { id: 4001 } },
      routing: {
        srv6: {
          ...running.routing.srv6,
          policies: {
            ...running.routing.srv6.policies,
            'fd00:4:bb::3': { type: 'spray', encap: false, sidLists: [{ sids: ['fd00:4:ee::3'] }] },
          },
          steering: [
            { type: 'l2', interface: 'host-w4l1', bsid: 'fd00:4:bb::1' },
            { type: 'l3', prefix: 'fd00:4:100::/48', vrf: 'default', bsid: 'fd00:4:bb::3' },
            { type: 'l3', prefix: '10.4.100.0/24', vrf: 'cust-a', bsid: 'fd00:4:bb::1' },
          ],
        },
      },
    });
    setFakeSrv6Counters(agent, 'fd00:4:ff::1', {
      goodPackets: 3,
      goodBytes: 300,
      badPackets: 0,
      badBytes: 0,
    });
    const r = await ask('w4');
    expect(r.localSids.map((l) => [l.sid, l.fibTable, l.lookupTable, l.goodPackets])).toEqual([
      ['fd00:4:ff::1', 0, 0, '3'],
      ['fd00:4:ff::a', 0, 4001, '0'],
    ]);
    expect(r.policies.map((p) => [p.bsid, p.encap, p.encapSource, p.sidLists[0]!.weight])).toEqual([
      ['fd00:4:bb::1', true, 'fd00:4::1', 1],
      ['fd00:4:bb::3', false, '', 1],
    ]);
    expect(r.steering.map((s) => [s.trafficType, s.fibTable, s.prefix || s.interface])).toEqual([
      ['ipv6', 0, 'fd00:4:100::/48'],
      ['ipv4', 4001, '10.4.100.0/24'],
      ['l2', 0, 'host-w4l1'],
    ]);
    expect(agent.calls.at(-1)).toEqual({ method: 'Srv6State', request: { owner: 'w4' } });
  });

  it('refuses another owner', async () => {
    await expect(ask('w9')).rejects.toMatchObject({ code: 3 });
  });
});
