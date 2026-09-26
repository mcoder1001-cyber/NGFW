import { sql } from 'drizzle-orm';
import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest';
import { ROOT_KEYS } from '@ngfw/schema';
import { hashPassword } from '../../src/auth/password.js';
import { runSecret, startHarness, type Harness } from '../support/harness.js';

/** Passwords of this run only — generated, never literal (gitleaks, 00-CONTEXT secrets rule). */
const PW = { op: runSecret(), ro: runSecret() };

/**
 * P06 flow on the host PostgreSQL with the fake agent: patch → diff → commit → state → revisions → rollback,
 * confirmed commit with and without confirm, validation failures with pointers, RBAC, lock, secrets, audit.
 * Interface names/addresses follow the slot rules (loop1xx, 10.1.0.0/16 for slot 1).
 */
describe('config e2e (PostgreSQL + fake agent)', () => {
  let h: Harness;
  let admin: string;
  let op: string;
  let ro: string;
  const IF = 'loop101';

  beforeAll(async () => {
    h = await startHarness({ VRX_LOCK_TTL_SEC: '3' });
    admin = await h.login('admin', h.adminPassword);
    await h.createUsers(admin, [
      { username: 'op1', role: 'operator', password: PW.op },
      { username: 'ro1', role: 'readonly', password: PW.ro },
    ]);
    op = await h.login('op1', PW.op);
    ro = await h.login('ro1', PW.ro);
  });
  afterAll(async () => h?.close());

  it('GET running is redacted: no password hash anywhere', async () => {
    const r = await h.call(admin, 'GET', '/api/v1/config');
    expect(r.status).toBe(200);
    expect(r.headers['x-vrx-revision']).toBe('1');
    expect(r.body.management.users.map((u: { username: string }) => u.username)).toEqual([
      'admin',
      'op1',
      'ro1',
    ]);
    expect(r.raw).not.toContain('passwordHash');
    expect(r.raw).not.toContain('$argon2id$');
    const rev = await h.call(admin, 'GET', '/api/v1/config/revisions/1');
    expect(rev.raw).not.toContain('$argon2id$');
  });

  it('patch → diff → commit → running + Retrieve reflect it', async () => {
    const p = await h.call(
      op,
      'PATCH',
      `/api/v1/config/interfaces/${IF}`,
      { ipv4: ['10.1.101.1/24'], enabled: true },
      {
        'content-type': 'application/merge-patch+json',
      },
    );
    expect(p.status).toBe(200);
    expect(p.body).toMatchObject({
      pointer: `/interfaces/${IF}`,
      after: { ipv4: ['10.1.101.1/24'] },
    });
    const d = await h.call(op, 'GET', '/api/v1/config/diff');
    expect(d.body.changes).toEqual([
      expect.objectContaining({
        op: 'add',
        pointer: `/interfaces/${IF}`,
        to: expect.objectContaining({ ipv4: ['10.1.101.1/24'] }),
      }),
    ]);
    const c = await h.call(op, 'POST', '/api/v1/config/commit?comment=loopback');
    expect(c.status).toBe(200);
    expect(c.body).toMatchObject({
      status: 'applied',
      revision: { id: 2, comment: 'loopback', author: 'op1' },
    });
    const run = await h.call(ro, 'GET', `/api/v1/config/interfaces/${IF}/ipv4`);
    expect(run.body).toEqual(['10.1.101.1/24']);
    const st = await h.call(ro, 'GET', '/api/v1/state/interfaces');
    expect(st.status).toBe(200);
    const item = st.body.items.find((i: { name: string }) => i.name === IF);
    expect(item.config.ipv4).toEqual(['10.1.101.1/24']);
    expect(item.counters).toMatchObject({ name: IF });
    const routes = await h.call(ro, 'GET', '/api/v1/state/routes?vrf=default&page=1&pageSize=10');
    expect(routes.body.items).toContainEqual(
      expect.objectContaining({ prefix: '10.1.101.0/24', origin: 'connected' }),
    );
    const drift = await h.call(ro, 'GET', '/api/v1/state/drift');
    expect(drift.body.changes).toEqual([]);
  });

  it('rollback creates a new revision with the old payload and reverts the agent', async () => {
    await h.call(op, 'PATCH', `/api/v1/config/interfaces/${IF}`, { ipv4: ['10.1.101.2/24'] });
    const c = await h.call(op, 'POST', '/api/v1/config/commit');
    expect(c.body.revision.id).toBe(3);
    expect((h.fake.current['interfaces'] as any)[IF].ipv4).toEqual(['10.1.101.2/24']);
    const rb = await h.call(op, 'POST', '/api/v1/config/rollback/2');
    expect(rb.status).toBe(200);
    expect(rb.body.revision).toMatchObject({ id: 4, kind: 'rollback', parentId: 3 });
    expect((h.fake.current['interfaces'] as any)[IF].ipv4).toEqual(['10.1.101.1/24']);
    const revs = await h.call(ro, 'GET', '/api/v1/config/revisions?limit=10');
    expect(revs.body.total).toBe(4);
    expect(revs.body.items.map((r: { id: number }) => r.id)).toEqual([4, 3, 2, 1]);
    const [r2, r4] = await Promise.all(
      [2, 4].map((id) => h.call(ro, 'GET', `/api/v1/config/revisions/${id}`)),
    );
    expect(r4!.body.payload).toEqual(r2!.body.payload);
    expect(r4!.body.hash).toBe(r2!.body.hash);
  });

  it('commit with ?confirm=2 and no confirm is reverted after the deadline', async () => {
    await h.call(op, 'PATCH', `/api/v1/config/interfaces/${IF}`, { ipv4: ['10.1.101.3/24'] });
    const c = await h.call(op, 'POST', '/api/v1/config/commit?confirm=2');
    expect(c.body.status).toBe('pending');
    expect((h.fake.current['interfaces'] as any)[IF].ipv4).toEqual(['10.1.101.3/24']);
    const pend = await h.call(ro, 'GET', '/api/v1/config/commit/pending');
    expect(pend.body.pending.txnId).toBe(c.body.txnId);
    await vi.waitFor(
      async () =>
        expect((await h.call(ro, 'GET', '/api/v1/config/commit/pending')).body.pending).toBeNull(),
      { timeout: 8000, interval: 250 },
    );
    expect((h.fake.current['interfaces'] as any)[IF].ipv4).toEqual(['10.1.101.1/24']);
    const run = await h.call(ro, 'GET', `/api/v1/config/interfaces/${IF}/ipv4`);
    expect(run.body).toEqual(['10.1.101.1/24']);
    const ev = await h.call(ro, 'GET', '/api/v1/state/events?limit=5');
    expect(ev.body.items.map((e: { code: string }) => e.code)).toContain('CONFIRM_REVERTED');
    // the candidate is kept; confirm this time
    const c2 = await h.call(op, 'POST', '/api/v1/config/commit?confirm=30');
    expect(c2.body.status).toBe('pending');
    const ok = await h.call(op, 'POST', '/api/v1/config/commit/confirm');
    expect(ok.body).toMatchObject({ status: 'confirmed', revision: { id: 5 } });
    expect((await h.call(ro, 'GET', `/api/v1/config/interfaces/${IF}/ipv4`)).body).toEqual([
      '10.1.101.3/24',
    ]);
  });

  it('overlapping IPs → 400 problem+json with the pointer', async () => {
    await h.call(op, 'PUT', '/api/v1/config/interfaces/loop102', { ipv4: ['10.1.101.200/25'] });
    const v = await h.call(op, 'POST', '/api/v1/config/validate');
    expect(v.status).toBe(400);
    const c = await h.call(op, 'POST', '/api/v1/config/commit');
    expect(c.status).toBe(400);
    expect(c.headers['content-type']).toMatch(/^application\/problem\+json/);
    expect(c.body).toMatchObject({
      status: 400,
      tier: 'semantic',
      type: 'https://vrx.dev/problems/validation',
    });
    expect(c.body.errors).toContainEqual(
      expect.objectContaining({
        pointer: '/interfaces/loop102/ipv4/0',
        message: expect.stringMatching(/overlap/),
      }),
    );
    expect((await h.call(op, 'POST', '/api/v1/config/discard')).body).toEqual({ discarded: true });
  });

  it('schema errors and bad pointers are 400/404 at edit time', async () => {
    const bad = await h.call(op, 'PATCH', `/api/v1/config/interfaces/${IF}`, { mtu: 1 });
    expect(bad.status).toBe(400);
    expect(bad.body.errors).toEqual([
      expect.objectContaining({ pointer: `/interfaces/${IF}/mtu` }),
    ]);
    const proto = await h.call(
      op,
      'PATCH',
      '/api/v1/config/system',
      '{"__proto__":{"polluted":true}}',
      {
        'content-type': 'application/merge-patch+json',
      },
    );
    expect(proto.status).toBe(400);
    expect(proto.body.errors[0].pointer).toBe('/system/__proto__');
    expect((await h.call(op, 'GET', '/api/v1/config/nosuchdomain')).status).toBe(404);
    expect((await h.call(op, 'DELETE', '/api/v1/config/interfaces/loop199')).status).toBe(404);
    await h.call(op, 'POST', '/api/v1/config/discard');
  });

  it('RBAC: readonly may read but not PATCH (403); operator may not touch users (403)', async () => {
    expect((await h.call(ro, 'GET', '/api/v1/config/candidate')).status).toBe(200);
    const p = await h.call(ro, 'PATCH', `/api/v1/config/interfaces/${IF}`, { mtu: 1500 });
    expect(p.status).toBe(403);
    expect(p.headers['content-type']).toMatch(/^application\/problem\+json/);
    expect((await h.call(ro, 'POST', '/api/v1/config/commit')).status).toBe(403);
    const u = await h.call(op, 'PATCH', '/api/v1/config/management', { users: [] });
    expect(u.status).toBe(403);
    expect((await h.call(op, 'GET', '/api/v1/audit')).status).toBe(403);
    expect((await h.call(op, 'DELETE', '/api/v1/config/lock')).status).toBe(403);
  });

  it('single writer: 409 with the owner; admin can break the lock', async () => {
    await h.call(op, 'PATCH', '/api/v1/config/system', { hostname: 'vrx-w1' });
    const other = await h.call(admin, 'PATCH', '/api/v1/config/system', { hostname: 'other' });
    expect(other.status).toBe(409);
    expect(other.body.lock).toMatchObject({ owner: 'op1', locked: true });
    expect((await h.call(admin, 'POST', '/api/v1/config/commit')).status).toBe(409);
    const broken = await h.call(admin, 'DELETE', '/api/v1/config/lock');
    expect(broken.body.owner).toBe('op1');
    expect((await h.call(ro, 'GET', '/api/v1/config/lock')).body.locked).toBe(false);
  });

  it('export → import → diff round trip', async () => {
    const ex = await h.call(ro, 'GET', '/api/v1/config/export');
    expect(ex.headers['content-disposition']).toContain('vrx-config.json');
    ex.body.system.hostname = 'imported-w1';
    const im = await h.call(admin, 'POST', '/api/v1/config/import', ex.body);
    expect(im.status).toBe(200);
    const d = await h.call(admin, 'GET', '/api/v1/config/diff');
    expect(d.body.changes).toEqual([
      { op: 'replace', pointer: '/system/hostname', from: 'vrx', to: 'imported-w1' },
    ]);
    // the redacted users came back without hashes: nobody lost a password (D-046)
    await h.call(admin, 'POST', '/api/v1/config/commit');
    await h.login('op1', PW.op);
  });

  it('secrets: value in, never out; referenced ones cannot be deleted', async () => {
    const value = `VRX_TEST_PSK_P06_${Date.now()}`;
    const s = await h.call(admin, 'POST', '/api/v1/secrets', {
      kind: 'psk',
      name: 'tacacs-w1',
      value,
    });
    expect(s.body).toEqual({ ref: 'psk/tacacs-w1', created: true, version: 1 });
    // review M2: secret writes are admin-only; an existing secret is never overwritten without ?replace=true
    expect(
      (await h.call(op, 'POST', '/api/v1/secrets', { kind: 'psk', name: 'op-psk', value })).status,
    ).toBe(403);
    expect(
      (await h.call(admin, 'POST', '/api/v1/secrets', { kind: 'psk', name: 'tacacs-w1', value }))
        .status,
    ).toBe(409);
    const list = await h.call(ro, 'GET', '/api/v1/secrets');
    expect(list.body).toEqual([
      expect.objectContaining({ ref: 'psk/tacacs-w1', kind: 'psk', name: 'tacacs-w1' }),
    ]);
    expect(list.raw).not.toContain(value);
    const rows = await h.db.execute(sql`select ciphertext from secret`);
    expect(JSON.stringify(rows.rows)).not.toContain(value);
    await h.call(admin, 'PATCH', '/api/v1/config/management/aaa', {
      tacacs: { servers: [{ address: '10.1.0.9', secretRef: 'psk/tacacs-w1' }] },
    });
    const c1 = await h.call(admin, 'POST', '/api/v1/config/commit');
    expect(c1.status).toBe(200);
    expect((await h.call(admin, 'DELETE', '/api/v1/secrets/psk/tacacs-w1')).status).toBe(409);
    // rotation = a new version; the revision keeps the version it was committed with (rollback-safe)
    const rot = await h.call(admin, 'POST', '/api/v1/secrets?replace=true', {
      kind: 'psk',
      name: 'tacacs-w1',
      value: value + '_2',
    });
    expect(rot.body).toEqual({ ref: 'psk/tacacs-w1', created: false, version: 2 });
    const pinned = await h.db.execute(
      sql`select secret_versions from config_revision where id = ${c1.body.revision.id}`,
    );
    expect(pinned.rows[0]).toEqual({ secret_versions: { 'psk/tacacs-w1': 1 } });
    const versions = await h.db.execute(
      sql`select version from secret_version where ref = 'psk/tacacs-w1' order by version`,
    );
    expect(versions.rows).toEqual([{ version: 1 }, { version: 2 }]);
    const ev = await h.call(ro, 'GET', '/api/v1/state/events?limit=20');
    expect(ev.body.items.map((e: { code: string }) => e.code)).toContain('SECRET_REPLACED');
    // review L2: a pending confirmed commit that references it blocks the delete as well
    await h.call(admin, 'PATCH', '/api/v1/config/management/aaa', { tacacs: { servers: [] } });
    await h.call(admin, 'POST', '/api/v1/config/commit');
    await h.call(admin, 'PATCH', '/api/v1/config/management/aaa', {
      tacacs: { servers: [{ address: '10.1.0.9', secretRef: 'psk/tacacs-w1' }] },
    });
    expect((await h.call(admin, 'POST', '/api/v1/config/commit?confirm=30')).body.status).toBe(
      'pending',
    );
    expect((await h.call(admin, 'DELETE', '/api/v1/secrets/psk/tacacs-w1')).status).toBe(409);
    expect((await h.call(admin, 'POST', '/api/v1/config/commit/confirm')).status).toBe(200);
    await h.call(admin, 'PATCH', '/api/v1/config/management/aaa', { tacacs: { servers: [] } });
    await h.call(admin, 'POST', '/api/v1/config/commit');
    expect((await h.call(admin, 'DELETE', '/api/v1/secrets/psk/tacacs-w1')).status).toBe(204);
    const audit = await h.db.execute(sql`select * from audit_log`);
    expect(JSON.stringify(audit.rows)).not.toContain(value);
  });

  it("review M1: the stale-lock takeover by an operator does not smuggle the admin's `evil` user into running", async () => {
    const evilPw = runSecret();
    const users = (await h.call(admin, 'GET', '/api/v1/config/management/users')).body;
    const hash = await hashPassword(evilPw);
    await h.call(admin, 'PUT', '/api/v1/config/management/users', [
      ...users,
      { username: 'evil', role: 'admin', passwordHash: hash },
    ]);
    expect(
      (await h.call(op, 'PATCH', '/api/v1/config/system', { hostname: 'op-edit' })).status,
    ).toBe(409);
    await new Promise((r) => setTimeout(r, 3300)); // VRX_LOCK_TTL_SEC=3 in this file
    const take = await h.call(op, 'PATCH', '/api/v1/config/system', { hostname: 'op-edit' });
    expect(take.status).toBe(200);
    expect(take.body.discardedStaleCandidateOf).toBe('admin');
    const d = await h.call(op, 'GET', '/api/v1/config/diff');
    expect(d.body.changes).toEqual([
      expect.objectContaining({ pointer: '/system/hostname', to: 'op-edit' }),
    ]);
    expect((await h.call(op, 'POST', '/api/v1/config/commit')).status).toBe(200);
    const running = await h.call(admin, 'GET', '/api/v1/config/management/users');
    expect(running.body.map((u: { username: string }) => u.username)).not.toContain('evil');
    expect(
      (
        await h.call(undefined, 'POST', '/api/v1/auth/login', {
          username: 'evil',
          password: evilPw,
        })
      ).status,
    ).toBe(401);
  });

  it('review M4: a commit that only touches domains the agent does not implement says not-applied', async () => {
    h.fake.implemented = ['interfaces', 'vrfs', 'routing'];
    try {
      await h.call(op, 'PATCH', '/api/v1/config/system', { hostname: 'not-enforced' });
      const c = await h.call(op, 'POST', '/api/v1/config/commit');
      expect(c.body).toMatchObject({
        status: 'not-applied',
        notApplied: ['system'],
        sync: { state: 'in-sync' },
      });
      const st = await h.call(ro, 'GET', '/api/v1/state/system');
      expect(st.body.sync.state).toBe('in-sync');
    } finally {
      h.fake.implemented = [...ROOT_KEYS];
    }
  });

  it('every mutation is audited with user, ip, route, before/after and result', async () => {
    const a = await h.call(admin, 'GET', '/api/v1/audit?limit=200');
    expect(a.status).toBe(200);
    const items = a.body.items as any[];
    const patch = items.find(
      (e) =>
        e.action === 'PATCH /api/v1/config/*' &&
        e.resource === `/interfaces/${IF}` &&
        e.username === 'op1' &&
        e.result === 'success',
    );
    expect(patch).toMatchObject({ sourceIp: '127.0.0.1', status: 200 });
    expect(patch.after).toMatchObject({ ipv4: expect.any(Array) });
    expect(items).toContainEqual(
      expect.objectContaining({
        action: 'POST /api/v1/config/commit',
        result: 'failure',
        status: 400,
      }),
    );
    expect(items).toContainEqual(
      expect.objectContaining({
        action: 'PATCH /api/v1/config/*',
        username: 'ro1',
        result: 'failure',
        status: 403,
      }),
    );
    expect(items).toContainEqual(
      expect.objectContaining({ action: 'POST /api/v1/config/rollback/:rev', result: 'success' }),
    );
    expect(items).toContainEqual(
      expect.objectContaining({ action: 'auth.login', username: 'op1', result: 'success' }),
    );
    const all = await h.db.execute(sql`select * from audit_log`);
    const text = JSON.stringify(all.rows);
    for (const secretText of ['$argon2id$', PW.op, h.adminPassword]) {
      expect(text).not.toContain(secretText);
    }
    // TD-2 #6: a changed hash is marked, never shown — every passwordHash in the audit log is a redaction marker
    const hashes = [...text.matchAll(/\\?"passwordHash\\?":\\?"([^"\\]*)/g)].map((m) => m[1]);
    expect(hashes.length).toBeGreaterThan(0);
    expect(hashes.every((v) => v === '<redacted>' || v === '<redacted:changed>')).toBe(true);
    // no GET is audited
    expect(items.some((e) => String(e.action).startsWith('GET '))).toBe(false);
  });

  it('actions answer 501; unknown ones 404', async () => {
    // ping runs since F-vrf-static-ecmp (the Action bridge, vrf-static-ecmp.e2e.test.ts); reboot has no agent action yet
    expect((await h.call(op, 'POST', '/api/v1/actions/reboot')).status).toBe(501);
    expect((await h.call(op, 'POST', '/api/v1/actions/format-disk')).status).toBe(404);
    expect((await h.call(ro, 'GET', '/api/v1/state/neighbors')).status).toBe(501);
  });

  it('agent down → 503 problem, running untouched', async () => {
    await h.call(op, 'PATCH', '/api/v1/config/system', { hostname: 'while-down' });
    h.fake.failAllWith = 14; // UNAVAILABLE
    const c = await h.call(op, 'POST', '/api/v1/config/commit');
    expect(c.status).toBe(503);
    expect(c.body.type).toBe('https://vrx.dev/problems/agent-unavailable');
    h.fake.failAllWith = undefined;
    expect((await h.call(op, 'POST', '/api/v1/config/commit')).status).toBe(200);
  });
});
