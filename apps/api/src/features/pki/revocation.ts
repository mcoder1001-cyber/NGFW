import { createHash, createPublicKey, X509Certificate, type KeyObject } from 'node:crypto';
import { request as httpRequest } from 'node:http';
import { request as httpsRequest } from 'node:https';
import {
  Asn1Error,
  children,
  colonHex,
  expect,
  int,
  nul,
  octets,
  oid,
  parse,
  readBits,
  readIntBytes,
  readOctets,
  readOid,
  readSmallInt,
  readTime,
  seq,
  TAG,
} from './asn1.js';
import {
  formatName,
  OID,
  pemBlocks,
  PkiError,
  readSigAlg,
  toPem,
  verifyWith,
  type CertFacts,
} from './x509.js';

/**
 * Revocation for F-pki: CRL fetch + verification (RFC 5280 §5) and an OCSP client (RFC 6960). No responder or CA
 * revocation service of our own (out of scope). Fetches are bounded (size, time, no redirects) and never log bodies.
 */

// ---------------------------------------------------------------------------------------------------------------
// HTTP
// ---------------------------------------------------------------------------------------------------------------

export interface FetchOptions {
  method?: 'GET' | 'POST';
  body?: Buffer;
  contentType?: string;
  accept?: string;
  maxBytes: number;
  timeoutMs: number;
}

/** A bounded HTTP(S) fetcher (injectable for tests). */
export type Fetcher = (url: string, opts: FetchOptions) => Promise<Buffer>;

/** http/https only, no redirects, status 200 only, the body capped at maxBytes, one overall deadline. */
export const httpFetch: Fetcher = (url, opts) =>
  new Promise((resolve, reject) => {
    let u: URL;
    try {
      u = new URL(url);
    } catch {
      reject(new PkiError('not a URL'));
      return;
    }
    if (u.protocol !== 'http:' && u.protocol !== 'https:') {
      reject(new PkiError('only http and https URLs are fetched'));
      return;
    }
    const req = (u.protocol === 'https:' ? httpsRequest : httpRequest)(
      u,
      {
        method: opts.method ?? 'GET',
        headers: {
          ...(opts.contentType !== undefined ? { 'content-type': opts.contentType } : {}),
          ...(opts.accept !== undefined ? { accept: opts.accept } : {}),
          ...(opts.body !== undefined ? { 'content-length': String(opts.body.length) } : {}),
          'user-agent': 'vrx-pki',
        },
        timeout: opts.timeoutMs,
      },
      (res) => {
        if (res.statusCode !== 200) {
          res.resume();
          reject(new PkiError(`HTTP ${res.statusCode ?? '?'} from ${u.host}`));
          return;
        }
        const parts: Buffer[] = [];
        let n = 0;
        res.on('data', (c: Buffer) => {
          n += c.length;
          if (n > opts.maxBytes) {
            req.destroy();
            reject(new PkiError(`response from ${u.host} is larger than ${opts.maxBytes} bytes`));
            return;
          }
          parts.push(c);
        });
        res.on('end', () => resolve(Buffer.concat(parts)));
        res.on('error', () => reject(new PkiError(`reading the response from ${u.host} failed`)));
      },
    );
    const timer = setTimeout(() => req.destroy(new Error('timeout')), opts.timeoutMs);
    timer.unref();
    req.on('close', () => clearTimeout(timer));
    req.on('timeout', () => req.destroy(new Error('timeout')));
    req.on('error', (e) =>
      reject(
        new PkiError(
          e.message === 'timeout'
            ? `no answer from ${u.host} within ${opts.timeoutMs} ms`
            : `cannot reach ${u.host}`,
        ),
      ),
    );
    req.end(opts.body);
  });

// ---------------------------------------------------------------------------------------------------------------
// CRL
// ---------------------------------------------------------------------------------------------------------------

export const MAX_CRL = 10 * 1024 * 1024;

export interface CrlFacts {
  issuer: string;
  thisUpdate: string;
  nextUpdate: string | null;
  /** Revoked serial numbers (colon hex) with their revocation time. */
  revoked: { serial: string; at: string }[];
  /** CRL number extension, when present (decimal). */
  number: string | null;
  der: Buffer;
  pem: string;
}

/**
 * Parses a CRL (PEM or DER) and verifies it against the CA: issuer equal to the CA's subject (byte for byte), signature
 * by the CA's key. `PkiError` otherwise.
 */
