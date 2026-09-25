import { readFileSync } from 'node:fs';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { startHarness, type Harness } from '../support/harness.js';

const FULL = JSON.parse(
  readFileSync(
    new URL('../../../../packages/proto/test/fixtures/lisp-full.json', import.meta.url),
    'utf8',
  ),
) as Record<string, Record<string, unknown>>;

/**
 * F-lisp over HTTP on the host PostgreSQL with the fake agent: `tunnels.lisp` through the generic pointer routes,
 * commit, `GET /api/v1/state/lisp`, and the duplicate-EID 400 problem+json with its pointer.
 */
describe('F-lisp e2e (PostgreSQL + fake agent)', () => {
  let h: Harness;
  let admin: string;
  const mp = { 'content-type': 'application/merge-patch+json' };

  beforeAll(async () => {
    h = await startHarness({});
    admin = await h.login('admin', h.adminPassword);
  });
  afterAll(async () => h?.close());

  it('commits tunnels.lisp and reports the state', async () => {
    expect((await h.call(admin, 'PATCH', '/api/v1/config/vrfs', FULL['vrfs'], mp)).status).toBe(
      200,
    );
    expect(
      (await h.call(admin, 'PATCH', '/api/v1/config/interfaces', FULL['interfaces'], mp)).status,
    ).toBe(200);
    expect(
      (await h.call(admin, 'PUT', '/api/v1/config/tunnels/lisp', FULL['tunnels']!['lisp'])).status,
    ).toBe(200);
    expect((await h.call(admin, 'POST', '/api/v1/config/commit?comment=lisp')).status).toBe(200);
    const st = await h.call(admin, 'GET', '/api/v1/state/lisp');
    expect(st.status).toBe(200);
    expect(st.body).toMatchObject({ enabled: true, pitr: 'w11-rloc', gpeVnis: [1101] });
  });

  it('duplicate EID in one VNI → 400 problem+json with a pointer', async () => {
    const eids = [
      { vni: 1100, eid: '10.11.100.0/24', locatorSet: 'w11-rloc' },
      { vni: 1100, eid: '10.11.100.0/24', locatorSet: 'w11-rloc' },
    ];
    expect((await h.call(admin, 'PUT', '/api/v1/config/tunnels/lisp/localEids', eids)).status).toBe(
      200,
    );
    const r = await h.call(admin, 'POST', '/api/v1/config/commit');
    expect(r.status).toBe(400);
    expect(String(r.headers['content-type'])).toContain('application/problem+json');
    expect((r.body.errors as { pointer: string }[]).map((e) => e.pointer)).toContain(
      '/tunnels/lisp/localEids/1/eid',
    );
  });
});
