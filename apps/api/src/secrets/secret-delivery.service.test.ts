import {
  buildCsr,
  generateKey,
  normaliseKeySpec,
  parseCsr,
  parseDn,
  selfSignedCa,
  signCsr,
  toPem,
} from '../features/pki/x509.js';
import { DesiredState } from '@ngfw/proto';
import { createCipheriv, randomBytes } from 'node:crypto';
import { mkdtempSync, writeFileSync, rmSync } from 'node:fs';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
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
  const dir = mkdtempSync(join(tmpdir(), 'ngfw-secret-delivery-test-'));
  dirs.push(dir);
  const key = randomBytes(32);
  const file = join(dir, 'master');
  writeFileSync(file, key, { mode: 0o600 });
  const encrypt = (plain: string, associatedRef = ref) => {
    const iv = randomBytes(12);
    const c = createCipheriv('aes-256-gcm', key, iv);
    c.setAAD(Buffer.from(associatedRef));
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

describe('operational secret delivery', () => {
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
    await expect(delivery.resolve(state())).rejects.toThrow('operational secret is unavailable');
    await expect(delivery.resolve(state('key/native'))).rejects.toThrow(
      'operational secret kind is invalid',
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
      expect(String(e)).toContain('operational secret cannot be decrypted');
      expect(String(e)).not.toContain(payload);
    }
    expect(log).not.toHaveBeenCalled();
    expect(error).not.toHaveBeenCalled();
    log.mockRestore();
    error.mockRestore();
  });
});

describe('operational PKI selection', () => {
  it('delivers certificate and operational key, excludes CA signing keys, CSR-only keys and unrelated secrets', async () => {
    const { delivery, where, encrypt } = setup([]);
    const ds = DesiredState.fromJSON({
      vpn: {
        pki: {
          cas: { root: { certificateRef: 'cert/root' } },
          certificates: {
            gateway: { certificateRef: 'cert/gateway', privateKeyRef: 'key/gateway' },
            pending: { privateKeyRef: 'key/pending' },
          },
        },
      },
      management: { aaa: { radius: { servers: [{ secretRef: 'key/unrelated' }] } } },
    });
    const caKey = generateKey(normaliseKeySpec({ type: 'ecdsa', curve: 'p256' }));
    const ca = selfSignedCa(parseDn('CN=Root'), caKey, 365);
    const leafKey = generateKey(normaliseKeySpec({ type: 'ecdsa', curve: 'p256' }));
    const csr = parseCsr(
      toPem('CERTIFICATE REQUEST', buildCsr(parseDn('CN=Gateway'), [], leafKey)),
    );
    const leaf = signCsr(csr, { facts: ca.facts, key: caKey.privateKey }, { days: 30 });
    const payloads: Record<string, string> = {
      'cert/root': ca.pem,
      'cert/gateway': leaf.pem,
      'key/gateway': 'operational-key-test',
    };
    const selected = ['cert/root', 'cert/gateway', 'key/gateway'];
    where.mockResolvedValue([]);
    for (const r of selected)
      where.mockResolvedValueOnce([
        { ref: r, kind: r.split('/')[0], version: 3, ciphertext: encrypt(payloads[r]!, r) },
      ]);
    const got = await delivery.resolveVersioned(ds);
    expect(Object.keys(got.bundle.values)).toEqual(selected);
    expect(where).toHaveBeenCalledTimes(4);
    expect(got.versions).toEqual(Object.fromEntries(selected.map((r) => [r, 3])));
  });
  it('rejects mixed kind and oversized material without exposing decrypted bytes', async () => {
    const { delivery, where, encrypt } = setup([]);
    const ds = DesiredState.fromJSON({
      vpn: {
        pki: {
          certificates: {
            gateway: { certificateRef: 'cert/gateway', privateKeyRef: 'cert/gateway' },
          },
        },
      },
    });
    await expect(delivery.resolve(ds)).rejects.toThrow('operational secret kind is invalid');
    expect(where).not.toHaveBeenCalled();
    const oversized = 'x'.repeat(65537);
    where.mockResolvedValueOnce([
      { kind: 'cert', version: 1, ciphertext: encrypt(oversized, 'cert/root') },
    ]);
    await expect(
      delivery.resolve(
        DesiredState.fromJSON({ vpn: { pki: { cas: { root: { certificateRef: 'cert/root' } } } } }),
      ),
    ).rejects.toThrow('transport limit');
  });
});

