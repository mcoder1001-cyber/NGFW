import { describe, expect, it, vi } from 'vitest';
import type { DatastoreService } from '../datastore/datastore.service.js';
import { LoginBannerController } from './login-banner.controller.js';

describe('public login banner', () => {
  it('returns only committed banner text, preserving literal markup', async () => {
    const ds = {
      getRunning: vi
        .fn()
        .mockResolvedValue({
          doc: {
            system: { hostname: 'private', banner: { login: '<b>Notice</b>', motd: 'private' } },
            users: { admin: {} },
          },
        }),
    };
    expect(await new LoginBannerController(ds as unknown as DatastoreService).banner()).toEqual({
      banner: '<b>Notice</b>',
    });
    expect(ds.getRunning).toHaveBeenCalledTimes(1);
  });
  it('caps legacy text without exceeding the contract length without splitting surrogate pairs', async () => {
    const ds = {
      getRunning: async () => ({ doc: { system: { banner: { login: '😀'.repeat(5000) } } } }),
    };
    const result = await new LoginBannerController(ds as unknown as DatastoreService).banner();
    expect(Array.from(result.banner)).toHaveLength(2048);
    expect(result.banner).not.toContain('\uFFFD');
  });
  it('returns empty text without a configured banner and propagates datastore failure', async () => {
    const ds = { getRunning: vi.fn().mockResolvedValue({ doc: {} }) };
    const controller = new LoginBannerController(ds as unknown as DatastoreService);
    expect(await controller.banner()).toEqual({ banner: '' });
    ds.getRunning.mockRejectedValueOnce(new Error('offline'));
    await expect(controller.banner()).rejects.toThrow('offline');
  });
});
