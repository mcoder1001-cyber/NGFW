import type { NestFastifyApplication } from '@nestjs/platform-fastify';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { buildOpenApi, createApp } from '../app.js';
import { loadEnv } from '../config.js';
import { HealthOut } from './health.controller.js';

describe('GET /api/v1/health', () => {
  let app: NestFastifyApplication;
  beforeAll(async () => {
    app = await createApp({ env: loadEnv({}), logger: false });
    await app.init();
    await app.getHttpAdapter().getInstance().ready();
  });
  afterAll(async () => app.close());

  it('returns ok without credentials (public liveness)', async () => {
    const res = await app.inject({ method: 'GET', url: '/api/v1/health' });
    expect(res.statusCode).toBe(200);
    expect(res.json()).toMatchObject({ status: 'ok', service: 'vrx-api' });
  });

  it('the answer matches the documented response schema (TD-2 #3)', async () => {
    const res = await app.inject({ method: 'GET', url: '/api/v1/health' });
    expect(HealthOut.strict().safeParse(res.json()).success).toBe(true);
    const op = buildOpenApi(app).paths['/api/v1/health']?.get;
    const schema = (op?.responses['200'] as { content?: Record<string, { schema?: unknown }> })
      .content?.['application/json']?.schema as { required?: string[]; properties?: object };
    expect(schema.required).toEqual(['status', 'service', 'version', 'time']);
    expect(Object.keys(schema.properties ?? {})).toEqual(['status', 'service', 'version', 'time']);
  });
});
