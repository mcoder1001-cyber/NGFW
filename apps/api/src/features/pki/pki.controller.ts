import { Body, Controller, Get, HttpCode, Inject, Param, Post, Query, Req } from '@nestjs/common';
import { ApiBody, ApiOperation, ApiQuery, ApiTags } from '@nestjs/swagger';
import { PkiKeySpecSchema, pkiDistinguishedName, pkiSubjectAltName } from '@ngfw/schema';
import { z } from 'zod';
import { AuditUnavailableDoc } from '../../audit/audit.interceptor.js';
import { MinRole } from '../../auth/decorators.js';
import type { VrxRequest } from '../../common/principal.js';
import { problems } from '../../common/problem.js';
import { ApiOut, Protected, type Out } from '../../common/responses.js';
import { openapi, SafeParamPipe, ZodPipe } from '../../common/zod.js';
import { PkiService, type ImportBody } from './pki.service.js';
import { PkiError } from './x509.js';
import {
  PkiCaOut,
  PkiCsrOut,
  PkiSignOut,
  PkiImportOut,
  PkiExportOut,
  PkiCrlOut,
  PkiOcspOut,
  PkiStateOut,
} from './dto.js';

/** A vpn.pki object name short enough that `cert/<name>.crl` stays a valid secret name. */
const pkiName = z
  .string()
  .regex(/^[A-Za-z0-9][A-Za-z0-9_.-]{0,58}$/)
  .describe('vpn.pki object name (≤ 59 characters)');
const flags = {
  stage: z.boolean().default(true).describe('merge the result into the candidate'),
  replace: z
    .boolean()
    .default(false)
    .describe('store new secret versions when the references exist'),
};

const CaBody = z.strictObject({
  name: pkiName,
  subject: pkiDistinguishedName,
  keySpec: PkiKeySpecSchema.optional(),
  days: z.int().min(1).max(36500).default(3650),
  description: z.string().max(255).optional(),
  ...flags,
});
const CsrBody = z.strictObject({
  name: pkiName,
  subject: pkiDistinguishedName,
  san: z.array(pkiSubjectAltName).max(64).default([]),
  keySpec: PkiKeySpecSchema.optional(),
  replace: flags.replace,
});
const SignBody = z.strictObject({
  ca: pkiName,
  name: pkiName,
  csrPem: z.string().min(1).max(65536),
  days: z.int().min(1).max(3650).default(365),
  san: z.array(pkiSubjectAltName).max(64).optional(),
  ...flags,
});
const ImportIn = z.discriminatedUnion('format', [
  z.strictObject({
    format: z.literal('pem'),
    as: z.enum(['ca', 'certificate']),
    name: pkiName,
    certificatePem: z.string().min(1).max(262144),
    privateKeyPem: z.string().min(1).max(65536).optional().describe('write-only; never returned'),
    privateKeyRef: z
      .string()
      .regex(/^key\/[A-Za-z0-9_.-]{1,64}$/)
      .optional(),
    ca: pkiName.optional(),
    ...flags,
  }),
  z.strictObject({
    format: z.literal('pkcs12'),
    as: z.enum(['ca', 'certificate']),
    name: pkiName,
    pkcs12: z.base64().max(360000),
    passphrase: z.string().min(1).max(1024).describe('write-only; never returned or logged'),
    ca: pkiName.optional(),
    ...flags,
  }),
]);
const ExportQuery = z.object({
  kind: z.enum(['certificate', 'ca', 'crl', 'key']).default('certificate'),
});
const CrlBody = z.strictObject({ ca: pkiName.optional() });
const OcspBody = z.strictObject({ certificate: pkiName.optional() });
/** PkiError (a request the X.509 layer refuses) → 400 problem+json with its pointer. */
async function run<T>(fn: () => Promise<T>): Promise<T> {
  try {
    return await fn();
  } catch (e) {
    if (e instanceof PkiError)
      throw problems.validation([{ pointer: e.pointer, message: e.message }], e.message);
    throw e;
  }
}

/**
 * F-pki: the certificate store (CA, key + CSR, sign, import PEM/PKCS#12, export public parts, CRL refresh, OCSP check)
 * and its live state. Material goes to the secret store; private keys and passphrases are write-only — never returned,
 * logged or audited (`req.audit` carries references and fingerprints only).
 */
@ApiTags('pki')
@Controller('api/v1')
export class PkiController {
  constructor(@Inject(PkiService) private readonly pki: PkiService) {}

  @Post('actions/pki/ca')
  @AuditUnavailableDoc()
  @MinRole('admin')
  @HttpCode(200)
  @Protected(400, 409)
  @ApiOperation({
    summary:
      'Generate a self-signed CA (key key/<name>, certificate cert/<name>) and stage vpn.pki.cas.<name>',
  })
  @ApiBody({ schema: openapi(CaBody) })
  @ApiOut(PkiCaOut)
  ca(
    @Body(new ZodPipe(CaBody)) b: z.output<typeof CaBody>,
    @Req() req: VrxRequest,
  ): Promise<Out<typeof PkiCaOut>> {
    return run(async () => {
      const r = await this.pki.createCa(b, req.principal!);
      req.audit = {
        resource: `pki/ca/${b.name}`,
        after: {
          certificateRef: r.certificateRef,
          keyRef: r.keyRef,
          fingerprint: r.issued['fingerprint'],
          staged: r.staged,
        },
      };
      return r;
    });
  }

