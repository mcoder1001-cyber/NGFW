import { Controller, Get } from '@nestjs/common';
import { ApiOkResponse, ApiOperation, ApiTags } from '@nestjs/swagger';
import { z } from 'zod';
import { Protected } from '../../common/responses.js';
import { openapi } from '../../common/zod.js';
import { MgmtTlsService } from './mgmt-tls.service.js';

export const MgmtTlsStateOut = z
  .object({
    configured: z.boolean().describe('running `management.tls` names a certificate and key'),
    certificateRef: z.string().nullable(),
    minVersion: z.enum(['1.2', '1.3']),
    active: z
      .object({
        subject: z.string(),
        issuer: z.string(),
        subjectAltNames: z.array(z.string()),
        serialNumber: z.string(),
        notBefore: z.string(),
        notAfter: z.string(),
        fingerprintSha256: z.string(),
        chainLength: z.number().int(),
        daysLeft: z.number().int(),
      })
      .nullable()
      .describe('the certificate new TLS handshakes use (never the key); null = none loaded'),
    listener: z.object({
      enabled: z.boolean().describe('VRX_HTTPS_PORT is set: the API serves HTTPS with this certificate'),
      port: z.number().int().nullable(),
    }),
    loadedRevision: z.number().int().nullable().describe('running revision the certificate was loaded from'),
    loadedAt: z.string().nullable(),
    error: z.string().nullable().describe('why the running certificate could not be loaded (previous one kept)'),
  })
  .describe('API TLS certificate in force (F-management-ui). Read-only.');

/** F-management-ui: the API's TLS certificate as loaded from `management.tls`. */
@ApiTags('state')
@Controller('api/v1')
export class MgmtTlsController {
  constructor(private readonly tls: MgmtTlsService) {}

  @Get('state/management/tls')
  @Protected()
  @ApiOperation({ summary: 'API TLS certificate in force: subject, SANs, expiry, fingerprint (never the key)' })
  @ApiOkResponse({ schema: openapi(MgmtTlsStateOut, 'output') })
  state() {
    return this.tls.state();
  }
}
