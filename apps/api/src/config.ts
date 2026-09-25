import { isIP } from 'node:net';
import { z } from 'zod';

/** proxy-addr's named ranges (Fastify trustProxy) that VRX_TRUST_PROXY accepts next to addresses and CIDRs. */
const TRUST_NAMES = new Set(['loopback', 'linklocal', 'uniquelocal']);

/** VRX_TRUST_PROXY → the trusted peers; `none` → []. */
export function trustProxyList(v: string): string[] {
  const t = v
    .split(',')
    .map((s) => s.trim())
    .filter((s) => s !== '');
  return t.length === 1 && t[0] === 'none' ? [] : t;
}

/** `loopback`/`linklocal`/`uniquelocal`, an IP address or `<ip>/<bits>` (bits ≥ 1: never "trust everyone"). */
function validTrustEntry(tok: string): boolean {
  if (TRUST_NAMES.has(tok)) return true;
  const [addr = '', bits, ...rest] = tok.split('/');
  const fam = isIP(addr);
  if (fam === 0 || rest.length > 0) return false;
  if (bits === undefined) return true;
  if (!/^\d{1,3}$/.test(bits)) return false;
  const n = Number(bits);
  return n >= 1 && n <= (fam === 4 ? 32 : 128);
}

/**
 * All runtime configuration comes from VRX_* environment variables, validated once at boot.
 * Secrets (database password, JWT key, bootstrap password) come from the environment or files under /run —
 * never from the repository, never logged.
 */
const Env = z.object({
  VRX_HTTP_HOST: z.string().default('127.0.0.1'),
  VRX_HTTP_PORT: z.coerce.number().int().min(1).max(65535).default(3000),
  /**
   * TD-10b (review 2.3b): peers whose X-Forwarded-For / X-Forwarded-Proto are believed — they decide the client
   * address (rate limits, lockout, audit) and whether the client connected over TLS (the password transport rule).
   * Comma-separated `loopback` (default: the product nginx and the dev proxy run on this host), `linklocal`,
   * `uniquelocal`, IP addresses and `<ip>/<bits>` ranges; `none` = no proxy, the socket peer is the client.
   */
  VRX_TRUST_PROXY: z
    .string()
    .default('loopback')
    .refine(
      (v) => v.trim() === 'none' || trustProxyList(v).every(validTrustEntry),
      'a comma-separated list of loopback, linklocal, uniquelocal, IP addresses or <ip>/<bits> ranges, or none',
    )
    .transform(trustProxyList),
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
  /**
   * TD-10b (P06 tech debt): HS256 key RING for access tokens, one key (≥ 32 chars) per line, newest first — the first
   * signs, the others still verify (rotation: add a line on top, drop the old one after VRX_ACCESS_TTL_SEC). Must be
   * owned by the API's user (or root) and not accessible to group/others. Wins over VRX_JWT_SECRET; re-read on change.
   */
  VRX_JWT_KEY_FILE: z.string().min(1).optional(),
  VRX_ACCESS_TTL_SEC: z.coerce.number().int().min(5).default(900),
  /** Idle lifetime of a refresh chain: every refresh renews it (sliding). */
  VRX_REFRESH_TTL_SEC: z.coerce
    .number()
    .int()
    .min(60)
    .default(7 * 24 * 3600),
  /**
   * TD-10b (review 2.3d): ABSOLUTE lifetime of a login session — from the login, refreshes do not extend it (the
   * refresh cookie and the access token never outlive it). Default 12 h: one working day, then log in again.
   */
  VRX_SESSION_MAX_SEC: z.coerce
    .number()
    .int()
    .min(5)
    .default(12 * 3600),
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
  /**
   * Login protection: attempts per source IP and minute; failures until lockout; lockout duration. TD-10b (review
   * 2.3a): a login lockout is per (user, client address); the last admin who could still log in from that address is
   * throttled (1 s, 2 s, 4 s … ≤ 60 s) instead of locked.
   */
  VRX_LOGIN_RATE_PER_MIN: z.coerce.number().int().min(1).default(20),
  VRX_LOGIN_MAX_FAILURES: z.coerce.number().int().min(1).default(10),
  VRX_LOGIN_LOCKOUT_SEC: z.coerce.number().int().min(1).default(900),
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
