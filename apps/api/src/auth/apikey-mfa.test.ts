import { describe, expect, it } from 'vitest';
import { loadEnv } from '../config.js';
import type { Db } from '../db/db.js';
import { AuthService } from './auth.service.js';

/** F-aaa-hardening: an API key minted without MFA does not bypass a policy raised since (refuse-at-use). */
const TOKEN = `vrxk_${'A'.repeat(43)}`;

function service(policy: () => Promise<unknown>, role: string, mfaVerified: boolean) {
  const row = {
    key: { id: 'k1', name: 'k', scopes: [role], expiresAt: null, mfaVerified },
    user: { id: 1, username: 'a', role, disabled: false },
  };
  const chain = {
    from: () => chain,
    innerJoin: () => chain,
    where: () => Promise.resolve([row]),
  };
  const db = {
    select: () => chain,
    update: () => ({ set: () => ({ where: () => Promise.resolve() }) }),
  };
  return new AuthService(
    db as unknown as Db,
    loadEnv({}),
    {} as never,
    {} as never,
    {} as never,
    {} as never,
    {} as never,
    { cachedPolicy: policy } as never,
    {} as never,
  );
}

const auth = (s: AuthService) => s.authenticate(`ApiKey ${TOKEN}`);
const pol = (mfaRequired: string) => () => Promise.resolve({ mfaRequired });

describe('API keys vs the MFA policy', () => {
  it('policy none: a key minted without MFA works', async () => {
    expect(await auth(service(pol('none'), 'admin', false))).toMatchObject({ via: 'apikey' });
  });
  it('policy admins: an admin key minted without MFA → 401 mfa-required', async () => {
    const err = await auth(service(pol('admins'), 'admin', false)).catch((e: unknown) => e);
    expect(err).toMatchObject({ slug: 'mfa-required' });
    expect((err as { getStatus(): number }).getStatus()).toBe(401);
  });
  it('policy admins: an operator key is not covered; an MFA-minted admin key works', async () => {
    expect(await auth(service(pol('admins'), 'operator', false))).toMatchObject({ id: 1 });
    expect(await auth(service(pol('all'), 'admin', true))).toMatchObject({ id: 1 });
  });
  it('policy unreadable: fails closed for keys minted without MFA only', async () => {
    const down = () => Promise.reject(new Error('db down'));
    await expect(auth(service(down, 'readonly', false))).rejects.toMatchObject({
      slug: 'mfa-required',
    });
    expect(await auth(service(down, 'readonly', true))).toMatchObject({ id: 1 });
  });
});
