import { createPrivateKey, X509Certificate, type KeyObject } from 'node:crypto';
import { createSecureContext, type SecureContextOptions } from 'node:tls';
import type { ProblemIssue } from '../../common/problem.js';

/** `management.tls.minVersion` → the Node.js protocol name. */
export type MinVersion = '1.2' | '1.3';
const PROTOCOL: Record<MinVersion, 'TLSv1.2' | 'TLSv1.3'> = { '1.2': 'TLSv1.2', '1.3': 'TLSv1.3' };

export const TLS_POINTER = '/management/tls';
export const CERT_POINTER = `${TLS_POINTER}/certificateRef`;
export const KEY_POINTER = `${TLS_POINTER}/privateKeyRef`;

/** Public facts of the leaf certificate. Never carries key material. */
export interface CertInfo {
  subject: string;
  issuer: string;
  subjectAltNames: string[];
  serialNumber: string;
  notBefore: string;
  notAfter: string;
  fingerprintSha256: string;
  chainLength: number;
}

export interface TlsCheck {
  issues: ProblemIssue[];
  info?: CertInfo;
  /**
   * Options for `tls.Server#setSecureContext` (holds the key: never log or return it). The protocol floor must sit
   * on the server's own context: an SNICallback context does not change the negotiated version.
   */
  options?: SecureContextOptions;
}

const PEM_CERT = /-----BEGIN CERTIFICATE-----[\s\S]+?-----END CERTIFICATE-----/g;

function certInfo(leaf: X509Certificate, chainLength: number): CertInfo {
  return {
    subject: leaf.subject.replace(/\n/g, ', '),
    issuer: leaf.issuer.replace(/\n/g, ', '),
    subjectAltNames: (leaf.subjectAltName ?? '')
      .split(',')
      .map((s) => s.trim())
      .filter((s) => s.length > 0),
    serialNumber: leaf.serialNumber,
    notBefore: new Date(leaf.validFrom).toISOString(),
    notAfter: new Date(leaf.validTo).toISOString(),
    fingerprintSha256: leaf.fingerprint256,
    chainLength,
  };
}

const issue = (pointer: string, message: string, rule: string): ProblemIssue => ({
  pointer,
  message,
  rule: `management.tls.${rule}`,
});

/**
 * Checks a certificate chain + private key for the API's TLS listener (F-management-ui): the PEM parses, the key
 * matches the leaf, the leaf is valid at `now`, and Node.js builds a secure context with `minVersion`. Messages
 * never quote the key or parser output that could contain it.
 */
export function validateTlsMaterial(
  certPem: string,
  keyPem: string,
  minVersion: MinVersion,
  now: Date = new Date(),
): TlsCheck {
  const blocks = certPem.match(PEM_CERT) ?? [];
  if (blocks.length === 0)
    return { issues: [issue(CERT_POINTER, 'the secret does not hold a PEM certificate', 'certificate-pem')] };
  let chain: X509Certificate[];
  try {
    chain = blocks.map((b) => new X509Certificate(b));
  } catch {
    return { issues: [issue(CERT_POINTER, 'the PEM certificate cannot be parsed', 'certificate-pem')] };
  }
  const leaf = chain[0]!;
  const info = certInfo(leaf, chain.length);
  let key: KeyObject;
  try {
    key = createPrivateKey(keyPem);
  } catch {
    return { info, issues: [issue(KEY_POINTER, 'the secret does not hold a readable PEM private key (encrypted keys are not supported)', 'key-pem')] };
  }
  const issues: ProblemIssue[] = [];
  if (!leaf.checkPrivateKey(key))
    issues.push(issue(KEY_POINTER, 'the private key does not match the certificate', 'key-mismatch'));
  if (now.getTime() > new Date(leaf.validTo).getTime())
    issues.push(issue(CERT_POINTER, `the certificate expired on ${info.notAfter}`, 'expired'));
  else if (now.getTime() < new Date(leaf.validFrom).getTime())
    issues.push(issue(CERT_POINTER, `the certificate is not valid before ${info.notBefore}`, 'not-yet-valid'));
  if (issues.length > 0) return { info, issues };
  try {
    const options: SecureContextOptions = { cert: certPem, key: keyPem, minVersion: PROTOCOL[minVersion] };
    createSecureContext(options); // throws on anything Node.js cannot serve
    return { info, issues, options };
  } catch {
    return { info, issues: [issue(TLS_POINTER, 'Node.js refused the certificate and key for a TLS context', 'context')] };
  }
}
