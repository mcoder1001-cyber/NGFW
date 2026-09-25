import { readFileSync, realpathSync } from 'node:fs';
import { pathToFileURL } from 'node:url';
import { AuditService } from '../audit/audit.service.js';
import { SystemEventsService } from '../audit/system-events.service.js';
import { loadEnv } from '../config.js';
import { createDb } from '../db/db.js';
import { createValkey } from '../infra/valkey.js';
import {
  auditRotation,
  DEFAULT_API_USER,
  listLocks,
  rotateKeyFile,
  unlockUser,
  type BreakGlassDeps,
} from './break-glass.js';

/**
 * `vrx-authctl` — root-only break-glass for the API's authentication on this device (TD-10b, review 2.3a and P06
 * tech debt). Wrapper: deploy/sbin/vrx-authctl (P10 installs both). Never prints a secret: settings are read, never
 * echoed; errors name keys and files, not values.
 */
export const USAGE = `usage: vrx-authctl [--env-file <file>]... <command>
  unlock <username>          clear every login lockout of the user: account-wide and per client address
  locks                      list the lockouts in force
  rotate-jwt-key [<file>]    new signing key on top of the key ring (default: VRX_JWT_KEY_FILE); the previous
                             signing key stays for tokens it signed; the API reloads within 5 s. The file may belong
                             to root or to the API's user (VRX_API_USER, default vrx); mode 0600, owner kept
Root only; every action is audited. The API's settings (VRX_DATABASE_URL or VRX_PG_DSN, VRX_VALKEY_URL/DB/PREFIX,
VRX_JWT_KEY_FILE) come from the environment and from --env-file files (KEY=VALUE lines, as the API's unit reads them);
the environment wins.`;

/** KEY=VALUE lines (optional `export `, optional quotes, `#` comments); only VRX_* keys are taken. */
export function parseEnvFile(text: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const raw of text.split('\n')) {
    const m = /^\s*(?:export\s+)?(VRX_[A-Z0-9_]+)\s*=\s*(.*?)\s*$/.exec(raw);
    if (m === null) continue;
    let v = m[2]!;
    if (v.length >= 2 && (v[0] === '"' || v[0] === "'") && v.at(-1) === v[0]) v = v.slice(1, -1);
    out[m[1]!] = v;
  }
  return out;
}

export interface CliIo {
  getuid: () => number | undefined;
  env: NodeJS.ProcessEnv;
  out: (line: string) => void;
  err: (line: string) => void;
}

const defaultIo: CliIo = {
  getuid: () => process.getuid?.(),
  env: process.env,
  out: (l) => console.log(l),
  err: (l) => console.error(l),
};

/** Exit code: 0 done, 1 failed, 2 usage. */
export async function main(argv: string[], io: CliIo = defaultIo): Promise<number> {
  const envFiles: string[] = [];
  const rest: string[] = [];
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i]!;
    if (a === '--env-file') {
      const f = argv[++i];
      if (f === undefined) {
        io.err(USAGE);
        return 2;
      }
      envFiles.push(f);
    } else if (a === '-h' || a === '--help') {
      io.out(USAGE);
      return 0;
    } else rest.push(a);
  }
  const [cmd, ...args] = rest;
  const valid =
    (cmd === 'unlock' && args.length === 1) ||
    (cmd === 'locks' && args.length === 0) ||
    (cmd === 'rotate-jwt-key' && args.length <= 1);
  if (!valid) {
    io.err(USAGE);
    return 2;
  }
  if (io.getuid() !== 0) {
    io.err('vrx-authctl: root only (the break-glass acts on the database and Valkey directly)');
    return 1;
  }
  let fileVars: Record<string, string> = {};
  try {
    for (const f of envFiles) fileVars = { ...fileVars, ...parseEnvFile(readFileSync(f, 'utf8')) };
  } catch (e) {
    io.err(
      `vrx-authctl: cannot read an --env-file (${(e as NodeJS.ErrnoException).code ?? 'error'})`,
    );
    return 1;
  }
  let env;
  try {
    env = loadEnv({ ...fileVars, ...io.env });
  } catch (e) {
    io.err(`vrx-authctl: ${(e as Error).message}`);
    return 1;
  }

  const file = cmd === 'rotate-jwt-key' ? (args[0] ?? env.VRX_JWT_KEY_FILE) : undefined;
  if (cmd === 'rotate-jwt-key' && file === undefined) {
    io.err('vrx-authctl: no key file — pass <file> or set VRX_JWT_KEY_FILE');
    return 1;
  }

  const dbh = createDb(env);
  const kv = createValkey(env);
  const events = new SystemEventsService(dbh.db);
  const audit = new AuditService(dbh.db, events);
  const d: BreakGlassDeps = { db: dbh.db, kv, prefix: env.VRX_VALKEY_PREFIX, audit, events };
  try {
    if (cmd === 'rotate-jwt-key') {
      // review M1: the API's system user may own the file (VRX_API_USER, default vrx)
      const apiUser = io.env['VRX_API_USER'] ?? fileVars['VRX_API_USER'] ?? DEFAULT_API_USER;
      const r = rotateKeyFile(file!, apiUser);
      io.out(
        `rotated ${file}: ${r.keys} key(s), new signing key ${r.kid} first, owner uid ${r.uid}${r.created && r.uid === 0 ? ` (new file owned by root: no user '${apiUser}' here — chown it to the API's user)` : ''}; the API reloads it within 5 s`,
      );
      // review L5: audited like an unlock; the rotation itself stands even if the row cannot be written
      const failures = audit.writeFailures;
      await auditRotation(d, file!, r);
      if (audit.writeFailures > failures)
        io.err('vrx-authctl: warning — the audit row of this rotation could not be written');
      return 0;
    }
    if (cmd === 'unlock') {
      const r = await unlockUser(d, args[0]!);
      io.out(
        `unlocked '${r.username}' (id ${r.userId}): account-wide lock ${r.accountLockCleared ? 'cleared' : 'was not set'}, ${r.addressKeysCleared} per-address lock/counter key(s) removed`,
      );
    } else {
      const rows = await listLocks(d);
      if (rows.length === 0) io.out('no lockouts in force');
      for (const r of rows)
        io.out(
          `${r.username}\t${r.scope}${r.client ? ` ${r.client}` : ''}\t${r.state}\t${r.secondsLeft}s left`,
        );
    }
    return 0;
  } catch (e) {
    io.err(`vrx-authctl: ${(e as Error).message}`);
    return 1;
  } finally {
    kv.disconnect();
    await dbh.close();
  }
}

/** Run when executed (node dist/auth/break-glass-cli.js …), not when imported by a test. */
function isEntry(): boolean {
  try {
    const entry = process.argv[1];
    return entry !== undefined && import.meta.url === pathToFileURL(realpathSync(entry)).href;
  } catch {
    return false;
  }
}
if (isEntry()) process.exitCode = await main(process.argv.slice(2));
