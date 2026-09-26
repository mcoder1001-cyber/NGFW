import { IssueSeverity } from '@ngfw/proto';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { setFakeHostAclCounters } from '../../src/features/host-acl-nftables/fake.js';
import { runSecret, startHarness, type Harness } from '../support/harness.js';

const PW = { op: runSecret(), ro: runSecret() };
const MP = { 'content-type': 'application/merge-patch+json' };

/**
 * F-host-acl-nftables on the host PostgreSQL with the fake agent: host lists, attachments and settings through the
 * generic pointer routes; `/state/host-acl` (HostAclState) with per-rule counter aggregation; schema and semantic
 * failures of `acl.hostSettings` → 400 problem+json with pointers at edit / validate time; the agent tier's
 * anti-lockout finding (rule acl.host-anti-lockout, simulated by the fake's DryRun) → 400 with the rule's pointer and
 * nothing applied; an agent without `acl` → 501. The real renderer is exercised by the agent's own tests and the
 * topology run. Fixture prefixes: 10.9.0.0/16 (slot 9) and 2001:db8::/32.
 */
describe('host ACL e2e (PostgreSQL + fake agent)', () => {
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

  const acl = {
    host: {
      'mgmt-in': {
        description: 'management plane',
        rules: [
          {
            sequence: 10,
            action: 'accept',
            source: { kind: 'prefix', prefix: '10.9.0.0/24' },
            service: { kind: 'inline', spec: { protocol: 'tcp', destinationPorts: ['22', '443'] } },
            log: true,
          },
          {
            sequence: 20,
            action: 'drop',
            service: { kind: 'inline', spec: { protocol: 'tcp', destinationPorts: ['22'] } },
          },
        ],
      },
    },
    hostAttachments: [{ list: 'mgmt-in', chain: 'input', priority: 0 }],
    hostSettings: { antiLockout: { sources: ['10.9.0.0/24'], interfaces: ['w9l0'] } },
  };

  it('commits host lists, attachments and settings; /state/host-acl reports chains, rules and aggregated counters', async () => {
    expect((await h.call(op, 'PATCH', '/api/v1/config/acl', acl, MP)).status).toBe(200);
    const c = await h.call(op, 'POST', '/api/v1/config/commit?comment=host-acl');
    expect(c.status).toBe(200);
    expect(c.body.status).toBe('applied');
    expect(h.fake.current['acl']).toMatchObject({
      hostAttachments: [{ list: 'mgmt-in', chain: 'input', priority: 0, enabled: true }],
      hostSettings: {
        defaultInput: 'accept',
        allowIcmp: true,
        antiLockout: {
          enabled: true,
          sources: ['10.9.0.0/24'],
          interfaces: ['w9l0'],
          ports: [22, 443],
        },
      },
    });
    // the running document carries the defaults of the new optional key
    const running = await h.call(ro, 'GET', '/api/v1/config/acl/hostSettings');
    expect(running.status).toBe(200);
    expect(running.body).toMatchObject({
      defaultInput: 'accept',
      antiLockout: { ports: [22, 443] },
    });

    setFakeHostAclCounters(h.fake, { 'mgmt-in:20': { packets: '9007199254740993', bytes: 540 } });
    const s = await h.call(ro, 'GET', '/api/v1/state/host-acl');
    expect(s.status).toBe(200);
    expect(s.body).toMatchObject({
      table: `vrx_${h.fake.owner}`,
      mode: 'check',
      present: false,
      inSync: true,
      sets: [],
    });
    expect(s.body.chains).toEqual([
      expect.objectContaining({
        name: 'in_mgmt-in',
        hook: 'input',
        priority: 0,
        policy: 'accept',
        list: 'mgmt-in',
      }),
    ]);
    expect(
      s.body.chains[0].rules.map((r: { sequence: number; verdict: string }) => [
        r.sequence,
        r.verdict,
      ]),
    ).toEqual([
      [10, 'accept'],
      [20, 'drop'],
    ]);
    expect(s.body.rules).toEqual([
      {
        list: 'mgmt-in',
        sequence: 10,
        pointer: '/acl/host/mgmt-in/rules/0',
        packets: '0',
        bytes: '0',
        nftRules: 1,
      },
      {
        list: 'mgmt-in',
        sequence: 20,
        pointer: '/acl/host/mgmt-in/rules/1',
        packets: '9007199254740993',
        bytes: '540',
        nftRules: 1,
      },
    ]);
    expect(h.fake.calls.filter((x) => x.method === 'HostAclState').at(-1)?.request).toEqual({
      owner: h.fake.owner,
    });
  });

  it('schema-invalid hostSettings → 400 problem+json with the pointer at edit time', async () => {
    const bad = await h.call(
      op,
      'PATCH',
      '/api/v1/config/acl/hostSettings',
      { antiLockout: { ports: [] } },
      MP,
    );
    expect(bad.status).toBe(400);
    expect(bad.headers['content-type']).toMatch(/^application\/problem\+json/);
    expect(bad.body.errors).toEqual([
      expect.objectContaining({ pointer: '/acl/hostSettings/antiLockout/ports' }),
    ]);
    const iface = await h.call(
      op,
      'PATCH',
      '/api/v1/config/acl/hostSettings',
      { antiLockout: { interfaces: ['eth0"; flush ruleset'] } },
      MP,
    );
    expect(iface.status).toBe(400);
    expect(iface.body.errors[0].pointer).toBe('/acl/hostSettings/antiLockout/interfaces/0');
    await h.call(op, 'POST', '/api/v1/config/discard');
  });

  it('a duplicate anti-lockout port → 400 at the semantic tier (acl.host-settings); nothing is applied', async () => {
    expect(
      (
        await h.call(
          op,
          'PATCH',
          '/api/v1/config/acl/hostSettings',
          { antiLockout: { ports: [22, 443, 22] } },
          MP,
        )
      ).status,
    ).toBe(200);
    const applies = h.fake.calls.filter((x) => x.method === 'Apply').length;
    for (const path of ['/api/v1/config/validate', '/api/v1/config/commit']) {
      const r = await h.call(op, 'POST', path);
      expect(r.status).toBe(400);
      expect(r.headers['content-type']).toMatch(/^application\/problem\+json/);
      expect(r.body.tier).toBe('semantic');
      expect(r.body.errors).toContainEqual({
        pointer: '/acl/hostSettings/antiLockout/ports/2',
        message: 'the same port as ports[0]',
      });
    }
    expect(h.fake.calls.filter((x) => x.method === 'Apply').length).toBe(applies);
    expect((await h.call(op, 'POST', '/api/v1/config/discard')).status).toBe(200);
  });

  it('the agent tier refusing a rule that drops management SSH → 400 with the rule pointer; running unchanged', async () => {
    // anti-lockout off and a rule dropping SSH from the management source: the real agent's DryRun reports
    // acl.host-anti-lockout at the rule (renderers/nftables); the fake reproduces the finding.
    const patch = {
      host: {
        'mgmt-in': {
          rules: [
            { sequence: 5, action: 'drop', source: { kind: 'prefix', prefix: '10.9.0.0/24' } },
          ],
        },
      },
      hostSettings: { antiLockout: { enabled: false } },
    };
    expect((await h.call(op, 'PATCH', '/api/v1/config/acl', patch, MP)).status).toBe(200);
    h.fake.dryRunIssues = (desired) => {
      const lockout = ((desired['acl'] as Record<string, any> | undefined)?.['hostSettings'] ?? {})[
        'antiLockout'
      ];
      return lockout?.enabled === false
        ? [
            {
              pointer: '/acl/host/mgmt-in/rules/0',
              message: 'would drop management SSH (tcp/22) from 10.9.0.0/24',
              severity: IssueSeverity.ISSUE_SEVERITY_ERROR,
              rule: 'acl.host-anti-lockout',
            },
          ]
        : [];
    };
    try {
      const before = await h.call(ro, 'GET', '/api/v1/config/acl');
      const r = await h.call(op, 'POST', '/api/v1/config/commit');
      expect(r.status).toBe(400);
      expect(r.headers['content-type']).toMatch(/^application\/problem\+json/);
      expect(r.body.tier).toBe('agent');
      expect(r.body.errors).toContainEqual(
        expect.objectContaining({
          pointer: '/acl/host/mgmt-in/rules/0',
          message: expect.stringMatching(/SSH/),
        }),
      );
      expect((await h.call(ro, 'GET', '/api/v1/config/acl')).body).toEqual(before.body);
    } finally {
      h.fake.dryRunIssues = undefined;
      await h.call(op, 'POST', '/api/v1/config/discard');
    }
  });

  it('requires authentication; an agent without acl answers 501', async () => {
    expect((await h.call(undefined, 'GET', '/api/v1/state/host-acl')).status).toBe(401);
    h.fake.implemented = h.fake.implemented.filter((d) => d !== 'acl');
    try {
      const r = await h.call(ro, 'GET', '/api/v1/state/host-acl');
      expect(r.status).toBe(501);
      expect(r.headers['content-type']).toMatch(/^application\/problem\+json/);
    } finally {
      h.fake.implemented.push('acl');
    }
  });
});
