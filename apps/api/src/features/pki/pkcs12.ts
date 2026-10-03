import {
  createDecipheriv,
  createHash,
  createHmac,
  createPrivateKey,
  pbkdf2Sync,
  timingSafeEqual,
  type KeyObject,
} from 'node:crypto';
import {
  Asn1Error,
  children,
  expect,
  parse,
  readOctets,
  readOid,
  readSmallInt,
  TAG,
  type Node,
} from './asn1.js';
import { PkiError } from './x509.js';

/**
 * PKCS#12 (RFC 7292) import for F-pki: a PFX with a password MAC, holding certificates and one private key (keyBag or
 * pkcs8ShroudedKeyBag), encrypted with PBES2 (PBKDF2 + AES-CBC / 3DES — the OpenSSL 3 default) or the PKCS#12 PBE
 * pbeWithSHAAnd3-KeyTripleDES-CBC. RC2 (OpenSSL 1.x `-legacy` exports) needs OpenSSL's legacy provider, which Node 22
 * does not load: such files are refused with the re-export command. Errors never quote content or the passphrase.
 */

const O = {
  data: '1.2.840.113549.1.7.1',
  encryptedData: '1.2.840.113549.1.7.6',
  keyBag: '1.2.840.113549.1.12.10.1.1',
  shroudedKeyBag: '1.2.840.113549.1.12.10.1.2',
  certBag: '1.2.840.113549.1.12.10.1.3',
  x509Certificate: '1.2.840.113549.1.9.22.1',
  pbes2: '1.2.840.113549.1.5.13',
  pbkdf2: '1.2.840.113549.1.5.12',
  pbmac1: '1.2.840.113549.1.5.14',
  pbeSha3Des: '1.2.840.113549.1.12.1.3',
  pbeSha2Des: '1.2.840.113549.1.12.1.4',
  pbeShaRc2_128: '1.2.840.113549.1.12.1.5',
  pbeShaRc2_40: '1.2.840.113549.1.12.1.6',
} as const;

const HASHES: Record<
  string,
  { name: 'sha1' | 'sha256' | 'sha384' | 'sha512'; v: number; u: number }
> = {
  '1.3.14.3.2.26': { name: 'sha1', v: 64, u: 20 },
  '2.16.840.1.101.3.4.2.1': { name: 'sha256', v: 64, u: 32 },
  '2.16.840.1.101.3.4.2.2': { name: 'sha384', v: 128, u: 48 },
  '2.16.840.1.101.3.4.2.3': { name: 'sha512', v: 128, u: 64 },
};

const PRFS: Record<string, 'sha1' | 'sha256' | 'sha384' | 'sha512'> = {
  '1.2.840.113549.2.7': 'sha1',
  '1.2.840.113549.2.9': 'sha256',
  '1.2.840.113549.2.10': 'sha384',
  '1.2.840.113549.2.11': 'sha512',
};

const CIPHERS: Record<string, { name: string; key: number }> = {
  '2.16.840.1.101.3.4.1.2': { name: 'aes-128-cbc', key: 16 },
  '2.16.840.1.101.3.4.1.22': { name: 'aes-192-cbc', key: 24 },
  '2.16.840.1.101.3.4.1.42': { name: 'aes-256-cbc', key: 32 },
  '1.2.840.113549.3.7': { name: 'des-ede3-cbc', key: 24 },
};

