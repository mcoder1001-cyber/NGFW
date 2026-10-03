import { DesiredState } from '@ngfw/proto';
import { createCipheriv, randomBytes } from 'node:crypto';
import { mkdtempSync, writeFileSync, rmSync } from 'node:fs';
import { join } from 'node:path';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Db } from '../db/db.js';
import { testEnv } from '../testing/fixtures.js';
import { SecretDeliveryService } from './secret-delivery.service.js';

const ref = 'psk/native';
const state = (secretRef = ref) =>
  DesiredState.fromJSON({
    vpn: {
      ipsec: {
        tunnels: {
          site: { engine: 'vpp-ikev2', auth: { method: 'psk', secretRef } },
        },
      },
    },
  });
const dirs: string[] = [];
afterEach(() => dirs.splice(0).forEach((d) => rmSync(d, { recursive: true, force: true })));

function setup(rows: unknown[][], payload = randomBytes(24).toString('hex')) {
  const dir = mkdtempSync('/run/ngfw-secret-delivery-test-');
  dirs.push(dir);
  const key = randomBytes(32);
  const file = join(dir, 'master');
  writeFileSync(file, key, { mode: 0o600 });
  const encrypt = (plain: string) => {
    const iv = randomBytes(12);
    const c = createCipheriv('aes-256-gcm', key, iv);
    c.setAAD(Buffer.from(ref));
    const ct = Buffer.concat([c.update(plain), c.final()]);
    return Buffer.concat([iv, c.getAuthTag(), ct]).toString('base64');
  };
  const where = vi.fn();
  rows.forEach((r) => where.mockResolvedValueOnce(r));
  const select = vi.fn(() => ({ from: () => ({ where }) }));
  const delivery = new SecretDeliveryService({ select } as unknown as Db, {
    ...testEnv(),
    NGFW_SECRET_KEY_FILE: file,
  });
  return { delivery, select, where, encrypt, payload };
}

describe('native IPsec secret delivery', () => {
  it('does no secret reads for empty or unsupported reference domains', async () => {
    const { delivery, select } = setup([]);
    const ds = DesiredState.fromJSON({
      management: { aaa: { radius: { servers: [{ secretRef: ref }] } } },
    });
    expect(await delivery.resolve(ds)).toEqual({ values: {} });
    expect(select).not.toHaveBeenCalled();
  });

  it('decrypts current selected native PSK with its exact version', async () => {
    const { delivery, where, encrypt, payload } = setup([]);
    const current = { kind: 'psk', ref, version: 2, ciphertext: encrypt(payload) };
    where.mockResolvedValueOnce([current]);
    const got = await delivery.resolveVersioned(state());
    expect(got.bundle.values[ref]).toEqual(Buffer.from(payload));
    expect(got.versions).toEqual({ [ref]: 2 });
  });

  it('delivers revision-pinned material rather than the current rotated value', async () => {
    const { delivery, where, encrypt } = setup([]);
    const old = randomBytes(24).toString('hex');
    where
      .mockResolvedValueOnce([
        { kind: 'psk', ref, version: 2, ciphertext: encrypt(randomBytes(24).toString('hex')) },
      ])
      .mockResolvedValueOnce([{ ref, version: 1, ciphertext: encrypt(old) }]);
    const got = await delivery.resolveVersioned(state(), { [ref]: 1 });
    expect(got.bundle.values[ref]).toEqual(Buffer.from(old));
    expect(got.versions[ref]).toBe(1);
  });

  it('rejects absent or wrong-kind references without returning raw material', async () => {
    const { delivery } = setup([[]]);
    await expect(delivery.resolve(state())).rejects.toThrow('native IPsec secret is unavailable');
    await expect(delivery.resolve(state('key/native'))).rejects.toThrow(
      'native IPsec secret kind is invalid',
    );
  });

  it('sanitizes decryption errors and never logs decrypted material', async () => {
    const { delivery, payload } = setup([
      [{ kind: 'psk', ref, version: 1, ciphertext: 'invalid' }],
    ]);
    const log = vi.spyOn(console, 'log');
    const error = vi.spyOn(console, 'error');
    try {
      await delivery.resolve(state());
      throw new Error('expected rejection');
    } catch (e) {
      expect(String(e)).toContain('native IPsec secret cannot be decrypted');
      expect(String(e)).not.toContain(payload);
    }
    expect(log).not.toHaveBeenCalled();
    expect(error).not.toHaveBeenCalled();
    log.mockRestore();
    error.mockRestore();
  });
});
