import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { startHarness, type Harness } from '../support/harness.js';

/**
 * F-mpls-ldp e2e: LDP config commits (with the semantic guards), and the state routes reflect it through the fake
 * agent's MplsLdpState. The FRR ldpd section and the FRR→VPP label sync are the host follow-up.
 */
const MP = { 'content-type': 'application/merge-patch+json' };

describe('F-mpls-ldp e2e (PostgreSQL + fake agent)', () => {
  let h: Harness;
  let admin: string;

  beforeAll(async () => {
    h = await startHarness({});
    admin = await h.login('admin', h.adminPassword);
    const ifs = await h.call(admin, 'PUT', '/api/v1/config/interfaces', {
      lo0: { enabled: true, ipv4: ['10.0.0.1/32'] },
      eth0: { enabled: true },
    });
    expect(ifs.status, ifs.raw).toBe(200);
    const mpls = await h.call(
      admin,
      'PATCH',
      '/api/v1/config/routing/mpls',
      {
        interfaces: ['eth0'],
        ldp: {
          routerId: '10.0.0.1',
          transportAddress: '10.0.0.1',
          interfaces: ['eth0'],
          neighbors: { '10.0.0.2': {} },
        },
      },
      MP,
    );
    expect(mpls.status, mpls.raw).toBe(200);
    expect((await h.call(admin, 'POST', '/api/v1/config/commit?comment=ldp')).status).toBe(200);
  });
  afterAll(async () => {
    await h?.close();
  });

  it('rejects an LDP interface that is not MPLS-enabled (validate → 400 with a pointer)', async () => {
    // lo0 exists but is not in routing.mpls.interfaces — a cross-object (semantic) failure, caught at validate/commit
    const patch = await h.call(
      admin,
      'PATCH',
      '/api/v1/config/routing/mpls',
      { ldp: { routerId: '10.0.0.1', transportAddress: '10.0.0.1', interfaces: ['eth0', 'lo0'] } },
      MP,
    );
    expect(patch.status, patch.raw).toBe(200);
    const v = await h.call(admin, 'POST', '/api/v1/config/validate');
    expect(v.status).toBe(400);
    expect(v.body.errors.some((e: { pointer: string }) => e.pointer.startsWith('/routing/mpls/ldp/interfaces'))).toBe(true);
    // restore the good candidate so later state reads are unaffected
    expect((await h.call(admin, 'POST', '/api/v1/config/discard')).status).toBe(200);
  });

  it('reports LDP neighbours from the agent', async () => {
    const r = await h.call(admin, 'GET', '/api/v1/state/routing/mpls/ldp/neighbors');
    expect(r.status, r.raw).toBe(200);
    expect(r.body.agentError).toBeNull();
    expect(r.body.neighbors).toContainEqual({ lsrId: '10.0.0.2', address: '10.0.0.2', state: 'OPERATIONAL', uptimeSec: 0 });
  });

  it('serves paged bindings and the sync status', async () => {
    const b = await h.call(admin, 'GET', '/api/v1/state/routing/mpls/ldp/bindings?page=1&pageSize=50');
    expect(b.status, b.raw).toBe(200);
    expect(b.body).toMatchObject({ total: 0, bindings: [] });
    const s = await h.call(admin, 'GET', '/api/v1/state/routing/mpls/ldp/sync');
    expect(s.status, s.raw).toBe(200);
    expect(s.body).toMatchObject({ agentError: null, installed: 0, conflicts: 0, source: 'ldp-bindings' });
  });
});
