import { describe, expect, it } from 'vitest';
import { loadEnv } from '../config.js';
import type { Db } from '../db/db.js';
import { AuthService } from './auth.service.js';

/** F-aaa-login review 7: when the MFA policy cannot be read, only a session that passed the second factor gets in. */
function service(policy: () => Promise<unknown>, mfaSid: string | null) {
  const kv = { get: async (k: string) => (k === `mfasid:${mfaSid}` ? '1' : null) };
  const tokens = {
    verifyAccess: async (t: string) => ({ id: 1, username: 'a', role: 'admin', sid: t, gen: 0 }),
    accessTtl: 900,
  };
  return new AuthService(
    {} as Db,
    loadEnv({}),
    tokens as never,
    {} as never,
    {} as never,
    kv as never,
    {} as never,
    { cachedPolicy: policy } as never,
    {} as never,
  );
}

describe('MFA gate fails closed', () => {
  it('policy unreadable: a session without MFA is refused, an MFA-verified one passes', async () => {
    const down = () => Promise.reject(new Error('db down'));
    expect(await service(down, 'good').authenticate('Bearer plain')).toBeNull();
    expect(await service(down, 'good').authenticate('Bearer good')).toMatchObject({ id: 1 });
  });
  it('policy none: no MFA needed', async () => {
    const none = () => Promise.resolve({ mfaRequired: 'none' });
    expect(await service(none, null).authenticate('Bearer plain')).toMatchObject({ id: 1 });
  });
  it('policy admins: an admin session without MFA is refused', async () => {
    const admins = () => Promise.resolve({ mfaRequired: 'admins' });
    expect(await service(admins, null).authenticate('Bearer plain')).toBeNull();
  });
});
