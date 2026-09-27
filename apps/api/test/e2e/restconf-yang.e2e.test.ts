import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { startHarness, type Harness } from '../support/harness.js';

/**
 * F-restconf-yang e2e: the RESTCONF layer maps onto the candidate/commit engine — GET running, PATCH + commit, error
 * bodies (RFC 8040 §7), secret redaction, and rollback.
 */
const YANG = { 'content-type': 'application/yang-data+json' };

describe('F-restconf-yang e2e (PostgreSQL + fake agent)', () => {
  let h: Harness;
  let admin: string;

  beforeAll(async () => {
    h = await startHarness({});
    admin = await h.login('admin', h.adminPassword);
    // a user with a password hash, to prove secret leaves are never returned by RESTCONF
    await h.createUsers(admin, [{ username: 'bob', role: 'operator', password: 'Bob-pw-1234567890' }]);
  });
  afterAll(async () => {
    await h?.close();
  });

  it('serves the API resource and the YANG library', async () => {
    const root = await h.call(admin, 'GET', '/restconf');
    expect(root.status, root.raw).toBe(200);
    expect(root.body).toHaveProperty('ietf-restconf:restconf');

    const lib = await h.call(admin, 'GET', '/restconf/data/ietf-yang-library:yang-library');
    expect(lib.status, lib.raw).toBe(200);
    const modules = lib.body['ietf-yang-library:yang-library']['module-set'][0].module;
    expect(modules.some((m: { name: string }) => m.name === 'vrx-interfaces')).toBe(true);
  });

  it('GET returns the running config, module-qualified', async () => {
    const r = await h.call(admin, 'GET', '/restconf/data/vrx-system:system');
    expect(r.status, r.raw).toBe(200);
    expect(r.body).toHaveProperty('vrx-system:system');
    expect(String(r.headers['content-type'])).toContain('application/yang-data+json');
  });

  it('PATCH + operations/vrx:commit changes the running config and makes a new revision', async () => {
    const before = await h.call(admin, 'GET', '/api/v1/config/revisions?limit=1&offset=0');
    const beforeRev = before.body.items[0]?.id ?? 0;

    const patch = await h.call(
      admin,
      'PATCH',
      '/restconf/data/vrx-system:system',
      { 'vrx-system:system': { hostname: 'restconf-box' } },
      YANG,
    );
    expect(patch.status, patch.raw).toBe(200);

    const commit = await h.call(admin, 'POST', '/restconf/operations/vrx:commit', { input: {} }, YANG);
    expect(commit.status, commit.raw).toBe(200);
    expect(commit.body['vrx-restconf:output'].status).toBeDefined();

    const cfg = await h.call(admin, 'GET', '/api/v1/config/system');
    expect(cfg.body.hostname).toBe('restconf-box');

    const after = await h.call(admin, 'GET', '/api/v1/config/revisions?limit=1&offset=0');
    expect(after.body.items[0].id).toBeGreaterThan(beforeRev);
  });

  it('rejects an invalid value with an RFC 8040 error body and error-path', async () => {
    const bad = await h.call(
      admin,
      'PATCH',
      '/restconf/data/vrx-system:system',
      { 'vrx-system:system': { hostname: 'not a valid hostname!' } },
      YANG,
    );
    expect(bad.status).toBe(400);
    expect(String(bad.headers['content-type'])).toContain('application/yang-data+json');
    const errors = bad.body['ietf-restconf:errors'].error;
    expect(errors[0]['error-tag']).toBe('invalid-value');
    expect(errors[0]['error-path']).toContain('/system/hostname');
  });

  it('never returns a secret leaf (passwordHash)', async () => {
    const all = await h.call(admin, 'GET', '/restconf/data');
    expect(all.status, all.raw).toBe(200);
    expect(all.raw).not.toContain('passwordHash');
    const mgmt = await h.call(admin, 'GET', '/restconf/data/vrx-management:management');
    expect(mgmt.raw).not.toContain('passwordHash');
  });

  it('rolls back via RESTCONF', async () => {
    // change the hostname again and commit -> revision N+1
    await h.call(
      admin,
      'PATCH',
      '/restconf/data/vrx-system:system',
      { 'vrx-system:system': { hostname: 'second-name' } },
      YANG,
    );
    await h.call(admin, 'POST', '/restconf/operations/vrx:commit', { input: {} }, YANG);
    const revs = await h.call(admin, 'GET', '/api/v1/config/revisions?limit=5&offset=0');
    // the revision that set hostname=restconf-box (one before the latest)
    const target = revs.body.items[1].id;

    const rb = await h.call(
      admin,
      'POST',
      '/restconf/operations/vrx:rollback',
      { input: { revision: target } },
      YANG,
    );
    expect(rb.status, rb.raw).toBe(200);
    const cfg = await h.call(admin, 'GET', '/api/v1/config/system');
    expect(cfg.body.hostname).toBe('restconf-box');
  });

  it('lists and downloads generated YANG modules', async () => {
    const list = await h.call(admin, 'GET', '/api/v1/system/yang');
    expect(list.body.modules.length).toBeGreaterThan(10);
    const one = await h.call(admin, 'GET', '/api/v1/system/yang/vrx-interfaces');
    expect(one.body.name).toBe('vrx-interfaces');
    expect(one.body.yang).toContain('module vrx-interfaces {');
    const missing = await h.call(admin, 'GET', '/api/v1/system/yang/vrx-nope');
    expect(missing.status).toBe(404);
  });
});
