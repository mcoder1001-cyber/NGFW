import { describe, expect, it } from 'vitest';
import {
  PkiCaSchema,
  PkiCertificateSchema,
  PkiCsrSchema,
  PkiKeySpecSchema,
  PkiSchema,
} from './vpn.js';

const issued = {
  subject: 'CN=server',
  issuer: 'CN=CA',
  serial: '01',
  notBefore: '2030-01-01T00:00:00Z',
  notAfter: '2030-02-01T00:00:00Z',
  fingerprint: Array(32).fill('AB').join(':'),
  ca: false,
};
const certificate = { certificateRef: 'cert/server', privateKeyRef: 'key/server' };

describe('additive PKI action contracts', () => {
  it('preserves existing reference-only certificate configuration', () => {
    expect(PkiCertificateSchema.safeParse(certificate).success).toBe(true);
  });
  it.each([
    { type: 'ecdsa', curve: 'p256' },
    { type: 'ecdsa', curve: 'p384' },
    { type: 'rsa', bits: 2048 },
    { type: 'rsa', bits: 3072 },
    { type: 'rsa', bits: 4096 },
  ])('accepts supported key specification %j', (keySpec) => {
    expect(PkiKeySpecSchema.safeParse(keySpec).success).toBe(true);
  });
  it.each([
    { type: 'rsa', curve: 'p256' },
    { type: 'ecdsa', bits: 2048 },
  ])('rejects algorithm-mismatched fields %j', (keySpec) => {
    expect(PkiKeySpecSchema.safeParse(keySpec).success).toBe(false);
  });
  it('accepts CSR parameters without material', () => {
    expect(
      PkiCsrSchema.safeParse({
        subject: 'CN=server.example.test, O=Example',
        san: ['server.example.test', '192.0.2.1', 'user@example.test'],
      }).success,
    ).toBe(true);
  });
  it('rejects non-CA issued metadata on a CA', () => {
    const parsed = PkiCaSchema.safeParse({ certificateRef: 'cert/ca', issued });
    expect(parsed.success).toBe(false);
    if (!parsed.success)
      expect(parsed.error.issues).toContainEqual(
        expect.objectContaining({ path: ['issued', 'ca'] }),
      );
  });
  it('rejects expiry alerts at or beyond validity with their field pointer', () => {
    const parsed = PkiCertificateSchema.safeParse({ ...certificate, issued, expiryAlertDays: 31 });
    expect(parsed.success).toBe(false);
    if (!parsed.success)
      expect(parsed.error.issues).toContainEqual(
        expect.objectContaining({ path: ['expiryAlertDays'] }),
      );
  });
  it('rejects conflicting recorded issuer metadata', () => {
    const parsed = PkiSchema.safeParse({
      cas: {
        ca: { certificateRef: 'cert/ca', issued: { ...issued, subject: 'CN=Other CA', ca: true } },
      },
      certificates: { server: { ...certificate, ca: 'ca', issued } },
    });
    expect(parsed.success).toBe(false);
    if (!parsed.success)
      expect(parsed.error.issues).toContainEqual(
        expect.objectContaining({ path: ['certificates', 'server', 'ca'] }),
      );
  });
});