export function parseCrl(input: Buffer, ca: CertFacts): CrlFacts {
  let der = input;
  const text = input.subarray(0, 64).toString('latin1');
  if (text.includes('-----BEGIN')) {
    const blocks = pemBlocks(input.toString('latin1')).filter((b) => b.label === 'X509 CRL');
    if (blocks.length !== 1) throw new PkiError('expected exactly one PEM X509 CRL');
    der = blocks[0]!.der;
  }
  if (der.length > MAX_CRL) throw new PkiError(`the CRL is larger than ${MAX_CRL} bytes`);
  try {
    const [tbs, alg, sig] = children(parse(der), 3, 3, 'CertificateList');
    const t = children(tbs!, 3, 7, 'TBSCertList');
    let i = 0;
    if (t[0]!.tag === TAG.INTEGER) {
      if (readSmallInt(t[0]!, 'CRL version') !== 1) throw new PkiError('unsupported CRL version');
      i = 1;
    }
    i++; // signature AlgorithmIdentifier (inner)
    const issuer = expect(t[i++], TAG.SEQUENCE, 'CRL issuer');
    const thisUpdate = readTime(t[i++]!, 'thisUpdate');
    let nextUpdate: Date | null = null;
    if (t[i] !== undefined && (t[i]!.tag === TAG.UTC_TIME || t[i]!.tag === TAG.GENERALIZED_TIME))
      nextUpdate = readTime(t[i++]!, 'nextUpdate');
    const revoked: { serial: string; at: string }[] = [];
    if (t[i] !== undefined && t[i]!.tag === TAG.SEQUENCE) {
      for (const e of t[i++]!.children) {
        const [serial, at] = children(e, 2, 3, 'revoked certificate');
        revoked.push({
          serial: colonHex(readIntBytes(serial!)),
          at: readTime(at!, 'revocationDate').toISOString(),
        });
      }
    }
    let number: string | null = null;
    if (t[i] !== undefined && t[i]!.tag === 0xa0) {
      for (const x of children(t[i]!.children[0]!, 0, 32, 'CRL extensions')) {
        const p = x.children;
        if (readOid(p[0]!) === '2.5.29.20') {
          let v = 0n;
          for (const b of readIntBytes(parse(readOctets(p[p.length - 1]!)), 'cRLNumber'))
            v = (v << 8n) | BigInt(b);
          number = v.toString();
        }
      }
    }
    if (!issuer.raw.equals(ca.subjectDer))
      throw new PkiError(
        `the CRL was issued by '${formatName(issuer)}', not by this CA ('${ca.subject}')`,
      );
    const pub = createPublicKey({ key: ca.spki, format: 'der', type: 'spki' });
    if (!verifyWith(readSigAlg(alg!, 'CRL signature algorithm'), tbs!.raw, pub, readBits(sig!)))
      throw new PkiError('the CRL signature does not verify with the CA key');
    return {
      issuer: formatName(issuer),
      thisUpdate: thisUpdate.toISOString(),
      nextUpdate: nextUpdate?.toISOString() ?? null,
      revoked,
      number,
      der: Buffer.from(der),
      pem: toPem('X509 CRL', Buffer.from(der)),
    };
  } catch (e) {
    if (e instanceof PkiError) throw e;
    throw new PkiError(
      `the CRL is malformed (${e instanceof Asn1Error ? e.message : 'unexpected structure'})`,
    );
  }
}

// ---------------------------------------------------------------------------------------------------------------
// OCSP
// ---------------------------------------------------------------------------------------------------------------

export const MAX_OCSP = 64 * 1024;

/** CertID (SHA-1, RFC 6960 §4.1.1) of a certificate issued by ca. */
function certId(serial: Buffer, ca: CertFacts): Buffer {
  const nameHash = createHash('sha1').update(ca.subjectDer).digest();
  const [, pk] = children(parse(ca.spki), 2, 2, 'SubjectPublicKeyInfo');
  const keyHash = createHash('sha1').update(readBits(pk!)).digest();
  return seq(seq(oid(OID.sha1), nul()), octets(nameHash), octets(keyHash), int(serial));
}

/** An OCSPRequest for one certificate (no nonce, no signature). */
export function ocspRequest(serial: Buffer, ca: CertFacts): Buffer {
  return seq(seq(seq(seq(certId(serial, ca)))));
}

export type OcspStatus = 'good' | 'revoked' | 'unknown';

export interface OcspResult {
  status: OcspStatus;
  thisUpdate: string;
  nextUpdate: string | null;
  revokedAt: string | null;
  producedAt: string;
}

const RESPONSE_STATUS = [
  'successful',
  'malformedRequest',
  'internalError',
  'tryLater',
  '4',
  'sigRequired',
  'unauthorized',
];

/**
 * Parses an OCSP response for the certificate with `serial` issued by ca and verifies it: signed by the CA itself or by
 * a responder certificate the CA issued with the OCSPSigning EKU; the single response must name this certificate.
 */
// OCSP without nextUpdate has no responder-provided expiry; cap its age at 24h.
export const MAX_OCSP_AGE_MS = 24 * 60 * 60_000;
const OCSP_CLOCK_SKEW_MS = 5 * 60_000;

