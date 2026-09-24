import { eq, gt, sql } from 'drizzle-orm';
import { randomBytes } from 'node:crypto';
import {
  chownSync,
  closeSync,
  existsSync,
  fsyncSync,
  openSync,
  readFileSync,
  renameSync,
  rmSync,
  writeSync,
} from 'node:fs';
import { dirname } from 'node:path';
import type { AuditService } from '../audit/audit.service.js';
import type { SystemEventsService } from '../audit/system-events.service.js';
import type { Db } from '../db/db.js';
import { appUser } from '../db/schema.js';
import type { Valkey } from '../infra/valkey.js';
import { checkKeyFile } from './key-file.js';
import { parseKeyRing } from './tokens.service.js';

/**
 * TD-10b (review 2.3a): root-only break-glass operations on the box — `vrx-authctl` (break-glass-cli.ts,
 * deploy/sbin/vrx-authctl; packaged by P10). They talk to PostgreSQL and Valkey directly with the API's own settings,
 * so they work when nobody can log in. Every action writes an audit row and a system_event.
 */
export interface BreakGlassDeps {
  db: Db;
  kv: Valkey;
  /** VRX_VALKEY_PREFIX: SCAN patterns are not prefixed by the client */
  prefix: string;
  audit: Pick<AuditService, 'write'>;
  events: Pick<SystemEventsService, 'record'>;
}

const ACTOR = 'root (break-glass)';

async function scan(d: BreakGlassDeps, pattern: string): Promise<string[]> {
  const out: string[] = [];
  let cursor = '0';
  do {
    const [next, keys] = await d.kv.scan(cursor, 'MATCH', `${d.prefix}${pattern}`, 'COUNT', 500);
    cursor = next;
    for (const k of keys) out.push(k.slice(d.prefix.length));
  } while (cursor !== '0');
  return out;
}

export interface UnlockResult {
  userId: number;
  username: string;
  /** app_user.locked_until was in the future (the account-wide lock of a session-held password check) */
  accountLockCleared: boolean;
  /** per-(user, address) lock/throttle keys and failure counters removed (lockout.ts) */
  addressKeysCleared: number;
}

/**
 * Clear every login lockout of `username`: the account-wide lock and failure count in app_user, and all its
 * per-address locks, throttles and failure counters in Valkey (every generation, every address).
 */
export async function unlockUser(d: BreakGlassDeps, username: string): Promise<UnlockResult> {
  const [u] = await d.db
    .select({
      id: appUser.id,
      username: appUser.username,
      locked: sql<boolean>`${appUser.lockedUntil} is not null and ${appUser.lockedUntil} > now()`,
    })
    .from(appUser)
    .where(eq(appUser.username, username));
  if (u === undefined) throw new Error(`no user named '${username}'`);
  await d.db
    .update(appUser)
    .set({ failedLogins: 0, lockedUntil: null })
    .where(eq(appUser.id, u.id));
  const keys = [...(await scan(d, `lk:${u.id}:*`)), ...(await scan(d, `lkf:${u.id}:*`))];
  if (keys.length > 0) await d.kv.del(...keys);
  const r: UnlockResult = {
    userId: u.id,
    username: u.username,
    accountLockCleared: u.locked,
    addressKeysCleared: keys.length,
  };
  await d.audit.write({
    userId: null,
    username: ACTOR,
    sourceIp: null,
    action: 'auth.break-glass-unlock',
    resource: `user/${u.username}`,
    after: { accountLockCleared: r.accountLockCleared, addressKeysCleared: r.addressKeysCleared },
    result: 'success',
    status: null,
  });
  await d.events.record(
    'warning',
    'auth',
    'BREAK_GLASS_UNLOCK',
    `root cleared the login lockout of '${u.username}' on the device`,
    { user: u.username, ...r },
  );
  return r;
}

export interface LockRow {
  username: string;
  scope: 'account' | 'address';
  /** client address key (IPv4 or IPv6 /64) for `address` */
  client?: string;
  state: 'locked' | 'throttled';
  secondsLeft: number;
}

/** Every lock in force right now (read-only). */
export async function listLocks(d: BreakGlassDeps): Promise<LockRow[]> {
  const rows: LockRow[] = [];
  const users = new Map(
    (await d.db.select({ id: appUser.id, username: appUser.username }).from(appUser)).map((u) => [
      u.id,
      u.username,
    ]),
  );
  const account = await d.db
    .select({
      username: appUser.username,
      left: sql<number>`ceil(extract(epoch from ${appUser.lockedUntil} - now()))::int`,
    })
    .from(appUser)
    .where(gt(appUser.lockedUntil, sql`now()`));
  for (const a of account)
    rows.push({ username: a.username, scope: 'account', state: 'locked', secondsLeft: a.left });
  for (const key of await scan(d, 'lk:*')) {
    // lk:<uid>:<gen>:<client> — the client key may itself contain ':' (IPv6)
    const m = /^lk:(\d+):(\d+):(.+)$/.exec(key);
    if (m === null) continue;
    const [v, ttl] = await Promise.all([d.kv.get(key), d.kv.ttl(key)]);
    if (v === null || ttl < 0) continue;
    rows.push({
      username: users.get(Number(m[1])) ?? `#${m[1]}`,
      scope: 'address',
      client: m[3]!,
      state: v === 'throttle' ? 'throttled' : 'locked',
      secondsLeft: ttl,
    });
  }
  return rows;
}

export interface RotateResult {
  keys: number;
  created: boolean;
}

/**
 * P06 tech debt (JWT key rotation): put a new random signing key on top of the VRX_JWT_KEY_FILE ring and keep the
 * previous signing key (tokens it signed stay valid until they expire); older keys are dropped. Atomic (temp file,
 * fsync, rename), mode 0600, the existing file's owner kept. The API reloads the ring within 5 s.
 * Rotate at most once per VRX_ACCESS_TTL_SEC: a token signed by a dropped key is refused (the web UI and the CLI then
 * refresh, which does not depend on this key).
 */
export function rotateKeyFile(path: string): RotateResult {
  const created = !existsSync(path);
  let previous: string[] = [];
  let owner: { uid: number; gid: number } | undefined;
  if (!created) {
    const st = checkKeyFile(path);
    owner = { uid: st.uid, gid: st.gid };
    previous = parseKeyRing(readFileSync(path, 'utf8'), path);
  }
  const ring = [randomBytes(48).toString('base64url'), ...previous.slice(0, 1)];
  const tmp = `${dirname(path)}/.${process.pid}.${randomBytes(4).toString('hex')}.jwtkeys`;
  const fd = openSync(tmp, 'wx', 0o600);
  try {
    writeSync(
      fd,
      `# VRX access-token key ring (VRX_JWT_KEY_FILE): newest first; written by vrx-authctl rotate-jwt-key\n${ring.join('\n')}\n`,
    );
    fsyncSync(fd);
  } finally {
    closeSync(fd);
  }
  try {
    if (owner !== undefined) chownSync(tmp, owner.uid, owner.gid);
    renameSync(tmp, path);
  } catch (e) {
    rmSync(tmp, { force: true });
    throw e;
  }
  // the rename itself survives a power loss only once the directory is on disk
  const dfd = openSync(dirname(path), 'r');
  try {
    fsyncSync(dfd);
  } finally {
    closeSync(dfd);
  }
  return { keys: ring.length, created };
}
