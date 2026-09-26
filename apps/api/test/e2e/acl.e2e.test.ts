import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import {
  setFakeAclCounters,
  setFakeAclForeign,
  setFakeAclHits,
} from '../../src/features/acl/fake.js';
import { runSecret, startHarness, type Harness } from '../support/harness.js';

const PW = { op: runSecret(), ro: runSecret() };
const MP = { 'content-type': 'application/merge-patch+json' };
const CSV = { 'content-type': 'text/csv' };

/**
 * F-acl on the host PostgreSQL (slot database) with the fake agent: ACL lists through the generic pointer routes, the
 * state routes (list status, a rule page with hit counters per configuration rule, bindings incl. another owner's
 * ACL), CSV import (dry run first) and export, bulk edits as one candidate edit with a compact audit row, and the
 * validation failure for a rule that names an empty address group. Addresses: documentation ranges and 10.3.0.0/16.
 */
describe('acl e2e (PostgreSQL + fake agent)', () => {
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

  it('commits a list with attachments; lists, rule page with counters, attachments with a foreign ACL', async () => {
    expect(
      (await h.call(op, 'PATCH', '/api/v1/config/interfaces', { loop3001: { enabled: true } }, MP))
        .status,
    ).toBe(200);
    const objects = {
      addresses: {
        web1: { type: 'host', address: '192.0.2.10' },
        web2: { type: 'host', address: '192.0.2.11' },
      },
      addressGroups: { 'web-servers': { members: ['web1', 'web2'] }, empty: { members: [] } },
      services: { https: { protocol: 'tcp', destinationPorts: ['443'] } },
    };
    expect((await h.call(op, 'PATCH', '/api/v1/config/objects', objects, MP)).status).toBe(200);
    const acl = {
      lists: {
        'web-in': {
          description: 'web',
          rules: [
            { sequence: 30, action: 'deny', ipVersion: 'ipv4' },
            {
              sequence: 10,
              action: 'permit',
              destination: { kind: 'object', name: 'web-servers' },
              service: { kind: 'object', name: 'https' },
            },
            { sequence: 20, action: 'reflect', source: { kind: 'prefix', prefix: '10.3.1.0/24' } },
          ],
        },
      },
      attachments: [
        {
          list: 'web-in',
          target: { kind: 'interface', interface: 'loop3001' },
          direction: 'in',
          sequence: 1,
        },
      ],
    };
    expect((await h.call(op, 'PATCH', '/api/v1/config/acl', acl, MP)).status).toBe(200);
    const c = await h.call(op, 'POST', '/api/v1/config/commit?comment=acl');
    expect(c.status).toBe(200);
    expect(c.body.status).toBe('applied');

    const lists = await h.call(ro, 'GET', '/api/v1/state/acl/lists');
    expect(lists.status).toBe(200);
    expect(lists.body.lists).toEqual([
      expect.objectContaining({
        name: 'web-in',
        rules: 3,
        pending: null,
        attachments: [
          {
            target: { kind: 'interface', interface: 'loop3001' },
            direction: 'in',
            sequence: 1,
            enabled: true,
          },
        ],
        live: expect.objectContaining({ aclIndex: 0, vppRules: 3, mappingKnown: true, packets: 0 }),
      }),
    ]);
    expect(lists.body.countersAvailable).toBe(false);

    setFakeAclCounters(h.fake, true);
    setFakeAclHits(h.fake, 'web-in', 10, 5);
    const page = await h.call(
      ro,
      'GET',
      '/api/v1/state/acl/lists/web-in/rules?page=1&pageSize=2&source=running',
    );
    expect(page.status).toBe(200);
    expect(page.body).toMatchObject({
      total: 3,
      size: 3,
      applied: true,
      countersAvailable: true,
      mappingKnown: true,
    });
    expect(
      page.body.items.map((i: { sequence: number; index: number }) => [i.sequence, i.index]),
    ).toEqual([
      [10, 1],
      [20, 2],
    ]);
    expect(page.body.items[0].live).toEqual({
      status: 'applied',
      vppRules: 1,
      packets: 5,
      bytes: 500,
    });
    // the agent was asked for exactly the page's sequences
    expect(h.fake.calls.filter((x) => x.method === 'AclState').at(-1)?.request).toMatchObject({
      list: 'web-in',
      filter: { sequences: [10, 20], hitsOnly: false },
    });
    const hits = await h.call(ro, 'GET', '/api/v1/state/acl/lists/web-in/rules?hitsOnly=true');
    expect(hits.body.items.map((i: { sequence: number }) => i.sequence)).toEqual([10]);
    const found = await h.call(
      ro,
      'GET',
      '/api/v1/state/acl/lists/web-in/rules?filter=reflect%2010.3.1',
    );
    expect(found.body.items.map((i: { sequence: number }) => i.sequence)).toEqual([20]);

    setFakeAclForeign(h.fake, 'loop3001', [
      { aclIndex: 77, name: '', tag: 'w9:foreign', foreign: true },
    ]);
    const att = await h.call(ro, 'GET', '/api/v1/state/acl/attachments');
    expect(att.status).toBe(200);
    expect(att.body.interfaces).toEqual([
      expect.objectContaining({
        interface: 'loop3001',
        input: [
          { aclIndex: 77, name: null, tag: 'w9:foreign', foreign: true },
          { aclIndex: 0, name: 'web-in', tag: `${h.fake.owner}:web-in`, foreign: false },
        ],
        expected: { input: ['web-in'], output: [], macip: null },
        inSync: true,
      }),
    ]);
    expect((await h.call(ro, 'GET', '/api/v1/state/acl/lists/nope/rules')).status).toBe(404);
  });

  it('bulk edits the candidate in one edit; pending marks; readonly is refused', async () => {
    const b = await h.call(op, 'POST', '/api/v1/actions/acl/lists/web-in/rules/bulk', {
      op: 'disable',
      sequences: [10, 30],
    });
    expect(b.status).toBe(200);
    expect(b.body).toMatchObject({ op: 'disable', changed: 2, total: 3 });
    const m = await h.call(op, 'POST', '/api/v1/actions/acl/lists/web-in/rules/bulk', {
      op: 'move',
      sequences: [30],
      to: 5,
    });
    expect(m.body).toMatchObject({ changed: 1 });
    const page = await h.call(op, 'GET', '/api/v1/state/acl/lists/web-in/rules');
    expect(
      page.body.items.map(
        (i: { sequence: number; pending: string | null; rule: { enabled: boolean } }) => [
          i.sequence,
          i.pending,
          i.rule.enabled,
        ],
      ),
    ).toEqual([
      [5, 'added', false],
      [10, 'changed', false],
      [20, null, true],
    ]);
    expect(
      (
        await h.call(ro, 'POST', '/api/v1/actions/acl/lists/web-in/rules/bulk', {
          op: 'enable',
          sequences: [5],
        })
      ).status,
    ).toBe(403);
    const bad = await h.call(op, 'POST', '/api/v1/actions/acl/lists/web-in/rules/bulk', {
      op: 'enable',
      sequences: [999],
    });
    expect(bad.status).toBe(400);
    expect(bad.headers['content-type']).toContain('application/problem+json');
    const audit = await h.call(
      await h.login('admin', h.adminPassword),
      'GET',
      '/api/v1/audit?limit=10',
    );
    const rows = (
      audit.body.items as { action: string; result: string; after: { op?: string } | null }[]
    ).filter((r) => r.action.includes('/rules/bulk'));
    // compact rows (never the 100k-rule arrays): the successful move, and the refused edit as a failure
    expect(rows.find((r) => r.after?.op === 'move')?.after).toEqual({
      rules: 3,
      op: 'move',
      changed: 1,
      sequences: [30],
      count: 1,
      to: 5,
    });
    expect(rows.some((r) => r.result === 'failure')).toBe(true);
    expect((await h.call(op, 'POST', '/api/v1/config/discard')).status).toBe(200);
  });

  it('imports CSV with a dry run first, appends, exports', async () => {
    const csv =
      'sequence,action,enabled,ipVersion,source,destination,service,schedule,log,description\r\n' +
      '40,permit,true,any,10.3.2.0/24,object:web-servers,tcp:80|443,,false,"http, https"\r\n' +
      '50,deny,,ipv6,any,any,icmp6:128,,,\r\n' +
      '60,permit,true,any,object:ghost,any,any,,false,\r\n';
    const dry = await h.call(
      op,
      'POST',
      '/api/v1/actions/acl/import?list=web-in&mode=append',
      csv,
      CSV,
    );
    expect(dry.status).toBe(200);
    expect(dry.body).toMatchObject({
      dryRun: true,
      rows: 3,
      valid: 3,
      errorCount: 0,
      imported: 0,
      existingRules: 3,
    });
    expect(dry.body.warnings).toEqual([
      { message: "address object 'ghost' is not defined in objects (the commit will reject it)" },
    ]);
    expect(dry.body.preview[0]).toMatchObject({
      sequence: 40,
      description: 'http, https',
      service: { kind: 'inline', spec: { protocol: 'tcp', destinationPorts: ['80', '443'] } },
    });
    const errs = await h.call(
      op,
      'POST',
      '/api/v1/actions/acl/import?list=web-in&mode=append&dryRun=false',
      'sequence,action\r\n10,permit\r\nx,deny\r\n',
      CSV,
    );
    expect(errs.status).toBe(400);
    expect(errs.body.errors.map((e: { pointer: string }) => e.pointer)).toEqual([
      '/csv/2/sequence',
      '/csv/3/sequence',
    ]);
    const ok = await h.call(
      op,
      'POST',
      '/api/v1/actions/acl/import?list=web-in&mode=append&dryRun=false',
      csv.replace('object:ghost', 'any'),
      CSV,
    );
    expect(ok.status).toBe(200);
    expect(ok.body).toMatchObject({ imported: 3, total: 6 });
    const exp = await h.call(
      op,
      'GET',
      '/api/v1/actions/acl/export.csv?list=web-in&source=candidate',
    );
    expect(exp.status).toBe(200);
    expect(exp.headers['content-type']).toContain('text/csv');
    const lines = exp.raw.trim().split('\r\n');
    expect(lines[0]).toBe(
      'sequence,action,enabled,ipVersion,source,destination,service,schedule,log,description',
    );
    expect(lines.slice(1).map((l) => l.split(',')[0])).toEqual([
      '10',
      '20',
      '30',
      '40',
      '50',
      '60',
    ]);
    expect(lines[4]).toBe(
      '40,permit,true,any,10.3.2.0/24,object:web-servers,tcp:80|443,,false,"http, https"',
    );
    expect(
      (await h.call(ro, 'POST', '/api/v1/actions/acl/import?list=web-in', csv, CSV)).status,
    ).toBe(403);
    expect((await h.call(op, 'POST', '/api/v1/config/discard')).status).toBe(200);
  });

  it('a rule naming an empty address group: 400 problem+json with the pointer; nothing is applied', async () => {
    const applies = h.fake.calls.filter((x) => x.method === 'Apply').length;
    expect(
      (
        await h.call(op, 'PUT', '/api/v1/config/acl/lists/web-in/rules/3', {
          sequence: 70,
          action: 'permit',
          destination: { kind: 'object', name: 'empty' },
        })
      ).status,
    ).toBe(200);
    const c = await h.call(op, 'POST', '/api/v1/config/commit');
    expect(c.status).toBe(400);
    expect(c.headers['content-type']).toContain('application/problem+json');
    expect(c.body.errors).toContainEqual(
      expect.objectContaining({
        pointer: '/acl/lists/web-in/rules/3/destination/name',
        message: expect.stringContaining('has no members'),
      }),
    );
    expect(h.fake.calls.filter((x) => x.method === 'Apply').length).toBe(applies);
    expect((await h.call(op, 'POST', '/api/v1/config/discard')).status).toBe(200);
  });

  it('degrades without the agent RPC: config columns stay, live is null with the reason', async () => {
    h.fake.implemented = h.fake.implemented.filter((d) => d !== 'acl');
    try {
      const lists = await h.call(ro, 'GET', '/api/v1/state/acl/lists');
      expect(lists.status).toBe(200);
      expect(lists.body.lists[0]).toMatchObject({ name: 'web-in', live: null });
      expect(lists.body.agentError).toContain('acl not implemented');
    } finally {
      h.fake.implemented.push('acl');
    }
  });
});
