import { createApp } from './app.js';
import { AuthService } from './auth/auth.service.js';
import { TokensService } from './auth/tokens.service.js';
import { CommitService } from './commit/commit.service.js';
import { loadEnv } from './config.js';
import { DB, runMigrations, type Db } from './db/db.js';
import { RelayService } from './telemetry/relay.service.js';
import { AlarmsService } from './features/dashboard-prom-alarms/index.js';
import { AutoBlockService } from './features/auto-block/index.js';

const env = loadEnv();
const app = await createApp({ env });
app.enableShutdownHooks();
// boot order: schema migrations → first admin (D-048) → confirm-watch of a pending commit → agent streams → listen
await runMigrations(app.get<Db>(DB));
await app.get(AuthService).seedBootstrapAdmin();
// D-097 (review L3): access-token revocations survive an API restart
await app.get(TokensService).loadRevocations();
await app.get(CommitService).resumePending();
await app.get(CommitService).resumeSync();
app.get(RelayService).start();
// F-dashboard-prom-alarms: load alarm rules and open the engine's stats subscription
await app.get(AlarmsService).start();
// F-bruteforce-block: load thresholds/allow-list and start the auto-block expiry sweep
await app.get(AutoBlockService).start();
await app.listen({ port: env.VRX_HTTP_PORT, host: env.VRX_HTTP_HOST });
console.log(
  `vrx-api listening on http://${env.VRX_HTTP_HOST}:${env.VRX_HTTP_PORT} (docs at /api/docs)`,
);
