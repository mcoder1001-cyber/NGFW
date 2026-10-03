import { createCipheriv, createHmac } from 'node:crypto';
import type * as Crypto from 'node:crypto';
import { describe, expect, it, vi, afterEach } from 'vitest';
import * as a from './asn1.js';
import * as x from './x509.js';
import { MAX_PKCS12_KDF_ROUNDS, parsePkcs12, pkcs12Kdf } from './pkcs12.js';
import { MAX_OCSP_AGE_MS, ocspRequest, parseOcspResponse } from './revocation.js';

const mocks = vi.hoisted(() => ({ pbkdf2: vi.fn(() => Buffer.alloc(32)) }));
vi.mock('node:crypto', async () => ({
  ...(await vi.importActual<typeof Crypto>('node:crypto')),
  pbkdf2Sync: mocks.pbkdf2,
}));
afterEach(() => vi.clearAllMocks());

describe('DER parent boundaries', () => {
  it.each(['3006300204020500', '30053001000500', '300730030482010500'])(
    'rejects child crossing parent in %s',
    (hex) => {
      expect(() => a.parse(Buffer.from(hex, 'hex'))).toThrow(/truncated/);
    },
  );
  it('preserves adjacent valid nested elements', () => {
    const value = a.parse(a.seq(a.seq(a.octets(Buffer.from([5, 0]))), a.nul()));
    expect(value.children[0]!.children[0]!.value).toEqual(Buffer.from([5, 0]));
    expect(value.children[1]!.tag).toBe(a.TAG.NULL);
  });
});

const pass = 'NGFW_TEST_PSK_CRYPTO_REVIEW';
function pfx(safe: Buffer, iterations = 1, validMac = true): Buffer {
  const salt = Buffer.from([1]);
  const password = Buffer.from(pass + '\0', 'utf16le').swap16();
  const mac = validMac
    ? createHmac('sha256', pkcs12Kdf('2.16.840.1.101.3.4.2.1', password, salt, 3, 1, 32))
        .update(safe)
        .digest()
    : Buffer.alloc(32);
  return a.seq(
    a.int(3),
    a.seq(a.oid('1.2.840.113549.1.7.1'), a.explicit(0, a.octets(safe))),
    a.seq(
      a.seq(a.seq(a.oid('2.16.840.1.101.3.4.2.1'), a.nul()), a.octets(mac)),
      a.octets(salt),
      a.int(iterations),
    ),
  );
}
function encryptedPart(iterations: number): Buffer {
  const iv = Buffer.alloc(16);
  const cipher = createCipheriv('aes-256-cbc', Buffer.alloc(32), iv);
  const ciphertext = Buffer.concat([cipher.update(a.seq()), cipher.final()]);
  // Omitted PRF defaults to SHA1: AES256 requires two PBKDF2 digest blocks.
  const alg = a.seq(
    a.oid('1.2.840.113549.1.5.13'),
    a.seq(
      a.seq(a.oid('1.2.840.113549.1.5.12'), a.seq(a.octets(Buffer.from([1])), a.int(iterations))),
      a.seq(a.oid('2.16.840.1.101.3.4.1.42'), a.octets(iv)),
    ),
  );
  return a.seq(
    a.oid('1.2.840.113549.1.7.6'),
    a.explicit(
      0,
      a.seq(
        a.int(0),
        a.seq(a.oid('1.2.840.113549.1.7.1'), alg, a.implicitPrimitive(0, ciphertext)),
      ),
    ),
  );
}
describe('PKCS12 synchronous KDF budgets', () => {
  it('rejects expensive unauthenticated MAC before doing KDF work', () => {
    expect(() => parsePkcs12(pfx(a.seq(), MAX_PKCS12_KDF_ROUNDS + 1, false), pass)).toThrow(
      /iteration count out of range/,
    );
    expect(mocks.pbkdf2).not.toHaveBeenCalled();
  });
  it('charges PBKDF2 blocks cumulatively across encrypted parts before the second KDF', () => {
    expect(() =>
      parsePkcs12(pfx(a.seq(encryptedPart(30_000), encryptedPart(30_000))), pass),
    ).toThrow(/work budget exceeded/);
    expect(mocks.pbkdf2).toHaveBeenCalledTimes(1);
  });
  it('rejects a single multiblock derivation over budget before PBKDF2', () => {
    expect(() => parsePkcs12(pfx(a.seq(encryptedPart(50_001))), pass)).toThrow(
      /work budget exceeded/,
    );
    expect(mocks.pbkdf2).not.toHaveBeenCalled();
  });
  it('permits ordinary 2048-round encrypted parts to reach content validation', () => {
    expect(() => parsePkcs12(pfx(a.seq(encryptedPart(2048), encryptedPart(2048))), pass)).toThrow(
      /exactly one private key/,
    );
    expect(mocks.pbkdf2).toHaveBeenCalledTimes(2);
  });
  it('bounds direct legacy KDF calls too', () => {
    expect(() =>
      pkcs12Kdf(
        '1.3.14.3.2.26',
        Buffer.alloc(0),
        Buffer.alloc(0),
        3,
        MAX_PKCS12_KDF_ROUNDS + 1,
        20,
      ),
    ).toThrow(/work budget exceeded/);
  });
});

