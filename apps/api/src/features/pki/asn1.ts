/**
 * A small, strict DER codec for the PKI feature (F-pki, decision D-F-pki-1 in docs/status/tasks/F-pki.md): Node 22's
 * `crypto` does every primitive (keys, ECDSA/RSA signatures, hashes, PBKDF2, ciphers) and parses certificates, but it
 * cannot encode a certificate or a CSR, nor parse PKCS#12, CRLs or OCSP. This module covers exactly the ASN.1 those
 * need. Decoding is for untrusted input (CSRs, PKCS#12 files, CRLs, OCSP responses): definite lengths only, minimal
 * length octets, bounded depth, every byte accounted for; errors are `Asn1Error` and never quote content.
 */

export class Asn1Error extends Error {}

/** Universal tags used by the PKI structures. */
export const TAG = {
  BOOLEAN: 0x01,
  INTEGER: 0x02,
  BIT_STRING: 0x03,
  OCTET_STRING: 0x04,
  NULL: 0x05,
  OID: 0x06,
  ENUMERATED: 0x0a,
  UTF8_STRING: 0x0c,
  SEQUENCE: 0x30,
  SET: 0x31,
  PRINTABLE_STRING: 0x13,
  T61_STRING: 0x14,
  IA5_STRING: 0x16,
  UTC_TIME: 0x17,
  GENERALIZED_TIME: 0x18,
  UNIVERSAL_STRING: 0x1c,
  BMP_STRING: 0x1e,
} as const;

// ---------------------------------------------------------------------------------------------------------------
// encoding
// ---------------------------------------------------------------------------------------------------------------

function lengthOctets(n: number): Buffer {
  if (n < 0x80) return Buffer.from([n]);
  const bytes: number[] = [];
  for (let v = n; v > 0; v = Math.floor(v / 256)) bytes.unshift(v & 0xff);
  return Buffer.from([0x80 | bytes.length, ...bytes]);
}

/** One TLV with a single-byte tag. */
export function tlv(tag: number, content: Uint8Array): Buffer {
  return Buffer.concat([Buffer.from([tag]), lengthOctets(content.length), content]);
}

export const seq = (...items: Uint8Array[]): Buffer => tlv(TAG.SEQUENCE, Buffer.concat(items));

/** SET OF in DER order (elements sorted by their encodings). */
export const setOf = (...items: Buffer[]): Buffer =>
  tlv(TAG.SET, Buffer.concat([...items].sort(Buffer.compare)));

/** INTEGER from a bigint/number, or from an unsigned big-endian magnitude (a serial number). */
export function int(v: bigint | number | Uint8Array): Buffer {
  let bytes: Buffer;
  if (v instanceof Uint8Array) {
    let i = 0;
    while (i < v.length - 1 && v[i] === 0) i++;
    bytes = Buffer.from(v.subarray(i));
    if (bytes.length === 0) bytes = Buffer.from([0]);
    if (bytes[0]! & 0x80) bytes = Buffer.concat([Buffer.from([0]), bytes]);
  } else {
    let n = BigInt(v);
    if (n < 0n) throw new Asn1Error('negative INTEGER not supported');
    const out: number[] = [];
    do {
      out.unshift(Number(n & 0xffn));
      n >>= 8n;
    } while (n > 0n);
    if (out[0]! & 0x80) out.unshift(0);
    bytes = Buffer.from(out);
  }
  return tlv(TAG.INTEGER, bytes);
}

export function oid(dotted: string): Buffer {
  const arcs = dotted.split('.').map((a) => {
    if (!/^\d+$/.test(a)) throw new Asn1Error(`bad OID ${dotted}`);
    return BigInt(a);
  });
  if (arcs.length < 2) throw new Asn1Error(`bad OID ${dotted}`);
  const out: number[] = [];
  const push = (n: bigint) => {
    const b: number[] = [Number(n & 0x7fn)];
    for (let x = n >> 7n; x > 0n; x >>= 7n) b.unshift(Number(x & 0x7fn) | 0x80);
    out.push(...b);
  };
  push(arcs[0]! * 40n + arcs[1]!);
  for (const a of arcs.slice(2)) push(a);
  return tlv(TAG.OID, Buffer.from(out));
}

export const octets = (b: Uint8Array): Buffer => tlv(TAG.OCTET_STRING, b);
export const bits = (b: Uint8Array, unused = 0): Buffer =>
  tlv(TAG.BIT_STRING, Buffer.concat([Buffer.from([unused]), b]));
export const nul = (): Buffer => Buffer.from([TAG.NULL, 0x00]);
export const bool = (v: boolean): Buffer => tlv(TAG.BOOLEAN, Buffer.from([v ? 0xff : 0x00]));
export const utf8 = (s: string): Buffer => tlv(TAG.UTF8_STRING, Buffer.from(s, 'utf8'));
export const printable = (s: string): Buffer => tlv(TAG.PRINTABLE_STRING, Buffer.from(s, 'latin1'));
export const ia5 = (s: string): Buffer => tlv(TAG.IA5_STRING, Buffer.from(s, 'latin1'));

