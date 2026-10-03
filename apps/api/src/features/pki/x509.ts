import {
  createHash,
  createPrivateKey,
  createPublicKey,
  generateKeyPairSync,
  randomBytes,
  sign as cryptoSign,
  verify as cryptoVerify,
  X509Certificate,
  type KeyObject,
} from 'node:crypto';
import { isIP } from 'node:net';
import {
  Asn1Error,
  bits,
  bool,
  children,
  colonHex,
  expect,
  explicit,
  ia5,
  implicitConstructed,
  implicitPrimitive,
  int,
  nul,
  octets,
  oid,
  parse,
  printable,
  readBits,
  readOctets,
  readOid,
  readString,
  seq,
  setOf,
  TAG,
  time,
  utf8,
  type Node,
} from './asn1.js';

/** Errors of the X.509 layer: the message is safe to show (never material). */
export class PkiError extends Error {
  constructor(
    message: string,
    /** RFC 6901 pointer into the request body the problem refers to ("" = the request). */
    readonly pointer = '',
  ) {
    super(message);
  }
}

export const OID = {
  CN: '2.5.4.3',
  serialNumber: '2.5.4.5',
  C: '2.5.4.6',
  L: '2.5.4.7',
  ST: '2.5.4.8',
  O: '2.5.4.10',
  OU: '2.5.4.11',
  DC: '0.9.2342.19200300.100.1.25',
  E: '1.2.840.113549.1.9.1',
  ecPublicKey: '1.2.840.10045.2.1',
  prime256v1: '1.2.840.10045.3.1.7',
  secp384r1: '1.3.132.0.34',
  rsaEncryption: '1.2.840.113549.1.1.1',
  ecdsaSha256: '1.2.840.10045.4.3.2',
  ecdsaSha384: '1.2.840.10045.4.3.3',
  ecdsaSha512: '1.2.840.10045.4.3.4',
  sha256Rsa: '1.2.840.113549.1.1.11',
  sha384Rsa: '1.2.840.113549.1.1.12',
  sha512Rsa: '1.2.840.113549.1.1.13',
  basicConstraints: '2.5.29.19',
  keyUsage: '2.5.29.15',
  extKeyUsage: '2.5.29.37',
  subjectAltName: '2.5.29.17',
  subjectKeyIdentifier: '2.5.29.14',
  authorityKeyIdentifier: '2.5.29.35',
  crlDistributionPoints: '2.5.29.31',
  authorityInfoAccess: '1.3.6.1.5.5.7.1.1',
  accessOcsp: '1.3.6.1.5.5.7.48.1',
  serverAuth: '1.3.6.1.5.5.7.3.1',
  clientAuth: '1.3.6.1.5.5.7.3.2',
  ocspSigning: '1.3.6.1.5.5.7.3.9',
  ikeIntermediate: '1.3.6.1.5.5.8.2.2',
  extensionRequest: '1.2.840.113549.1.9.14',
  sha1: '1.3.14.3.2.26',
} as const;

// ---------------------------------------------------------------------------------------------------------------
// distinguished names
// ---------------------------------------------------------------------------------------------------------------

const ATTR_OID: Record<string, string> = {
  CN: OID.CN,
  O: OID.O,
  OU: OID.OU,
  C: OID.C,
  L: OID.L,
  ST: OID.ST,
  DC: OID.DC,
  E: OID.E,
  serialNumber: OID.serialNumber,
};
const OID_ATTR: Record<string, string> = Object.fromEntries(
  Object.entries(ATTR_OID).map(([k, v]) => [v, k]),
);

export interface Rdn {
  type: string;
  value: string;
}

/**
 * Parses `CN=gw.example.com, O=Example, C=CH` (attributes of PKI_DN_ATTRIBUTES, no escapes) into RDNs in the order
 * written — which is also the DER order (the convention of OpenSSL's -subj and of strongSwan identities).
 */
