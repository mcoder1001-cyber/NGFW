import { ROOT_KEYS } from '@ngfw/schema';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { fakeQosResets, setFakeQosCounters } from '../../src/features/qos-flat/fake.js';
import { runSecret, startHarness, type Harness } from '../support/harness.js';

const PW = { op: runSecret(), ro: runSecret() };
const MP = { 'content-type': 'application/merge-patch+json' };

/** Slot-1 names (docs/lab/shared-host-rules.md): loopbacks loop1001/loop1002, policers and maps prefixed w1. */
const IFS = {
  loop1001: { enabled: true, ipv4: ['10.1.1.1/24'] },
  loop1002: { enabled: true, ipv4: ['10.1.2.1/24'] },
};
const QOS = {
  policers: {
    'w1-gold': {
      description: 'customer ingress',
      type: '2r3c-rfc2698',
      cir: 20000,
      eir: 40000,
      cb: 25000,
      eb: 50000,
      exceedAction: { action: 'mark-and-transmit', dscp: 10 },
    },
    'w1-pps': { rateUnit: 'pps', cir: 1000, cb: 100 },
  },
  shapers: { 'w1-uplink': { rateKbps: 50000 } },
  maps: { 'w1-remark': { id: 1001, rows: { ip: [{ from: 46, to: 34 }] } } },
  interfaces: {
    loop1001: {
      description: 'customer',
      policer: { input: 'w1-gold' },
      shaper: 'w1-uplink',
      record: 'vlan',
      store: { source: 'ip', value: 0 },
    },
    loop1002: {
      policer: { output: 'w1-pps' },
      record: 'ip',
      mark: { map: 'w1-remark', output: 'ip' },
    },
  },
};

/**
 * F-qos-flat on the host PostgreSQL with the fake agent: services.qos through the generic pointer routes (validation
 * with pointers, commit, rollback), `GET /api/v1/state/services/qos/policers` (QosPolicerState joined with running)
 * and `POST /api/v1/actions/qos/policers/{name}/reset` (QosPolicerReset: RBAC, 404, 400, audit).
 */
