import { beforeAll, describe, expect, it, vi } from 'vitest';
import type { Db } from '../db/db.js';
import { ADMIN, testEnv } from '../testing/fixtures.js';
import { AuthService } from './auth.service.js';
import { hashPassword } from './password.js';

describe('setup bootstrap administrator', () => {
  let hash: string;
  const current = 'NGFW_TEST_PSK_setup_bootstrap';
  beforeAll(async () => {
    hash = await hashPassword(current);
  });
  function service(overrides: Record<string, unknown> = {}, rateHits = 1) {
    const where = vi.fn(async () => [
      {
        username: ADMIN.username,
        source: 'bootstrap',
        role: 'admin',
        disabled: false,
        hash,
        gen: 0,
        lockedUntil: null,
        ...overrides,
      },
    ]);
    const db = { select: () => ({ from: () => ({ where }) }) } as unknown as Db;
    const auth = new AuthService(
      db,
      testEnv(),
      {
        hit: async () => rateHits,
        sessionCurrent: () => true,
      } as never,
      {} as never,
      {} as never,
      {} as never,
      {} as never,
      {} as never,
      {} as never,
    );
    return { auth, where };
  }
  it('recognizes the initial admin only after a successful current-password check', async () => {
    const { auth, where } = service();
    expect(await auth.checkSetupPassword(ADMIN, current, true)).toBe(true);
    expect(where).toHaveBeenCalledTimes(2);
  });
  it.each([
    { source: 'config' },
    { source: 'external' },
    { disabled: true },
    { role: 'operator' },
    { username: 'another-admin' },
  ])('does not adopt an account with mismatched provenance or identity: %j', async (overrides) => {
    expect(await service(overrides).auth.isBootstrapAdmin(ADMIN)).toBe(false);
  });
  it('rejects API keys and insecure password submissions before checking accounts', async () => {
    const { auth, where } = service();
    await expect(auth.checkSetupPassword(ADMIN, current, false)).rejects.toThrow();
    await expect(
      auth.checkSetupPassword({ ...ADMIN, via: 'apikey' }, current, true),
    ).rejects.toThrow('interactive login');
    expect(where).not.toHaveBeenCalled();
  });
  it('retains password-check rate limiting', async () => {
    const { auth, where } = service({}, 1_000_000);
    await expect(auth.checkSetupPassword(ADMIN, current, true)).rejects.toThrow(
      'too many password checks',
    );
    expect(where).not.toHaveBeenCalled();
  });
});