/** RFC 5280 §4.1.2.5: UTCTime through 2049, GeneralizedTime from 2050; whole seconds, Zulu. */
export function time(d: Date): Buffer {
  const iso = d.toISOString(); // YYYY-MM-DDTHH:MM:SS.sssZ
  const compact = iso.slice(0, 19).replace(/[-:T]/g, '') + 'Z';
  const year = d.getUTCFullYear();
  if (year < 1950 || year > 9999) throw new Asn1Error('time out of range');
  return year < 2050
    ? tlv(TAG.UTC_TIME, Buffer.from(compact.slice(2), 'latin1'))
    : tlv(TAG.GENERALIZED_TIME, Buffer.from(compact, 'latin1'));
}

export const generalizedTime = (d: Date): Buffer =>
  tlv(
    TAG.GENERALIZED_TIME,
    Buffer.from(d.toISOString().slice(0, 19).replace(/[-:T]/g, '') + 'Z', 'latin1'),
  );

/** `[n] EXPLICIT` around a complete TLV (constructed context tag). */
export const explicit = (n: number, inner: Uint8Array): Buffer => tlv(0xa0 | n, inner);
/** `[n] IMPLICIT` primitive: the context tag replaces the universal one of a primitive value. */
export const implicitPrimitive = (n: number, content: Uint8Array): Buffer => tlv(0x80 | n, content);
/** `[n] IMPLICIT` constructed (SET OF / SEQUENCE content under a context tag). */
export const implicitConstructed = (n: number, content: Uint8Array): Buffer =>
  tlv(0xa0 | n, content);

// ---------------------------------------------------------------------------------------------------------------
// decoding
// ---------------------------------------------------------------------------------------------------------------

/** One decoded TLV. `raw` is the whole encoding (tag, length, content); `value` the content octets. */
export interface Node {
  /** The identifier octet (class, constructed bit, tag number < 31). */
  tag: number;
  cls: 'universal' | 'application' | 'context' | 'private';
  constructed: boolean;
  /** The tag number (low five bits). */
  num: number;
  raw: Buffer;
  value: Buffer;
  /** Child TLVs of a constructed node. */
  children: Node[];
}

const CLASSES = ['universal', 'application', 'context', 'private'] as const;

export interface ParseOptions {
  /** Maximum nesting depth (default 32). */
  maxDepth?: number;
}

function parseAt(
  buf: Buffer,
  pos: number,
  depth: number,
  maxDepth: number,
  limit = buf.length,
): [Node, number] {
  if (depth > maxDepth) throw new Asn1Error(`nesting deeper than ${maxDepth}`);
  if (pos + 2 > limit) throw new Asn1Error('truncated TLV');
  const tag = buf[pos]!;
  if ((tag & 0x1f) === 0x1f) throw new Asn1Error('multi-byte tags are not supported');
  let len = buf[pos + 1]!;
  let hdr = 2;
  if (len === 0x80)
    throw new Asn1Error('indefinite length (BER) is not supported: re-export the file as DER');
  if (len & 0x80) {
    const n = len & 0x7f;
    if (n > 4) throw new Asn1Error('length too large');
    if (pos + 2 + n > limit) throw new Asn1Error('truncated length');
    len = 0;
    for (let i = 0; i < n; i++) len = len * 256 + buf[pos + 2 + i]!;
    if (len < 0x80 || buf[pos + 2] === 0) throw new Asn1Error('non-minimal length encoding');
    hdr += n;
  }
  const end = pos + hdr + len;
  if (end > limit) throw new Asn1Error('truncated content');
  const constructed = (tag & 0x20) !== 0;
  const node: Node = {
    tag,
    cls: CLASSES[tag >> 6]!,
    constructed,
    num: tag & 0x1f,
    raw: buf.subarray(pos, end),
    value: buf.subarray(pos + hdr, end),
    children: [],
  };
  if (constructed) {
    let p = pos + hdr;
    while (p < end) {
      const [child, next] = parseAt(buf, p, depth + 1, maxDepth, end);
      node.children.push(child);
      p = next;
    }
  }
  return [node, end];
}

/** Parses exactly one TLV covering all of `buf`. */
export function parse(buf: Uint8Array, opts: ParseOptions = {}): Node {
  const b = Buffer.isBuffer(buf) ? buf : Buffer.from(buf);
  const [node, end] = parseAt(b, 0, 0, opts.maxDepth ?? 32);
  if (end !== b.length) throw new Asn1Error('trailing data after the DER structure');
  return node;
}

/** The node's children, checked against an expected tag and a count range. */
export function expect(n: Node | undefined, tag: number, what: string): Node {
  if (n === undefined || n.tag !== tag) throw new Asn1Error(`${what}: unexpected structure`);
  return n;
}

