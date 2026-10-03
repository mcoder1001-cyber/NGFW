import { X509Certificate } from 'node:crypto';
import { createServer, type Server } from 'node:http';
import { readFileSync, writeFileSync } from 'node:fs';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { httpFetch, ocspCheck, parseCrl, parseOcspResponse } from './revocation.js';
import { openssl, opensslBytes, scratch } from './testkit.test.js';
import {
  buildCsr,
  certFacts,
  generateKey,
  parseCsr,
  parseDn,
  privateKeyPem,
  selfSignedCa,
  signCsr,
  type Issued,
} from './x509.js';

/** Slot 5's local CRL/OCSP responder port (wave-BC-numbers: TCP 3000+100·N+61), loopback only. */
const SLOT = Number(process.env['VRX_SLOT'] ?? '5');
const PORT = 3000 + 100 * SLOT + 61;

/**
 * A CA made by our code, two certificates it signed (one revoked), and openssl as the independent CRL issuer and OCSP
 * responder (`openssl ca -gencrl`, `openssl ocsp -reqin … -respout …`), served by a loopback HTTP server on the slot port.
 */
describe('F-pki revocation: CRL fetch + verify, OCSP check (openssl responder on 127.0.0.1)', () => {
  const s = scratch();
  let server: Server;
  let ca: Issued;
  let good: Issued;
  let revoked: Issued;
  const caKey = generateKey({ type: 'ecdsa', curve: 'p256' });
  const otherKey = generateKey({ type: 'ecdsa', curve: 'p256' });

  beforeAll(async () => {
    ca = selfSignedCa(parseDn('CN=w5 revocation CA, O=VRX test'), caKey, 30);
    const issue = (cn: string) => {
      const k = generateKey({ type: 'ecdsa', curve: 'p256' });
      return signCsr(
        parseCsr(buildCsr(parseDn(`CN=${cn}`), [], k)),
        { facts: ca.facts, key: caKey.privateKey },
        { days: 10 },
      );
    };
    good = issue('good.w5.example');
    revoked = issue('revoked.w5.example');
    const caPem = s.file('ca.pem', ca.pem);
    const keyPem = s.file('ca.key', privateKeyPem(caKey.privateKey));
    s.file('index.txt', '');
    s.file('crlnumber', '1000\n');
    const cfg = s.file(
      'ca.cnf',
      `[ ca ]\ndefault_ca = w5\n[ w5 ]\ndatabase = ${s.dir}/index.txt\ncrlnumber = ${s.dir}/crlnumber\ndefault_md = sha256\ndefault_crl_days = 1\n`,
    );
    openssl([
      'ca',
      '-config',
      cfg,
      '-keyfile',
      keyPem,
      '-cert',
      caPem,
      '-valid',
      s.file('good.pem', good.pem),
    ]);
    openssl([
      'ca',
      '-config',
      cfg,
      '-keyfile',
      keyPem,
      '-cert',
      caPem,
      '-revoke',
      s.file('revoked.pem', revoked.pem),
      '-crl_reason',
      'keyCompromise',
    ]);
    openssl([
      'ca',
      '-config',
      cfg,
      '-keyfile',
      keyPem,
      '-cert',
      caPem,
      '-gencrl',
      '-out',
      `${s.dir}/crl.pem`,
    ]);
    server = createServer((req, res) => {
      if (req.method === 'GET' && req.url === '/w5-ca.crl') {
        res
          .writeHead(200, { 'content-type': 'application/pkix-crl' })
          .end(readFileSync(`${s.dir}/crl.pem`));
        return;
      }
      if (req.method === 'GET' && req.url === '/big.crl') {
        res.writeHead(200).end(Buffer.alloc(11 * 1024 * 1024));
        return;
      }
      if (req.method === 'POST' && req.url === '/ocsp') {
        const parts: Buffer[] = [];
        req.on('data', (c: Buffer) => parts.push(c));
        req.on('end', () => {
          writeFileSync(`${s.dir}/req.der`, Buffer.concat(parts));
          opensslBytes([
            'ocsp',
            '-index',
            `${s.dir}/index.txt`,
            '-CA',
            caPem,
            '-rsigner',
            caPem,
            '-rkey',
            keyPem,
            '-reqin',
            `${s.dir}/req.der`,
            '-respout',
            `${s.dir}/resp.der`,
            '-ndays',
            '1',
          ]);
          res
            .writeHead(200, { 'content-type': 'application/ocsp-response' })
            .end(readFileSync(`${s.dir}/resp.der`));
        });
        return;
      }
      res.writeHead(404).end();
    });
    await new Promise<void>((resolve) => server.listen(PORT, '127.0.0.1', resolve));
  });
  afterAll(async () => {
    await new Promise<void>((resolve) => server?.close(() => resolve()));
    s.done();
  });

  it('fetches the CRL and verifies it against the CA; the revoked serial is listed', async () => {
    const body = await httpFetch(`http://127.0.0.1:${PORT}/w5-ca.crl`, {
      maxBytes: 10 << 20,
      timeoutMs: 5000,
    });
    const crl = parseCrl(body, ca.facts);
    expect(crl.issuer).toBe('CN=w5 revocation CA, O=VRX test');
    expect(crl.revoked.map((r) => r.serial)).toEqual([revoked.facts.serial]);
    expect(crl.number).toBe(String(0x1000));
    expect(crl.nextUpdate).not.toBeNull();
    console.log(
      `CRL from 127.0.0.1:${PORT}: issuer "${crl.issuer}", thisUpdate ${crl.thisUpdate}, revoked [${crl.revoked.map((r) => r.serial).join(', ')}]`,
    );
  });

  it('refuses a CRL of another CA and an oversized body', async () => {
    const otherCa = selfSignedCa(parseDn('CN=w5 revocation CA, O=VRX test'), otherKey, 30); // same name, other key
    const body = await httpFetch(`http://127.0.0.1:${PORT}/w5-ca.crl`, {
      maxBytes: 10 << 20,
      timeoutMs: 5000,
    });
    expect(() => parseCrl(body, otherCa.facts)).toThrow(/signature does not verify/);
    await expect(
      httpFetch(`http://127.0.0.1:${PORT}/big.crl`, { maxBytes: 10 << 20, timeoutMs: 5000 }),
    ).rejects.toThrow(/larger than/);
    await expect(
      httpFetch(`http://127.0.0.1:${PORT}/missing`, { maxBytes: 1024, timeoutMs: 5000 }),
    ).rejects.toThrow(/HTTP 404/);
  });

  it('OCSP: good / revoked from the openssl responder, signature checked against the CA', async () => {
    const caX = certFacts(ca.der);
    const g = await ocspCheck(
      `http://127.0.0.1:${PORT}/ocsp`,
      Buffer.from(good.facts.serial.replace(/:/g, ''), 'hex'),
      caX,
    );
    const r = await ocspCheck(
      `http://127.0.0.1:${PORT}/ocsp`,
      Buffer.from(revoked.facts.serial.replace(/:/g, ''), 'hex'),
      caX,
    );
    expect(g.status).toBe('good');
    expect(r.status).toBe('revoked');
    expect(r.revokedAt).not.toBeNull();
    console.log(
      `OCSP 127.0.0.1:${PORT}: ${good.facts.subject} → ${g.status}; ${revoked.facts.subject} → ${r.status} (at ${r.revokedAt})`,
    );
    // a response checked against the wrong CA is refused
    const otherCa = certFacts(
      selfSignedCa(parseDn('CN=w5 revocation CA, O=VRX test'), otherKey, 30).der,
    );
    const resp = readFileSync(`${s.dir}/resp.der`);
    expect(() =>
      parseOcspResponse(resp, Buffer.from(revoked.facts.serial.replace(/:/g, ''), 'hex'), otherCa),
    ).toThrow(/not signed by the CA|does not cover/);
    expect(new X509Certificate(ca.der).ca).toBe(true);
  });
});