  @Post('actions/pki/csr')
  @AuditUnavailableDoc()
  @MinRole('admin')
  @HttpCode(200)
  @Protected(400, 409)
  @ApiOperation({
    summary: 'Generate a key pair (stored as key/<name>, never returned) and return a CSR for it',
  })
  @ApiBody({ schema: openapi(CsrBody) })
  @ApiOut(PkiCsrOut)
  csr(
    @Body(new ZodPipe(CsrBody)) b: z.output<typeof CsrBody>,
    @Req() req: VrxRequest,
  ): Promise<Out<typeof PkiCsrOut>> {
    return run(async () => {
      const r = await this.pki.createCsr(b, req.principal!);
      req.audit = {
        resource: `pki/csr/${b.name}`,
        after: { keyRef: r.keyRef, subject: b.subject },
      };
      return r;
    });
  }

  @Post('actions/pki/sign')
  @AuditUnavailableDoc()
  @MinRole('admin')
  @HttpCode(200)
  @Protected(400, 409)
  @ApiOperation({
    summary:
      'An internal CA signs a CSR (cert/<name>); staged as vpn.pki.certificates.<name> when key/<name> is its key',
  })
  @ApiBody({ schema: openapi(SignBody) })
  @ApiOut(PkiSignOut)
  sign(
    @Body(new ZodPipe(SignBody)) b: z.output<typeof SignBody>,
    @Req() req: VrxRequest,
  ): Promise<Out<typeof PkiSignOut>> {
    return run(async () => {
      const r = await this.pki.sign(b, req.principal!);
      req.audit = {
        resource: `pki/certificate/${b.name}`,
        after: {
          ca: b.ca,
          certificateRef: r.certificateRef,
          fingerprint: r.issued['fingerprint'],
          staged: r.staged,
        },
      };
      return r;
    });
  }

  @Post('actions/pki/import')
  @AuditUnavailableDoc()
  @MinRole('admin')
  @HttpCode(200)
  @Protected(400, 409)
  @ApiOperation({
    summary:
      'Import a CA or a certificate: PEM (+ key PEM or key reference) or PKCS#12 with passphrase; validates chain and key match',
  })
  @ApiBody({ schema: openapi(ImportIn) })
  @ApiOut(PkiImportOut)
  import(
    @Body(new ZodPipe(ImportIn)) b: z.output<typeof ImportIn>,
    @Req() req: VrxRequest,
  ): Promise<Out<typeof PkiImportOut>> {
    return run(async () => {
      const r = await this.pki.import(b as ImportBody, req.principal!);
      req.audit = {
        resource: `pki/${b.as}/${b.name}`,
        after: {
          format: b.format,
          certificateRef: r.certificateRef,
          keyRef: r.keyRef,
          fingerprint: r.issued['fingerprint'],
          staged: r.staged,
        },
      };
      return r;
    });
  }

  @Get('actions/pki/export/:name')
  @Protected(400, 403, 404)
  @ApiOperation({
    summary:
      'Public PEM of a certificate, CA (chain) or CRL — a private key is never exported (403)',
  })
  @ApiOut(PkiExportOut)
  @ApiQuery({ name: 'kind', required: false, schema: openapi(ExportQuery.shape.kind) })
  export(
    @Param('name', new SafeParamPipe('name', 64)) name: string,
    @Query(new ZodPipe(ExportQuery)) q: z.output<typeof ExportQuery>,
  ): Promise<Out<typeof PkiExportOut>> {
    return run(() => this.pki.exportPem(name, q.kind));
  }

  @Post('actions/pki/crl/refresh')
  @AuditUnavailableDoc()
  @MinRole('admin')
  @HttpCode(200)
  @Protected(400)
  @ApiOperation({
    summary:
      'Fetch, verify and store the CRL of one CA (or of every CA with crl.url) as cert/<ca>.crl',
  })
  @ApiBody({ schema: openapi(CrlBody) })
  @ApiOut(PkiCrlOut)
  crl(
    @Body(new ZodPipe(CrlBody)) b: z.output<typeof CrlBody>,
    @Req() req: VrxRequest,
  ): Promise<Out<typeof PkiCrlOut>> {
    return run(async () => {
      const r = await this.pki.refreshCrl(b.ca, req.principal!);
      req.audit = {
        resource: `pki/crl/${b.ca ?? '*'}`,
        after: r.results.map((x) => ({ ca: x.ca, stored: x.stored, error: x.error })),
      };
      return r;
    });
  }

  @Post('actions/pki/ocsp/check')
  @MinRole('operator')
  @HttpCode(200)
  @Protected(400)
  @ApiOperation({
    summary: 'Ask the OCSP responder of the issuing CA about one certificate (or all)',
  })
  @ApiBody({ schema: openapi(OcspBody) })
  @ApiOut(PkiOcspOut)
  ocsp(
    @Body(new ZodPipe(OcspBody)) b: z.output<typeof OcspBody>,
    @Req() req: VrxRequest,
  ): Promise<Out<typeof PkiOcspOut>> {
    return run(async () => {
      const r = await this.pki.checkOcsp(b.certificate);
      req.audit = {
        resource: `pki/ocsp/${b.certificate ?? '*'}`,
        after: r.results.map((x) => ({ certificate: x.certificate, status: x.status })),
      };
      return r;
    });
  }

  @Get('state/pki')
  @Protected()
  @ApiOperation({
    summary:
      'PKI state: subject, issuer, SAN, validity, days left, CRL age, OCSP status, materialised files',
  })
  @ApiOut(PkiStateOut)
  state(): Promise<Out<typeof PkiStateOut>> {
    return run(async () => PkiStateOut.parse(await this.pki.state()));
  }
}