/** Upper bounds (a PKCS#12 bundle of a key and a short chain is a few kB). */
export const MAX_PKCS12 = 256 * 1024;
// A request may spend at most 100k KDF digest rounds across MAC and all decryptions.
// Default OpenSSL 2048-round bundles fit comfortably; unusually expensive exports must
// be re-exported with fewer iterations. Limits are charged before synchronous crypto.
export const MAX_PKCS12_KDF_ROUNDS = 100_000;
const MAX_ITERATIONS = MAX_PKCS12_KDF_ROUNDS;
type WorkBudget = { remaining: number };
const newBudget = (): WorkBudget => ({ remaining: MAX_PKCS12_KDF_ROUNDS });
function charge(budget: WorkBudget, iterations: number, blocks: number): void {
  const cost = iterations * blocks;
  if (
    !Number.isSafeInteger(iterations) ||
    iterations < 1 ||
    iterations > MAX_ITERATIONS ||
    !Number.isSafeInteger(cost) ||
    cost > budget.remaining
  ) {
    throw fail('PKCS#12 KDF work budget exceeded: re-export with fewer iterations');
  }
  budget.remaining -= cost;
}

const fail = (m: string) => new PkiError(m, '/pkcs12');

/** RFC 7292 B.2 key derivation (id 1 = key, 2 = IV, 3 = MAC key) over the BMPString password. */
export function pkcs12Kdf(
  hashOid: string,
  password: Buffer,
  salt: Buffer,
  id: 1 | 2 | 3,
  iterations: number,
  n: number,
  budget: WorkBudget = newBudget(),
): Buffer {
  const h = HASHES[hashOid];
  if (h === undefined) throw fail('unsupported PKCS#12 hash');
  const { v, u, name } = h;
  charge(budget, iterations, Math.ceil(n / u));
  const D = Buffer.alloc(v, id);
  const fill = (b: Buffer) => {
    if (b.length === 0) return Buffer.alloc(0);
    const out = Buffer.alloc(v * Math.ceil(b.length / v));
    for (let i = 0; i < out.length; i++) out[i] = b[i % b.length]!;
    return out;
  };
  const I = Buffer.concat([fill(salt), fill(password)]);
  const out = Buffer.alloc(n);
  for (let done = 0; done < n;) {
    let A = createHash(name).update(D).update(I).digest();
    for (let k = 1; k < iterations; k++) A = createHash(name).update(A).digest();
    A.copy(out, done, 0, Math.min(u, n - done));
    done += u;
    if (done >= n) break;
    const B = Buffer.alloc(v);
    for (let i = 0; i < v; i++) B[i] = A[i % u]!;
    for (let j = 0; j < I.length; j += v) {
      let carry = 1;
      for (let k = v - 1; k >= 0; k--) {
        const sum = I[j + k]! + B[k]! + carry;
        I[j + k] = sum & 0xff;
        carry = sum >> 8;
      }
    }
  }
  return out;
}

/** The PKCS#12 password form: UTF-16BE with a two-byte NUL terminator. */
function bmpPassword(pass: string): Buffer {
  const le = Buffer.from(pass + '\0', 'utf16le');
  return le.swap16();
}

function iterationsOf(n: Node | undefined, what: string): number {
  if (n === undefined) return 1;
  const it = readSmallInt(n, what);
  if (it < 1 || it > MAX_ITERATIONS) throw fail(`${what} out of range`);
  return it;
}

