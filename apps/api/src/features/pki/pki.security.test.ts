import type { NestFastifyApplication } from '@nestjs/platform-fastify';
import { afterAll, beforeAll, describe, expect, it, vi, type MockInstance } from 'vitest';
import { createApp } from '../../app.js';
import { AuditService } from '../../audit/audit.service.js';
import { TokensService } from '../../auth/tokens.service.js';
import { loadEnv } from '../../config.js';
import { AaaService } from '../aaa/aaa.service.js';
import { PkiService } from './pki.service.js';

// Offline HTTP verification exercises registration, auth, Zod validation and global write-ahead audit together.
describe('PKI HTTP authorization and write-ahead audit', () => {
  let app: NestFastifyApplication;
  let admin: string;
  let operator: string;
  let begin: MockInstance<AuditService['begin']>;
  let finish: MockInstance<AuditService['finish']>;
  let createCsr: MockInstance<PkiService['createCsr']>;
  const routes = ['ca', 'csr', 'sign', 'import', 'crl/refresh'];

  beforeAll(async () => {
    app = await createApp({ env: loadEnv({}), logger: false });
    vi.spyOn(app.get(AaaService), 'cachedPolicy').mockResolvedValue({
      order: ['local'],
      fallbackLocal: true,
      mfaRequired: 'none',
      mfaIssuer: 'ngfw',
    });
    vi.spyOn(app.get(AuditService), 'write').mockResolvedValue(undefined);
    begin = vi.spyOn(app.get(AuditService), 'begin').mockResolvedValue(42);
    finish = vi.spyOn(app.get(AuditService), 'finish').mockResolvedValue(undefined);
    createCsr = vi.spyOn(app.get(PkiService), 'createCsr').mockResolvedValue({
      name: 'server',
      keyRef: 'key/server',
      csr: { subject: 'CN=server', san: [], keySpec: { type: 'ecdsa', curve: 'p256' } },
      csrPem: 'public-csr',
    });
    admin = await app
      .get(TokensService)
      .signAccess({ id: 9001, username: 'pki-admin', role: 'admin' });
    operator = await app
      .get(TokensService)
      .signAccess({ id: 9002, username: 'pki-operator', role: 'operator' });
    await app.init();
    await app.getHttpAdapter().getInstance().ready();
  });
  afterAll(async () => app.close());

  it.each(routes)('requires administrator role for secret mutation %s', async (route) => {
    begin.mockClear();
    const response = await app.inject({
      method: 'POST',
      url: `/api/v1/actions/pki/${route}`,
      headers: { authorization: `Bearer ${operator}` },
      payload: {},
    });
    expect(response.statusCode).toBe(403);
    expect(begin).not.toHaveBeenCalled();
  });

  it.each(routes)(
    'refuses %s before invoking the handler when audit begin fails',
    async (route) => {
      begin.mockRejectedValueOnce(new Error('audit offline'));
      createCsr.mockClear();
      const response = await app.inject({
        method: 'POST',
        url: `/api/v1/actions/pki/${route}`,
        headers: { authorization: `Bearer ${admin}` },
        payload: {},
      });
      expect(response.statusCode).toBe(503);
      expect(response.json()).toMatchObject({ type: expect.stringContaining('audit-unavailable') });
      expect(createCsr).not.toHaveBeenCalled();
    },
  );

  it('records a redacted outcome after a successful CSR action', async () => {
    finish.mockClear();
    const response = await app.inject({
      method: 'POST',
      url: '/api/v1/actions/pki/csr',
      headers: { authorization: `Bearer ${admin}` },
      payload: { name: 'server', subject: 'CN=server' },
    });
    expect(response.statusCode).toBe(200);
    expect(response.json()).toMatchObject({ name: 'server', keyRef: 'key/server' });
    expect(finish).toHaveBeenCalledWith(
      42,
      expect.objectContaining({
        result: 'success',
        status: 200,
        resource: 'pki/csr/server',
        after: { keyRef: 'key/server', subject: 'CN=server' },
      }),
    );
    expect(JSON.stringify(finish.mock.calls)).not.toContain('public-csr');
  });
});
