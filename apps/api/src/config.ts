import { z } from 'zod';

/**
 * All runtime configuration comes from VRX_* environment variables, validated once at boot.
 * Secrets (database password, JWT key, bootstrap password) come from the environment or files under /run —
 * never from the repository, never logged.
 */
const Env = z.object({
  VRX_HTTP_HOST: z.string().default('127.0.0.1'),
  VRX_HTTP_PORT: z.coerce.number().int().min(1).max(65535).default(3000),
  VRX_LOG_LEVEL: z.enum(['debug', 'info', 'warn', 'error']).default('info'),

  /** PostgreSQL DSN. `deploy/dev/pg-test.sh` writes it as VRX_PG_DSN; VRX_DATABASE_URL wins when both are set. */
  VRX_DATABASE_URL: z.string().optional(),
  VRX_PG_DSN: z.string().optional(),
  VRX_DB_POOL_MAX: z.coerce.number().int().min(1).max(100).default(10),

  /** Valkey: logical db index and key prefix (shared host: slot N uses db N and prefix `vrx:w<N>:`). */
  VRX_VALKEY_URL: z.string().default('redis://127.0.0.1:6379'),
  VRX_VALKEY_DB: z.coerce.number().int().min(0).max(15).default(0),
  VRX_VALKEY_PREFIX: z.string().default('vrx:'),

  /** vrx-agent gRPC unix socket and the owner the API expects it to serve (empty = whatever the agent serves). */
  VRX_AGENT_SOCKET: z.string().default('/run/vrx/agent.sock'),
  VRX_AGENT_OWNER: z.string().default(''),
  VRX_AGENT_TIMEOUT_MS: z.coerce.number().int().min(100).default(60_000),

  /** HS256 key for access tokens (≥ 32 chars). Unset: a random key per process (tokens die with the process). */
  VRX_JWT_SECRET: z.string().min(32).optional(),
  VRX_ACCESS_TTL_SEC: z.coerce.number().int().min(5).default(900),
  VRX_REFRESH_TTL_SEC: z.coerce
    .number()
    .int()
    .min(60)
    .default(7 * 24 * 3600),
  /** Mark the refresh cookie `Secure` (production: behind TLS). */
  VRX_COOKIE_SECURE: z
    .enum(['0', '1', 'true', 'false'])
    .default('1')
    .transform((v) => v === '1' || v === 'true'),

  /** Master key of the secret store (AES-256-GCM): a file with 32 random bytes; created 0600 when missing. */
  VRX_SECRET_KEY_FILE: z.string().default('/var/lib/vrx/secret.key'),

  /** First admin (D-048): created at boot when app_user is empty. */
  VRX_BOOTSTRAP_ADMIN_USER: z.string().default('admin'),
  VRX_BOOTSTRAP_ADMIN_PASSWORD: z.string().min(8).optional(),

  /** Candidate lock: a lock untouched for longer than this may be taken over by another user. */
  VRX_LOCK_TTL_SEC: z.coerce.number().int().min(1).default(1800),
  /** Login protection: attempts per source IP and minute; failures until lockout; lockout duration. */
  VRX_LOGIN_RATE_PER_MIN: z.coerce.number().int().min(1).default(20),
  VRX_LOGIN_MAX_FAILURES: z.coerce.number().int().min(1).default(10),
  VRX_LOGIN_LOCKOUT_SEC: z.coerce.number().int().min(1).default(900),
  /**
   * DEVELOPMENT ONLY: accept new passwords shorter than PASSWORD_MIN (any non-empty one, e.g. admin/admin on a lab box).
   * Off by default; a warning is logged at boot while it is on. Never set it in a product image.
   */
  VRX_DEV_WEAK_PASSWORDS: z
    .enum(['0', '1', 'true', 'false'])
    .default('0')
    .transform((v) => v === '1' || v === 'true'),
  /** Password set/change attempts per caller and minute (TD-2 #1; wrong current passwords count too). */
  VRX_PASSWORD_RATE_PER_MIN: z.coerce.number().int().min(1).default(5),
  /** Telemetry relay heartbeat interval. */
  VRX_WS_HEARTBEAT_MS: z.coerce.number().int().min(50).default(15_000),
});
export type Env = z.infer<typeof Env>;

export function loadEnv(source: NodeJS.ProcessEnv = process.env): Env {
  // an empty variable (`VRX_JWT_SECRET=` as in .env.example) means "not set"
  const set = Object.fromEntries(
    Object.entries(source).filter(([, v]) => v !== undefined && v !== ''),
  );
  const r = Env.safeParse(set);
  if (!r.success) {
    // paths and messages only — never echo the offending values (they may be secrets)
    const issues = r.error.issues.map((i) => `${i.path.join('.')}: ${i.message}`);
    throw new Error(`invalid environment: ${issues.join('; ')}`);
  }
  return r.data;
}

export function databaseUrl(env: Env): string | undefined {
  return env.VRX_DATABASE_URL ?? env.VRX_PG_DSN;
}

/** DI token for the parsed environment. */
export const ENV = Symbol('VRX_ENV');
