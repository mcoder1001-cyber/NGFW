import { hash, verify } from '@node-rs/argon2';

/**
 * Local passwords: argon2id (the library default) with the OWASP 2024 minimum profile (19 MiB, t=2, p=1), stored as
 * a PHC string. Hashes of other algorithms (crypt `$6$` set through `management.users[].passwordHash` for console
 * logins) never verify here — API logins need an argon2id hash.
 */
const OPTIONS = { memoryCost: 19456, timeCost: 2, parallelism: 1 } as const;

export function hashPassword(password: string): Promise<string> {
  return hash(password, OPTIONS);
}

// verified against when the user does not exist, so both paths cost one argon2 run (no username oracle by timing)
let dummy: Promise<string> | undefined;

export async function verifyPassword(
  stored: string | null | undefined,
  password: string,
): Promise<boolean> {
  if (stored === null || stored === undefined || !stored.startsWith('$argon2id$')) {
    dummy ??= hashPassword('vrx-timing-equaliser');
    await verify(await dummy, password).catch(() => false);
    return false;
  }
  try {
    return await verify(stored, password);
  } catch {
    return false;
  }
}