describe('F-qos-flat e2e (PostgreSQL + fake agent)', () => {
  let h: Harness;
  let admin: string;
  let op: string;
  let ro: string;
  let base: number;

  beforeAll(async () => {
    h = await startHarness({});
    admin = await h.login('admin', h.adminPassword);
    await h.createUsers(admin, [
      { username: 'op1', role: 'operator', password: PW.op },
      { username: 'ro1', role: 'readonly', password: PW.ro },
    ]);
    op = await h.login('op1', PW.op);
    ro = await h.login('ro1', PW.ro);
    base = Number((await h.call(ro, 'GET', '/api/v1/config')).headers['x-vrx-revision']);
    expect((await h.call(op, 'PATCH', '/api/v1/config/interfaces', IFS, MP)).status).toBe(200);
  });
  afterAll(async () => h?.close());

  it('store.source mpls → 400 problem+json at …/store/source (services.qos-flat-store-source)', async () => {
    // record is vlan on loop1001, so the store uses another slot (record and store on one slot is a schema error)
    const bad = {
      ...QOS,
      interfaces: {
        ...QOS.interfaces,
        loop1001: { ...QOS.interfaces.loop1001, store: { source: 'mpls', value: 3 } },
      },
    };
    expect((await h.call(op, 'PUT', '/api/v1/config/services/qos', bad)).status).toBe(200);
    const applies = h.fake.calls.filter((c) => c.method === 'Apply').length;
    for (const route of ['/api/v1/config/validate', '/api/v1/config/commit']) {
      const r = await h.call(op, 'POST', route);
      expect(r.status).toBe(400);
      expect(r.headers['content-type']).toMatch(/^application\/problem\+json/);
      expect(r.body).toMatchObject({ status: 400, tier: 'semantic' });
      expect(r.body.errors).toContainEqual(
        expect.objectContaining({
          pointer: '/services/qos/interfaces/loop1001/store/source',
          message: expect.stringMatching(/ip source only/),
        }),
      );
    }
    expect(h.fake.calls.filter((c) => c.method === 'Apply')).toHaveLength(applies); // never sent to the agent
  });

  it('mark without map and shaper + policer.output are 400 with pointers at edit time', async () => {
    const mark = await h.call(
      op,
      'PATCH',
      '/api/v1/config/services/qos/interfaces/loop1002',
      { mark: { map: null } },
      MP,
    );
    expect(mark.status).toBe(400);
    expect(mark.body.errors).toContainEqual(
      expect.objectContaining({ pointer: '/services/qos/interfaces/loop1002/mark/map' }),
    );
    const both = await h.call(
      op,
      'PATCH',
      '/api/v1/config/services/qos/interfaces/loop1001',
      { policer: { output: 'w1-pps' } },
      MP,
    );
    expect(both.status).toBe(400);
    expect(both.body.errors).toContainEqual(
      expect.objectContaining({ pointer: '/services/qos/interfaces/loop1001/shaper' }),
    );
  });

  it('a valid flat QoS configuration commits, reaches the agent and shows in the policer state', async () => {
    expect((await h.call(op, 'PUT', '/api/v1/config/services/qos', QOS)).status).toBe(200);
    const c = await h.call(op, 'POST', '/api/v1/config/commit?comment=qos');
    expect(c.status).toBe(200);
    expect(c.body).toMatchObject({ status: 'applied' });
    const applied = (h.fake.current['services'] as any).qos;
    expect(Object.keys(applied.policers).sort()).toEqual(['w1-gold', 'w1-pps']);
    expect(applied.interfaces.loop1001).toMatchObject({
      shaper: 'w1-uplink',
      policer: { input: 'w1-gold' },
    });

    setFakeQosCounters(h.fake, 'w1-gold', {
      conform: { packets: 10, bytes: 15000 },
      exceed: { packets: 2, bytes: 3000 },
    });
    const st = await h.call(ro, 'GET', '/api/v1/state/services/qos/policers');
    expect(st.status).toBe(200);
    expect(st.body.countersError).toBeNull();
    expect(st.body.items.map((i: { kind: string; name: string }) => `${i.kind}:${i.name}`)).toEqual(
      ['policer:w1-gold', 'policer:w1-pps', 'shaper:w1-uplink'],
    );
    const [gold, pps, uplink] = st.body.items;
    expect(gold).toMatchObject({
      vppName: 'w1-gold',
      configured: true,
      present: true,
      type: '2r3c-rfc2698',
      cir: 20000,
      eir: 40000,
      cb: 25000,
      conform: { packets: '10', bytes: '15000' },
      exceed: { packets: '2', bytes: '3000' },
      violate: { packets: '0', bytes: '0' },
      attachments: [{ interface: 'loop1001', direction: 'input' }],
    });
    expect(gold.bucket).toMatchObject({ current: 25000, limit: 25000 });
    expect(pps).toMatchObject({
      rateUnit: 'pps',
      attachments: [{ interface: 'loop1002', direction: 'output' }],
    });
    // shaper = egress policer shaper:<name>, 1r2c, cir = rateKbps, burst ≈ 10 ms (62 500 B at 50 Mbit/s)
    expect(uplink).toMatchObject({
      kind: 'shaper',
      vppName: 'shaper:w1-uplink',
      type: '1r2c',
      cir: 50000,
      cb: 62500,
      present: true,
      attachments: [{ interface: 'loop1001', direction: 'output' }],
    });
    const one = await h.call(
      ro,
      'GET',
      '/api/v1/state/services/qos/policers?name=shaper:w1-uplink',
    );
    expect(one.body.items.map((i: { vppName: string }) => i.vppName)).toEqual(['shaper:w1-uplink']);
    expect((await h.call(ro, 'GET', '/api/v1/state/services/qos/policers?name=a%20b')).status).toBe(
      400,
    );
  });

  it('a configured policer VPP lacks is listed with present:false; a VPP-only one with configured:false', async () => {
    const applied = (h.fake.current['services'] as any).qos;
    const saved = structuredClone(applied);
    delete applied.policers['w1-pps'];
    applied.policers['w1-extra'] = { cir: 5, cb: '1000' };
    try {
      const st = await h.call(ro, 'GET', '/api/v1/state/services/qos/policers');
      const by = new Map(st.body.items.map((i: { vppName: string }) => [i.vppName, i]));
      expect(by.get('w1-pps')).toMatchObject({
        configured: true,
        present: false,
        index: null,
        bucket: null,
        cir: 1000,
        cb: 100,
      });
      expect(by.get('w1-extra')).toMatchObject({
        configured: false,
        present: true,
        attachments: [],
      });
    } finally {
      (h.fake.current['services'] as any).qos = saved;
    }
  });

  it('reset: operator 200 (audited), unknown 404, bad name 400, readonly 403', async () => {
    const r = await h.call(op, 'POST', '/api/v1/actions/qos/policers/shaper:w1-uplink/reset');
    expect(r.status).toBe(200);
    expect(r.body).toMatchObject({ name: 'shaper:w1-uplink', index: expect.any(Number) });
    expect(typeof r.body.resetAt).toBe('string');
    expect(fakeQosResets(h.fake)).toEqual(['shaper:w1-uplink']);

    const missing = await h.call(op, 'POST', '/api/v1/actions/qos/policers/w1-nope/reset');
    expect(missing.status).toBe(404);
    expect(missing.headers['content-type']).toMatch(/^application\/problem\+json/);
    const bad = await h.call(op, 'POST', '/api/v1/actions/qos/policers/a%20b/reset');
    expect(bad.status).toBe(400);
    expect(bad.body.errors).toContainEqual(expect.objectContaining({ pointer: '/name' }));
    expect((await h.call(ro, 'POST', '/api/v1/actions/qos/policers/w1-gold/reset')).status).toBe(
      403,
    );
    expect(fakeQosResets(h.fake)).toEqual(['shaper:w1-uplink']);

    const a = await h.call(admin, 'GET', '/api/v1/audit?limit=200');
    const items = a.body.items as any[];
    expect(items).toContainEqual(
      expect.objectContaining({
        action: 'POST /api/v1/actions/qos/policers/:name/reset',
        resource: 'qos/policers/shaper:w1-uplink',
        username: 'op1',
        result: 'success',
        status: 200,
      }),
    );
    expect(items).toContainEqual(
      expect.objectContaining({
        action: 'POST /api/v1/actions/qos/policers/:name/reset',
        result: 'failure',
        status: 404,
      }),
    );
    expect(items).toContainEqual(
      expect.objectContaining({
        action: 'POST /api/v1/actions/qos/policers/:name/reset',
        username: 'ro1',
        result: 'failure',
        status: 403,
      }),
    );
  });

  it('an agent without the RPCs answers 501 (no fake data)', async () => {
    h.fake.implemented = ROOT_KEYS.filter((k) => k !== 'services');
    try {
      expect((await h.call(ro, 'GET', '/api/v1/state/services/qos/policers')).status).toBe(501);
      expect((await h.call(op, 'POST', '/api/v1/actions/qos/policers/w1-gold/reset')).status).toBe(
        501,
      );
    } finally {
      h.fake.implemented = [...ROOT_KEYS];
    }
  });

  it('rollback to the revision before QoS empties services.qos on the agent', async () => {
    const rb = await h.call(op, 'POST', `/api/v1/config/rollback/${base}`);
    expect(rb.status).toBe(200);
    const qos = ((h.fake.current['services'] ?? {}) as any).qos ?? {};
    expect(qos.policers ?? {}).toEqual({});
    expect(qos.shapers ?? {}).toEqual({});
    expect(qos.maps ?? {}).toEqual({});
    expect(qos.interfaces ?? {}).toEqual({});
    const st = await h.call(ro, 'GET', '/api/v1/state/services/qos/policers');
    expect(st.body.items).toEqual([]);
  });
});
