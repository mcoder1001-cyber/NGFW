import type { NestFastifyApplication } from '@nestjs/platform-fastify';
import { randomUUID } from 'node:crypto';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { createApp } from '../../app.js';
import { loadEnv } from '../../config.js';

describe('public HA delivery remains cluster authenticated', () => {
  let app: NestFastifyApplication;
  beforeAll(async () => {
    app = await createApp({ env: loadEnv({}), logger: false });
    await app.init();
    await app.getHttpAdapter().getInstance().ready();
  });
  afterAll(async () => app.close());
  it('valid unauthenticated envelope gets 401 problem+json', async () => {
    const r = await app.inject({
      method: 'POST',
      url: '/api/v1/actions/ha/receive',
      payload: {
        origin: 'node-a',
        revision: 1,
        timestamp: Date.now(),
        nonce: randomUUID(),
        document: {},
      },
    });
    expect(r.statusCode).toBe(401);
    expect(r.headers['content-type']).toContain('application/problem+json');
  });
});
