import { sql } from 'drizzle-orm';
import { afterAll, beforeAll, beforeEach, describe, expect, it } from 'vitest';
import { fakePcap, resetCaptureFake } from '../../src/features/capture-trace/fake.js';
import { runSecret, startHarness, type Harness } from '../support/harness.js';

// Real authentication, audit, HTTP routing and gRPC; only the dataplane is fake.
describe('capture HTTP lifecycle', () => {
  let h: Harness;
  let admin: string;
  let viewer: string;
  const body = { interface: 'any', seconds: 1, maxPackets: 10, snaplen: 128 };
  beforeAll(async () => {
    h = await startHarness();
    admin = await h.login('admin', h.adminPassword);
    const password = runSecret();
    await h.createUsers(admin, [{ username: 'captureviewer', role: 'readonly', password }]);
    viewer = await h.login('captureviewer', password);
  });
  beforeEach(() => resetCaptureFake());
  afterAll(async () => {
    resetCaptureFake();
    await h?.close();
  });

  it('downloads exact bytes, audits, deletes and returns 404 afterwards', async () => {
    const started = await h.call(admin, 'POST', '/api/v1/actions/capture', body);
    expect(started.status, started.raw).toBe(202);
    const id = started.body.id as string;
    const list = await h.call(viewer, 'GET', '/api/v1/state/captures');
    expect(list.status, list.raw).toBe(200);
    expect(list.body.captures).toEqual([
      expect.objectContaining({ id, state: 'done', packets: '1' }),
    ]);
    expect(list.body.trace.available).toBe(false);
    expect(list.body.trace.reason).not.toBe('');
    expect(list.body.pg.available).toBe(false);
    const file = await h.app.inject({
      method: 'GET',
      url: `/api/v1/state/captures/${id}/file`,
      headers: { authorization: `Bearer ${admin}` },
    });
    expect(file.statusCode).toBe(200);
    expect(file.rawPayload).toEqual(fakePcap());
    expect(file.headers['content-type']).toContain('application/vnd.tcpdump.pcap');
    const audit = await h.db.execute(
      sql`select result, status, after from audit_log where resource = ${`captures/${id}`} and action like 'GET %'`,
    );
    expect(audit.rows).toEqual([
      { result: 'success', status: 200, after: { bytes: fakePcap().length } },
    ]);
    expect((await h.call(admin, 'DELETE', `/api/v1/state/captures/${id}`)).status).toBe(204);
    expect((await h.call(admin, 'GET', '/api/v1/state/captures')).body.captures).toEqual([]);
    expect((await h.call(admin, 'GET', `/api/v1/state/captures/${id}/file`)).status).toBe(404);
    expect((await h.call(admin, 'DELETE', `/api/v1/state/captures/${id}`)).status).toBe(404);
  });

  it('requires authentication and admin for start, download, stop and delete', async () => {
    expect((await h.call(undefined, 'POST', '/api/v1/actions/capture', body)).status).toBe(401);
    expect((await h.call(viewer, 'POST', '/api/v1/actions/capture', body)).status).toBe(403);
    const started = await h.call(admin, 'POST', '/api/v1/actions/capture', body);
    const id = started.body.id as string;
    expect((await h.call(viewer, 'GET', `/api/v1/state/captures/${id}/file`)).status).toBe(403);
    expect((await h.call(viewer, 'DELETE', `/api/v1/state/captures/${id}`)).status).toBe(403);
    expect((await h.call(viewer, 'POST', `/api/v1/actions/capture/${id}/stop`)).status).toBe(403);
  });

  it('returns /bpf validation and capture-busy problems and refuses active deletion', async () => {
    const invalid = await h.call(admin, 'POST', '/api/v1/actions/capture', {
      ...body,
      bpf: 'ip; echo bad',
    });
    expect(invalid.status, invalid.raw).toBe(400);
    expect(invalid.headers['content-type']).toContain('application/problem+json');
    expect(JSON.stringify(invalid.body)).toContain('/bpf');
    resetCaptureFake(true);
    const first = await h.call(admin, 'POST', '/api/v1/actions/capture', body);
    expect(first.status, first.raw).toBe(202);
    const busy = await h.call(admin, 'POST', '/api/v1/actions/capture', body);
    expect(busy.status, busy.raw).toBe(409);
    expect(busy.body.type).toMatch(/capture-busy$/);
    expect((await h.call(admin, 'DELETE', `/api/v1/state/captures/${first.body.id}`)).status).toBe(
      409,
    );
  });
});
