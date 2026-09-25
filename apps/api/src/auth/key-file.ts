import { closeSync, constants, fstatSync, openSync, readFileSync, type Stats } from 'node:fs';

/**
 * TD-10b (P06 tech debt, "owner check of the key file"): a file that holds key material must be a regular file (not
 * a symlink another user could re-point), owned by an allowed user — by default the user this process runs as, or
 * root — and give group/others no access at all. Anything else is refused with a message that names the path and the
 * problem — never the content.
 * Review L7: the file is opened ONCE (O_NOFOLLOW) and checked and read through that descriptor (fstat), so a path swap
 * between the check and the read cannot slip another file in.
 * Used for the JWT key ring (VRX_JWT_KEY_FILE); the secret store's master key (VRX_SECRET_KEY_FILE,
 * secrets.service.ts) should call it too — that file belongs to TD-10a (TD-10b questions).
 */
export class KeyFileError extends Error {
  constructor(path: string, problem: string) {
    super(`key file ${path}: ${problem}`);
    this.name = 'KeyFileError';
  }
}

/** Who may own a key file: uids, and how to say it in an error. */
export interface KeyFileOwners {
  uids: readonly number[];
  label: string;
}

/** The default: this process's user or root (no check where the platform has no uids). */
export function processOwners(): KeyFileOwners {
  const uid = process.geteuid?.();
  return uid === undefined
    ? { uids: [], label: '' }
    : { uids: [uid, 0], label: `uid ${uid} or root` };
}

/** uid/gid of a local user from /etc/passwd (Node has no getpwnam); undefined when there is no such user. */
export function localUser(
  name: string,
  passwd = '/etc/passwd',
): { uid: number; gid: number } | undefined {
  let text: string;
  try {
    text = readFileSync(passwd, 'utf8');
  } catch {
    return undefined;
  }
  for (const line of text.split('\n')) {
    const f = line.split(':');
    if (f[0] === name && f.length >= 4) {
      const uid = Number(f[2]);
      const gid = Number(f[3]);
      if (Number.isInteger(uid) && Number.isInteger(gid)) return { uid, gid };
    }
  }
  return undefined;
}

function checkStat(path: string, st: Stats, owners: KeyFileOwners): void {
  if (!st.isFile()) throw new KeyFileError(path, 'is not a regular file');
  if (owners.uids.length > 0 && !owners.uids.includes(st.uid)) {
    throw new KeyFileError(path, `is owned by uid ${st.uid}; it must belong to ${owners.label}`);
  }
  if ((st.mode & 0o077) !== 0) {
    throw new KeyFileError(
      path,
      `mode ${(st.mode & 0o777).toString(8).padStart(4, '0')} lets group/others access it; chmod 0600`,
    );
  }
}

/** Open `path` for reading without following a symlink; check it through the descriptor; hand the descriptor over. */
function withKeyFile<T>(path: string, owners: KeyFileOwners, fn: (fd: number, st: Stats) => T): T {
  let fd: number;
  try {
    fd = openSync(path, constants.O_RDONLY | constants.O_NOFOLLOW);
  } catch (e) {
    const code = (e as NodeJS.ErrnoException).code;
    if (code === 'ELOOP') throw new KeyFileError(path, 'is a symbolic link (use the file itself)');
    throw new KeyFileError(path, `cannot be read (${code ?? 'error'})`);
  }
  try {
    const st = fstatSync(fd);
    checkStat(path, st, owners);
    return fn(fd, st);
  } finally {
    closeSync(fd);
  }
}

/** Check a key file (owner, mode, type) — the stat of the file that was checked. */
export function checkKeyFile(path: string, owners: KeyFileOwners = processOwners()): Stats {
  return withKeyFile(path, owners, (_fd, st) => st);
}

/** Check and read a key file through one descriptor. */
export function readKeyFile(
  path: string,
  owners: KeyFileOwners = processOwners(),
): { text: string; st: Stats } {
  return withKeyFile(path, owners, (fd, st) => ({ text: readFileSync(fd, 'utf8'), st }));
}
