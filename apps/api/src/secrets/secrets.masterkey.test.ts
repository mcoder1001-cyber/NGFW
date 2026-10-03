import { chmodSync, mkdtempSync, rmSync, statSync, symlinkSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { afterEach, describe, expect, it } from 'vitest';
import type { Env } from '../config.js';
import { ProblemError } from '../common/problem.js';
import { SecretsService } from './secrets.service.js';

/** SEC-auth L1: the master key goes through the key-file checks of the JWT key ring (readKeyFileBytes). */
describe('SecretsService master key file (SEC-auth L1)', () => {
  const dirs: string[] = [];
  afterEach(() => {
    for (const d of dirs.splice(0)) rmSync(d, { recursive: true, force: true });
  });
  const svc = (file: string) =>
    new SecretsService(
      null as never,
      { NGFW_SECRET_KEY_FILE: file } as unknown as Env,
      null as never,
      null as never,
    ) as unknown as { masterKey(): Buffer };
  const dir = () => {
    const d = mkdtempSync(join(tmpdir(), 'ngfw-mk-'));
    dirs.push(d);
    return d;
  };
  const refused = (file: string, why: RegExp) => {
    let err: unknown;
    try {
      svc(file).masterKey();
    } catch (e) {
      err = e;
    }
    expect(err).toBeInstanceOf(ProblemError);
    expect((err as ProblemError).getStatus()).toBe(503);
    expect((err as Error).message).toMatch(why);
  };

  it('creates a missing key 0600 and reads it back', () => {
    const file = join(dir(), 'sub', 'secret.key');
    const key = svc(file).masterKey();
    expect(key).toHaveLength(32);
    expect(statSync(file).mode & 0o777).toBe(0o600);
    expect(svc(file).masterKey().equals(key)).toBe(true);
  });

  it('refuses a symlink (before: read and chmod-ed through it)', () => {
    const d = dir();
    const target = join(d, 'elsewhere');
    writeFileSync(target, Buffer.alloc(32, 7), { mode: 0o644 });
    const link = join(d, 'secret.key');
    symlinkSync(target, link);
    refused(link, /symbolic link/);
    expect(statSync(target).mode & 0o777).toBe(0o644); // the target was not chmod-ed
  });

  it('refuses a key group/others can read (before: silently chmod-ed)', () => {
    const file = join(dir(), 'secret.key');
    writeFileSync(file, Buffer.alloc(32, 1));
    chmodSync(file, 0o640);
    refused(file, /chmod 0600/);
  });
});
