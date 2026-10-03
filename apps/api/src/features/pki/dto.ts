import {
  PkiCaSchema,
  PkiCertificateSchema,
  PkiCsrSchema,
  PkiIssuedSchema,
  PkiKeySpecSchema,
} from '@ngfw/schema';
import { z } from 'zod';

// Public response contracts only. Reuse the configuration's certificate/key metadata schemas; secret material
// belongs solely to request bodies and is never a response property.
const name = z.string();
const date = z.iso.datetime({ offset: true });
const issued = PkiIssuedSchema.extend({ ca: z.boolean() });
/** Parsed external requests can carry dotted-OID attributes and omit unsupported key metadata. This is public
 * observation data, distinct from the product's stricter editable CSR configuration. Bounds follow the 64 KiB CSR.
 */
export const PkiParsedCsrOut = z.strictObject({
  subject: z.string().min(1).max(65536),
  san: z.array(z.string().max(65536)).max(65536),
  keySpec: PkiKeySpecSchema.optional(),
});
const patch = z
  .strictObject({ ...PkiCaSchema.shape, ...PkiCertificateSchema.shape, csr: PkiParsedCsrOut })
  .partial();
const staging = { staged: z.boolean(), pointer: z.string(), patch, reason: z.string().optional() };
const certificate = { name, certificateRef: z.string(), issued };

export const PkiCaOut = z.strictObject({
  ...certificate,
  keyRef: z.string(),
  keySpec: PkiKeySpecSchema,
  certificatePem: z.string(),
  ...staging,
});
export const PkiCsrOut = z.strictObject({
  name,
  keyRef: z.string(),
  csr: PkiCsrSchema,
  csrPem: z.string(),
});
export const PkiSignOut = z.strictObject({
  ...certificate,
  certificatePem: z.string(),
  ...staging,
});
export const PkiImportOut = z.strictObject({
  ...certificate,
  as: z.enum(['ca', 'certificate']),
  keyRef: z.string().nullable(),
  chainLength: z.int().min(1).max(8),
  ...staging,
});
export const PkiExportOut = z.strictObject({
  name,
  kind: z.enum(['certificate', 'ca', 'crl']),
  ref: z.string(),
  pem: z.string(),
  fingerprint: z.string().nullable(),
});

const crl = z.strictObject({
  url: z.string(),
  fetchedAt: date.nullable(),
  thisUpdate: date.nullable(),
  nextUpdate: date.nullable(),
  revoked: z.array(z.string()),
  number: z.string().nullable(),
  error: z.string().nullable(),
});
export const PkiCrlOut = z.strictObject({
  results: z.array(crl.extend({ ca: name, stored: z.boolean() })),
});
const ocspStatus = z.enum(['good', 'revoked', 'unknown', 'error']);
const ocsp = z.strictObject({
  status: ocspStatus,
  checkedAt: date,
  revokedAt: date.nullable(),
  error: z.string().nullable(),
});
export const PkiOcspOut = z.strictObject({
  results: z.array(ocsp.extend({ certificate: name, url: z.string() })),
});

const publicFacts = {
  subject: z.string().nullable(),
  issuer: z.string().nullable(),
  serial: z.string().nullable(),
  san: z
    .array(z.string())
    .describe('Decoded certificate subject alternative names, with DNS:, IP: or email: labels'),
  notBefore: date.nullable(),
  notAfter: date.nullable(),
  daysLeft: z.int().nullable(),
  fingerprint: z.string().nullable(),
  keySpec: PkiKeySpecSchema.nullable(),
  isCa: z.boolean().nullable(),
};
const caState = z.strictObject({
  name,
  certificateRef: z.string().nullable(),
  ...publicFacts,
  signingKey: z.boolean(),
  crl: z
    .strictObject({
      url: z.string(),
      refreshIntervalSec: z.int(),
      fetchedAt: date.nullable(),
      ageSec: z.int().nullable(),
      thisUpdate: date.nullable(),
      nextUpdate: date.nullable(),
      revoked: z.int().nullable(),
      error: z.string().nullable(),
    })
    .nullable(),
  ocspUrl: z.string().nullable(),
  problems: z.array(z.string()),
});
const certificateState = z.strictObject({
  name,
  ca: z.string().nullable(),
  certificateRef: z.string().nullable(),
  privateKeyRef: z.string().nullable(),
  ...publicFacts,
  expiryAlertDays: z.int(),
  expiring: z.boolean(),
  revokedByCrl: z.boolean(),
  ocsp: ocsp.nullable(),
  problems: z.array(z.string()),
});
const alert = z.strictObject({
  kind: z.enum(['ca', 'certificate']),
  name,
  notAfter: date,
  daysLeft: z.int(),
  alertDays: z.int(),
  severity: z.enum(['warning', 'critical']),
});
const agentFile = z.strictObject({
  kind: z.enum(['cert', 'ca', 'key', 'crl']),
  name,
  ref: z.string(),
  fingerprint: z.string(),
  mode: z.string().regex(/^[0-7]{4}$/),
  size: z.int().min(0),
  present: z.boolean(),
});
export const PkiStateOut = z.strictObject({
  generatedAt: date,
  cas: z.array(caState),
  certificates: z.array(certificateState),
  expiry: z.strictObject({ lastCheck: date.nullable(), active: z.array(alert) }),
  agentFiles: z.strictObject({
    root: z.string().nullable(),
    unavailable: z.string().nullable(),
    files: z.array(agentFile),
  }),
  unsupported: z.array(z.string()),
});
