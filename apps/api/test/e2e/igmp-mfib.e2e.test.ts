import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { startHarness, type Harness } from '../support/harness.js';

/**
 * F-igmp-mfib e2e: the multicast config commits, and the state routes reflect it through the fake agent's
 * MulticastState. Live VPP mFIB / FRR pimd is the host follow-up.
 */
const MP = { 'content-type': 'application/merge-patch+json' };

describe('F-igmp-mfib e2e (PostgreSQL + fake agent)', () => {
  let h: Harness;
  let admin: string;
  let op: string;

  beforeAll(async () => {
    h = await startHarness({});
    admin = await h.login('admin', h.adminPassword);
    await h.createUsers(admin, [{ username: 'op1', role: 'operator', password: 'Op1-pw-1234567890' }]); // gitleaks:allow — test fixture (dummy password / RFC 6238 test vector), never a real secret
    op = await h.login('op1', 'Op1-pw-1234567890');

    // two interfaces to reference
    const ifs = await h.call(
      admin,
      'PUT',
      '/api/v1/config/interfaces',
      { eth0: { enabled: true }, eth1: { enabled: true } },
    );
    expect(ifs.status, ifs.raw).toBe(200);
    const mc = await h.call(
      admin,
      'PATCH',
      '/api/v1/config/routing/multicast',
      {
        igmp: { interfaces: { eth0: { mode: 'host', joins: [{ group: '239.1.1.1', sources: ['10.0.0.5'] }] } } },
        mroutes: [
          {
            group: '239.2.2.2',
            source: '10.0.0.9',
            paths: [
              { interface: 'eth0', flags: 'accept' },
              { interface: 'eth1', flags: 'forward' },
            ],
          },
        ],
      },
      MP,
    );
    expect(mc.status, mc.raw).toBe(200);
    expect((await h.call(admin, 'POST', '/api/v1/config/commit?comment=mc')).status).toBe(200);
  });
  afterAll(async () => {
    await h?.close();
  });

  it('rejects an EXCLUDE join (no sources) with a pointer', async () => {
    const bad = await h.call(
      admin,
      'PATCH',
      '/api/v1/config/routing/multicast',
      { igmp: { interfaces: { eth1: { mode: 'host', joins: [{ group: '239.9.9.9', sources: [] }] } } } },
      MP,
    );
    expect(bad.status).toBe(400);
    expect(JSON.stringify(bad.body)).toContain('sources');
  });

  it('reports live IGMP groups from the agent', async () => {
    const r = await h.call(admin, 'GET', '/api/v1/state/routing/multicast/groups');
    expect(r.status, r.raw).toBe(200);
    expect(r.body.agentError).toBeNull();
    expect(r.body.groups).toContainEqual({ interface: 'eth0', group: '239.1.1.1', sources: ['10.0.0.5'] });
  });

  it('reports the live mFIB with accept/forward', async () => {
    const r = await h.call(op, 'GET', '/api/v1/state/routing/multicast/mroutes');
    expect(r.status, r.raw).toBe(200);
    const m = r.body.mroutes.find((x: { group: string }) => x.group === '239.2.2.2');
    expect(m).toMatchObject({ source: '10.0.0.9', accept: 'eth0', forward: ['eth1'] });
  });

  it('serves pim-neighbors (empty from the fake agent)', async () => {
    const r = await h.call(admin, 'GET', '/api/v1/state/routing/multicast/pim-neighbors');
    expect(r.status, r.raw).toBe(200);
    expect(r.body.neighbors).toEqual([]);
  });
});
