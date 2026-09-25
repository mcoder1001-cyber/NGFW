import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { RootConfig } from '../index.js';
import { validateSemantics } from './index.js';

const base = (): Record<string, unknown> =>
  JSON.parse(
    readFileSync(
      new URL('../../../proto/test/fixtures/host-stack-basic.json', import.meta.url),
      'utf8',
    ),
  ) as Record<string, unknown>;

type Doc = { services: { hostStack: Record<string, unknown> } } & Record<string, unknown>;
const hs = (d: Record<string, unknown>) => (d as Doc).services.hostStack;

function schemaPaths(doc: unknown): string[] {
  const r = RootConfig.safeParse(doc);
  return r.success ? [] : r.error.issues.map((i) => '/' + i.path.join('/'));
}

describe('services.hostStack schema', () => {
  it('accepts the fixture and its semantics are clean', () => {
    const r = RootConfig.safeParse(base());
    expect(r.success).toBe(true);
    if (r.success) expect(validateSemantics(r.data)).toEqual([]);
  });

  it('rejects an inline namespace secret (D-051)', () => {
    const d = base();
    (hs(d).namespaces as Record<string, Record<string, unknown>>)['w13-app']!['secret'] = 'hunter2';
    expect(schemaPaths(d)).toContain('/services/hostStack/namespaces/w13-app');
    const d2 = base();
    (hs(d2).namespaces as Record<string, Record<string, unknown>>)['w13-app']!['secretRef'] =
      'hunter2';
    expect(schemaPaths(d2)).toContain('/services/hostStack/namespaces/w13-app/secretRef');
    const d3 = base();
    (hs(d3).namespaces as Record<string, Record<string, unknown>>)['w13-app']!['secretRef'] =
      'key/ns-secret';
    expect(schemaPaths(d3)).toEqual([]);
  });

  it.each([
    '/var/lib/vrx/www/../../etc',
    '/etc/passwd',
    '/var/lib/vrx/www/a\u0001b',
    '/var/lib/vrx/wwwx',
  ])('rejects wwwRootPath %j (D-049)', (p) => {
    const d = base();
    hs(d).httpStatic = { enabled: true, wwwRootPath: p, uri: 'tcp://10.1.1.1/80' };
    expect(schemaPaths(d)).toContain('/services/hostStack/httpStatic/wwwRootPath');
  });

  it('accepts a web root under /var/lib/vrx/www/', () => {
    const d = base();
    hs(d).httpStatic = {
      enabled: true,
      wwwRootPath: '/var/lib/vrx/www/site',
      uri: 'tcp://10.1.1.1/80',
    };
    expect(schemaPaths(d)).toEqual([]);
  });

  it('rejects non-canonical and mixed-family rule prefixes, duplicate tags, `..` ids', () => {
    const rules = (d: Record<string, unknown>) => hs(d).sessionRules as Record<string, unknown>[];
    let d = base();
    rules(d)[0]!.local = '10.13.1.1/24';
    expect(schemaPaths(d)).toContain('/services/hostStack/sessionRules/0/local');
    d = base();
    rules(d)[0]!.remote = 'fd00::/64';
    expect(schemaPaths(d)).toContain('/services/hostStack/sessionRules/0/remote');
    d = base();
    rules(d)[1]!.tag = 'w13-deny-ssh';
    expect(schemaPaths(d)).toContain('/services/hostStack/sessionRules/1/tag');
    d = base();
    rules(d)[0]!.tag = 'w13..x';
    expect(schemaPaths(d)).toContain('/services/hostStack/sessionRules/0/tag');
    d = base();
    rules(d)[0]!.action = 'redirect';
    expect(schemaPaths(d)).toContain('/services/hostStack/sessionRules/0/redirectAppIndex');
  });

  it('needs the session layer for rules and namespaces', () => {
    const d = base();
    hs(d).enabled = false;
    expect(schemaPaths(d)).toContain('/services/hostStack/enabled');
  });
});

describe('services.hostStack semantics', () => {
  const sem = (d: Record<string, unknown>): string[] => {
    const r = RootConfig.safeParse(d);
    if (!r.success) throw new Error(JSON.stringify(r.error.issues));
    return validateSemantics(r.data).map((i) => i.pointer);
  };

  it('reports missing VRFs, interfaces and namespaces', () => {
    const d = base();
    (hs(d).namespaces as Record<string, Record<string, unknown>>)['w13-app']!['interface'] =
      'loop99';
    (hs(d).sessionRules as Record<string, unknown>[])[1]!.appNamespace = 'nope';
    (hs(d).tcpSourceAddresses as Record<string, unknown>).vrf = 'ghost';
    expect(sem(d)).toEqual(
      expect.arrayContaining([
        '/services/hostStack/namespaces/w13-app/interface',
        '/services/hostStack/sessionRules/1/appNamespace',
        '/services/hostStack/tcpSourceAddresses/vrf',
      ]),
    );
  });

  it('refuses session rules together with Auto-SDL (one rt engine)', () => {
    const r = RootConfig.safeParse(base());
    if (!r.success) throw new Error('fixture');
    const cfg = { ...r.data, services: { ...r.data.services, autoSdl: { enabled: true } } };
    expect(validateSemantics(cfg as typeof r.data).map((i) => i.pointer)).toContain(
      '/services/hostStack/sessionRules',
    );
  });
});

describe('services.hostStack.httpStatic uri', () => {
  it.each(['tcp://0.0.0.0/80', 'tcp://::/80', 'tls://0000::/443', 'tcp://::ffff:0.0.0.0/80', 'tcp://::FFFF:0:0/80', 'tcp://999.1.1.1/80'])(
    'refuses %s',
    (uri) => {
      const d = base();
      hs(d).httpStatic = { enabled: true, wwwRootPath: '/var/lib/vrx/www/site', uri };
      expect(schemaPaths(d)).toContain('/services/hostStack/httpStatic/uri');
    },
  );
});
