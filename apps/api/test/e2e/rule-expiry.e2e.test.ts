import { sql } from 'drizzle-orm';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { RuleExpiryService } from '../../src/features/rule-expiry/rule-expiry.service.js';
import { runSecret, startHarness, type Harness } from '../support/harness.js';

/**
 * F-rule-expiry API: `rule.expires-in-past` at commit (new rule / changed date only; rollback restores as it was),
 * `rule.expired` skipped by the drift view, RULE_EXPIRING / RULE_EXPIRED system events raised once per rule and date.
 */
const MP = { 'content-type': 'application/merge-patch+json' };
const iso = (ms: number) => new Date(Date.now() + ms).toISOString();

describe('F-rule-expiry e2e (PostgreSQL + fake agent)', () => {
  let h: Harness;
  let op: string;
  const pw = runSecret();

  beforeAll(async () => {
    h = await startHarness({});
    const admin = await h.login('admin', h.adminPassword);
    await h.createUsers(admin, [{ username: 'op1', role: 'operator', password: pw }]);
    op = await h.login('op1', pw);
  });
  afterAll(async () => h?.close());

  const list = (expiresAt?: string, extra: Record<string, unknown> = {}) => ({
    lists: {
      tmp: {
        rules: [
          {
            sequence: 10,
            action: 'permit',
            ...(expiresAt ? { expiresAt, owner: 'netops', ticket: 'CHG-7' } : {}),
            ...extra,
          },
          { sequence: 20, action: 'deny' },
        ],
      },
    },
  });
  const stage = async (acl: unknown) =>
    expect((await h.call(op, 'PATCH', '/api/v1/config/acl', acl, MP)).status).toBe(200);
  const commit = (comment: string) =>
    h.call(op, 'POST', `/api/v1/config/commit?comment=${comment}`);

  it('refuses a past expiry on a new rule; keeps an unchanged expired rule; rollback restores it as it was', async () => {
    await stage(list(iso(-60_000)));
    const bad = await commit('past');
    expect(bad.status, bad.raw).toBe(400);
    expect(bad.body.errors).toEqual([
      expect.objectContaining({
        pointer: '/acl/lists/tmp/rules/0/expiresAt',
        rule: 'rule.expires-in-past',
      }),
    ]);

    // a rule that expires 1.5 s after it is committed …
    const soon = iso(1500);
    await stage(list(soon));
    const ok = await commit('soon');
    expect(ok.status, ok.raw).toBe(200);
    const rev1 = ok.body.revision.id as number;
    await new Promise((r) => setTimeout(r, 2000));
    // … is expired now, and an unrelated change still commits (unchanged expiresAt)
    await stage({ lists: { tmp: { description: 'unrelated edit' } } });
    const again = await commit('unrelated');
    expect(again.status, again.raw).toBe(200);
    // re-dating it to another past time is refused; extending it is fine
    await stage(list(iso(-5_000)));
    expect((await commit('redate')).status).toBe(400);
    await stage(list(iso(86_400_000)));
    expect((await commit('extend')).status).toBe(200);

    // a rollback to the revision with the (now expired) rule is not refused
    const rb = await h.call(op, 'POST', `/api/v1/config/rollback/${rev1}`);
    expect(rb.status, rb.raw).toBe(200);
  });

  it('raises RULE_EXPIRING and RULE_EXPIRED once per rule and date', async () => {
    await stage(list(iso(2 * 86_400_000)));
    expect((await commit('warn')).status).toBe(200);
    const svc = h.app.get(RuleExpiryService);
    const first = await svc.scan(new Date());
    expect(first).toEqual([{ code: 'RULE_EXPIRING', id: 'acl/tmp/10' }]);
    expect(await svc.scan(new Date())).toEqual([]); // once
    const later = await svc.scan(new Date(Date.now() + 3 * 86_400_000));
    expect(later).toEqual([{ code: 'RULE_EXPIRED', id: 'acl/tmp/10' }]);
    const rows = await h.db.execute(
      sql`select code from system_event where subsystem = 'rules' order by id`,
    );
    const events = (rows.rows as { code: string }[]).map((r) => r.code);
    expect(events).toEqual(expect.arrayContaining(['RULE_EXPIRING', 'RULE_EXPIRED']));
  });
});
