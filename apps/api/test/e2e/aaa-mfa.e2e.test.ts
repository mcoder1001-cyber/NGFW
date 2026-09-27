import { eq } from 'drizzle-orm';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { hashPassword } from '../../src/auth/password.js';
import { appUser } from '../../src/db/schema.js';
import { totpCode } from '../../src/features/aaa/totp.js';
import { startHarness, type Harness } from '../support/harness.js';
import { startRadius, type RadiusTestServer } from '../support/radius-server.js';

/**
 * F-aaa-login (increment 2): TOTP enrolment and the two-step login driven by `management.aaa.mfa.required`.
 *
 * Covers: self-service enrolment (nothing active until a code proves it, and the seed never stored in clear); the
 * login challenge and its bounded retries; single-use tickets; recovery codes (shown once, spent on use); forced
 * enrolment when the policy requires MFA and the account has none; `admins` vs `all` scope; an admin reset;
 * self-disable needing a current code; and an externally authenticated identity covered exactly like a local one.
 */
const MP = { 'content-type': 'application/merge-patch+json' };
const RSECRET = 'e2e-mfa-radius';
const ANA_PW = 'Ana-admin-pw-1234567890';
const OP_PW = 'Op1-operator-pw-1234567890';

describe('F-aaa-mfa e2e (TOTP enrolment, two-step login)', () => {
  let h: Harness;
  let admin: string;
  let radius: RadiusTestServer;
  /** Captured as the accounts enrol, and used by the login tests that follow. */
  let anaSecret = '';
  let anaRecovery: string[] = [];
  let op1Secret = '';

  const setAaa = async (aaa: Record<string, unknown>) => {
    const p = await h.call(admin, 'PATCH', '/api/v1/config/management', { aaa }, MP);
    expect(p.status, p.raw).toBe(200);
    const c = await h.call(admin, 'POST', '/api/v1/config/commit?comment=mfa');
    expect(c.status, c.raw).toBe(200);
  };
  const login = (username: string, password: string) =>
    h.call(undefined, 'POST', '/api/v1/auth/login', { username, password });
  const second = (ticket: string, code: string) =>
    h.call(undefined, 'POST', '/api/v1/auth/login/mfa', { ticket, code });
  const row = async (username: string) =>
    (await h.db.select().from(appUser).where(eq(appUser.username, username)))[0];
  /** Log in an account that owes a code, using its TOTP secret. */
  const loginWithCode = async (username: string, password: string, secret: string) => {
    const first = await login(username, password);
    expect(first.body.mfa, first.raw).toBe('code');
    const done = await second(first.body.ticket as string, totpCode(secret));
    expect(done.status, done.raw).toBe(200);
    return done.body.accessToken as string;
  };

  beforeAll(async () => {
    // this file logs in many times over (each step of the two-step flow), so the per-client login budget is raised
    // as auth.e2e does — the limit itself is exercised there, not here.
    h = await startHarness({ VRX_LOGIN_RATE_PER_MIN: '200' });
    admin = await h.login('admin', h.adminPassword);
    radius = await startRadius(RSECRET, { bob: { password: 'bob-pw', groups: ['netadmins'] } });
    const s = await h.call(admin, 'POST', '/api/v1/secrets', {
      kind: 'psk',
      name: 'radius',
      value: RSECRET,
    });
    expect(s.status, s.raw).toBe(200);
    const put = await h.call(admin, 'PUT', '/api/v1/config/management/users', [
      { username: 'admin', role: 'admin' },
      { username: 'ana', role: 'admin', passwordHash: await hashPassword(ANA_PW) },
      { username: 'op1', role: 'operator', passwordHash: await hashPassword(OP_PW) },
    ]);
    expect(put.status, put.raw).toBe(200);
    expect((await h.call(admin, 'POST', '/api/v1/config/commit?comment=users')).status).toBe(200);
  });
  afterAll(async () => {
    radius?.close();
    await h?.close();
  });

  describe('self-service enrolment', () => {
    let ana: string;
    beforeAll(async () => {
      ana = await h.login('ana', ANA_PW);
    });

    it('reports the unenrolled state', async () => {
      const r = await h.call(ana, 'GET', '/api/v1/auth/mfa');
      expect(r.status, r.raw).toBe(200);
      expect(r.body).toMatchObject({
        enrolled: false,
        required: false,
        pending: false,
        recoveryCodesLeft: 0,
      });
    });

    it('a started enrolment is not active until a code proves it', async () => {
      const start = await h.call(ana, 'POST', '/api/v1/auth/mfa/enroll');
      expect(start.status, start.raw).toBe(200);
      expect(start.body.otpauthUri).toContain('otpauth://totp/vrx%3Aana?');
      anaSecret = start.body.secret as string;
      expect((await h.call(ana, 'GET', '/api/v1/auth/mfa')).body).toMatchObject({
        enrolled: false,
        pending: true,
      });
      // the seed is stored encrypted, never in clear, and is not active yet
      const r = await row('ana');
      expect(r?.mfaPendingSecret).toBeTypeOf('string');
      expect(r?.mfaPendingSecret).not.toContain(anaSecret);
      expect(r?.mfaSecret).toBeNull();
      // a wrong code does not activate it
      const bad = await h.call(ana, 'POST', '/api/v1/auth/mfa/verify', { code: '000000' });
      expect(bad.status, bad.raw).toBe(403);
      expect((await h.call(ana, 'GET', '/api/v1/auth/mfa')).body.enrolled).toBe(false);
      // the right code does, and mints the recovery codes
      const ok = await h.call(ana, 'POST', '/api/v1/auth/mfa/verify', {
        code: totpCode(anaSecret),
      });
      expect(ok.status, ok.raw).toBe(200);
      anaRecovery = ok.body.recoveryCodes as string[];
      expect(anaRecovery).toHaveLength(10);
      expect(new Set(anaRecovery).size).toBe(10);
      expect((await h.call(ana, 'GET', '/api/v1/auth/mfa')).body).toMatchObject({
        enrolled: true,
        pending: false,
        recoveryCodesLeft: 10,
      });
    });

    it('an active factor is never replaced silently', async () => {
      const again = await h.call(ana, 'POST', '/api/v1/auth/mfa/enroll');
      expect(again.status, again.raw).toBe(409);
    });
  });

  describe('two-step login (required: admins)', () => {
    beforeAll(async () => {
      await setAaa({ mfa: { required: 'admins', issuer: 'vrx' } });
    });

    it('an operator is not challenged', async () => {
      const r = await login('op1', OP_PW);
      expect(r.status, r.raw).toBe(200);
      expect(r.body.accessToken).toBeTypeOf('string');
      expect(r.body.mfa).toBeUndefined();
    });

    it('an enrolled admin gets a challenge with no token, then a session', async () => {
      const first = await login('ana', ANA_PW);
      expect(first.status, first.raw).toBe(200);
      expect(first.body).toMatchObject({ mfa: 'code', expiresIn: 180, attemptsLeft: 3 });
      expect(first.body.accessToken).toBeUndefined();
      expect(first.headers['set-cookie']).toBeUndefined();
      const done = await second(first.body.ticket as string, totpCode(anaSecret));
      expect(done.status, done.raw).toBe(200);
      expect(done.body.user).toMatchObject({ username: 'ana', role: 'admin' });
      expect(done.body.accessToken).toBeTypeOf('string');
    });

    it('a wrong code costs one of three tries, and a ticket is single-use', async () => {
      const first = await login('ana', ANA_PW);
      const t1 = first.body.ticket as string;
      const r1 = await second(t1, '000000');
      expect(r1.status, r1.raw).toBe(200);
      expect(r1.body).toMatchObject({ mfa: 'code', attemptsLeft: 2 });
      expect(r1.body.ticket).not.toBe(t1);
      // the spent ticket cannot be replayed, not even with a good code
      const replay = await second(t1, totpCode(anaSecret));
      expect(replay.status, replay.raw).toBe(401);
      const r2 = await second(r1.body.ticket as string, '000000');
      expect(r2.body).toMatchObject({ mfa: 'code', attemptsLeft: 1 });
      // the third wrong code ends the login: the password has to be presented again
      const r3 = await second(r2.body.ticket as string, '000000');
      expect(r3.status, r3.raw).toBe(401);
    });

    it('a recovery code logs in once and is then spent', async () => {
      const code = anaRecovery[0] as string;
      const first = await login('ana', ANA_PW);
      const ok = await second(first.body.ticket as string, code);
      expect(ok.status, ok.raw).toBe(200);
      expect(ok.body.accessToken).toBeTypeOf('string');
      // the same code again is refused (it counts as a wrong code, not a login)
      const again = await login('ana', ANA_PW);
      const reuse = await second(again.body.ticket as string, code);
      expect(reuse.status, reuse.raw).toBe(200);
      expect(reuse.body).toMatchObject({ mfa: 'code', attemptsLeft: 2 });
      const token = await loginWithCode('ana', ANA_PW, anaSecret);
      expect((await h.call(token, 'GET', '/api/v1/auth/mfa')).body.recoveryCodesLeft).toBe(9);
    });
  });

  describe('forced enrolment (required: all, no factor yet)', () => {
    beforeAll(async () => {
      await setAaa({ mfa: { required: 'all', issuer: 'vrx' } });
    });

    it('an unenrolled user gets an enrol ticket, enrols, and lands in a session', async () => {
      const first = await login('op1', OP_PW);
      expect(first.status, first.raw).toBe(200);
      expect(first.body).toMatchObject({ mfa: 'enrol' });
      expect(first.body.accessToken).toBeUndefined();
      // an enrol ticket buys enrolment and nothing else
      const wrongRoute = await second(first.body.ticket as string, '000000');
      expect(wrongRoute.status, wrongRoute.raw).toBe(401);
      const again = await login('op1', OP_PW);
      const start = await h.call(undefined, 'POST', '/api/v1/auth/login/mfa/enroll', {
        ticket: again.body.ticket,
      });
      expect(start.status, start.raw).toBe(200);
      expect(start.body.otpauthUri).toContain('otpauth://totp/vrx%3Aop1?');
      op1Secret = start.body.secret as string;
      const done = await h.call(undefined, 'POST', '/api/v1/auth/login/mfa/verify', {
        ticket: start.body.ticket,
        code: totpCode(op1Secret),
      });
      expect(done.status, done.raw).toBe(200);
      expect(done.body.recoveryCodes).toHaveLength(10);
      expect(done.body.user).toMatchObject({ username: 'op1', role: 'operator' });
      expect(done.body.accessToken).toBeTypeOf('string');
      // from now on op1 is challenged for a code, not for enrolment
      expect((await login('op1', OP_PW)).body).toMatchObject({ mfa: 'code' });
    });

    it('an externally authenticated identity is covered too', async () => {
      await setAaa({
        order: ['local', 'radius'],
        radius: {
          servers: [
            { address: '127.0.0.1', authPort: radius.port, secretRef: 'psk/radius', timeoutSec: 2 },
          ],
        },
        roleMap: [{ group: 'netadmins', role: 'operator' }],
        mfa: { required: 'all', issuer: 'vrx' },
      });
      const first = await login('bob', 'bob-pw');
      expect(first.status, first.raw).toBe(200);
      expect(first.body).toMatchObject({ mfa: 'enrol' });
      // the shadow account exists and carries no password hash
      expect(await row('bob')).toMatchObject({ source: 'aaa:radius', passwordHash: null });
      const start = await h.call(undefined, 'POST', '/api/v1/auth/login/mfa/enroll', {
        ticket: first.body.ticket,
      });
      expect(start.status, start.raw).toBe(200);
      const done = await h.call(undefined, 'POST', '/api/v1/auth/login/mfa/verify', {
        ticket: start.body.ticket,
        code: totpCode(start.body.secret as string),
      });
      expect(done.status, done.raw).toBe(200);
      expect(done.body.user).toMatchObject({ username: 'bob', role: 'operator' });
    });
  });

  describe('administration', () => {
    beforeAll(async () => {
      await setAaa({ mfa: { required: 'admins', issuer: 'vrx' } });
    });

    it('an admin reset sends the user back to enrolment', async () => {
      const r = await h.call(admin, 'POST', '/api/v1/actions/aaa/mfa/reset', { username: 'ana' });
      expect(r.status, r.raw).toBe(204);
      expect((await login('ana', ANA_PW)).body).toMatchObject({ mfa: 'enrol' });
      expect(await row('ana')).toMatchObject({ mfaSecret: null, mfaEnrolledAt: null });
    });

    it('reset is admin-only, and 404 for an unknown user', async () => {
      // 'admins' is in force and op1 is an operator, so it gets a session straight away
      const f = await login('op1', OP_PW);
      const op = f.body.accessToken as string;
      expect(op, f.raw).toBeTypeOf('string');
      const forbidden = await h.call(op, 'POST', '/api/v1/actions/aaa/mfa/reset', {
        username: 'ana',
      });
      expect(forbidden.status, forbidden.raw).toBe(403);
      const missing = await h.call(admin, 'POST', '/api/v1/actions/aaa/mfa/reset', {
        username: 'nobody',
      });
      expect(missing.status, missing.raw).toBe(404);
    });

    it('turning MFA off needs a current code', async () => {
      const f = await login('op1', OP_PW);
      const token = f.body.accessToken as string;
      const bad = await h.call(token, 'DELETE', '/api/v1/auth/mfa', { code: '000000' });
      expect(bad.status, bad.raw).toBe(403);
      const ok = await h.call(token, 'DELETE', '/api/v1/auth/mfa', { code: totpCode(op1Secret) });
      expect(ok.status, ok.raw).toBe(204);
      expect((await h.call(token, 'GET', '/api/v1/auth/mfa')).body.enrolled).toBe(false);
    });
  });
});
