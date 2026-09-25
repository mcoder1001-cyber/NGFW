import { Logger } from '@nestjs/common';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { loadEnv } from '../config.js';
import type { Db } from '../db/db.js';
import { AuthService } from './auth.service.js';

/** A database with users already present: seedBootstrapAdmin only counts them and creates nobody. */
const populatedDb = { select: () => ({ from: async () => [{ n: 1 }] }) } as unknown as Db;
const service = (flag: '0' | '1') =>
  new AuthService(populatedDb, loadEnv({ VRX_DEV_WEAK_PASSWORDS: flag }), {} as never, {} as never, {} as never);

describe('VRX_DEV_WEAK_PASSWORDS boot warning', () => {
  afterEach(() => vi.restoreAllMocks());

  it('is logged at boot while the flag is on', async () => {
    const warn = vi.spyOn(Logger.prototype, 'warn').mockImplementation(() => undefined);
    await service('1').seedBootstrapAdmin();
    expect(warn).toHaveBeenCalledWith(expect.stringMatching(/VRX_DEV_WEAK_PASSWORDS is on/));
  });

  it('is not logged while the flag is off', async () => {
    const warn = vi.spyOn(Logger.prototype, 'warn').mockImplementation(() => undefined);
    await service('0').seedBootstrapAdmin();
    expect(warn).not.toHaveBeenCalled();
  });
});
