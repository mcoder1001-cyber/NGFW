import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { startHarness, type Harness } from '../support/harness.js';

/**
 * F-multiwan API: a WAN group commits, /state/wan reports live member health from the agent, and an unknown member
 * interface is rejected at commit (semantic tier).
 */
const MP = { 'content-type': 'application/merge-patch+json' };

describe('F-multiwan e2e (PostgreSQL + fake agent)', () => {
  let h: Harness;
  let admin: string;

  beforeAll(async () => {
    h = await startHarness({});
    admin = await h.login('admin', h.adminPassword);
  });
  afterAll(async () => h?.close());

  it('commits a WAN group and reports live member health', async () => {
    const patch = await h.call(
      admin,
      'PATCH',
      '/api/v1/config/interfaces',
      { wan0: { enabled: true }, wan1: { enabled: true } },
      MP,
    );
    expect(patch.status, patch.raw).toBe(200);
    const g = await h.call(
      admin,
      'PATCH',
      '/api/v1/config/routing',
      {
        wanGroups: [
          {
            name: 'internet',
            mode: 'failover',
            members: [
              { interface: 'wan0', nextHop: 'dhcp', priority: 10 },
              { interface: 'wan1', nextHop: 'dhcp', priority: 20 },
            ],
            monitors: [{ type: 'icmp', target: '1.1.1.1', downAfter: 3, upAfter: 3 }],
          },
        ],
      },
      MP,
    );
    expect(g.status, g.raw).toBe(200);
    const c = await h.call(admin, 'POST', '/api/v1/config/commit?comment=wan');
    expect(c.status, c.raw).toBe(200);

    const st = await h.call(admin, 'GET', '/api/v1/state/wan');
    expect(st.status, st.raw).toBe(200);
    expect(st.body.agentError).toBeNull();
    expect(st.body.groups).toHaveLength(1);
    const grp = st.body.groups[0];
    expect(grp).toMatchObject({ name: 'internet', mode: 'failover', active: 'wan0' });
    expect(grp.members).toEqual([
      expect.objectContaining({ interface: 'wan0', up: true, lossPct: 0 }),
      expect.objectContaining({ interface: 'wan1', up: false, lossPct: 100 }),
    ]);
  });

  it('rejects a WAN member on an interface that does not exist', async () => {
    await h.call(
      admin,
      'PATCH',
      '/api/v1/config/routing',
      {
        wanGroups: [
          {
            name: 'bad',
            mode: 'failover',
            members: [{ interface: 'ghost0', nextHop: 'dhcp' }],
            monitors: [{ type: 'icmp', target: '1.1.1.1' }],
          },
        ],
      },
      MP,
    );
    const bad = await h.call(admin, 'POST', '/api/v1/config/commit?comment=badwan');
    expect(bad.status, bad.raw).toBe(400);
    expect(JSON.stringify(bad.body.errors)).toContain('/routing/wanGroups/');
  });
});
