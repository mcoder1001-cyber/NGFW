import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { startHarness, type Harness } from '../support/harness.js';
describe('BFD redistribution (PostgreSQL and fake agent)', () => {
  let h: Harness;
  let admin: string;
  beforeAll(async () => {
    h = await startHarness({});
    admin = await h.login('admin', h.adminPassword);
  });
  afterAll(async () => h?.close());
  it('commits standalone sessions and exposes observed fake Down state', async () => {
    expect(
      (await h.call(admin, 'PUT', '/api/v1/config/interfaces/loop1401', { ipv4: ['10.14.1.1/24'] }))
        .status,
    ).toBe(200);
    expect(
      (
        await h.call(admin, 'PUT', '/api/v1/config/routing/bfd', {
          sessions: [
            { interface: 'loop1401', localAddress: '10.14.1.1', peerAddress: '10.14.1.2' },
          ],
        })
      ).status,
    ).toBe(200);
    expect((await h.call(admin, 'POST', '/api/v1/config/commit?comment=bfd')).status).toBe(200);
    const live = await h.call(admin, 'GET', '/api/v1/state/routing/bfd/sessions');
    expect(live.status).toBe(200);
    expect(live.body).toMatchObject({
      sessions: [{ engine: 'vpp', state: 'down', peerAddress: '10.14.1.2' }],
    });
    expect((await h.call(admin, 'GET', '/api/v1/state/routing/redistribution')).body).toMatchObject(
      { edges: [] },
    );
  });
  it('rejects different-peer FRR BFD on the same AF with a protocol pointer', async () => {
    expect(
      (
        await h.call(admin, 'PUT', '/api/v1/config/routing/bgp', {
          asn: 65000,
          neighbors: { '10.14.2.2': { remoteAs: 65001, bfd: true } },
        })
      ).status,
    ).toBe(200);
    const r = await h.call(admin, 'POST', '/api/v1/config/commit');
    expect(r.status).toBe(400);
    expect(String(r.headers['content-type'])).toContain('application/problem+json');
    expect((r.body.errors as { pointer: string }[]).map((e) => e.pointer)).toContain(
      '/routing/bgp/neighbors/10.14.2.2/bfd',
    );
  });
});