export function parseOcspResponse(
  der: Buffer,
  serial: Buffer,
  ca: CertFacts & { x509: X509Certificate },
  now = new Date(),
): OcspResult {
  if (der.length > MAX_OCSP) throw new PkiError('the OCSP response is too large');
  try {
    const [st, bytes] = children(parse(der), 1, 2, 'OCSPResponse');
    if (st!.tag !== TAG.ENUMERATED) throw new Asn1Error('responseStatus');
    const code = st!.value[0] ?? 255;
    if (code !== 0)
      throw new PkiError(`the OCSP responder answered ${RESPONSE_STATUS[code] ?? code}`);
    const [rtype, rbody] = children(
      expect(bytes, 0xa0, 'responseBytes').children[0]!,
      2,
      2,
      'ResponseBytes',
    );
    if (readOid(rtype!) !== '1.3.6.1.5.5.7.48.1.1') throw new PkiError('not a basic OCSP response');
    const [tbs, alg, sig, certsNode] = children(
      parse(readOctets(rbody!)),
      3,
      4,
      'BasicOCSPResponse',
    );
    const rd = children(tbs!, 3, 5, 'ResponseData');
    let i = rd[0]!.tag === 0xa0 ? 1 : 0;
    i++; // responderID
    const producedAt = readTime(rd[i++]!, 'producedAt');
    if (producedAt.getTime() > now.getTime() + OCSP_CLOCK_SKEW_MS)
      throw new PkiError('the OCSP response producedAt is in the future');
    const responses = expect(rd[i], TAG.SEQUENCE, 'responses').children;
    // signer: the CA, or a delegated responder certificate the CA issued for OCSP signing
    const caKey: KeyObject = createPublicKey({ key: ca.spki, format: 'der', type: 'spki' });
    const sigAlg = readSigAlg(alg!, 'OCSP signature algorithm');
    if (!verifyWith(sigAlg, tbs!.raw, caKey, readBits(sig!))) {
      const certs =
        certsNode === undefined ? [] : children(certsNode.children[0]!, 0, 8, 'OCSP certs');
      // Node's X509Certificate.keyUsage lists the extended key usages
      const delegated = certs
        .map((c) => new X509Certificate(c.raw))
        .find(
          (c) =>
            c.checkIssued(ca.x509) &&
            c.verify(ca.x509.publicKey) &&
            (c.keyUsage ?? []).includes(OID.ocspSigning),
        );
      if (
        delegated === undefined ||
        !verifyWith(sigAlg, tbs!.raw, delegated.publicKey, readBits(sig!))
      )
        throw new PkiError(
          'the OCSP response is not signed by the CA or a responder it authorised',
        );
    }
    const wanted = certId(serial, ca);
    for (const r of responses) {
      const parts = children(r, 3, 5, 'SingleResponse');
      if (!parts[0]!.raw.equals(wanted)) continue;
      const cs = parts[1]!;
      const status: OcspStatus = cs.num === 0 ? 'good' : cs.num === 1 ? 'revoked' : 'unknown';
      const thisUpdate = readTime(parts[2]!, 'thisUpdate');
      const next = parts.slice(3).find((p) => p.tag === 0xa0);
      const nextUpdate = next !== undefined ? readTime(next.children[0]!, 'nextUpdate') : null;
      if (
        thisUpdate.getTime() > now.getTime() + OCSP_CLOCK_SKEW_MS ||
        thisUpdate.getTime() > producedAt.getTime() + OCSP_CLOCK_SKEW_MS
      )
        throw new PkiError('the OCSP response thisUpdate is in the future or after producedAt');
      if (nextUpdate !== null) {
        if (nextUpdate.getTime() < thisUpdate.getTime())
          throw new PkiError('the OCSP response nextUpdate precedes thisUpdate');
        if (nextUpdate.getTime() < now.getTime() - OCSP_CLOCK_SKEW_MS)
          throw new PkiError('the OCSP response is stale (nextUpdate passed)');
      } else if (
        now.getTime() - thisUpdate.getTime() > MAX_OCSP_AGE_MS + OCSP_CLOCK_SKEW_MS ||
        now.getTime() - producedAt.getTime() > MAX_OCSP_AGE_MS + OCSP_CLOCK_SKEW_MS
      ) {
        throw new PkiError('the OCSP response is stale (maximum age without nextUpdate passed)');
      }
      const revokedAt =
        status === 'revoked' && cs.children[0] !== undefined
          ? readTime(cs.children[0], 'revocationTime').toISOString()
          : null;
      return {
        status,
        thisUpdate: thisUpdate.toISOString(),
        nextUpdate: nextUpdate?.toISOString() ?? null,
        revokedAt,
        producedAt: producedAt.toISOString(),
      };
    }
    throw new PkiError('the OCSP response does not cover this certificate');
  } catch (e) {
    if (e instanceof PkiError) throw e;
    throw new PkiError(
      `the OCSP response is malformed (${e instanceof Asn1Error ? e.message : 'unexpected structure'})`,
    );
  }
}

/** Asks the OCSP responder at url about the certificate with `serial` issued by ca. */
export async function ocspCheck(
  url: string,
  serial: Buffer,
  ca: CertFacts & { x509: X509Certificate },
  fetcher: Fetcher = httpFetch,
): Promise<OcspResult> {
  const body = await fetcher(url, {
    method: 'POST',
    body: ocspRequest(serial, ca),
    contentType: 'application/ocsp-request',
    accept: 'application/ocsp-response',
    maxBytes: MAX_OCSP,
    timeoutMs: 10_000,
  });
  return parseOcspResponse(body, serial, ca);
}
