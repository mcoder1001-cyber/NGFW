import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { startHarness, type Harness } from '../support/harness.js';
import { AutoBlockService } from '../../src/features/auto-block/index.js';

/**
 * F-bruteforce-block e2e (API detector): failed web logins from a source cross the threshold and the source lands in
 * the live auto-block set; an admin can unblock it; it expires after the block window; an allow-listed source is never
 * blocked. Data-plane enforcement (VPP acl / nftables local-in) is the host follow-up.
 */
const MP = { 'content-type': 'application/merge-patch+json' };
const BAD = '203.0.113.7';
const GOOD = '198.51.100.9';

async function failLogin(h: Harness, ip: string): Promise<void> {
  const r = await h.call(
    undefined,
    'POST',
    '/api/v1/auth/login',
    { username: 'admin', password: 'definitely-wrong-password' },
    // a remote client behind the trusted proxy, over TLS (so the password is acted on, not refused with tls-required)
    { 'x-forwarded-for': ip, 'x-forwarded-proto': 'https' },
  );
  expect([401, 429]).toContain(r.status);
}

describe('F-bruteforce-block e2e (PostgreSQL + fake agent)', () => {
  let h: Harness;
  let admin: string;
  let op: string;
  let svc: AutoBlockService;
  let clock = Date.UTC(2026, 8, 27, 12, 0, 0);

  beforeAll(async () => {
    h = await startHarness({ VRX_LOGIN_RATE_PER_MIN: '100' });
    admin = await h.login('admin', h.adminPassword);
    await h.createUsers(admin, [
      { username: 'op1', role: 'operator', password: 'Op1-pw-1234567890' },
    ]);
    op = await h.login('op1', 'Op1-pw-1234567890');

    const cfg = await h.call(
      admin,
      'PATCH',
      '/api/v1/config/security',
      {
        autoBlock: {
          enabled: true,
          rules: [{ source: 'webLogin', threshold: 3, windowSec: 3600, blockSec: 900 }],
          allowlist: ['198.51.100.0/24'],
        },
      },
      MP,
    );
    expect(cfg.status, cfg.raw).toBe(200);
    expect((await h.call(admin, 'POST', '/api/v1/config/commit?comment=autoblock')).status).toBe(
      200,
    );

    svc = h.app.get(AutoBlockService);
    svc.now = () => clock;
    await svc.reload();
  });
  afterAll(async () => {
    await h?.close();
  });

  it('blocks a source after the threshold of failed logins', async () => {
    for (let i = 0; i < 3; i++) await failLogin(h, BAD);

    const r = await h.call(admin, 'GET', '/api/v1/state/auto-block');
    expect(r.status, r.raw).toBe(200);
    const entry = r.body.items.find((e: { source: string }) => e.source === `${BAD}/32`);
    expect(entry, r.raw).toBeTruthy();
    expect(entry).toMatchObject({ reason: 'webLogin', origin: 'auto' });
    expect(entry.hits).toBeGreaterThanOrEqual(3);
  });

  it('never blocks an allow-listed source', async () => {
    for (let i = 0; i < 6; i++) await failLogin(h, GOOD);
    const r = await h.call(admin, 'GET', '/api/v1/state/auto-block');
    expect(r.body.items.some((e: { source: string }) => e.source.startsWith('198.51.100.'))).toBe(
      false,
    );
  });

  it('lets an admin unblock, and refuses an operator', async () => {
    const forbidden = await h.call(op, 'POST', '/api/v1/actions/auto-block/unblock', {
      source: BAD,
    });
    expect(forbidden.status).toBe(403);

    const ok = await h.call(admin, 'POST', '/api/v1/actions/auto-block/unblock', { source: BAD });
    expect(ok.status, ok.raw).toBe(200);
    expect(ok.body).toEqual({ unblocked: true });

    const gone = await h.call(admin, 'GET', '/api/v1/state/auto-block');
    expect(gone.body.items.some((e: { source: string }) => e.source === `${BAD}/32`)).toBe(false);

    const again = await h.call(admin, 'POST', '/api/v1/actions/auto-block/unblock', { source: BAD });
    expect(again.status).toBe(404);
  });

  it('expires a block after the block window', async () => {
    for (let i = 0; i < 3; i++) await failLogin(h, BAD);
    let r = await h.call(admin, 'GET', '/api/v1/state/auto-block');
    expect(r.body.items.some((e: { source: string }) => e.source === `${BAD}/32`)).toBe(true);

    // advance past the 900s block and sweep
    clock += 901_000;
    await svc.sweep();

    r = await h.call(admin, 'GET', '/api/v1/state/auto-block');
    expect(r.body.items.some((e: { source: string }) => e.source === `${BAD}/32`)).toBe(false);
  });

  it('lets an admin block by hand but refuses an allow-listed source', async () => {
    const manual = await h.call(admin, 'POST', '/api/v1/actions/auto-block/block', {
      source: '203.0.113.200',
      note: 'seen scanning',
    });
    expect(manual.status, manual.raw).toBe(200);
    expect(manual.body).toMatchObject({ source: '203.0.113.200/32', origin: 'manual', note: 'seen scanning' });

    const refused = await h.call(admin, 'POST', '/api/v1/actions/auto-block/block', {
      source: '198.51.100.5',
    });
    expect(refused.status).toBe(409);
  });
});
