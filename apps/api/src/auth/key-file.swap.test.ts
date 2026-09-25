import { randomBytes } from 'node:crypto';
import * as fs from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { afterAll, describe, expect, it, vi } from 'vitest';
import { Bus } from '../infra/bus.js';
import type { Valkey } from '../infra/valkey.js';
import { testEnv } from '../testing/fixtures.js';
import { TokensService } from './tokens.service.js';

/**
 * Review L7: the key file is checked and read through ONE descriptor. Simulated path swap: the check by path (lstat)
 * sees a clean 0600 file, but what is read at that path is a 0644 file — a check by path followed by a read by path
 * accepts it; a check of the descriptor that is read refuses it.
 */
vi.mock('node:fs', async (importOriginal) => {
  const m = await importOriginal<typeof import('node:fs')>();
  return { ...m, lstatSync: vi.fn(m.lstatSync) };
});

describe('key file: check and read are one descriptor (review L7)', () => {
  const dir = fs.mkdtempSync(join(tmpdir(), 'vrx-td10b-swap-'));
  afterAll(() => fs.rmSync(dir, { recursive: true, force: true }));

  it('a file swapped in after a clean check by path is refused', () => {
    const clean = join(dir, 'clean');
    const swapped = join(dir, 'swapped');
    fs.writeFileSync(clean, `${randomBytes(36).toString('base64url')}\n`, { mode: 0o600 });
    fs.writeFileSync(swapped, `${randomBytes(36).toString('base64url')}\n`, { mode: 0o644 });
    fs.chmodSync(swapped, 0o644);
    const real = vi.mocked(fs.lstatSync).getMockImplementation()!;
    // the path check sees the clean file (the state before the swap) …
    vi.mocked(fs.lstatSync).mockImplementation(((p: fs.PathLike, o?: unknown) =>
      String(p) === swapped ? real(clean) : real(p, o as never)) as typeof fs.lstatSync);
    // … and the file that is actually read is group/world-readable
    expect(
      () => new TokensService(testEnv({ VRX_JWT_KEY_FILE: swapped }), {} as Valkey, new Bus()),
    ).toThrow(/mode 0644 lets group\/others access it/);
  });
});
