import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { runSecret, startHarness, type Harness } from '../support/harness.js';

const PW = { ro: runSecret() };

/**
 * F-vlan-qinq: QinQ sub-interfaces through the generic config routes and `GET /api/v1/state/interfaces` on the host
 * PostgreSQL with the fake agent — one row per sub-interface with `parent`, the tag stack (`vlanId`, `innerVlanId`,
 * `dot1ad`) in `config` (Retrieve view) and `running`, the live tag stack in `state`; a duplicate tag stack is a 400
 * problem+json with the pointer to the second entry; a rollback removes the rows. Slot names: host-w5w0, 10.5.0.0/16.
 */
describe('state/interfaces QinQ e2e (PostgreSQL + fake agent)', () => {
  let h: Harness;
  let admin: string;
  let ro: string;
  const W = 'host-w5w0';
  const SUBS = `/api/v1/config/interfaces/${W}/subinterfaces`;
  const mp = { 'content-type': 'application/merge-patch+json' };
  let parentOnly = 0;

  const items = async () => {
    const st = await h.call(ro, 'GET', '/api/v1/state/interfaces');
    expect(st.status).toBe(200);
    return new Map(
      (st.body.items as { name: string }[]).map((i) => [i.name, i as Record<string, unknown>]),
    );
  };

  beforeAll(async () => {
    h = await startHarness({});
    admin = await h.login('admin', h.adminPassword);
    await h.createUsers(admin, [{ username: 'ro1', role: 'readonly', password: PW.ro }]);
    ro = await h.login('ro1', PW.ro);
    const p = await h.call(
      admin,
      'PATCH',
      '/api/v1/config/interfaces',
      { [W]: { enabled: true, ipv4: ['10.5.2.1/24'] } },
      mp,
    );
    expect(p.status).toBe(200);
    const c = await h.call(admin, 'POST', '/api/v1/config/commit?comment=qinq-parent');
    expect(c.status).toBe(200);
    parentOnly = c.body.revision.id as number;
  });
  afterAll(async () => h?.close());

  it('commits dot1q 100 and dot1ad 200 + dot1q 100 and lists both with parent, vlanId, innerVlanId and dot1ad', async () => {
    expect(
      (
        await h.call(
          admin,
          'PATCH',
          `/api/v1/config/interfaces/${W}`,
          {
            subinterfaces: {
              '100': { vlanId: 100, enabled: true, ipv4: ['10.5.100.1/24'] },
              '200': {
                vlanId: 200,
                innerVlanId: 100,
                dot1ad: true,
                enabled: true,
                ipv4: ['10.5.200.1/24'],
              },
            },
          },
          mp,
        )
      ).status,
    ).toBe(200);
    const diff = await h.call(admin, 'GET', '/api/v1/config/diff');
    expect(diff.body.changes).toContainEqual(
      expect.objectContaining({
        op: 'add',
        pointer: `/interfaces/${W}/subinterfaces/200`,
        to: expect.objectContaining({ vlanId: 200, innerVlanId: 100, dot1ad: true }),
      }),
    );
    expect((await h.call(admin, 'POST', '/api/v1/config/commit?comment=qinq')).status).toBe(200);

    // The fake agent's InterfaceState reports innerVlanId 0 for every sub-interface (questions Q2); the row below is
    // what the real agent sends for the QinQ sub-interface (TestQinQRoundTripOnFake). The API takes the last live row
    // of a name, so this one replaces the fake's own.
    h.fake.liveExtra = [
      {
        name: `${W}.200`,
        vppName: `${W}.200`,
        type: 'sub-interface',
        parent: W,
        vlanId: 200,
        innerVlanId: 100,
        managed: true,
        adminUp: true,
        linkUp: true,
        ipv4: ['10.5.200.1/24'],
      },
    ];
    try {
      const by = await items();
      expect([...by.keys()]).toEqual([W, `${W}.100`, `${W}.200`]);
      expect(by.get(`${W}.100`)).toMatchObject({
        kind: 'subinterface',
        parent: W,
        state: { type: 'sub-interface', parent: W, vlanId: 100, innerVlanId: 0 },
        config: { vlanId: 100, ipv4: ['10.5.100.1/24'] },
        running: { vlanId: 100, enabled: true, ipv4: ['10.5.100.1/24'] },
        hasPendingChange: false,
      });
      expect(by.get(`${W}.100`)!['running']).not.toHaveProperty('innerVlanId');
      expect(by.get(`${W}.200`)).toMatchObject({
        kind: 'subinterface',
        parent: W,
        state: { type: 'sub-interface', parent: W, vlanId: 200, innerVlanId: 100, managed: true },
        config: { vlanId: 200, innerVlanId: 100, dot1ad: true, ipv4: ['10.5.200.1/24'] },
        running: { vlanId: 200, innerVlanId: 100, dot1ad: true, enabled: true },
        hasPendingChange: false,
      });
      // the parent row carries the tag stacks of its sub-interfaces too
      expect(by.get(W)).toMatchObject({
        kind: 'interface',
        parent: null,
        running: { subinterfaces: { '200': { vlanId: 200, innerVlanId: 100, dot1ad: true } } },
      });
    } finally {
      h.fake.liveExtra = [];
    }
  });

  it('a duplicate (dot1ad, vlanId, innerVlanId) on one parent is a 400 problem+json with the pointer to the second entry', async () => {
    const put = await h.call(admin, 'PUT', `${SUBS}/201`, {
      vlanId: 200,
      innerVlanId: 100,
      dot1ad: true,
      ipv4: ['10.5.201.1/24'],
    });
    expect(put.status).toBe(200); // schema-valid: the cross-entry rule runs at validate/commit (tier b)
    for (const path of ['/api/v1/config/validate', '/api/v1/config/commit']) {
      const r = await h.call(admin, 'POST', path);
      expect(r.status).toBe(400);
      expect(r.headers['content-type']).toMatch(/^application\/problem\+json/);
      expect(r.body).toMatchObject({
        status: 400,
        tier: 'semantic',
        type: 'https://vrx.dev/problems/validation',
      });
      expect(r.body.errors).toContainEqual(
        expect.objectContaining({
          pointer: `/interfaces/${W}/subinterfaces/201/vlanId`,
          message: `VLAN dot1ad 200.100 is already used by sub-interface ${W}.200`,
        }),
      );
    }
    // the same numbers as 802.1Q (dot1ad false) are a different stack: valid
    const patch = await h.call(admin, 'PATCH', `${SUBS}/201`, { dot1ad: false }, mp);
    expect(patch.status).toBe(200);
    expect((await h.call(admin, 'POST', '/api/v1/config/validate')).status).toBe(200);
    expect((await h.call(admin, 'POST', '/api/v1/config/discard')).body).toEqual({
      discarded: true,
    });
  });

  it('an inner tag without the outer tag, or out of range, is refused at edit time with its pointer', async () => {
    const noOuter = await h.call(admin, 'PUT', `${SUBS}/202`, { innerVlanId: 100, dot1ad: true });
    expect(noOuter.status).toBe(400);
    expect(noOuter.body.errors).toContainEqual(
      expect.objectContaining({ pointer: `/interfaces/${W}/subinterfaces/202/vlanId` }),
    );
    const range = await h.call(admin, 'PUT', `${SUBS}/203`, { vlanId: 203, innerVlanId: 4095 });
    expect(range.status).toBe(400);
    expect(range.body.errors).toContainEqual(
      expect.objectContaining({ pointer: `/interfaces/${W}/subinterfaces/203/innerVlanId` }),
    );
    expect((await h.call(admin, 'GET', `${SUBS}/202`)).status).toBe(404);
  });

  it('a rollback to the revision without sub-interfaces removes both rows; the parent stays', async () => {
    const rb = await h.call(
      admin,
      'POST',
      `/api/v1/config/rollback/${parentOnly}?comment=qinq-undo`,
    );
    expect(rb.status).toBe(200);
    expect(rb.body.status).toBe('applied');
    const by = await items();
    expect([...by.keys()]).toEqual([W]);
    expect(by.get(W)).toMatchObject({ running: { subinterfaces: {} }, hasPendingChange: false });
  });
});