it('refuses configured CA signing key reuse as an operational key before decrypting it', async () => {
  const { delivery, where, encrypt } = setup([]);
  where.mockResolvedValueOnce([
    { kind: 'cert', version: 1, ciphertext: encrypt('public', 'cert/root') },
  ]);
  where.mockResolvedValueOnce([
    { kind: 'cert', version: 1, ciphertext: encrypt('public', 'cert/gateway') },
  ]);
  const ds = DesiredState.fromJSON({
    vpn: {
      pki: {
        cas: { root: { certificateRef: 'cert/root' } },
        certificates: { gateway: { certificateRef: 'cert/gateway', privateKeyRef: 'key/root' } },
      },
    },
  });
  await expect(delivery.resolve(ds)).rejects.toThrow('kind is invalid');
  expect(where).toHaveBeenCalledTimes(2);
});

it('rejects a CA certificate hidden in operational inventory before reading its signing key', async () => {
  const { delivery, where, encrypt } = setup([]);
  const key = generateKey(normaliseKeySpec({ type: 'ecdsa', curve: 'p256' }));
  const ca = selfSignedCa(parseDn('CN=Hidden CA'), key, 365);
  where.mockResolvedValueOnce([
    { kind: 'cert', version: 1, ciphertext: encrypt(ca.pem, 'cert/alias') },
  ]);
  await expect(
    delivery.resolve(
      DesiredState.fromJSON({
        vpn: {
          pki: {
            certificates: { alias: { certificateRef: 'cert/alias', privateKeyRef: 'key/alias' } },
          },
        },
      }),
    ),
  ).rejects.toThrow('CA signing keys cannot be delivered');
  expect(where).toHaveBeenCalledTimes(1);
});
it('does not add a newly available implicit CRL to a revision that had no CRL version', async () => {
  const { delivery, where, encrypt } = setup([]);
  where.mockResolvedValueOnce([
    { kind: 'cert', version: 1, ciphertext: encrypt('public', 'cert/root') },
  ]);
  where.mockResolvedValueOnce([{ version: 1, ciphertext: encrypt('public', 'cert/root') }]);
  const result = await delivery.resolveVersioned(
    DesiredState.fromJSON({ vpn: { pki: { cas: { root: { certificateRef: 'cert/root' } } } } }),
    { 'cert/root': 1 },
  );
  expect(Object.keys(result.bundle.values)).toEqual(['cert/root']);
  expect(where).toHaveBeenCalledTimes(2);
});

describe('IS-IS transaction-selected password delivery', () => {
  it('delivers only the two configured routing authentication leaves', async () => {
    const { delivery, where, encrypt } = setup([]);
    const passwordRef = 'password/isis';
    where.mockResolvedValueOnce([
      {
        kind: 'password',
        ref: passwordRef,
        version: 3,
        ciphertext: encrypt('NGFW_TEST_PSK_isis', passwordRef),
      },
    ]);
    const ds = DesiredState.fromJSON({
      routing: {
        isis: {
          areaPasswordRef: passwordRef,
          domainPasswordRef: passwordRef,
        },
      },
      management: { aaa: { radius: { servers: [{ secretRef: 'psk/other' }] } } },
    });
    const result = await delivery.resolveVersioned(ds);
    expect(result.bundle.values[passwordRef]).toEqual(Buffer.from('NGFW_TEST_PSK_isis'));
    expect(result.versions).toEqual({ [passwordRef]: 3 });
    expect(where).toHaveBeenCalledTimes(1);
  });
  it('refuses a key reference at a routing password leaf before database access', async () => {
    const { delivery, select } = setup([]);
    await expect(
      delivery.resolve(
        DesiredState.fromJSON({
          routing: {
            isis: {
              areaPasswordRef: 'key/ca',
            },
          },
        }),
      ),
    ).rejects.toThrow('operational secret kind is invalid');
    expect(select).not.toHaveBeenCalled();
  });
});

