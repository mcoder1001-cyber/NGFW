import { createApp } from './app.js';
import { AuthService } from './auth/auth.service.js';
import { CommitService } from './commit/commit.service.js';
import { loadEnv } from './config.js';
import { DB, runMigrations, type Db } from './db/db.js';
import { RelayService } from './telemetry/relay.service.js';

const env = loadEnv();
const app = await createApp({ env });
app.enableShutdownHooks();
// boot order: schema migrations → first admin (D-048) → confirm-watch of a pending commit → agent streams → listen
await runMigrations(app.get<Db>(DB));
await app.get(AuthService).seedBootstrapAdmin();
await app.get(CommitService).resumePending();
await app.get(CommitService).resumeSync();
app.get(RelayService).start();
await app.listen({ port: env.VRX_HTTP_PORT, host: env.VRX_HTTP_HOST });
console.log(
  `vrx-api listening on http://${env.VRX_HTTP_HOST}:${env.VRX_HTTP_PORT} (docs at /api/docs)`,
);
