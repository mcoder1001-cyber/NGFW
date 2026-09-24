import { lstatSync, type Stats } from 'node:fs';

/**
 * TD-10b (P06 tech debt, "owner check of the key file"): a file that holds key material must be a regular file (not
 * a symlink another user could re-point), owned by the user this process runs as (or root), and give group/others no
 * access at all. Anything else is refused with a message that names the path and the problem — never the content.
 * Used for the JWT key ring (VRX_JWT_KEY_FILE); the secret store's master key (VRX_SECRET_KEY_FILE,
 * secrets.service.ts) should call it too — that file belongs to TD-10a (TD-10b questions).
 */
export class KeyFileError extends Error {
  constructor(path: string, problem: string) {
    super(`key file ${path}: ${problem}`);
    this.name = 'KeyFileError';
  }
}

export function checkKeyFile(path: string, uid: number | undefined = process.geteuid?.()): Stats {
  let st: Stats;
  try {
    st = lstatSync(path);
  } catch (e) {
    throw new KeyFileError(
      path,
      `cannot be read (${(e as NodeJS.ErrnoException).code ?? 'error'})`,
    );
  }
  if (st.isSymbolicLink()) throw new KeyFileError(path, 'is a symbolic link (use the file itself)');
  if (!st.isFile()) throw new KeyFileError(path, 'is not a regular file');
  if (uid !== undefined && st.uid !== uid && st.uid !== 0) {
    throw new KeyFileError(path, `is owned by uid ${st.uid}; it must belong to uid ${uid} or root`);
  }
  if ((st.mode & 0o077) !== 0) {
    throw new KeyFileError(
      path,
      `mode ${(st.mode & 0o777).toString(8).padStart(4, '0')} lets group/others access it; chmod 0600`,
    );
  }
  return st;
}
