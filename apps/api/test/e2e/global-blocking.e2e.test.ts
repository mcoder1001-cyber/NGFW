import http from 'node:http';
import type { AddressInfo } from 'node:net';
import { sql } from 'drizzle-orm';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { setFakeAclCounters, setFakeAclHits } from '../../src/features/acl/fake.js';
import { GlobalBlockingService } from '../../src/features/global-blocking/global-blocking.service.js';
import { runSecret, startHarness, type Harness } from '../support/harness.js';

/**
 * F-global-blocking API: upload preview (invalid line numbers) → stage → commit; export; status with hit counters;
 * a URL source fetched now and refreshed on schedule (the diff applied as a system revision); failures (HTTP error,
 * garbage, empty) keep the last good list and raise BLOCKLIST_FETCH_FAILED; a candidate being edited defers it.
 */
const MP = { 'content-type': 'application/merge-patch+json' };
const TXT = { 'content-type': 'text/plain' };

describe('F-global-blocking e2e (PostgreSQL + fake agent)', () => {
  let h: Harness;
  let op: string;
  const pw = runSecret();
  let server: http.Server;
  let url: string;
  let file = { status: 200, body: '192.0.2.7\n198.51.100.0/24\n' };

  beforeAll(async () => {
    h = await startHarness({});
    const admin = await h.login('admin', h.adminPassword);
    await h.createUsers(admin, [{ username: 'op1', role: 'operator', password: pw }]);
    op = await h.login('op1', pw);
    server = http.createServer((_req, res) => res.writeHead(file.status).end(file.body));
    await new Promise<void>((r) => server.listen(0, '127.0.0.1', r));
    url = `http://127.0.0.1:${(server.address() as AddressInfo).port}/list.txt`;
  });
  afterAll(async () => {
    server?.close();
    await h?.close();
  });

  const commit = async (comment: string) => {
    const c = await h.call(op, 'POST', `/api/v1/config/commit?comment=${comment}`);
    expect(c.status, c.raw).toBe(200);
    return c;
  };
  const running = async () =>
    (await h.call(op, 'GET', '/api/v1/config')).body.acl.globalBlocking.lists;

  it('previews an upload with invalid line numbers, stages it, commits, exports and counts hits', async () => {
    const lo = await h.call(
      op,
      'PATCH',
      '/api/v1/config/interfaces',
      { loop3101: { enabled: true } },
      MP,
    );
    expect(lo.status, lo.raw).toBe(200);
    const p0 = await h.call(
      op,
      'PATCH',
      '/api/v1/config/acl',
      {
        globalBlocking: { lists: { bad: { interfaces: ['loop3101'], entries: [] } } },
      },
      MP,
    );
    expect(p0.status, p0.raw).toBe(200);

    const text = '# threat feed\n192.0.2.7\n10.1.2.3/8\nnot-an-ip\n\n2001:DB8::1\n192.0.2.7/32\n';
    const pre = await h.call(
      op,
      'POST',
      '/api/v1/security/global-blocking/lists/bad/import',
      text,
      TXT,
    );
    expect(pre.status, pre.raw).toBe(200);
    expect(pre.body).toMatchObject({
      dryRun: true,
      staged: false,
      lines: 7,
      entries: 3,
      added: 3,
      removed: 0,
      invalidCount: 1,
      normalised: 1,
      collapsed: 1,
    });
    expect(pre.body.invalid).toEqual([expect.objectContaining({ line: 4, text: 'not-an-ip' })]);
    const cand0 = await h.call(
      op,
      'GET',
      '/api/v1/config/candidate/acl/globalBlocking/lists/bad/entries',
    );
    expect(cand0.body).toEqual([]); // preview changed nothing

    const st = await h.call(
      op,
      'POST',
      '/api/v1/security/global-blocking/lists/bad/import?dryRun=false',
      text,
      TXT,
    );
    expect(st.body).toMatchObject({ staged: true, entries: 3 });
    await commit('import');
    expect((await running()).bad.entries).toEqual([
      '10.0.0.0/8',
      '192.0.2.7/32',
      '2001:db8::1/128',
    ]);

    const ex = await h.call(op, 'GET', '/api/v1/security/global-blocking/lists/bad/export');
    expect(ex.raw).toBe(
      '# ngfw block list bad (running, 3 entries)\n10.0.0.0/8\n192.0.2.7/32\n2001:db8::1/128\n',
    );

    setFakeAclCounters(h.fake, true);
    setFakeAclHits(h.fake, '_gb.bad.i00', 0, 42);
    const s = await h.call(op, 'GET', '/api/v1/security/global-blocking');
    expect(s.status, s.raw).toBe(200);
    expect(s.body.lists).toEqual([
      expect.objectContaining({
        name: 'bad',
        entries: 3,
        runningEntries: 3,
        pending: null,
        source: { kind: 'upload', url: null, refreshSec: null },
        hits: expect.objectContaining({ dataplanePackets: 42 }),
      }),
    ]);

    // the ACL attachments view does not report the block lists' ACLs as out of sync
    const att = await h.call(op, 'GET', '/api/v1/state/acl/attachments');
    expect(att.status).toBe(200);
  });

  it('a list with credentials: its URL is admin-only; a fetched file is not echoed back', async () => {
    const admin = await h.login('admin', h.adminPassword);
    const tok = await h.call(admin, 'POST', '/api/v1/secrets', {
      kind: 'token',
      name: 'feed',
      value: 'feed-secret',
    });
    expect(tok.status, tok.raw).toBe(200);
    const add = await h.call(
      admin,
      'PATCH',
      '/api/v1/config/acl',
      {
        globalBlocking: {
          lists: {
            cred: { allInterfaces: true, source: { kind: 'url', url, authRef: 'token/feed' } },
          },
        },
      },
      MP,
    );
    expect(add.status, add.raw).toBe(200);
    file = {
      status: 200,
      body: `192.0.2.7\nsecret internal page\n${Array.from({ length: 12 }, (_, i) => `198.51.100.${i}`).join('\n')}\n`,
    };
    const f = await h.call(admin, 'POST', '/api/v1/security/global-blocking/lists/cred/fetch');
    expect(f.status, f.raw).toBe(200);
    expect(f.body.invalid).toEqual([{ line: 2, text: '', reason: expect.any(String) }]);
    const c = await h.call(admin, 'POST', '/api/v1/config/commit?comment=cred');
    expect(c.status, c.raw).toBe(200);

    // an operator may not re-point it (the credential would go to the new server)
    const moved = await h.call(
      op,
      'PATCH',
      '/api/v1/config/acl',
      {
        globalBlocking: { lists: { cred: { source: { url: 'https://collector.example.net/' } } } },
      },
      MP,
    );
    expect(moved.status, moved.raw).toBe(403);
    expect(moved.raw).toContain('/acl/globalBlocking/lists/cred/source/url');
    const insecure = await h.call(
      op,
      'PATCH',
      '/api/v1/config/acl',
      {
        globalBlocking: { lists: { cred: { source: { verifyTls: false } } } },
      },
      MP,
    );
    expect(insecure.status, insecure.raw).toBe(403);
    // … but may edit anything else of it
    const desc = await h.call(
      op,
      'PATCH',
      '/api/v1/config/acl',
      { globalBlocking: { lists: { cred: { description: 'ok' } } } },
      MP,
    );
    expect(desc.status, desc.raw).toBe(200);
    expect((await h.call(op, 'POST', '/api/v1/config/discard')).status).toBe(200);
    const del = await h.call(
      admin,
      'PATCH',
      '/api/v1/config/acl',
      { globalBlocking: { lists: { cred: null } } },
      MP,
    );
    expect(del.status, del.raw).toBe(200);
    expect((await h.call(admin, 'POST', '/api/v1/config/commit?comment=nocred')).status).toBe(200);
  });

  it('fetches a URL now, refreshes it on schedule as a system change and keeps the last good list on failures', async () => {
    file = { status: 200, body: '192.0.2.7\n198.51.100.0/24\n' };
    const p = await h.call(
      op,
      'PATCH',
      '/api/v1/config/acl',
      {
        globalBlocking: {
          lists: { feed: { allInterfaces: true, source: { kind: 'url', url, refreshSec: 300 } } },
        },
      },
      MP,
    );
    expect(p.status, p.raw).toBe(200);
    const f = await h.call(
      op,
      'POST',
      '/api/v1/security/global-blocking/lists/feed/fetch?dryRun=false',
    );
    expect(f.status, f.raw).toBe(200);
    expect(f.body).toMatchObject({ staged: true, entries: 2 });
    await commit('feed');

    const svc = h.app.get(GlobalBlockingService);
    let clock = Date.now();
    svc.now = () => new Date(clock);
    expect(await svc.refreshDue()).toEqual([]); // fetched just now: not due

    clock += 301_000;
    file = { status: 200, body: '192.0.2.7\n203.0.113.9\n' };
    expect(await svc.refreshDue()).toEqual([
      { list: 'feed', result: 'ok', detail: '1 added, 1 removed' },
    ]);
    expect((await running()).feed.entries).toEqual(['192.0.2.7/32', '203.0.113.9/32']);
    const rev = await h.db.execute(
      sql`select kind, author_id, comment from config_revision order by id desc limit 1`,
    );
    expect(rev.rows[0]).toMatchObject({ kind: 'system', author_id: null });

    for (const bad of [
      { status: 500, body: 'oops' },
      { status: 200, body: 'garbage\nmore garbage\n192.0.2.1\n' },
      { status: 200, body: '' },
    ]) {
      clock += 301_000;
      file = bad;
      const [r] = await svc.refreshDue();
      expect(r).toMatchObject({ list: 'feed', result: 'failed' });
      expect((await running()).feed.entries).toEqual(['192.0.2.7/32', '203.0.113.9/32']);
    }
    const ev = await h.db.execute(
      sql`select code from system_event where subsystem = 'global-blocking' order by id`,
    );
    const codes = (ev.rows as { code: string }[]).map((r) => r.code);
    expect(codes.filter((c) => c === 'BLOCKLIST_FETCH_FAILED')).toHaveLength(3);
    expect(codes).toContain('BLOCKLIST_REFRESHED');

    // an operator editing the candidate defers the refresh; nothing is committed under them
    clock += 301_000;
    file = { status: 200, body: '192.0.2.8\n' };
    const edit = await h.call(
      op,
      'PATCH',
      '/api/v1/config/acl',
      { globalBlocking: { lists: { feed: { description: 'x' } } } },
      MP,
    );
    expect(edit.status).toBe(200);
    expect(await svc.refreshDue()).toEqual([
      { list: 'feed', result: 'deferred', detail: 'the candidate has uncommitted changes' },
    ]);
    expect((await running()).feed.entries).toEqual(['192.0.2.7/32', '203.0.113.9/32']);
    const status = await h.call(op, 'GET', '/api/v1/security/global-blocking');
    expect(status.body.lists.find((l: { name: string }) => l.name === 'feed').fetch).toMatchObject({
      lastResult: 'deferred',
    });
  });
});
