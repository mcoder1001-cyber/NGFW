import { decodeProtectedHeader, decodeJwt, SignJWT } from 'jose';
import { randomBytes } from 'node:crypto';
import {
  chmodSync,
  chownSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  statSync,
  symlinkSync,
  writeFileSync,
} from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from 'vitest';
import { Bus } from '../infra/bus.js';
import type { Valkey } from '../infra/valkey.js';
import { testEnv } from '../testing/fixtures.js';
import { rotateKeyFile } from './break-glass.js';
import { checkKeyFile } from './key-file.js';
import { TokensService } from './tokens.service.js';

/** TD-10b (P06 tech debt): JWT signing-key rotation (VRX_JWT_KEY_FILE) and the key-file owner check. */
const key = () => randomBytes(36).toString('base64url');
const claims = { id: 7, username: 'u', role: 'operator' as const, sid: 'fam', gen: 1 };

describe('JWT key ring (VRX_JWT_KEY_FILE)', () => {
  let dir: string;
  const tokens = (file: string) =>
    new TokensService(testEnv({ VRX_JWT_KEY_FILE: file }), {} as Valkey, new Bus());
  const write = (file: string, text: string, mode = 0o600) => {
    writeFileSync(file, text, { mode });
    chmodSync(file, mode);
  };

  beforeAll(() => {
    dir = mkdtempSync(join(tmpdir(), 'vrx-td10b-ring-'));
  });
  afterAll(() => rmSync(dir, { recursive: true, force: true }));
  afterEach(() => vi.useRealTimers());

  it('signs with the newest key (kid header); rotation keeps tokens of the previous key valid, the next one drops them', async () => {
    const file = join(dir, 'ring1');
    write(file, `# ring\n${key()}\n`);
    const before = tokens(file);
    const t1 = await before.signAccess(claims);
    const kid1 = decodeProtectedHeader(t1).kid;
    expect(kid1).toMatch(/^[0-9a-f]{16}$/);

    expect(rotateKeyFile(file)).toEqual({ keys: 2, created: false });
    expect(statSync(file).mode & 0o777).toBe(0o600);
    const after = tokens(file);
    const t2 = await after.signAccess(claims);
    expect(decodeProtectedHeader(t2).kid).not.toBe(kid1);
    expect(await after.verifyAccess(t1)).toMatchObject({ id: 7, sid: 'fam' }); // previous key still verifies
    expect(await after.verifyAccess(t2)).toMatchObject({ id: 7 });

    // the running process picks the rotation up by itself (stat checked at most every 5 s)
    vi.useFakeTimers({ toFake: ['Date'] });
    vi.setSystemTime(Date.now() + 6_000);
    expect(await before.verifyAccess(t2)).toMatchObject({ id: 7 });
    expect(decodeProtectedHeader(await before.signAccess(claims)).kid).toBe(
      decodeProtectedHeader(t2).kid,
    );

    rotateKeyFile(file); // ring: [new, key of t2] — t1's key is gone
    const third = tokens(file);
    expect(await third.verifyAccess(t1)).toBeNull();
    expect(await third.verifyAccess(t2)).toMatchObject({ id: 7 });
    expect(readFileSync(file, 'utf8').trim().split('\n').slice(1)).toHaveLength(2);
  });

  it('a token from before the key ring (no kid) verifies against the ring; a forged kid does not', async () => {
    const k = key();
    const file = join(dir, 'ring2');
    write(file, `${key()}\n${k}\n`);
    const t = tokens(file);
    const legacy = await new SignJWT({ username: 'u', role: 'operator', typ: 'access', gen: 1 })
      .setProtectedHeader({ alg: 'HS256' })
      .setSubject('7')
      .setIssuer('vrx-api')
      .setAudience('vrx')
      .setIssuedAt()
      .setExpirationTime('60s')
      .sign(new TextEncoder().encode(k));
    expect(await t.verifyAccess(legacy)).toMatchObject({ id: 7 });
    const forged = await new SignJWT({ username: 'u', role: 'admin', typ: 'access', gen: 1 })
      .setProtectedHeader({ alg: 'HS256', kid: 'ffffffffffffffff' })
      .setSubject('7')
      .setIssuer('vrx-api')
      .setAudience('vrx')
      .setExpirationTime('60s')
      .sign(new TextEncoder().encode(key()));
    expect(await t.verifyAccess(forged)).toBeNull();
  });

  it('refuses at boot a file group/others can read, a symlink, a foreign owner, an empty ring or a short key — naming no key', () => {
    const k = key();
    const open = join(dir, 'open');
    write(open, `${k}\n`, 0o644);
    expect(() => tokens(open)).toThrow(/mode 0644 .*chmod 0600/);
    const link = join(dir, 'link');
    write(join(dir, 'target'), `${k}\n`);
    symlinkSync(join(dir, 'target'), link);
    expect(() => tokens(link)).toThrow(/symbolic link/);
    const empty = join(dir, 'empty');
    write(empty, '# nothing\n\n');
    expect(() => tokens(empty)).toThrow(/holds no key/);
    const short = join(dir, 'short');
    write(short, `${k}\nshort-key\n`);
    let msg = '';
    try {
      tokens(short);
    } catch (e) {
      msg = (e as Error).message;
    }
    expect(msg).toMatch(/line 2: a key needs ≥ 32 characters/);
    expect(msg).not.toContain('short-key');
    expect(msg).not.toContain(k);
    if (process.geteuid?.() === 0) {
      const foreign = join(dir, 'foreign');
      write(foreign, `${k}\n`);
      chownSync(foreign, 65534, 65534);
      expect(() => tokens(foreign)).toThrow(/owned by uid 65534; it must belong to uid 0 or root/);
    }
    // the owner check itself, for the other key file (secret store, TD-10b questions)
    expect(() => checkKeyFile(join(dir, 'missing'))).toThrow(/cannot be read \(ENOENT\)/);
  });

  it('a key file that turns bad later does not replace the ring (reload refused, the old keys stay)', async () => {
    const file = join(dir, 'ring3');
    write(file, `${key()}\n`);
    const t = tokens(file);
    const kid = decodeProtectedHeader(await t.signAccess(claims)).kid;
    write(file, `${key()}\n`, 0o640);
    vi.useFakeTimers({ toFake: ['Date'] });
    vi.setSystemTime(Date.now() + 6_000);
    expect(decodeProtectedHeader(await t.signAccess(claims)).kid).toBe(kid);
  });

  it('the key file wins over VRX_JWT_SECRET; VRX_JWT_SECRET alone is a one-key ring', async () => {
    const secret = key();
    const file = join(dir, 'ring4');
    write(file, `${key()}\n`);
    const both = new TokensService(
      testEnv({ VRX_JWT_KEY_FILE: file, VRX_JWT_SECRET: secret }),
      {} as Valkey,
      new Bus(),
    );
    const only = new TokensService(testEnv({ VRX_JWT_SECRET: secret }), {} as Valkey, new Bus());
    expect(await only.verifyAccess(await both.signAccess(claims))).toBeNull();
    expect(await only.verifyAccess(await only.signAccess(claims))).toMatchObject({ id: 7 });
  });
});

