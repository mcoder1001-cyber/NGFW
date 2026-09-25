import { describe, expect, it } from 'vitest';
import { main, parseEnvFile, USAGE, type CliIo } from './break-glass-cli.js';

/** TD-10b (review 2.3a): the break-glass CLI refuses non-root, prints usage, never echoes a value it read. */
function io(uid: number | undefined, env: NodeJS.ProcessEnv = {}) {
  const out: string[] = [];
  const err: string[] = [];
  const x: CliIo = { getuid: () => uid, env, out: (l) => out.push(l), err: (l) => err.push(l) };
  return { x, out, err };
}

describe('vrx-authctl', () => {
  it('usage errors → 2 before anything else; --help → 0', async () => {
    for (const argv of [
      [],
      ['unlock'],
      ['unlock', 'a', 'b'],
      ['locks', 'x'],
      ['nope'],
      ['--env-file'],
    ]) {
      const t = io(0);
      expect(await main(argv, t.x), argv.join(' ')).toBe(2);
      expect(t.err).toEqual([USAGE]);
    }
    const h = io(1000);
    expect(await main(['--help'], h.x)).toBe(0);
    expect(h.out).toEqual([USAGE]);
  });

  it('root only: any other uid → 1, nothing connected', async () => {
    for (const uid of [1000, undefined]) {
      const t = io(uid);
      expect(await main(['unlock', 'admin'], t.x)).toBe(1);
      expect(t.err).toEqual([
        'vrx-authctl: root only (the break-glass acts on the database and Valkey directly)',
      ]);
    }
  });

  it('an unreadable env file names no path content; bad settings name the key, never the value', async () => {
    const t = io(0);
    expect(await main(['--env-file', '/nonexistent/vrx.env', 'locks'], t.x)).toBe(1);
    expect(t.err).toEqual(['vrx-authctl: cannot read an --env-file (ENOENT)']);
    const secret = 'VRX_TEST_PSK_TD10B_not-a-url';
    const b = io(0, { VRX_VALKEY_DB: secret });
    expect(await main(['locks'], b.x)).toBe(1);
    expect(b.err.join('\n')).toMatch(/VRX_VALKEY_DB/);
    expect(b.err.join('\n')).not.toContain(secret);
  });

  it('parseEnvFile: VRX_ keys only, `export`, quotes and comments', () => {
    expect(
      parseEnvFile(
        [
          '# comment',
          'VRX_PG_DSN=postgres://h/db',
          'export VRX_VALKEY_DB="3"',
          "VRX_VALKEY_PREFIX='vrx:app:'",
          'PATH=/tmp',
          'vrx_lower=1',
          '',
        ].join('\n'),
      ),
    ).toEqual({ VRX_PG_DSN: 'postgres://h/db', VRX_VALKEY_DB: '3', VRX_VALKEY_PREFIX: 'vrx:app:' });
  });
});