it('delivers native local key and public-only peer certificate without inventing a peer key', async () => {
  const { delivery, where, encrypt } = setup([]);
  const caKey = generateKey(normaliseKeySpec({ type: 'rsa', bits: 2048 }));
  const ca = selfSignedCa(parseDn('CN=Root'), caKey, 365);
  const leafKey = generateKey(normaliseKeySpec({ type: 'rsa', bits: 2048 }));
  const csr = parseCsr(toPem('CERTIFICATE REQUEST', buildCsr(parseDn('CN=Leaf'), [], leafKey)));
  const leaf = signCsr(csr, { facts: ca.facts, key: caKey.privateKey }, { days: 30 });
  const ds = DesiredState.fromJSON({
    vpn: {
      ipsec: {
        tunnels: {
          native: {
            enabled: true,
            engine: 'vpp-ikev2',
            auth: { method: 'cert', certificate: 'local', peerCertificate: 'peer' },
          },
        },
      },
      pki: {
        certificates: {
          local: { certificateRef: 'cert/local', privateKeyRef: 'key/local' },
          peer: { certificateRef: 'cert/peer' },
        },
      },
    },
  });
  const payloads: Record<string, string> = {
    'cert/local': leaf.pem,
    'key/local': 'operational-key-test',
    'cert/peer': leaf.pem,
  };
  where.mockResolvedValue([]);
  for (const [ref, payload] of Object.entries(payloads).sort(
    ([a], [b]) => Number(a.startsWith('key/')) - Number(b.startsWith('key/')),
  ))
    where.mockResolvedValueOnce([
      { ref, kind: ref.split('/')[0], version: 1, ciphertext: encrypt(payload, ref) },
    ]);
  const got = await delivery.resolveVersioned(ds);
  expect(Object.keys(got.bundle.values).sort()).toEqual(Object.keys(payloads).sort());
  expect(got.bundle.values['key/peer']).toBeUndefined();
});

describe('OSPF/RIP transaction-selected MD5 password delivery', () => {
  it.each(['ospf', 'rip'] as const)(
    'delivers %s keys at escaped interface pointers',
    async (protocol) => {
      const { delivery, where, encrypt } = setup([]);
      const ref = 'password/igp';
      where.mockResolvedValueOnce([
        { kind: 'password', ref, version: 3, ciphertext: encrypt('NGFW_TEST_PSK_igp', ref) },
      ]);
      const ds = DesiredState.fromJSON({
        routing: {
          [protocol]: {
            interfaces: { 'host/0': { auth: { type: 'md5', keyId: 7, keyRef: ref } } },
          },
        },
      });
      const result = await delivery.resolveVersioned(ds);
      expect(result.bundle.values[ref]).toEqual(Buffer.from('NGFW_TEST_PSK_igp'));
      expect(result.versions).toEqual({ [ref]: 3 });
      expect(where).toHaveBeenCalledTimes(1);
    },
  );
  it('does not deliver disabled, unsupported or RIPng authentication leaves', async () => {
    const { delivery, select } = setup([]);
    const ds = DesiredState.fromJSON({
      routing: {
        rip: { interfaces: { loop0: { auth: { type: 'none', keyRef: 'password/disabled' } } } },
        ripng: { interfaces: { loop1: { auth: { type: 'md5', keyRef: 'password/unsupported' } } } },
      },
    });
    expect((await delivery.resolve(ds)).values).toEqual({});
    expect(select).not.toHaveBeenCalled();
  });
  it.each(['ospf', 'rip'] as const)(
    'rejects non-password %s keys before DB access',
    async (protocol) => {
      const { delivery, select } = setup([]);
      const ds = DesiredState.fromJSON({
        routing: {
          [protocol]: {
            interfaces: { loop0: { auth: { type: 'md5', keyId: 1, keyRef: 'key/ca' } } },
          },
        },
      });
      await expect(delivery.resolve(ds)).rejects.toThrow('operational secret kind is invalid');
      expect(select).not.toHaveBeenCalled();
    },
  );
});