describe('TD-10b sessions in TokensService', () => {
  it('revokeSession (logout, review 2.3c) refuses that session only — in this process even when Valkey fails', async () => {
    const failing = {
      set: async () => {
        throw new Error('simulated Valkey outage');
      },
    } as unknown as Valkey;
    const t = new TokensService(testEnv({ VRX_JWT_SECRET: key() }), failing, new Bus());
    const mine = await t.signAccess({ ...claims, sid: 's1' });
    const other = await t.signAccess({ ...claims, sid: 's2' });
    const noSid = await t.signAccess({ id: 7, username: 'u', role: 'operator', gen: 1 });
    expect(await t.revokeSession('s1')).toBe(false);
    expect(await t.verifyAccess(mine)).toBeNull();
    expect(await t.verifyAccess(other)).toMatchObject({ sid: 's2' });
    expect(await t.verifyAccess(noSid)).toMatchObject({ id: 7 });
  });

  it('an access token never outlives its session (VRX_SESSION_MAX_SEC, review 2.3d)', async () => {
    const t = new TokensService(testEnv({ VRX_JWT_SECRET: key() }), {} as Valkey, new Bus());
    const now = Math.floor(Date.now() / 1000);
    expect(decodeJwt(await t.signAccess(claims, now + 30)).exp).toBe(now + 30);
    for (const notAfter of [undefined, now + 5000]) {
      const exp = decodeJwt(await t.signAccess(claims, notAfter)).exp!;
      expect(exp - now, String(notAfter)).toBeGreaterThanOrEqual(900);
      expect(exp - now, String(notAfter)).toBeLessThanOrEqual(901);
    }
  });
});
