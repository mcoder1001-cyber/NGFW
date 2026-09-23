import type { NestFastifyApplication } from '@nestjs/platform-fastify';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { createApp } from '../app.js';
import { loadEnv } from '../config.js';

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
});
