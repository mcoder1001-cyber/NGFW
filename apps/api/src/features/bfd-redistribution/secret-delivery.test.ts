import { DesiredState } from '@ngfw/proto';
import { createCipheriv, randomBytes } from 'node:crypto';
import { mkdtempSync, writeFileSync, rmSync } from 'node:fs';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
import { afterEach, expect, it, vi } from 'vitest';
import type { Db } from '../../db/db.js';
import { testEnv } from '../../testing/fixtures.js';
import { SecretDeliveryService } from '../../secrets/secret-delivery.service.js';
const dirs: string[] = [];
afterEach(() => dirs.splice(0).forEach((d) => rmSync(d, { recursive: true, force: true })));
function fixture(material: string) {
  const dir = mkdtempSync(join(tmpdir(), 'ngfw-bfd-secret-'));
  dirs.push(dir);
  const key = randomBytes(32);
  const file = join(dir, 'master');
  writeFileSync(file, key, { mode: 0o600 });
  const ref = 'key/bfd-test';
  const iv = randomBytes(12);
  const cipher = createCipheriv('aes-256-gcm', key, iv);
  cipher.setAAD(Buffer.from(ref));
  const ct = Buffer.concat([cipher.update(material), cipher.final()]);
  const where = vi
    .fn()
    .mockResolvedValue([
      {
        ref,
        kind: 'key',
        version: 2,
        ciphertext: Buffer.concat([iv, cipher.getAuthTag(), ct]).toString('base64'),
      },
    ]);
  const delivery = new SecretDeliveryService(
    { select: () => ({ from: () => ({ where }) }) } as unknown as Db,
    { ...testEnv(), NGFW_SECRET_KEY_FILE: file },
  );
  const state = DesiredState.fromJSON({
    routing: {
      bfd: {
        sessions: [
          {
            interface: 'loop0',
            localAddress: '10.14.1.1',
            peerAddress: '10.14.1.2',
            auth: { type: 'keyed-sha1', keyId: 1, keyRef: ref },
          },
        ],
      },
    },
  });
  return { delivery, state, ref };
}
it('delivers BFD key references through the sealed channel with the exact version', async () => {
  const { delivery, state, ref } = fixture('NGFW_TEST_PSK_bfd');
  const r = await delivery.resolveVersioned(state);
  expect(r.bundle.values[ref]).toEqual(Buffer.from('NGFW_TEST_PSK_bfd'));
  expect(r.versions[ref]).toBe(2);
});
it('rejects oversized BFD material without exposing it', async () => {
  const secret = 'oversized-test-material-123';
  const { delivery, state } = fixture(secret);
  await expect(delivery.resolve(state)).rejects.toThrow();
  try {
    await delivery.resolve(state);
  } catch (e) {
    expect(String(e)).not.toContain(secret);
  }
});
