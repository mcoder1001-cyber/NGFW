// Run from the deployed API directory as user ngfw; never open a listener.
import { createApp } from './dist/app.js';
import { loadEnv } from './dist/config.js';
import { DB, runMigrations } from './dist/db/db.js';
import { AuthService } from './dist/auth/auth.service.js';
import { appUser } from './dist/db/schema.js';
import { verifyPassword } from './dist/auth/password.js';
import { and, eq } from 'drizzle-orm';

let app;
try {
  const env = loadEnv();
  if (!env.NGFW_DATABASE_URL || !env.NGFW_BOOTSTRAP_ADMIN_PASSWORD) {
    throw new Error('missing firstboot inputs');
  }
  app = await createApp({ env, logger: false, docs: false });
  const db = app.get(DB);
  await runMigrations(db);
  await app.get(AuthService).seedBootstrapAdmin();
  // A retry after the insert committed must positively verify durable usable
  // administrator state. seedBootstrapAdmin(false) alone proves nothing.
  const admins = await db.select({ id: appUser.id, hash: appUser.passwordHash })
    .from(appUser).where(and(eq(appUser.role, 'admin'), eq(appUser.disabled, false),
      eq(appUser.username, env.NGFW_BOOTSTRAP_ADMIN_USER)));
  if (!admins[0] || !await verifyPassword(admins[0].hash, env.NGFW_BOOTSTRAP_ADMIN_PASSWORD)) {
    throw new Error('no durable administrator');
  }
} catch {
  console.error('firstboot database bootstrap failed; credentials retained');
  process.exitCode = 1;
} finally {
  try { await app?.close(); } catch {
    console.error('firstboot database cleanup failed; credentials retained');
    process.exitCode = 1;
  }
}