export function parseDn(dn: string, pointer = ''): Rdn[] {
  const parts = dn.split(',').map((p) => p.trim());
  const out: Rdn[] = [];
  for (const p of parts) {
    const eq = p.indexOf('=');
    const type = eq > 0 ? p.slice(0, eq).trim() : '';
    const value = eq > 0 ? p.slice(eq + 1).trim() : '';
    const hasControl = Array.from(value).some(
      (c) => c.charCodeAt(0) < 32 || c.charCodeAt(0) === 127,
    );
    if (!(type in ATTR_OID) || value === '' || /[=+"\\<>;#]/.test(value) || hasControl)
      throw new PkiError(`'${p.slice(0, 40)}' is not an attribute like CN=…, O=…, C=…`, pointer);
    if (type === 'C' && !/^[A-Z]{2}$/.test(value))
      throw new PkiError('C= must be a two-letter country code', pointer);
    if (value.length > 128) throw new PkiError(`${type}= is longer than 128 characters`, pointer);
    out.push({ type, value });
  }
  if (out.length === 0) throw new PkiError('empty distinguished name', pointer);
  return out;
}

/** DER Name: C, DC and serialNumber as PrintableString/IA5 where they must be, the rest UTF8String. */
export function encodeName(rdns: Rdn[]): Buffer {
  return seq(
    ...rdns.map((r) => {
      const value =
        r.type === 'C' || r.type === 'serialNumber'
          ? printable(r.value)
          : r.type === 'DC' || r.type === 'E'
            ? ia5(r.value)
            : utf8(r.value);
      return setOf(seq(oid(ATTR_OID[r.type]!), value));
    }),
  );
}

/** A DER Name as `CN=…, O=…` in DER order (unknown attribute types as their dotted OID). */
export function formatName(name: Node): string {
  expect(name, TAG.SEQUENCE, 'Name');
  const out: string[] = [];
  for (const rdn of name.children) {
    expect(rdn, TAG.SET, 'RDN');
    for (const atv of rdn.children) {
      const [t, v] = children(atv, 2, 2, 'AttributeTypeAndValue');
      const o = readOid(t!);
      out.push(`${OID_ATTR[o] ?? o}=${readString(v!, 'attribute value')}`);
    }
  }
  return out.join(', ');
}

// ---------------------------------------------------------------------------------------------------------------
// keys and signatures
// ---------------------------------------------------------------------------------------------------------------

export type KeyType = 'ecdsa' | 'rsa';
export interface KeySpec {
  type: KeyType;
  curve?: 'p256' | 'p384' | undefined;
  bits?: 2048 | 3072 | 4096 | undefined;
}

/** Fills the defaults of a key spec (ECDSA P-256, RSA 2048). */
export function normaliseKeySpec(k: Partial<KeySpec> | undefined): KeySpec {
  const type = k?.type ?? 'ecdsa';
  return type === 'rsa' ? { type, bits: k?.bits ?? 2048 } : { type, curve: k?.curve ?? 'p256' };
}

/** A fresh key pair for spec. */
export function generateKey(spec: KeySpec): { privateKey: KeyObject; publicKey: KeyObject } {
  if (spec.type === 'rsa') return generateKeyPairSync('rsa', { modulusLength: spec.bits ?? 2048 });
  return generateKeyPairSync('ec', { namedCurve: spec.curve === 'p384' ? 'P-384' : 'P-256' });
}

/** The key spec of a public key (for recording what a CSR or an import used). */
export function keySpecOf(key: KeyObject): KeySpec | undefined {
  const d = key.asymmetricKeyDetails;
  if (key.asymmetricKeyType === 'ec') {
    if (d?.namedCurve === 'prime256v1' || d?.namedCurve === 'P-256')
      return { type: 'ecdsa', curve: 'p256' };
    if (d?.namedCurve === 'secp384r1' || d?.namedCurve === 'P-384')
      return { type: 'ecdsa', curve: 'p384' };
    return undefined;
  }
  if (key.asymmetricKeyType === 'rsa') {
    const b = d?.modulusLength;
    return b === 2048 || b === 3072 || b === 4096 ? { type: 'rsa', bits: b } : undefined;
  }
  return undefined;
}

interface SigAlg {
  oid: string;
  hash: 'sha256' | 'sha384' | 'sha512';
  rsa: boolean;
}

/** The signature algorithm a key signs with: ECDSA with SHA-256 (P-256) / SHA-384 (P-384), RSA PKCS#1 v1.5 SHA-256. */
export function sigAlgFor(key: KeyObject): SigAlg {
  if (key.asymmetricKeyType === 'ec') {
    const curve = key.asymmetricKeyDetails?.namedCurve;
    return curve === 'secp384r1' || curve === 'P-384'
      ? { oid: OID.ecdsaSha384, hash: 'sha384', rsa: false }
      : { oid: OID.ecdsaSha256, hash: 'sha256', rsa: false };
  }
  if (key.asymmetricKeyType === 'rsa') return { oid: OID.sha256Rsa, hash: 'sha256', rsa: true };
  throw new PkiError(
    `unsupported key type ${key.asymmetricKeyType ?? '?'} (ECDSA P-256/P-384 or RSA only)`,
  );
}

const SIG_BY_OID: Record<string, SigAlg> = {
  [OID.ecdsaSha256]: { oid: OID.ecdsaSha256, hash: 'sha256', rsa: false },
  [OID.ecdsaSha384]: { oid: OID.ecdsaSha384, hash: 'sha384', rsa: false },
  [OID.ecdsaSha512]: { oid: OID.ecdsaSha512, hash: 'sha512', rsa: false },
  [OID.sha256Rsa]: { oid: OID.sha256Rsa, hash: 'sha256', rsa: true },
  [OID.sha384Rsa]: { oid: OID.sha384Rsa, hash: 'sha384', rsa: true },
  [OID.sha512Rsa]: { oid: OID.sha512Rsa, hash: 'sha512', rsa: true },
};

export const algId = (a: SigAlg): Buffer => (a.rsa ? seq(oid(a.oid), nul()) : seq(oid(a.oid)));

/** Reads an AlgorithmIdentifier of a supported signature algorithm. */
export function readSigAlg(n: Node, what: string): SigAlg {
  const [o] = children(n, 1, 2, what);
  const a = SIG_BY_OID[readOid(o!, what)];
  if (a === undefined)
    throw new PkiError(
      `${what}: unsupported signature algorithm (ECDSA or RSA PKCS#1 with SHA-2 only)`,
    );
  return a;
}

/** Signs data (a TBS structure) with key. */
export function signWith(key: KeyObject, data: Buffer): { alg: SigAlg; sig: Buffer } {
  const alg = sigAlgFor(key);
  return { alg, sig: cryptoSign(alg.hash, data, key) };
}

/** Verifies a DER signature over data with a public key. */
export function verifyWith(alg: SigAlg, data: Buffer, pub: KeyObject, sig: Buffer): boolean {
  try {
    return cryptoVerify(alg.hash, data, pub, sig);
  } catch {
    return false;
  }
}

export function spkiOf(pub: KeyObject): Buffer {
  return pub.export({ type: 'spki', format: 'der' });
}

/** RFC 5280 §4.2.1.2 method 1: SHA-1 of the subjectPublicKey bits. */
export function keyIdentifier(spki: Buffer): Buffer {
  const [, pk] = children(parse(spki), 2, 2, 'SubjectPublicKeyInfo');
  return createHash('sha1').update(readBits(pk!)).digest();
}

// ---------------------------------------------------------------------------------------------------------------
// subject alternative names
// ---------------------------------------------------------------------------------------------------------------

/** A SAN as written in the document: an IP address, an e-mail address or a DNS name. */
export function encodeSan(names: string[], pointer = ''): Buffer {
  return seq(
    ...names.map((n, i) => {
      if (isIP(n) === 4) return implicitPrimitive(7, Buffer.from(n.split('.').map(Number)));
      if (isIP(n) === 6) return implicitPrimitive(7, ipv6Bytes(n));
      if (n.includes('@')) return implicitPrimitive(1, Buffer.from(n, 'latin1'));
      if (
        /^(?:\*\.)?[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?(?:\.[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*$/.test(
          n,
        )
      )
        return implicitPrimitive(2, Buffer.from(n.toLowerCase(), 'latin1'));
      throw new PkiError(
        `'${n.slice(0, 40)}' is not a DNS name, IP address or e-mail address`,
        `${pointer}/${i}`,
      );
    }),
  );
}

function ipv6Bytes(s: string): Buffer {
  const [head, tail] = s.includes('::') ? s.split('::') : [s, undefined];
  const parse16 = (part: string | undefined) =>
    part ? part.split(':').filter((x) => x !== '') : [];
  let h = parse16(head);
  let t = parse16(tail);
  const v4 = (arr: string[]) => {
    const last = arr[arr.length - 1];
    if (last?.includes('.')) {
      const b = last.split('.').map(Number);
      arr.splice(
        arr.length - 1,
        1,
        ((b[0]! << 8) | b[1]!).toString(16),
        ((b[2]! << 8) | b[3]!).toString(16),
      );
    }
    return arr;
  };
  h = v4(h);
  t = v4(t);
  const fill = tail === undefined ? [] : Array<string>(8 - h.length - t.length).fill('0');
  const groups = [...h, ...fill, ...t];
  const out = Buffer.alloc(16);
  groups.forEach((g, i) => out.writeUInt16BE(parseInt(g, 16), i * 2));
  return out;
}

/** RFC 5952 text of a 16-byte IPv6 address (the longest run of two or more zero groups becomes `::`). */
function formatIPv6(b: Buffer): string {
  const g = Array.from({ length: 8 }, (_, i) => b.readUInt16BE(i * 2));
  let best = -1;
  let bestLen = 1;
  for (let i = 0; i < 8;) {
    if (g[i] !== 0) {
      i++;
      continue;
    }
    let j = i;
    while (j < 8 && g[j] === 0) j++;
    if (j - i > bestLen) [best, bestLen] = [i, j - i];
    i = j;
  }
  const hex = (xs: number[]) => xs.map((x) => x.toString(16)).join(':');
  return best < 0 ? hex(g) : `${hex(g.slice(0, best))}::${hex(g.slice(best + bestLen))}`;
}

/** The SANs of a GeneralNames value as text (`DNS:…`, `IP:…`, `email:…`; other forms are skipped). */
export function decodeSan(extValue: Buffer): string[] {
  const out: string[] = [];
  for (const gn of children(parse(extValue), 0, 1024, 'GeneralNames')) {
    if (gn.cls !== 'context') continue;
    if (gn.num === 2) out.push(`DNS:${gn.value.toString('latin1')}`);
    else if (gn.num === 1) out.push(`email:${gn.value.toString('latin1')}`);
    else if (gn.num === 7 && gn.value.length === 4) out.push(`IP:${[...gn.value].join('.')}`);
    else if (gn.num === 7 && gn.value.length === 16) out.push(`IP:${formatIPv6(gn.value)}`);
  }
  return out;
}

// ---------------------------------------------------------------------------------------------------------------
// certificates
// ---------------------------------------------------------------------------------------------------------------

const ext = (o: string, critical: boolean, value: Buffer): Buffer =>
  critical ? seq(oid(o), bool(true), octets(value)) : seq(oid(o), octets(value));

/** KeyUsage bits (RFC 5280 §4.2.1.3) as a DER BIT STRING value. */
function keyUsage(flags: {
  digitalSignature?: boolean;
  keyEncipherment?: boolean;
  keyCertSign?: boolean;
  cRLSign?: boolean;
}): Buffer {
  let b0 = 0;
  if (flags.digitalSignature) b0 |= 0x80; // bit 0
  if (flags.keyEncipherment) b0 |= 0x20; // bit 2
  if (flags.keyCertSign) b0 |= 0x04; // bit 5
  if (flags.cRLSign) b0 |= 0x02; // bit 6
  // DER: trailing zero bits are not encoded — `unused` counts the trailing zero bits of the only byte
  let unused = 0;
  while (unused < 7 && ((b0 >> unused) & 1) === 0) unused++;
  return bits(Buffer.from([b0]), unused);
}

export interface CertProfile {
  serial: Buffer;
  /** DER Name of the subject and of the issuer (an issuer's Name is copied byte for byte from its certificate). */
  subject: Buffer;
  issuer: Buffer;
  notBefore: Date;
  notAfter: Date;
  spki: Buffer;
  ca: boolean;
  /** SANs (end-entity certificates). */
  san?: string[];
  /** Subject key id; authority key id (the issuer's subject key id; omitted on a self-signed root). */
  ski: Buffer;
  aki?: Buffer;
  /** CRL distribution point and OCSP responder of the issuing CA (when configured). */
  crlUrl?: string;
  ocspUrl?: string;
  /** RSA end-entity keys also get keyEncipherment. */
  rsa?: boolean;
}

/** The TBSCertificate of profile p (X.509 v3; CA: basicConstraints CA:TRUE critical + keyCertSign/cRLSign). */
export function buildTbs(p: CertProfile, alg: SigAlg): Buffer {
  const exts: Buffer[] = [
    ext(OID.basicConstraints, true, p.ca ? seq(bool(true)) : seq()),
    ext(
      OID.keyUsage,
      true,
      p.ca
        ? keyUsage({ digitalSignature: true, keyCertSign: true, cRLSign: true })
        : keyUsage({ digitalSignature: true, keyEncipherment: p.rsa === true }),
    ),
    ext(OID.subjectKeyIdentifier, false, octets(p.ski)),
  ];
  if (p.aki !== undefined)
    exts.push(ext(OID.authorityKeyIdentifier, false, seq(implicitPrimitive(0, p.aki))));
  if (!p.ca) {
    exts.push(
      ext(
        OID.extKeyUsage,
        false,
        seq(oid(OID.serverAuth), oid(OID.clientAuth), oid(OID.ikeIntermediate)),
      ),
    );
    if (p.san !== undefined && p.san.length > 0)
      exts.push(ext(OID.subjectAltName, false, encodeSan(p.san)));
  }
  if (p.crlUrl !== undefined)
    exts.push(
      ext(
        OID.crlDistributionPoints,
        false,
        seq(
          seq(
            explicit(
              0,
              implicitConstructed(0, implicitPrimitive(6, Buffer.from(p.crlUrl, 'latin1'))),
            ),
          ),
        ),
      ),
    );
  if (p.ocspUrl !== undefined)
    exts.push(
      ext(
        OID.authorityInfoAccess,
        false,
        seq(seq(oid(OID.accessOcsp), implicitPrimitive(6, Buffer.from(p.ocspUrl, 'latin1')))),
      ),
    );
  return seq(
    explicit(0, int(2)),
    int(p.serial),
    algId(alg),
    p.issuer,
    seq(time(p.notBefore), time(p.notAfter)),
    p.subject,
    p.spki,
    explicit(3, seq(...exts)),
  );
}

/** Signs a TBS structure into a DER certificate (or CRL, OCSP: any SEQUENCE { tbs, alg, sig }). */
export function signTbs(tbs: Buffer, alg: SigAlg, key: KeyObject): Buffer {
  return seq(tbs, algId(alg), bits(cryptoSign(alg.hash, tbs, key)));
}

/** A positive random 16-byte serial number. */
export function newSerial(): Buffer {
  const b = randomBytes(16);
  b[0] = (b[0]! & 0x7f) | 0x01;
  return b;
}

export function pemCert(der: Buffer): string {
  return toPem('CERTIFICATE', der);
}

export function toPem(label: string, der: Buffer): string {
  const b64 = der.toString('base64').match(/.{1,64}/g) ?? [];
  return `-----BEGIN ${label}-----\n${b64.join('\n')}\n-----END ${label}-----\n`;
}

const PEM_RE = /-----BEGIN ([A-Z0-9 ]+)-----\r?\n([A-Za-z0-9+/=\r\n]+?)-----END \1-----/g;

/** The PEM blocks of text (label + DER). Text outside the blocks is ignored. */
export function pemBlocks(text: string): { label: string; der: Buffer }[] {
  const out: { label: string; der: Buffer }[] = [];
  for (const m of text.matchAll(PEM_RE))
    out.push({ label: m[1]!, der: Buffer.from(m[2]!.replace(/\s+/g, ''), 'base64') });
  return out;
}

/** The labels of private-key PEM blocks (assembled so no source line holds a full private-key banner). */
const KEY_LABELS = new Set(
  ['PRIVATE', 'EC PRIVATE', 'RSA PRIVATE', 'ENCRYPTED PRIVATE'].map((p) => `${p} KEY`),
);
export const isPrivateKeyLabel = (label: string): boolean => KEY_LABELS.has(label);

/** Public facts of a certificate, from OpenSSL (X509Certificate) and the DER. Never key material. */
export interface CertFacts {
  subject: string;
  issuer: string;
  serial: string;
  notBefore: string;
  notAfter: string;
  fingerprint: string;
  ca: boolean;
  san: string[];
  keySpec?: KeySpec;
  /** DER of the subject / issuer Name (exact bytes, for chaining and OCSP). */
  subjectDer: Buffer;
  issuerDer: Buffer;
  /** subjectPublicKeyInfo DER. */
  spki: Buffer;
  /** The subject key identifier extension, when present. */
  ski?: Buffer;
}

/** Parses one DER certificate; `PkiError` when OpenSSL or the structure refuses it. */
export function certFacts(der: Buffer): CertFacts & { x509: X509Certificate } {
  let x: X509Certificate;
  try {
    x = new X509Certificate(der);
  } catch {
    throw new PkiError('the certificate does not parse');
  }
  try {
    const [tbs] = children(parse(der), 3, 3, 'Certificate');
    const t = children(tbs!, 6, 10, 'TBSCertificate');
    const off = t[0]!.tag === 0xa0 ? 1 : 0;
    const issuer = t[off + 2]!;
    const subject = t[off + 4]!;
    const spki = t[off + 5]!;
    let san: string[] = [];
    let ski: Buffer | undefined;
    const extsNode = t.find((n) => n.tag === 0xa3);
    if (extsNode !== undefined) {
      for (const e of children(extsNode.children[0]!, 0, 64, 'Extensions')) {
        const parts = e.children;
        const o = readOid(parts[0]!);
        const value = readOctets(parts[parts.length - 1]!);
        if (o === OID.subjectAltName) san = decodeSan(value);
        if (o === OID.subjectKeyIdentifier) ski = readOctets(parse(value));
      }
    }
    const pub = createPublicKey({ key: spki.raw, format: 'der', type: 'spki' });
    const facts: CertFacts = {
      subject: formatName(subject),
      issuer: formatName(issuer),
      serial: colonHex(
        Buffer.from(x.serialNumber.length % 2 ? '0' + x.serialNumber : x.serialNumber, 'hex'),
      ),
      notBefore: new Date(x.validFrom).toISOString(),
      notAfter: new Date(x.validTo).toISOString(),
      fingerprint: x.fingerprint256,
      ca: x.ca,
      san,
      subjectDer: Buffer.from(subject.raw),
      issuerDer: Buffer.from(issuer.raw),
      spki: Buffer.from(spki.raw),
      ...(ski !== undefined ? { ski: Buffer.from(ski) } : {}),
    };
    const ks = keySpecOf(pub);
    if (ks !== undefined) facts.keySpec = ks;
    return { ...facts, x509: x };
  } catch (e) {
    if (e instanceof PkiError) throw e;
    if (e instanceof Asn1Error) throw new PkiError(`the certificate is malformed (${e.message})`);
    throw new PkiError('the certificate is malformed');
  }
}

/** Days from now until notAfter (negative once expired), rounded down. */
export function daysLeft(notAfter: string, now: Date): number {
  return Math.floor((Date.parse(notAfter) - now.getTime()) / 86_400_000);
}

// ---------------------------------------------------------------------------------------------------------------
// CA and certificates from requests
// ---------------------------------------------------------------------------------------------------------------

export interface Issued {
  der: Buffer;
  pem: string;
  facts: CertFacts;
}

/** A self-signed CA certificate for key, valid `days` from now (backdated 5 minutes for clock skew). */
export function selfSignedCa(
  subject: Rdn[],
  key: { privateKey: KeyObject; publicKey: KeyObject },
  days: number,
  now = new Date(),
): Issued {
  const spki = spkiOf(key.publicKey);
  const name = encodeName(subject);
  const alg = sigAlgFor(key.privateKey);
  const tbs = buildTbs(
    {
      serial: newSerial(),
      subject: name,
      issuer: name,
      notBefore: new Date(now.getTime() - 5 * 60_000),
      notAfter: new Date(now.getTime() + days * 86_400_000),
      spki,
      ca: true,
      ski: keyIdentifier(spki),
    },
    alg,
  );
  const der = signTbs(tbs, alg, key.privateKey);
  return { der, pem: pemCert(der), facts: certFacts(der) };
}

/** A PKCS#10 CSR for subject + SANs signed by key. */
export function buildCsr(
  subject: Rdn[],
  san: string[],
  key: { privateKey: KeyObject; publicKey: KeyObject },
): Buffer {
  const attrs =
    san.length > 0
      ? implicitConstructed(
          0,
          seq(
            oid(OID.extensionRequest),
            setOf(seq(ext(OID.subjectAltName, false, encodeSan(san, '/san')))),
          ),
        )
      : implicitConstructed(0, Buffer.alloc(0));
  const cri = seq(int(0), encodeName(subject), spkiOf(key.publicKey), attrs);
  const alg = sigAlgFor(key.privateKey);
  return seq(cri, algId(alg), bits(cryptoSign(alg.hash, cri, key.privateKey)));
}

export interface ParsedCsr {
  subjectDer: Buffer;
  subject: string;
  spki: Buffer;
  publicKey: KeyObject;
  san: string[];
  keySpec?: KeySpec;
}

/** Parses a CSR (PEM or DER) and verifies its self-signature (proof of possession). */
export function parseCsr(input: string | Buffer, pointer = '/csr'): ParsedCsr {
  let der: Buffer;
  if (typeof input === 'string') {
    const blocks = pemBlocks(input).filter(
      (b) => b.label === 'CERTIFICATE REQUEST' || b.label === 'NEW CERTIFICATE REQUEST',
    );
    if (blocks.length !== 1)
      throw new PkiError('expected exactly one PEM CERTIFICATE REQUEST', pointer);
    der = blocks[0]!.der;
  } else der = input;
  if (der.length > 64 * 1024) throw new PkiError('the CSR is larger than 64 KiB', pointer);
  try {
    const [cri, alg, sig] = children(parse(der), 3, 3, 'CertificationRequest');
    const parts = children(cri!, 4, 4, 'CertificationRequestInfo');
    if (parts[0]!.tag !== TAG.INTEGER || parts[0]!.value.length !== 1 || parts[0]!.value[0] !== 0)
      throw new PkiError('unsupported CSR version', pointer);
    const subject = expect(parts[1], TAG.SEQUENCE, 'subject');
    const spki = expect(parts[2], TAG.SEQUENCE, 'subjectPKInfo');
    const attrs = parts[3]!;
    if (attrs.tag !== 0xa0) throw new PkiError('the CSR has no attribute set', pointer);
    let san: string[] = [];
    for (const a of attrs.children) {
      const [t, vals] = children(a, 2, 2, 'Attribute');
      if (readOid(t!) !== OID.extensionRequest) continue;
      for (const e of children(vals!.children[0]!, 0, 64, 'Extensions')) {
        const p = e.children;
        if (readOid(p[0]!) === OID.subjectAltName) san = decodeSan(readOctets(p[p.length - 1]!));
      }
    }
    let publicKey: KeyObject;
    try {
      publicKey = createPublicKey({ key: spki.raw, format: 'der', type: 'spki' });
    } catch {
      throw new PkiError('the CSR public key does not parse', pointer);
    }
    const sigAlg = readSigAlg(alg!, 'CSR signature algorithm');
    if (!verifyWith(sigAlg, cri!.raw, publicKey, readBits(sig!)))
      throw new PkiError(
        'the CSR signature does not verify (no proof of possession of the key)',
        pointer,
      );
    const out: ParsedCsr = {
      subjectDer: Buffer.from(subject.raw),
      subject: formatName(subject),
      spki: Buffer.from(spki.raw),
      publicKey,
      san,
    };
    const ks = keySpecOf(publicKey);
    if (ks !== undefined) out.keySpec = ks;
    return out;
  } catch (e) {
    if (e instanceof PkiError) throw e;
    throw new PkiError(
      `the CSR is malformed (${e instanceof Asn1Error ? e.message : 'unexpected structure'})`,
      pointer,
    );
  }
}

/** SANs as written in the document (`DNS:x` → `x`, `IP:a` → `a`, `email:e` → `e`). */
export function sanValues(san: string[]): string[] {
  return san.map((s) => s.replace(/^(DNS|IP|email):/, ''));
}

/** Issues an end-entity certificate for a CSR, signed by the CA (its certificate facts and key). */
export function signCsr(
  csr: ParsedCsr,
  ca: { facts: CertFacts; key: KeyObject },
  opts: { days: number; san?: string[]; crlUrl?: string; ocspUrl?: string; now?: Date },
): Issued {
  const now = opts.now ?? new Date();
  const notAfter = new Date(now.getTime() + opts.days * 86_400_000);
  if (notAfter.getTime() > Date.parse(ca.facts.notAfter))
    throw new PkiError(
      `the certificate would outlive its CA (CA valid until ${ca.facts.notAfter}); use fewer days`,
      '/days',
    );
  const alg = sigAlgFor(ca.key);
  const tbs = buildTbs(
    {
      serial: newSerial(),
      subject: csr.subjectDer,
      issuer: ca.facts.subjectDer,
      notBefore: new Date(now.getTime() - 5 * 60_000),
      notAfter,
      spki: csr.spki,
      ca: false,
      san: opts.san ?? sanValues(csr.san),
      ski: keyIdentifier(csr.spki),
      aki: ca.facts.ski ?? keyIdentifier(ca.facts.spki),
      rsa: csr.publicKey.asymmetricKeyType === 'rsa',
      ...(opts.crlUrl !== undefined ? { crlUrl: opts.crlUrl } : {}),
      ...(opts.ocspUrl !== undefined ? { ocspUrl: opts.ocspUrl } : {}),
    },
    alg,
  );
  const der = signTbs(tbs, alg, ca.key);
  return { der, pem: pemCert(der), facts: certFacts(der) };
}

/** Reads a PEM private key (unencrypted PKCS#8 / SEC1 / PKCS#1); `PkiError` without echoing anything. */
export function readPrivateKey(pem: string, pointer: string): KeyObject {
  const blocks = pemBlocks(pem).filter((b) => isPrivateKeyLabel(b.label));
  if (blocks.length !== 1) throw new PkiError('expected exactly one PEM private key', pointer);
  if (blocks[0]!.label.startsWith('ENCRYPTED'))
    throw new PkiError(
      'encrypted private keys are not accepted: send it unencrypted (the secret store encrypts it at rest) or use a PKCS#12 file',
      pointer,
    );
  try {
    return createPrivateKey({ key: pem, format: 'pem' });
  } catch {
    throw new PkiError('the private key does not parse', pointer);
  }
}

/** PEM of a private key (PKCS#8). */
export function privateKeyPem(key: KeyObject): string {
  return key.export({ type: 'pkcs8', format: 'pem' }).toString();
}
