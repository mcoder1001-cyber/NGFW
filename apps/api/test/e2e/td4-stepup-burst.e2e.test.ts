import { sql } from 'drizzle-orm';
import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest';
import type * as Password from '../../src/auth/password.js';
import { runSecret, startHarness, type Harness } from '../support/harness.js';

/**
 * TD-4 fix round 1, review H1 — the key-creation step-up (D-100 (2)) under concurrency, on the host PostgreSQL + Valkey:
 * one stolen JWT sends BURST key creations at once, each with another `current`, the right one among them. Every
 * request reads `locked_until` before any failure lands, so without a throttle each guess reaches argon2 and the
 * lockout bounds nothing. Expected: a per-account budget (VRX_PASSWORD_RATE_PER_MIN, shared with password changes)
 * stands in front of argon2 — at most that many checks per minute window, 429 for the rest — and every `locked` answer
 * has ONE body, so no answer tells a checked guess from an unchecked one. No api_key row unless the right guess got 201.
 * Every argon2 verification the API makes is counted through a pass-through wrapper of `verifyPassword`.
 */
const argon = vi.hoisted(() => ({ checks: 0 }));
vi.mock('../../src/auth/password.js', async (importOriginal) => {
  const m = await importOriginal<typeof Password>();
  return {
    ...m,
    verifyPassword: (...a: Parameters<typeof m.verifyPassword>) => {
      argon.checks++;
      return m.verifyPassword(...a);
    },
  };
});

const BURST = 30;
const RIGHT_AT = 20;
const RATE = 5; // VRX_PASSWORD_RATE_PER_MIN (the default)
const MAX_FAILURES = 3;
const PW = runSecret();
const P = 'https://vrx.dev/problems/';

describe('TD-4 step-up burst (review H1)', () => {
  let h: Harness;
  beforeAll(async () => {
    h = await startHarness({
      VRX_LOGIN_MAX_FAILURES: String(MAX_FAILURES),
      VRX_LOGIN_RATE_PER_MIN: '1000',
      VRX_PASSWORD_RATE_PER_MIN: String(RATE),
    });
    const admin = await h.login('admin', h.adminPassword);
    await h.createUsers(admin, [{ username: 'burst', role: 'operator', password: PW }]);
  });
  afterAll(async () => {
    await h?.close();
  });

  it(`${BURST} parallel step-ups from one JWT (right guess at #${RIGHT_AT}): ≤ ${RATE} argon2 checks per minute window, 429 for the rest, one body for every locked answer`, async () => {
    const tok = await h.login('burst', PW);
    const guesses = Array.from({ length: BURST }, (_, i) =>
      i === RIGHT_AT ? PW : `wrong-${i}-${runSecret()}`,
    );
    const before = argon.checks;
    const window0 = Math.floor(Date.now() / 60_000);
    const answers = await Promise.all(
      guesses.map((current, i) =>
        h.call(tok, 'POST', '/api/v1/auth/api-keys', { name: `burst-${i}`, current }),
      ),
    );
    // the budget is per minute window (Valkey INCR on rl:pwset:<id>:<minute>): a burst across a boundary gets two
    const windows = Math.floor(Date.now() / 60_000) - window0 + 1;
    const budget = RATE * windows;
    const checks = argon.checks - before;
    const slug = (i: number) => String(answers[i]!.body?.type ?? '').replace(P, '');
    const groups = new Map<string, number[]>();
    answers.forEach((r, i) => {
      const k = `${r.status} ${slug(i) || '-'} | ${r.body?.detail ?? r.body?.name ?? ''}`;
      groups.set(k, [...(groups.get(k) ?? []), i]);
    });
    console.log(
      `H1 step-up burst (right guess at #${RIGHT_AT}, MAX_FAILURES ${MAX_FAILURES}, VRX_PASSWORD_RATE_PER_MIN ${RATE}, ${windows} minute window(s)):\n` +
        [...groups].map(([k, ix]) => `  ${ix.length}× ${k}  ← #${ix.join(',')}`).join('\n'),
    );
    const locked = answers.filter((r, i) => r.status === 403 && slug(i) === 'locked');
    const n429 = answers.filter((r) => r.status === 429).length;
    const right = answers[RIGHT_AT]!;
    const keyRows = Number(
      (
        await h.db.execute(
          sql`select count(*)::int as n from api_key k join app_user u on u.id = k.user_id where u.username = 'burst'`,
        )
      ).rows[0]!['n'],
    );
    const summary = {
      argon2Checks: checks,
      rateLimited: n429,
      lockedAnswers: locked.length,
      distinctLockedBodies: new Set(locked.map((r) => r.raw)).size,
      rightGuess: `${right.status} ${slug(RIGHT_AT) || '-'}`,
      keyRows,
    };
    console.log(`H1 summary: ${JSON.stringify(summary)} (budget ${budget})`);
    // compared, not thrown one by one: a failure shows every property next to the others
    expect({
      checksWithinBudget: checks <= budget,
      restRateLimited: n429 >= BURST - budget,
      oneLockedBody: locked.length === 0 || summary.distinctLockedBodies === 1,
      keyOnlyIfRightGot201: keyRows === (right.status === 201 ? 1 : 0),
      // the right guess: 201, locked or 429 — never "wrong"; a wrong one: forbidden, locked or 429
      onlyKnownAnswers: answers.every(
        (r, i) =>
          r.status === 429 ||
          (r.status === 403 && slug(i) === 'locked') ||
          (i === RIGHT_AT ? r.status === 201 : r.status === 403 && slug(i) === 'forbidden'),
      ),
    }).toEqual({
      checksWithinBudget: true,
      restRateLimited: true,
      oneLockedBody: true,
      keyOnlyIfRightGot201: true,
      onlyKnownAnswers: true,
    });
    // every 429 is the rate limiter's own problem
    for (const r of answers.filter((a) => a.status === 429))
      expect(r.body).toMatchObject({ type: `${P}rate-limited`, status: 429 });
    // review M1: each failed creation is audited with its problem slug (never the detail or the password)
    const audit = await h.db.execute(
      sql`select status, after from audit_log where action = 'POST /api/v1/auth/api-keys' and username = 'burst' order by id`,
    );
    const reasons = (rows: { status: number; reason: string }[]) =>
      rows.map((r) => `${r.status} ${r.reason}`).sort();
    expect(
      reasons(
        audit.rows.map((r) => ({
          status: Number(r['status']),
          reason: String((r['after'] as { reason?: string }).reason ?? '-'),
        })),
      ),
    ).toEqual(
      reasons(
        answers.map((r, i) => ({ status: r.status, reason: r.status === 201 ? '-' : slug(i) })),
      ),
    );
    const stored = JSON.stringify(audit.rows);
    expect(guesses.filter((g) => stored.includes(g))).toEqual([]);
  }, 120_000);
});
