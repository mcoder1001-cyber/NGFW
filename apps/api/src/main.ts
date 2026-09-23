import { createApp, buildOpenApi } from './app.js';
import { loadEnv } from './config.js';
import { SwaggerModule } from '@nestjs/swagger';

const env = loadEnv();
const app = await createApp();
SwaggerModule.setup('api/docs', app, buildOpenApi(app));
await app.listen({ port: env.VRX_HTTP_PORT, host: env.VRX_HTTP_HOST });
console.log(
  `vrx-api listening on http://${env.VRX_HTTP_HOST}:${env.VRX_HTTP_PORT} (docs at /api/docs)`,
);
