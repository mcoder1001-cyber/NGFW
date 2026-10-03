import { X509Certificate } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { parsePkcs12, pkcs12Kdf } from './pkcs12.js';
import { openssl, scratch } from './testkit.test.js';
import {
  buildCsr,
  generateKey,
  parseCsr,
  parseDn,
  privateKeyPem,
  selfSignedCa,
  signCsr,
} from './x509.js';

const PASS = 'VRX_TEST_PSK_FPKI_p12'; // test fixture label, never a real passphrase

/** A CA and a gateway certificate it signed, written for openssl (key file 0600 in the scratch dir). */
function material(s: ReturnType<typeof scratch>) {
  const caKey = generateKey({ type: 'ecdsa', curve: 'p256' });
  const ca = selfSignedCa(parseDn('CN=w5 p12 CA'), caKey, 30);
  const key = generateKey({ type: 'ecdsa', curve: 'p256' });
  const cert = signCsr(
    parseCsr(buildCsr(parseDn('CN=gw.w5.example'), ['gw.w5.example'], key)),
    { facts: ca.facts, key: caKey.privateKey },
    { days: 10 },
  );
  return {
    ca,
    cert,
    key,
    files: {
      ca: s.file('ca.pem', ca.pem),
      cert: s.file('gw.pem', cert.pem),
      key: s.file('gw.key', privateKeyPem(key.privateKey)),
    },
  };
}

describe('F-pki PKCS#12 import (files made by openssl)', () => {
  it('RFC 7292 KDF matches a known vector (SHA-1, id 1)', () => {
    // RFC 7292 has no vectors; this one is from OpenSSL's test/pkcs12_format_test style: password "smeg", salt 0x0A58CF64530D823F, 1 iteration
    const pw = Buffer.from('0073006d006500670000', 'hex');
    const key = pkcs12Kdf('1.3.14.3.2.26', pw, Buffer.from('0A58CF64530D823F', 'hex'), 1, 1, 24);
    expect(key.toString('hex').toUpperCase()).toBe(
      '8AAAE6297B6CB04642AB5B077851284EB7128F1A2A7FBCA3',
    );
  });

  for (const variant of [
    { name: 'OpenSSL 3 default (PBES2 AES-256-CBC, PBKDF2, HMAC-SHA256 MAC)', args: [] },
    {
      name: 'PBE-SHA1-3DES with a SHA-1 MAC',
      args: ['-keypbe', 'PBE-SHA1-3DES', '-certpbe', 'PBE-SHA1-3DES', '-macalg', 'sha1'],
    },
    {
      name: 'AES-128 key, unencrypted certificates',
      args: ['-keypbe', 'AES-128-CBC', '-certpbe', 'NONE'],
    },
  ]) {
    it(`round trip: ${variant.name}`, () => {
      const s = scratch();
      try {
        const m = material(s);
        const p12 = s.dir + '/gw.p12';
        openssl([
          'pkcs12',
          '-export',
          '-in',
          m.files.cert,
          '-inkey',
          m.files.key,
          '-certfile',
          m.files.ca,
          '-name',
          'w5-gw',
          '-passout',
          `pass:${PASS}`,
          '-out',
          p12,
          ...variant.args,
        ]);
        const out = parsePkcs12(readFileSync(p12), PASS);
        expect(out.certs).toHaveLength(2);
        const leaf = out.certs
          .map((c) => new X509Certificate(c))
          .find((c) => c.checkPrivateKey(out.key));
        expect(leaf?.fingerprint256).toBe(m.cert.facts.fingerprint);
        expect(
          out.certs.some((c) => new X509Certificate(c).fingerprint256 === m.ca.facts.fingerprint),
        ).toBe(true);
      } finally {
        s.done();
      }
    });
  }

  it('wrong passphrase → refused without echoing it', () => {
    const s = scratch();
    try {
      const m = material(s);
      const p12 = s.dir + '/gw.p12';
      openssl([
        'pkcs12',
        '-export',
        '-in',
        m.files.cert,
        '-inkey',
        m.files.key,
        '-passout',
        `pass:${PASS}`,
        '-out',
        p12,
      ]);
      expect(() => parsePkcs12(readFileSync(p12), 'VRX_TEST_PSK_FPKI_wrong')).toThrow(
        /wrong passphrase/,
      );
      try {
        parsePkcs12(readFileSync(p12), 'VRX_TEST_PSK_FPKI_wrong');
      } catch (e) {
        expect(String(e)).not.toContain('VRX_TEST_PSK_FPKI_wrong');
      }
    } finally {
      s.done();
    }
  });

  it('a legacy RC2 file is refused with the re-export command', () => {
    const s = scratch();
    try {
      const m = material(s);
      const p12 = s.dir + '/gw.p12';
      try {
        openssl([
          'pkcs12',
          '-export',
          '-legacy',
          '-in',
          m.files.cert,
          '-inkey',
          m.files.key,
          '-passout',
          `pass:${PASS}`,
          '-out',
          p12,
        ]);
      } catch {
        return; // this openssl has no legacy provider either: nothing to test
      }
      expect(() => parsePkcs12(readFileSync(p12), PASS)).toThrow(/RC2.*re-export/);
    } finally {
      s.done();
    }
  });

  it('garbage and truncated files are refused cleanly', () => {
    expect(() => parsePkcs12(Buffer.from('not a pkcs12'), PASS)).toThrow(/malformed/);
    expect(() => parsePkcs12(Buffer.from([0x30, 0x82, 0xff, 0xff, 0x02]), PASS)).toThrow(
      /malformed/,
    );
  });
});
