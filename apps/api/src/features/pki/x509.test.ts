import { createPrivateKey, X509Certificate } from 'node:crypto';
import { describe, expect, it } from 'vitest';
import { openssl, scratch } from './testkit.test.js';
import {
  buildCsr,
  certFacts,
  generateKey,
  normaliseKeySpec,
  parseCsr,
  parseDn,
  pemBlocks,
  PkiError,
  selfSignedCa,
  signCsr,
  toPem,
} from './x509.js';

/**
 * Acceptance "CA → CSR → sign → `openssl verify -CAfile ca.pem cert.pem` OK, run by the test": every certificate and
 * CSR our DER encoder writes is read back by OpenSSL, and the chain verifies. Names carry the slot prefix w5.
 */
describe('F-pki X.509: CA → CSR → sign, verified by openssl', () => {
  for (const spec of [
    { ca: { type: 'ecdsa', curve: 'p384' }, leaf: { type: 'ecdsa', curve: 'p256' } },
    { ca: { type: 'rsa', bits: 3072 }, leaf: { type: 'rsa', bits: 2048 } },
    { ca: { type: 'ecdsa', curve: 'p256' }, leaf: { type: 'rsa', bits: 2048 } },
  ] as const) {
    it(`CA ${spec.ca.type}/${'curve' in spec.ca ? spec.ca.curve : spec.ca.bits} signs a ${spec.leaf.type} CSR`, () => {
      const s = scratch();
      try {
        const caKey = generateKey(normaliseKeySpec(spec.ca));
        const ca = selfSignedCa(parseDn('CN=w5 root CA, O=NGFW test, C=CH'), caKey, 3650);
        expect(ca.facts.ca).toBe(true);
        expect(ca.facts.subject).toBe('CN=w5 root CA, O=NGFW test, C=CH');
        expect(ca.facts.issuer).toBe(ca.facts.subject);

        const leafKey = generateKey(normaliseKeySpec(spec.leaf));
        const csrDer = buildCsr(
          parseDn('CN=gw.w5.example, O=NGFW test'),
          ['gw.w5.example', '10.5.250.1', '2001:db8:5::1', '*.vpn.w5.example'],
          leafKey,
        );
        const csrPem = toPem('CERTIFICATE REQUEST', csrDer);
        const csrFile = s.file('gw.csr', csrPem);
        // openssl checks the CSR's self-signature and reads the SAN extension request
        expect(openssl(['req', '-in', csrFile, '-noout', '-verify'])).toMatch(
          /verify OK|Certificate request self-signature verify OK/,
        );
        expect(openssl(['req', '-in', csrFile, '-noout', '-text'])).toContain('DNS:gw.w5.example');

        const parsed = parseCsr(csrPem);
        expect(parsed.subject).toBe('CN=gw.w5.example, O=NGFW test');
        expect(parsed.san).toEqual([
          'DNS:gw.w5.example',
          'IP:10.5.250.1',
          'IP:2001:db8:5::1',
          'DNS:*.vpn.w5.example',
        ]);

        const cert = signCsr(
          parsed,
          { facts: ca.facts, key: caKey.privateKey },
          {
            days: 365,
            crlUrl: 'http://localhost.localdomain:3561/w5-root.crl',
            ocspUrl: 'http://localhost.localdomain:3561/ocsp',
          },
        );
        expect(cert.facts.ca).toBe(false);
        expect(cert.facts.issuer).toBe(ca.facts.subject);
        expect(new X509Certificate(cert.der).checkIssued(new X509Certificate(ca.der))).toBe(true);
        expect(new X509Certificate(cert.der).checkPrivateKey(leafKey.privateKey)).toBe(true);

        const caFile = s.file('ca.pem', ca.pem);
        const certFile = s.file('cert.pem', cert.pem);
        const out = openssl(['verify', '-CAfile', caFile, certFile]);
        // the acceptance line, printed for the status file
        console.log(
          `openssl verify -CAfile ca.pem cert.pem → ${out.trim().replace(s.dir + '/', '')}`,
        );
        expect(out.trim()).toBe(`${certFile}: OK`);
        const text = openssl(['x509', '-in', certFile, '-noout', '-text']);
        expect(text).toContain('CA:FALSE');
        expect(text).toMatch(/TLS Web Server Authentication, TLS Web Client Authentication/);
        expect(text).toContain('URI:http://localhost.localdomain:3561/w5-root.crl');
        expect(text).toContain('OCSP - URI:http://localhost.localdomain:3561/ocsp');
        expect(openssl(['x509', '-in', caFile, '-noout', '-text'])).toMatch(/CA:TRUE/);
      } finally {
        s.done();
      }
    });
  }

  it('signs a CSR made by openssl (interop) and refuses a tampered one', () => {
    const s = scratch();
    try {
      const keyFile = s.file('k.pem', '');
      openssl([
        'req',
        '-new',
        '-newkey',
        'ec',
        '-pkeyopt',
        'ec_paramgen_curve:P-256',
        '-nodes',
        '-keyout',
        keyFile,
        '-subj',
        '/CN=peer.w5.example/O=NGFW test',
        '-addext',
        'subjectAltName=DNS:peer.w5.example',
        '-out',
        s.dir + '/peer.csr',
      ]);
      const csrPem = openssl(['req', '-in', s.dir + '/peer.csr']);
      const parsed = parseCsr(csrPem);
      expect(parsed.subject).toBe('CN=peer.w5.example, O=NGFW test');
      expect(parsed.san).toEqual(['DNS:peer.w5.example']);
      const caKey = generateKey({ type: 'ecdsa', curve: 'p256' });
      const ca = selfSignedCa(parseDn('CN=w5 interop CA'), caKey, 30);
      const cert = signCsr(parsed, { facts: ca.facts, key: caKey.privateKey }, { days: 7 });
      expect(
        openssl(['verify', '-CAfile', s.file('ca.pem', ca.pem), s.file('c.pem', cert.pem)]).trim(),
      ).toMatch(/: OK$/);

      const der = pemBlocks(csrPem)[0]!.der;
      der[der.length - 5]! ^= 0x01; // flip a signature bit
      expect(() => parseCsr(der)).toThrow(/signature does not verify|malformed/);
    } finally {
      s.done();
    }
  });

  it('a certificate would outlive its CA → refused with a pointer', () => {
    const caKey = generateKey({ type: 'ecdsa', curve: 'p256' });
    const ca = selfSignedCa(parseDn('CN=w5 short CA'), caKey, 10);
    const leafKey = generateKey({ type: 'ecdsa', curve: 'p256' });
    const csr = parseCsr(buildCsr(parseDn('CN=x'), [], leafKey));
    try {
      signCsr(csr, { facts: ca.facts, key: caKey.privateKey }, { days: 30 });
      expect.unreachable();
    } catch (e) {
      expect(e).toBeInstanceOf(PkiError);
      expect((e as PkiError).pointer).toBe('/days');
    }
  });

  it('parses distinguished names strictly', () => {
    expect(parseDn('CN=a, O=b c, C=CH, DC=example, E=x@y.z')).toHaveLength(5);
    for (const bad of ['CN=', 'XX=a', 'CN=a+b', 'C=Switzerland', 'CN="q"', 'CN=a;b']) {
      expect(() => parseDn(bad)).toThrow(PkiError);
    }
  });

  it('reads the facts openssl reports', () => {
    const s = scratch();
    try {
      const keyFile = s.file('k.pem', '');
      const certFile = s.dir + '/c.pem';
      openssl([
        'req',
        '-x509',
        '-newkey',
        'rsa:2048',
        '-nodes',
        '-keyout',
        keyFile,
        '-subj',
        '/C=CH/O=NGFW test/CN=w5-openssl-ca',
        '-days',
        '30',
        '-addext',
        'basicConstraints=critical,CA:TRUE',
        '-out',
        certFile,
      ]);
      const pem = openssl(['x509', '-in', certFile]);
      const f = certFacts(pemBlocks(pem)[0]!.der);
      expect(f.subject).toBe('C=CH, O=NGFW test, CN=w5-openssl-ca');
      expect(f.ca).toBe(true);
      expect(f.keySpec).toEqual({ type: 'rsa', bits: 2048 });
      const fp = openssl(['x509', '-in', certFile, '-noout', '-fingerprint', '-sha256'])
        .trim()
        .split('=')[1];
      expect(f.fingerprint).toBe(fp);
      const serial = openssl(['x509', '-in', certFile, '-noout', '-serial']).trim().split('=')[1]!;
      expect(BigInt('0x' + f.serial.replace(/:/g, ''))).toBe(BigInt('0x' + serial));
      expect(createPrivateKey(openssl(['pkey', '-in', keyFile])).asymmetricKeyType).toBe('rsa');
    } finally {
      s.done();
    }
  });
});

describe('PKI distinguished name control-character validation', () => {
  it.each([0, 10, 31, 127])('refuses control character %d inside an attribute', (code) => {
    expect(() => parseDn(`CN=a${String.fromCharCode(code)}b`, '/subject')).toThrow(PkiError);
  });
});