/** Decrypts content encrypted under a PBES2 or PKCS#12-PBE AlgorithmIdentifier. */
function decrypt(alg: Node, data: Buffer, pass: string, budget: WorkBudget): Buffer {
  const [algOid, params] = children(alg, 1, 2, 'encryption algorithm');
  const o = readOid(algOid!);
  let key: Buffer;
  let iv: Buffer;
  let cipher: string;
  if (o === O.pbes2) {
    const [kdf, enc] = children(params!, 2, 2, 'PBES2 parameters');
    const [kdfOid, kdfParams] = children(kdf!, 2, 2, 'PBES2 key derivation');
    if (readOid(kdfOid!) !== O.pbkdf2) throw fail('unsupported PBES2 key derivation (PBKDF2 only)');
    const kp = children(kdfParams!, 2, 4, 'PBKDF2 parameters');
    const salt = readOctets(kp[0]!, 'PBKDF2 salt');
    const iterations = iterationsOf(kp[1], 'PBKDF2 iteration count');
    let prf: 'sha1' | 'sha256' | 'sha384' | 'sha512' = 'sha1';
    for (const extra of kp.slice(2)) {
      if (extra.tag === TAG.SEQUENCE) {
        const p = PRFS[readOid(extra.children[0]!)];
        if (p === undefined) throw fail('unsupported PBKDF2 PRF');
        prf = p;
      }
    }
    const [encOid, encIv] = children(enc!, 2, 2, 'PBES2 encryption scheme');
    const c = CIPHERS[readOid(encOid!)];
    if (c === undefined) throw fail('unsupported PBES2 cipher (AES-CBC or 3DES only)');
    cipher = c.name;
    iv = readOctets(encIv!, 'PBES2 IV');
    const digestBytes = { sha1: 20, sha256: 32, sha384: 48, sha512: 64 }[prf];
    charge(budget, iterations, Math.ceil(c.key / digestBytes));
    key = pbkdf2Sync(Buffer.from(pass, 'utf8'), salt, iterations, c.key, prf);
  } else if (o === O.pbeSha3Des || o === O.pbeSha2Des) {
    const [salt, it] = children(params!, 2, 2, 'PKCS#12 PBE parameters');
    const iterations = iterationsOf(it, 'PBE iteration count');
    const pw = bmpPassword(pass);
    key = pkcs12Kdf(
      '1.3.14.3.2.26',
      pw,
      readOctets(salt!),
      1,
      iterations,
      o === O.pbeSha3Des ? 24 : 16,
      budget,
    );
    if (o === O.pbeSha2Des) key = Buffer.concat([key, key.subarray(0, 8)]);
    iv = pkcs12Kdf('1.3.14.3.2.26', pw, readOctets(salt!), 2, iterations, 8, budget);
    cipher = 'des-ede3-cbc';
  } else if (o === O.pbeShaRc2_40 || o === O.pbeShaRc2_128) {
    throw fail(
      'this PKCS#12 file uses RC2 (OpenSSL 1.x / -legacy export), which this build cannot decrypt: re-export it with `openssl pkcs12 -export -keypbe AES-256-CBC -certpbe AES-256-CBC -macalg sha256`',
    );
  } else {
    throw fail('unsupported PKCS#12 encryption algorithm');
  }
  try {
    const d = createDecipheriv(cipher, key, iv);
    return Buffer.concat([d.update(data), d.final()]);
  } catch {
    throw fail('the PKCS#12 content does not decrypt: wrong passphrase or damaged file');
  } finally {
    key.fill(0);
  }
}

/** contentType + [0] EXPLICIT content of a ContentInfo. */
function contentInfo(n: Node): { type: string; content: Node | undefined } {
  const [t, c] = children(n, 1, 2, 'ContentInfo');
  if (c !== undefined && c.tag !== 0xa0) throw new Asn1Error('ContentInfo: unexpected structure');
  return { type: readOid(t!), content: c?.children[0] };
}

export interface Pkcs12Content {
  /** DER certificates in file order. */
  certs: Buffer[];
  /** The private key (exactly one in the file). */
  key: KeyObject;
}

/**
 * Parses and decrypts a PKCS#12 file: checks the password MAC (HMAC over the PKCS#12 KDF, SHA-1/SHA-2), decrypts
 * every encrypted part and returns the certificates and the one private key.
 */