export function children(n: Node, min: number, max: number, what: string): Node[] {
  if (!n.constructed || n.children.length < min || n.children.length > max)
    throw new Asn1Error(`${what}: unexpected structure`);
  return n.children;
}

export function readInt(n: Node, what = 'INTEGER'): bigint {
  expect(n, TAG.INTEGER, what);
  if (n.value.length === 0) throw new Asn1Error(`${what}: empty`);
  if (n.value[0]! & 0x80) throw new Asn1Error(`${what}: negative`);
  let v = 0n;
  for (const b of n.value) v = (v << 8n) | BigInt(b);
  return v;
}

/** Small INTEGER as a number (versions, iteration counts). */
export function readSmallInt(n: Node, what: string, max = 0x7fffffff): number {
  const v = readInt(n, what);
  if (v > BigInt(max)) throw new Asn1Error(`${what}: too large`);
  return Number(v);
}

/** Unsigned magnitude of an INTEGER (serial numbers), without the sign byte. */
export function readIntBytes(n: Node, what = 'INTEGER'): Buffer {
  expect(n, TAG.INTEGER, what);
  if (n.value.length === 0 || n.value[0]! & 0x80)
    throw new Asn1Error(`${what}: not a positive integer`);
  let i = 0;
  while (i < n.value.length - 1 && n.value[i] === 0) i++;
  return n.value.subarray(i);
}

export function readOid(n: Node, what = 'OID'): string {
  expect(n, TAG.OID, what);
  const v = n.value;
  if (v.length === 0 || v[v.length - 1]! & 0x80) throw new Asn1Error(`${what}: malformed`);
  const arcs: bigint[] = [];
  let cur = 0n;
  for (const b of v) {
    cur = (cur << 7n) | BigInt(b & 0x7f);
    if ((b & 0x80) === 0) {
      arcs.push(cur);
      cur = 0n;
    }
  }
  const first = arcs.shift()!;
  const a = first < 80n ? first / 40n : 2n;
  const b = first - a * 40n;
  return [a, b, ...arcs].join('.');
}

export function readBits(n: Node, what = 'BIT STRING'): Buffer {
  expect(n, TAG.BIT_STRING, what);
  if (n.value.length === 0 || n.value[0]! > 7) throw new Asn1Error(`${what}: malformed`);
  return n.value.subarray(1);
}

export function readOctets(n: Node, what = 'OCTET STRING'): Buffer {
  return expect(n, TAG.OCTET_STRING, what).value;
}

/** A directory string or other character string as text. */
export function readString(n: Node, what = 'string'): string {
  switch (n.tag) {
    case TAG.UTF8_STRING:
      return n.value.toString('utf8');
    case TAG.PRINTABLE_STRING:
    case TAG.IA5_STRING:
    case TAG.T61_STRING:
      return n.value.toString('latin1');
    case TAG.BMP_STRING: {
      if (n.value.length % 2) throw new Asn1Error(`${what}: odd BMPString`);
      const swapped = Buffer.from(n.value);
      swapped.swap16();
      return swapped.toString('utf16le');
    }
    case TAG.UNIVERSAL_STRING: {
      if (n.value.length % 4) throw new Asn1Error(`${what}: bad UniversalString`);
      let s = '';
      for (let i = 0; i < n.value.length; i += 4)
        s += String.fromCodePoint(n.value.readUInt32BE(i));
      return s;
    }
  }
  throw new Asn1Error(`${what}: not a character string`);
}

/** UTCTime / GeneralizedTime (Zulu, whole seconds; fractional seconds of GeneralizedTime accepted). */
export function readTime(n: Node, what = 'time'): Date {
  const s = n.value.toString('latin1');
  let m: RegExpMatchArray | null;
  let iso: string;
  if (n.tag === TAG.UTC_TIME && (m = s.match(/^(\d{2})(\d{2})(\d{2})(\d{2})(\d{2})(\d{2})Z$/))) {
    const yy = Number(m[1]);
    iso = `${yy < 50 ? 2000 + yy : 1900 + yy}-${m[2]}-${m[3]}T${m[4]}:${m[5]}:${m[6]}Z`;
  } else if (
    n.tag === TAG.GENERALIZED_TIME &&
    (m = s.match(/^(\d{4})(\d{2})(\d{2})(\d{2})(\d{2})(\d{2})(\.\d{1,9})?Z$/))
  ) {
    iso = `${m[1]}-${m[2]}-${m[3]}T${m[4]}:${m[5]}:${m[6]}${m[7] ?? ''}Z`;
  } else {
    throw new Asn1Error(`${what}: not a Zulu UTCTime/GeneralizedTime`);
  }
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) throw new Asn1Error(`${what}: invalid date`);
  return d;
}

/** Hex of bytes, upper case, colon separated (serial numbers, fingerprints). */
export function colonHex(b: Uint8Array): string {
  return (Buffer.from(b).toString('hex').toUpperCase().match(/../g) ?? []).join(':');
}
