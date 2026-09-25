import type { NestFastifyApplication } from '@nestjs/platform-fastify';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { createApp } from '../../app.js';
import { ValidationService } from '../../commit/validation.service.js';
import { loadEnv } from '../../config.js';
import { LicensingService } from './licensing.service.js';

/** The licence stage is really wired: ValidationService's @Optional() LicensingService is injected by AppModule. */
describe('licensing wiring (AppModule, offline)', () => {
  let app: NestFastifyApplication;
  beforeAll(async () => {
    app = await createApp({ env: loadEnv({}), logger: false, docs: false });
    await app.init();
  });
  afterAll(async () => app?.close());

  it('ValidationService gets the LicensingService singleton', () => {
    const v = app.get(ValidationService) as unknown as { licensing?: unknown };
    expect(v.licensing).toBeInstanceOf(LicensingService);
    expect(v.licensing).toBe(app.get(LicensingService));
  });
});
