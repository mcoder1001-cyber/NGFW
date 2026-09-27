import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { startHarness, type Harness } from '../support/harness.js';

/**
 * F-pppoe-client API: interfaces.<name>.pppoe commits, the live PPPoE session shows on /state/interfaces, and
 * the reconnect action reaches the agent (404 for a non-PPPoE interface, accepted for a client).
 */
const MP = { 'content-type': 'application/merge-patch+json' };

describe('F-pppoe-client e2e (PostgreSQL + fake agent)', () => {
  let h: Harness;
  let admin: string;

  beforeAll(async () => {
    h = await startHarness({});
    admin = await h.login('admin', h.adminPassword);
    // the ISP password is a secret ref
    const s = await h.call(admin, 'POST', '/api/v1/secrets', {
      kind: 'password',
      name: 'isp',
      value: 'dialup-pw',
    });
    expect(s.status, s.raw).toBe(200);
  });
  afterAll(async () => h?.close());

  it('commits a PPPoE client and shows its live session on /state/interfaces', async () => {
    const patch = await h.call(
      admin,
      'PATCH',
      '/api/v1/config/interfaces',
      {
        wan0: {
          enabled: true,
          pppoe: { username: 'alice@isp', passwordRef: 'password/isp', dnsFromPeer: true },
        },
      },
      MP,
    );
    expect(patch.status, patch.raw).toBe(200);
    const c = await h.call(admin, 'POST', '/api/v1/config/commit?comment=pppoe');
    expect(c.status, c.raw).toBe(200);

    const st = await h.call(admin, 'GET', '/api/v1/state/interfaces');
    expect(st.status, st.raw).toBe(200);
    const wan = st.body.items.find((i: { name: string }) => i.name === 'wan0');
    expect(wan.state.pppoe).toMatchObject({
      phase: 'up',
      localIpv4: '203.0.113.5/32',
      peerIpv4: '203.0.113.1',
    });
    expect(wan.state.pppoe.dns).toEqual(['203.0.113.53', '203.0.113.54']);
    expect(wan.state.pppoe.since).toBe('2026-09-27T00:00:00.000Z');
  });

  it('reconnects a PPPoE client, and reports when there is none', async () => {
    const ok = await h.call(admin, 'POST', '/api/v1/actions/interfaces/wan0/pppoe/reconnect');
    expect(ok.status, ok.raw).toBe(200);
    expect(ok.body).toEqual({ accepted: true, message: 'redialling' });

    // an interface without pppoe: the agent says so (accepted=false)
    await h.call(admin, 'PATCH', '/api/v1/config/interfaces', { eth9: { enabled: true } }, MP);
    await h.call(admin, 'POST', '/api/v1/config/commit?comment=plain');
    const none = await h.call(admin, 'POST', '/api/v1/actions/interfaces/eth9/pppoe/reconnect');
    expect(none.status, none.raw).toBe(200);
    expect(none.body.accepted).toBe(false);
  });

  it('rejects a PPPoE client with a static address (semantic tier)', async () => {
    await h.call(
      admin,
      'PATCH',
      '/api/v1/config/interfaces',
      {
        wan1: {
          enabled: true,
          ipv4: ['192.0.2.9/24'],
          pppoe: { username: 'u', passwordRef: 'password/isp' },
        },
      },
      MP,
    );
    const bad = await h.call(admin, 'POST', '/api/v1/config/commit?comment=badpppoe');
    expect(bad.status, bad.raw).toBe(400);
    expect(JSON.stringify(bad.body.errors)).toContain('/interfaces/wan1/ipv4');
  });
});