const now = new Date('2026-10-03T12:00:00Z');
const key = x.generateKey({ type: 'ecdsa', curve: 'p256' });
const ca = x.certFacts(x.selfSignedCa(x.parseDn('CN=Crypto review fixture'), key, 30, now).der);
const serial = Buffer.from([1]);
function response(thisUpdate: Date, producedAt = thisUpdate, nextUpdate?: Date): Buffer {
  const id = a.parse(ocspRequest(serial, ca)).children[0]!.children[0]!.children[0]!.children[0]!
    .raw;
  const single = a.seq(
    id,
    a.implicitPrimitive(0, Buffer.alloc(0)),
    a.generalizedTime(thisUpdate),
    ...(nextUpdate ? [a.explicit(0, a.generalizedTime(nextUpdate))] : []),
  );
  const tbs = a.seq(a.explicit(1, ca.subjectDer), a.generalizedTime(producedAt), a.seq(single));
  const signed = x.signWith(key.privateKey, tbs);
  const basic = a.seq(tbs, x.algId(signed.alg), a.bits(signed.sig));
  return a.seq(
    a.tlv(a.TAG.ENUMERATED, Buffer.from([0])),
    a.explicit(0, a.seq(a.oid('1.3.6.1.5.5.7.48.1.1'), a.octets(basic))),
  );
}
const at = (offset: number) => new Date(now.getTime() + offset);
describe('authenticated OCSP freshness', () => {
  it('rejects a historical signed good response without nextUpdate', () => {
    expect(() =>
      parseOcspResponse(response(new Date('2000-01-01T00:00:00Z')), serial, ca, now),
    ).toThrow(/maximum age/);
  });
  it('accepts recent signed good responses without nextUpdate', () => {
    expect(parseOcspResponse(response(at(-60_000)), serial, ca, now).status).toBe('good');
  });
  it('rejects age beyond 24h plus skew', () => {
    expect(() =>
      parseOcspResponse(response(at(-MAX_OCSP_AGE_MS - 301_000)), serial, ca, now),
    ).toThrow(/maximum age/);
  });
  it('rejects future producedAt and future thisUpdate', () => {
    expect(() => parseOcspResponse(response(now, at(301_000)), serial, ca, now)).toThrow(
      /producedAt.*future/,
    );
    expect(() => parseOcspResponse(response(at(301_000), now), serial, ca, now)).toThrow(
      /thisUpdate.*future/,
    );
  });
  it('rejects reversed validity ranges', () => {
    expect(() => parseOcspResponse(response(now, now, at(-1000)), serial, ca, now)).toThrow(
      /precedes/,
    );
  });
  it('preserves current nextUpdate-based validity', () => {
    expect(parseOcspResponse(response(at(-60_000), now, at(60_000)), serial, ca, now).status).toBe(
      'good',
    );
  });
});
