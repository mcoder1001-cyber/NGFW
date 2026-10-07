import type { NestFastifyApplication } from '@nestjs/platform-fastify';
import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest';
import { createApp } from '../app.js';
import { loadEnv } from '../config.js';
import { AaaService } from '../features/aaa/aaa.service.js';
import { TokensService } from './tokens.service.js';

describe('security freeze: Swagger path and static asset boundaries', () => {
  let app: NestFastifyApplication;
  beforeAll(async () => {
    app = await createApp({ env: loadEnv({}), logger: false });
    // Offline policy fixture; authentication/token checks remain real.
    vi.spyOn(app.get(AaaService), 'cachedPolicy').mockResolvedValue({
      order: ['local'],
      fallbackLocal: true,
      mfaRequired: 'none',
      mfaIssuer: 'ngfw',
    });
    await app.init();
    await app.getHttpAdapter().getInstance().ready();
  });
  afterAll(async () => {
    await app?.close();
  });

  it.each([
    '/api/docs',
    '/api/docs-json',
    '/api/docs/swagger-ui-init.js',
    '/api/docs/swagger-ui.css',
    '/api/docs/swagger-ui-bundle.js',
    '/api%2fdocs/swagger-ui.css',
    '/api/%64ocs/swagger-ui.css',
    '/api/docs/../docs/swagger-ui.css',
    '/api/docs/%2e%2e/docs/swagger-ui.css',
    '/api/docs%2fswagger-ui.css',
    '/api/docs/%ZZ',
    '/api/v1/health/%ZZ',
  ])('never serves protected content to anonymous GET %s', async (url) => {
    const response = await app.inject({ method: 'GET', url });
    expect(response.statusCode).toBeGreaterThanOrEqual(400);
    expect(response.body).not.toContain('SwaggerUIBundle');
    expect(response.body).not.toContain('"openapi":');
  });

  it('serves static Swagger assets after authentication with the patched plugin', async () => {
    const token = await app
      .get(TokensService)
      .signAccess({ id: 9002, username: 'freeze', role: 'readonly' });
    const response = await app.inject({
      method: 'GET',
      url: '/api/docs/swagger-ui.css',
      headers: { authorization: `Bearer ${token}` },
    });
    expect(response.statusCode).toBe(200);
    expect(response.headers['content-type']).toContain('text/css');
  });
});
