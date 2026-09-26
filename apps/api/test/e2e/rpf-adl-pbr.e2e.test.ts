import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { runSecret, startHarness, type Harness } from '../support/harness.js';

const PW = runSecret();
const MERGE = { 'content-type': 'application/merge-patch+json' };

/**
 * F-rpf-adl-pbr through the API with the fake agent: uRPF/ADL on an interface and routing.pbr are edited through the
 * generic config pointer routes; a policy naming an unknown ACL, an unknown allow-list VRF and an attachment of an
 * unknown policy are 400 problem+json with pointers; the committed configuration shows up in GET /api/v1/state/pbr.
 * Names follow the slot rules (loop1xx, 10.1.0.0/16).
 */
describe('rpf-adl-pbr e2e (PostgreSQL + fake agent)', () => {
  let h: Harness;
  let op: string;

  beforeAll(async () => {
    h = await startHarness();
    const admin = await h.login('admin', h.adminPassword);
    await h.createUsers(admin, [{ username: 'op1', role: 'operator', password: PW }]);
    op = await h.login('op1', PW);
  });
  afterAll(async () => h?.close());

  it('a PBR policy naming an unknown ACL is a 400 problem+json with the pointer', async () => {
    await h.call(op, 'PUT', '/api/v1/config/interfaces/loop101', {
      enabled: true,
      ipv4: ['10.1.1.1/24'],
    });
    const put = await h.call(op, 'PUT', '/api/v1/config/routing/pbr', {
      policies: {
        'via-l2': {
          acl: 'lan-b',
          priority: 10,
          paths: [{ address: '10.1.1.254', interface: 'loop101' }],
        },
      },
      attachments: [
        { policy: 'via-l2', interface: 'loop101' },
        { policy: 'ghost', interface: 'loop101' },
      ],
    });
    expect(put.status).toBe(200);
    const c = await h.call(op, 'POST', '/api/v1/config/commit');
    expect(c.status).toBe(400);
    expect(c.headers['content-type']).toMatch(/^application\/problem\+json/);
    expect(c.body).toMatchObject({ status: 400, tier: 'semantic' });
    expect(c.body.errors).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          pointer: '/routing/pbr/policies/via-l2/acl',
          message: "ACL 'lan-b' does not exist",
        }),
        expect.objectContaining({
          pointer: '/routing/pbr/attachments/1/policy',
          message: "PBR policy 'ghost' does not exist",
        }),
      ]),
    );
    expect((await h.call(op, 'POST', '/api/v1/config/discard')).body).toEqual({ discarded: true });
  });

  it('schema and semantic errors of uRPF/ADL carry pointers at edit and commit time', async () => {
    const bad = await h.call(
      op,
      'PATCH',
      '/api/v1/config/interfaces/loop101',
      { urpf: { ipv4: 'feasible' } },
      MERGE,
    );
    expect(bad.status).toBe(400);
    expect(bad.body.errors).toContainEqual(
      expect.objectContaining({ pointer: '/interfaces/loop101/urpf/ipv4' }),
    );
    const noVrf = await h.call(
      op,
      'PATCH',
      '/api/v1/config/interfaces/loop101',
      { adl: { ipv4: true } },
      MERGE,
    );
    expect(noVrf.status).toBe(400);
    expect(noVrf.body.errors).toContainEqual(
      expect.objectContaining({ pointer: '/interfaces/loop101/adl/allowVrf' }),
    );
    await h.call(op, 'PUT', '/api/v1/config/interfaces/loop101', {
      adl: { ipv4: true, allowVrf: 'nope' },
    });
    const v = await h.call(op, 'POST', '/api/v1/config/validate');
    expect(v.status).toBe(400);
    expect(v.body.errors).toContainEqual(
      expect.objectContaining({
        pointer: '/interfaces/loop101/adl/allowVrf',
        message: "VRF 'nope' does not exist",
      }),
    );
    expect((await h.call(op, 'POST', '/api/v1/config/discard')).body).toEqual({ discarded: true });
  });

  it('commit → GET /api/v1/state/pbr shows the policies and attachments in sync', async () => {
    await h.call(op, 'PUT', '/api/v1/config/vrfs/allow', { id: 1002 });
    await h.call(op, 'PUT', '/api/v1/config/interfaces/loop101', {
      enabled: true,
      ipv4: ['10.1.1.1/24'],
      urpf: { ipv4: 'strict' },
      adl: { ipv4: true, allowVrf: 'allow' },
    });
    await h.call(op, 'PUT', '/api/v1/config/interfaces/loop102', {
      enabled: true,
      ipv4: ['10.1.2.1/24'],
    });
    await h.call(op, 'PUT', '/api/v1/config/acl/lists/lan-b', {
      rules: [
        {
          sequence: 10,
          action: 'permit',
          ipVersion: 'ipv4',
          source: { kind: 'prefix', prefix: '10.1.1.0/24' },
        },
      ],
    });
    await h.call(op, 'PUT', '/api/v1/config/routing/pbr', {
      policies: {
        'via-l2': {
          acl: 'lan-b',
          priority: 10,
          paths: [{ address: '10.1.2.254', interface: 'loop102' }],
        },
      },
      attachments: [{ policy: 'via-l2', interface: 'loop101' }],
    });
    await h.call(op, 'PUT', '/api/v1/config/services/autoSdl', { enabled: true });
    const c = await h.call(op, 'POST', '/api/v1/config/commit?comment=pbr');
    expect(c.status).toBe(200);
    const s = await h.call(op, 'GET', '/api/v1/state/pbr');
    expect(s.status).toBe(200);
    expect(s.body).toMatchObject({
      pendingChange: false,
      policies: [
        {
          name: 'via-l2',
          acl: 'lan-b',
          priority: 10,
          status: 'in-sync',
          attachments: 1,
          paths: [{ address: '10.1.2.254', interface: 'loop102', vrf: 'default', weight: 1 }],
        },
      ],
      attachments: {
        total: 1,
        items: [{ policy: 'via-l2', interface: 'loop101', family: 'ipv4', status: 'in-sync' }],
      },
      counters: { available: false },
    });
    const run = await h.call(op, 'GET', '/api/v1/config/interfaces/loop101');
    expect(run.body.urpf).toEqual({ ipv4: 'strict', direction: 'rx' });
    expect(run.body.adl).toEqual({
      ipv4: true,
      ipv6: false,
      allowVrf: 'allow',
      defaultAllow: true,
    });
    const sdl = await h.call(op, 'GET', '/api/v1/config/services/autoSdl');
    expect(sdl.body).toEqual({ enabled: true, threshold: 5, removeTimeoutSec: 300 });
    // a pending edit of routing.pbr is flagged
    await h.call(
      op,
      'PATCH',
      '/api/v1/config/routing/pbr/policies/via-l2',
      { priority: 20 },
      MERGE,
    );
    expect((await h.call(op, 'GET', '/api/v1/state/pbr')).body.pendingChange).toBe(true);
    expect((await h.call(op, 'GET', '/api/v1/state/pbr?pageSize=0')).status).toBe(400);
    await h.call(op, 'POST', '/api/v1/config/discard');
  });
});