export function parsePkcs12(der: Buffer, passphrase: string): Pkcs12Content {
  if (der.length > MAX_PKCS12)
    throw fail(`the PKCS#12 file is larger than ${MAX_PKCS12 / 1024} KiB`);
  const budget = newBudget();
  try {
    const [version, authSafe, macData] = children(parse(der), 2, 3, 'PFX');
    if (readSmallInt(version!, 'PFX version') !== 3) throw fail('unsupported PKCS#12 version');
    const ci = contentInfo(authSafe!);
    if (ci.type !== O.data || ci.content === undefined)
      throw fail('signed (public-key integrity) PKCS#12 files are not supported');
    const authSafeBytes = readOctets(ci.content, 'authSafe');
    if (macData === undefined) throw fail('the PKCS#12 file has no password MAC');
    const [digestInfo, macSalt, macIt] = children(macData, 2, 3, 'MacData');
    const [macAlg, macValue] = children(digestInfo!, 2, 2, 'DigestInfo');
    const hashOid = readOid(children(macAlg!, 1, 2, 'MAC algorithm')[0]!);
    if (hashOid === O.pbmac1)
      throw fail('PBMAC1-protected PKCS#12 files are not supported: re-export with -macalg sha256');
    const h = HASHES[hashOid];
    if (h === undefined) throw fail('unsupported PKCS#12 MAC hash');
    const macKey = pkcs12Kdf(
      hashOid,
      bmpPassword(passphrase),
      readOctets(macSalt!),
      3,
      iterationsOf(macIt, 'MAC iteration count'),
      h.u,
      budget,
    );
    const mac = createHmac(h.name, macKey).update(authSafeBytes).digest();
    macKey.fill(0);
    const expected = readOctets(macValue!, 'MAC');
    if (mac.length !== expected.length || !timingSafeEqual(mac, expected))
      throw fail('wrong passphrase (the PKCS#12 MAC does not verify)');

    const certs: Buffer[] = [];
    const keys: KeyObject[] = [];
    for (const part of children(parse(authSafeBytes), 0, 64, 'AuthenticatedSafe')) {
      const p = contentInfo(part);
      let safeContents: Buffer;
      if (p.type === O.data && p.content !== undefined) {
        safeContents = readOctets(p.content, 'SafeContents');
      } else if (p.type === O.encryptedData && p.content !== undefined) {
        const [, eci] = children(p.content, 2, 3, 'EncryptedData');
        const [ctype, alg, enc] = children(eci!, 2, 3, 'EncryptedContentInfo');
        if (readOid(ctype!) !== O.data || enc === undefined || enc.tag !== 0x80)
          throw fail('unsupported PKCS#12 encrypted content');
        safeContents = decrypt(alg!, enc.value, passphrase, budget);
      } else {
        throw fail('unsupported PKCS#12 content type (enveloped data)');
      }
      for (const bag of children(parse(safeContents), 0, 256, 'SafeContents')) {
        const [bagId, bagValue] = children(bag, 2, 3, 'SafeBag');
        const id = readOid(bagId!);
        const value = expect(bagValue, 0xa0, 'bagValue').children[0]!;
        if (id === O.certBag) {
          const [certType, certValue] = children(value, 2, 2, 'CertBag');
          if (readOid(certType!) !== O.x509Certificate) continue;
          certs.push(
            Buffer.from(
              readOctets(expect(certValue, 0xa0, 'certValue').children[0]!, 'certificate'),
            ),
          );
        } else if (id === O.keyBag) {
          keys.push(
            createPrivateKey({ key: Buffer.from(value.raw), format: 'der', type: 'pkcs8' }),
          );
        } else if (id === O.shroudedKeyBag) {
          const [alg, enc] = children(value, 2, 2, 'EncryptedPrivateKeyInfo');
          const plain = decrypt(alg!, readOctets(enc!), passphrase, budget);
          try {
            keys.push(createPrivateKey({ key: plain, format: 'der', type: 'pkcs8' }));
          } catch {
            throw fail('the PKCS#12 private key does not parse');
          } finally {
            plain.fill(0);
          }
        }
        // CRL, secret and nested bags are ignored
      }
    }
    if (keys.length !== 1)
      throw fail(`the PKCS#12 file must hold exactly one private key (found ${keys.length})`);
    if (certs.length === 0) throw fail('the PKCS#12 file holds no certificate');
    return { certs, key: keys[0]! };
  } catch (e) {
    if (e instanceof PkiError) throw e;
    throw fail(
      `the PKCS#12 file is malformed (${e instanceof Asn1Error ? e.message : 'unexpected structure'})`,
    );
  }
}
