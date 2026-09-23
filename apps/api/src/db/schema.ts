import { sql } from 'drizzle-orm';
import {
  bigserial,
  boolean,
  check,
  index,
  integer,
  jsonb,
  pgTable,
  serial,
  text,
  timestamp,
  uniqueIndex,
  uuid,
} from 'drizzle-orm/pg-core';

/**
 * PostgreSQL datastore (docs/04-api-datamodel.md "Datastore schema"). PostgreSQL is the source of truth; the config
 * document lives as whole JSON snapshots (`payload`). Migrations: `pnpm --filter @ngfw/api db:generate` writes SQL
 * into `migrations/` (committed); the API applies them at boot (`db/migrate.ts`).
 *
 * Columns beyond docs/04 are operational: login lockout (`failed_logins`, `locked_until`), `source` of a user
 * (bootstrap/config/local), revision `txn_id`/`kind`, denormalised `username` in the audit log (survives user
 * deletion), and `config_pending` — the single unconfirmed commit (proto.md §4: the revision is persisted only after
 * CONFIRMED).
 */

export const ROLES = ['admin', 'operator', 'readonly'] as const;
export type Role = (typeof ROLES)[number];

const ts = (name: string) => timestamp(name, { withTimezone: true, mode: 'date' });

export const appUser = pgTable(
  'app_user',
  {
    id: serial('id').primaryKey(),
    username: text('username').notNull(),
    passwordHash: text('password_hash'),
    role: text('role').$type<Role>().notNull(),
    mfaSecret: text('mfa_secret'),
    disabled: boolean('disabled').notNull().default(false),
    lastLogin: ts('last_login'),
    failedLogins: integer('failed_logins').notNull().default(0),
    lockedUntil: ts('locked_until'),
    /** bootstrap (seeded from VRX_BOOTSTRAP_ADMIN_PASSWORD) · config (management.users) */
    source: text('source').notNull().default('config'),
    createdAt: ts('created_at').notNull().defaultNow(),
  },
  (t) => [
    uniqueIndex('app_user_username_uq').on(t.username),
    check('app_user_role_ck', sql`${t.role} in ('admin', 'operator', 'readonly')`),
  ],
);

export const apiKey = pgTable(
  'api_key',
  {
    id: uuid('id').primaryKey().defaultRandom(),
    userId: integer('user_id')
      .notNull()
      .references(() => appUser.id, { onDelete: 'cascade' }),
    name: text('name').notNull(),
    /** sha256 of the random key secret (the secret has 256 bits of entropy — a slow hash adds nothing). */
    hash: text('hash').notNull(),
    /** Role cap of the key: the effective role is the lower of the user's role and this. */
    scopes: text('scopes')
      .array()
      .notNull()
      .default(sql`'{}'::text[]`),
    expiresAt: ts('expires_at'),
    lastUsed: ts('last_used'),
    createdAt: ts('created_at').notNull().defaultNow(),
  },
  (t) => [uniqueIndex('api_key_hash_uq').on(t.hash), index('api_key_user_idx').on(t.userId)],
);

export const configRevision = pgTable(
  'config_revision',
  {
    id: serial('id').primaryKey(),
    createdAt: ts('created_at').notNull().defaultNow(),
    authorId: integer('author_id').references(() => appUser.id, { onDelete: 'set null' }),
    comment: text('comment').notNull().default(''),
    parentId: integer('parent_id'),
    /** The whole running document, secret leaves removed (D-046) — hashes live in app_user. */
    payload: jsonb('payload').notNull(),
    /** sha256 of the canonical JSON of payload. */
    hash: text('hash').notNull(),
    txnId: text('txn_id'),
    /** commit · rollback · confirmed-commit */
    kind: text('kind').notNull().default('commit'),
  },
  (t) => [index('config_revision_created_idx').on(t.createdAt)],
);

export const configCandidate = pgTable('config_candidate', {
  /** Singleton row (id = 1): one candidate, one writer. */
  id: integer('id').primaryKey(),
  ownerId: integer('owner_id').references(() => appUser.id, { onDelete: 'set null' }),
  lockedAt: ts('locked_at'),
  /** null = the candidate equals running (nothing edited since the last commit/discard). */
  payload: jsonb('payload'),
  baseRevisionId: integer('base_revision_id'),
  updatedAt: ts('updated_at').notNull().defaultNow(),
});

export const configPending = pgTable('config_pending', {
  /** Singleton row (id = 1): the agent allows at most one unconfirmed transaction. */
  id: integer('id').primaryKey(),
  txnId: text('txn_id').notNull(),
  payload: jsonb('payload').notNull(),
  hash: text('hash').notNull(),
  authorId: integer('author_id').references(() => appUser.id, { onDelete: 'set null' }),
  comment: text('comment').notNull().default(''),
  parentId: integer('parent_id'),
  kind: text('kind').notNull(),
  deadline: ts('deadline').notNull(),
  createdAt: ts('created_at').notNull().defaultNow(),
});

export const auditLog = pgTable(
  'audit_log',
  {
    id: bigserial('id', { mode: 'number' }).primaryKey(),
    ts: ts('ts').notNull().defaultNow(),
    userId: integer('user_id').references(() => appUser.id, { onDelete: 'set null' }),
    username: text('username'),
    sourceIp: text('source_ip'),
    /** HTTP method + route pattern, e.g. `PATCH /api/v1/config/*`, or a named action such as `auth.login`. */
    action: text('action').notNull(),
    /** The concrete target, e.g. the JSON pointer `/interfaces/loop0`. */
    resource: text('resource'),
    before: jsonb('before'),
    after: jsonb('after'),
    /** success · failure */
    result: text('result').notNull(),
    status: integer('status'),
  },
  (t) => [index('audit_log_ts_idx').on(t.ts)],
);

export const secret = pgTable(
  'secret',
  {
    id: serial('id').primaryKey(),
    kind: text('kind').notNull(),
    /** The full reference `<kind>/<name>` (D-051). */
    ref: text('ref').notNull(),
    /** base64(iv ‖ tag ‖ AES-256-GCM ciphertext). */
    ciphertext: text('ciphertext').notNull(),
    createdAt: ts('created_at').notNull().defaultNow(),
  },
  (t) => [uniqueIndex('secret_ref_uq').on(t.ref)],
);

export const systemEvent = pgTable(
  'system_event',
  {
    id: bigserial('id', { mode: 'number' }).primaryKey(),
    ts: ts('ts').notNull().defaultNow(),
    severity: text('severity').notNull(),
    subsystem: text('subsystem').notNull(),
    code: text('code').notNull(),
    message: text('message').notNull(),
    data: jsonb('data'),
  },
  (t) => [index('system_event_ts_idx').on(t.ts)],
);
