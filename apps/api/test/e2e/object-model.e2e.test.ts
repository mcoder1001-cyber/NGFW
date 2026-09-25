import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { setFakeFqdn } from '../../src/features/object-model/fake.js';
import { runSecret, startHarness, type Harness } from '../support/harness.js';

const PW = { op: runSecret(), ro: runSecret() };
const MP = { 'content-type': 'application/merge-patch+json' };

/**
 * F-object-model on the host PostgreSQL with the fake agent: objects through the generic pointer routes, where-used
 * (`/state/objects/usage`) from running and candidate, FQDN state (`/state/objects/fqdn`, FqdnObjectState), deleting an
 * object an ACL rule still uses → 400 with the rule's pointer (`acl.rule-references`), rollback restores the object
 * set. Fixture addresses: documentation ranges and 10.3.0.0/16 (slot 3); FQDNs under the test-only zone w3.test.
 */
describe('object model e2e (PostgreSQL + fake agent)', () => {
  let h: Harness;
  let op: string;
  let ro: string;

  beforeAll(async () => {
    h = await startHarness({});
    const admin = await h.login('admin', h.adminPassword);
    await h.createUsers(admin, [
      { username: 'op1', role: 'operator', password: PW.op },
      { username: 'ro1', role: 'readonly', password: PW.ro },
    ]);
    op = await h.login('op1', PW.op);
    ro = await h.login('ro1', PW.ro);
  });
  afterAll(async () => h?.close());

  const usage = async (name: string, source = 'running') =>
    (await h.call(ro, 'GET', `/api/v1/state/objects/usage?name=${encodeURIComponent(name)}&source=${source}`)).body as {
      definedAs: string[];
      usedBy: { pointer: string; kind: string }[];
    };

  it('commits objects and an ACL that uses them; where-used and FQDN state reflect the running config', async () => {
    expect((await h.call(op, 'PATCH', '/api/v1/config/interfaces', { loop3001: { enabled: true } }, MP)).status).toBe(200);
    const objects = {
      tags: { prod: { color: '#1e88e5', description: 'production' } },
      addresses: {
        web1: { type: 'host', address: '192.0.2.10', tags: ['prod'] },
        web2: { type: 'host', address: '192.0.2.11' },
        cdn: { type: 'fqdn', fqdn: 'cdn.w3.test' },
        lan: { type: 'network', prefix: '10.3.1.0/24' },
      },
      addressGroups: { 'web-servers': { members: ['web1', 'web2'], tags: ['prod'] } },
      services: { https: { protocol: 'tcp', destinationPorts: ['443'] } },
      schedules: { 'office-hours': { type: 'recurring', days: ['mon', 'tue', 'wed', 'thu', 'fri'], start: '08:00', end: '18:00' } },
      zones: { lan: { interfaces: ['loop3001'] } },
    };
    expect((await h.call(op, 'PATCH', '/api/v1/config/objects', objects, MP)).status).toBe(200);
    const acl = {
      lists: {
        'web-in': {
          rules: [
            {
              sequence: 10,
              action: 'permit',
              destination: { kind: 'object', name: 'web-servers' },
              service: { kind: 'object', name: 'https' },
              schedule: 'office-hours',
            },
          ],
        },
      },
      attachments: [{ list: 'web-in', target: { kind: 'zone', zone: 'lan' }, sequence: 1 }],
    };
    expect((await h.call(op, 'PATCH', '/api/v1/config/acl', acl, MP)).status).toBe(200);
    const c = await h.call(op, 'POST', '/api/v1/config/commit?comment=objects');
    expect(c.status).toBe(200);
    expect(c.body.status).toBe('applied');

    // the objects document reached the agent (Retrieve of the fake = what was applied)
    expect(h.fake.current['objects']).toMatchObject({ addressGroups: { 'web-servers': { members: ['web1', 'web2'] } } });

    const u = await usage('web-servers');
    expect(u.definedAs).toEqual(['addressGroups']);
    expect(u.usedBy).toEqual([
      {
        pointer: '/acl/lists/web-in/rules/0/destination/name',
        container: '/acl/lists/web-in/rules/0',
        domain: 'acl',
        kind: 'acl-rule-destination',
      },
    ]);
    expect((await usage('web1')).usedBy.map((r) => r.kind)).toEqual(['address-group-member']);
    expect((await usage('lan')).definedAs).toEqual(['addresses', 'zones']);
    expect((await usage('lan')).usedBy.map((r) => r.pointer)).toEqual(['/acl/attachments/0/target/zone']);
    expect((await usage('loop3001')).usedBy.map((r) => r.kind)).toEqual(['zone-interface']);
    // interface names with a slash are valid queries (nothing references this one)
    expect(await usage('GigabitEthernet0/8/0')).toMatchObject({ definedAs: [], usedBy: [] });

    // FQDN state: never resolved, then resolved (the fake reports what a test sets)
    let f = await h.call(ro, 'GET', '/api/v1/state/objects/fqdn');
    expect(f.status).toBe(200);
    expect(f.body.items).toEqual([
      { name: 'cdn', fqdn: 'cdn.w3.test', addresses: [], lastResolved: null, nextRefresh: null, error: '', failures: 0 },
    ]);
    const at = new Date('2026-09-24T12:00:00Z');
    setFakeFqdn(h.fake, 'cdn', { addresses: ['192.0.2.53', '2001:db8::53'], lastResolved: at, nextRefresh: new Date(at.getTime() + 60_000) });
    f = await h.call(ro, 'GET', '/api/v1/state/objects/fqdn?name=cdn');
    expect(f.body.items[0]).toMatchObject({
      addresses: ['192.0.2.53', '2001:db8::53'],
      lastResolved: '2026-09-24T12:00:00.000Z',
      nextRefresh: '2026-09-24T12:01:00.000Z',
    });
    expect(h.fake.calls.filter((x) => x.method === 'FqdnObjectState').at(-1)?.request).toMatchObject({ names: ['cdn'], owner: h.fake.owner });
  });

  it('deleting an object an ACL rule still uses: 400 problem+json pointing at the rule; nothing is applied', async () => {
    expect((await h.call(op, 'DELETE', '/api/v1/config/objects/addressGroups/web-servers')).status).toBe(200);
    // the candidate no longer has the group; where-used on the candidate still shows the dangling reference
    expect((await usage('web-servers', 'candidate')).definedAs).toEqual([]);
    expect((await usage('web-servers', 'candidate')).usedBy).toHaveLength(1);
    const applies = h.fake.calls.filter((x) => x.method === 'Apply').length;
    for (const path of ['/api/v1/config/validate', '/api/v1/config/commit']) {
      const r = await h.call(op, 'POST', path);
      expect(r.status).toBe(400);
      expect(r.headers['content-type']).toMatch(/^application\/problem\+json/);
      expect(r.body.errors).toContainEqual(
        // acl.rule-references
        { pointer: '/acl/lists/web-in/rules/0/destination/name', message: "'web-servers' is not an entry of objects.addresses or objects.addressGroups" },
      );
    }
    expect(h.fake.calls.filter((x) => x.method === 'Apply').length).toBe(applies);
    expect((await h.call(op, 'POST', '/api/v1/config/discard')).status).toBe(200);
  });

  it('rollback restores the previous object set (running document and where-used)', async () => {
    const before = await h.call(ro, 'GET', '/api/v1/config/objects');
    const rev1 = (await h.call(ro, 'GET', '/api/v1/config/revisions?limit=1')).body.items[0].id as number;
    // revision 2: drop the rule's use of the group, shrink the group, add a service group
    const lists = { 'web-in': { rules: [{ sequence: 10, action: 'permit', destination: { kind: 'object', name: 'web1' } }] } };
    expect((await h.call(op, 'PUT', '/api/v1/config/acl/lists', lists)).status).toBe(200);
    expect(
      (await h.call(op, 'PATCH', '/api/v1/config/objects', { addressGroups: { 'web-servers': { members: ['web2'] } }, serviceGroups: { web: { members: ['https'] } } }, MP)).status,
    ).toBe(200);
    expect((await h.call(op, 'POST', '/api/v1/config/commit?comment=shrink')).body.status).toBe('applied');
    expect((await usage('web-servers')).usedBy).toEqual([]);
    expect((await usage('web2')).usedBy.map((r) => r.pointer)).toEqual(['/objects/addressGroups/web-servers/members/0']);

    const rb = await h.call(op, 'POST', `/api/v1/config/rollback/${rev1}`);
    expect(rb.status).toBe(200);
    expect(rb.body.status).toBe('applied');
    const after = await h.call(ro, 'GET', '/api/v1/config/objects');
    expect(after.body).toEqual(before.body);
    expect((after.body as { serviceGroups: object }).serviceGroups).toEqual({});
    expect((await usage('web-servers')).usedBy.map((r) => r.pointer)).toEqual(['/acl/lists/web-in/rules/0/destination/name']);
    expect(h.fake.current['objects']).toMatchObject({ addressGroups: { 'web-servers': { members: ['web1', 'web2'] } } });
  });

  it('validates the query; an agent without the RPC answers 501', async () => {
    expect((await h.call(ro, 'GET', '/api/v1/state/objects/usage')).status).toBe(400);
    const bad = await h.call(ro, 'GET', '/api/v1/state/objects/usage?name=..%2Fx');
    expect(bad.status).toBe(400);
    expect(bad.body.errors?.[0]?.pointer).toBe('/name');
    expect((await h.call(ro, 'GET', '/api/v1/state/objects/usage?name=x&source=agent')).status).toBe(400);
    expect((await h.call(undefined, 'GET', '/api/v1/state/objects/fqdn')).status).toBe(401);
    h.fake.implemented = h.fake.implemented.filter((d) => d !== 'objects');
    try {
      expect((await h.call(ro, 'GET', '/api/v1/state/objects/fqdn')).status).toBe(501);
    } finally {
      h.fake.implemented.push('objects');
    }
  });
});
