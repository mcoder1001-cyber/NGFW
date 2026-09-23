import { describe, expect, it } from 'vitest';
import { z } from 'zod';
import { diff } from './diff.js';
import { RootConfig } from './index.js';
import { redactSecrets, secretPointers } from './secrets.js';
import { withUi } from './ui.js';

const HASH = '$vrx-test$VRX_TEST_HASH_admin';
const doc = {
  system: { hostname: 'vrx-a' },
  management: {
    users: [
      { username: 'admin', role: 'admin', passwordHash: HASH, fullName: 'Admin' },
      { username: 'noc', role: 'readonly' },
      { username: 'ops', role: 'operator', passwordHash: HASH },
    ],
    aaa: { radius: { servers: [{ address: '10.0.0.1', secretRef: 'psk/radius-primary' }] } },
  },
};

describe('secretPointers / redactSecrets (D-046, review M4)', () => {
  it('finds every password hash and nothing else (secretRefs are references, not secrets)', () => {
    expect(secretPointers(doc)).toEqual([
      '/management/users/0/passwordHash',
      '/management/users/2/passwordHash',
    ]);
    expect(secretPointers(RootConfig.parse(doc))).toEqual([
      '/management/users/0/passwordHash',
      '/management/users/2/passwordHash',
    ]);
    expect(secretPointers({})).toEqual([]);
    expect(secretPointers(null)).toEqual([]);
  });

  it('removes them from a copy and leaves the input untouched', () => {
    const redacted = redactSecrets(doc);
    expect(redacted.management.users).toEqual([
      { username: 'admin', role: 'admin', fullName: 'Admin' },
      { username: 'noc', role: 'readonly' },
      { username: 'ops', role: 'operator' },
    ]);
    expect(redacted.management.aaa.radius.servers[0]?.secretRef).toBe('psk/radius-primary');
    expect(doc.management.users[0]?.passwordHash).toBe(HASH);
    expect(JSON.stringify(redactSecrets(RootConfig.parse(doc)))).not.toContain(HASH);
  });

  it('keeps hashes out of a diff of redacted documents (the M4 probe)', () => {
    const changed = structuredClone(doc);
    changed.management.users[1]!.role = 'operator';
    const changes = diff(redactSecrets(doc), redactSecrets(changed));
    expect(changes).toHaveLength(1);
    expect(JSON.stringify(changes)).not.toContain(HASH);
    expect(JSON.stringify(diff(doc, changed))).toContain(HASH); // why the API must redact first
  });

  it('follows records, unions, tuples and $ref in any schema', () => {
    const secret = withUi(z.string(), { secret: true });
    const leaf = z.object({ key: secret, name: z.string() });
    const schema = z.object({
      byName: z.record(z.string(), leaf),
      either: z.union([z.object({ a: secret }), z.object({ b: z.string() })]),
      pair: z.tuple([leaf, z.string()]),
      list: z.array(leaf),
    });
    const value = {
      byName: { x: { key: 's1', name: 'x' } },
      either: { a: 's2' },
      pair: [{ key: 's3', name: 'p' }, 'plain'],
      list: [{ key: 's4', name: 'l' }],
      unknown: { key: 'not-described' },
    };
    expect(secretPointers(value, schema)).toEqual([
      '/byName/x/key',
      '/either/a',
      '/pair/0/key',
      '/list/0/key',
    ]);
    expect(redactSecrets(value, schema)).toEqual({
      byName: { x: { name: 'x' } },
      either: {},
      pair: [{ name: 'p' }, 'plain'],
      list: [{ name: 'l' }],
      unknown: { key: 'not-described' },
    });
    type Tree = { key?: string | undefined; children: Tree[] };
    const tree: z.ZodType<Tree> = z.lazy(() =>
      z.object({ key: secret.optional(), children: z.array(tree) }),
    );
    expect(
      secretPointers({ key: 'a', children: [{ key: 'b', children: [] }] }, z.object({ t: tree })),
    ).toEqual([]); // not under a described property
    expect(
      secretPointers(
        { t: { key: 'a', children: [{ key: 'b', children: [] }] } },
        z.object({ t: tree }),
      ),
    ).toEqual(['/t/key', '/t/children/0/key']);
  });

  it('resolves # and $defs references, ignores unknown ones and survives self-referencing unions', () => {
    const secret = withUi(z.string(), { secret: true });
    type Node = { key?: string | undefined; next?: Node | undefined };
    const node: z.ZodType<Node> = z.lazy(() =>
      z.object({ key: secret.optional(), next: node.optional() }),
    );
    expect(secretPointers({ key: 'a', next: { key: 'b' } }, node)).toEqual(['/key', '/next/key']);
    const odd = z.object({
      viaNowhere: z.object({ key: secret }).meta({ $ref: '#/properties/elsewhere' }),
      viaMissingDefs: z.object({ key: secret }).meta({ $ref: '#/$defs/missing' }),
    });
    expect(
      secretPointers({ viaNowhere: { key: 'x' }, viaMissingDefs: { key: 'y' } }, odd),
    ).toEqual([]);
    type Loop = string | Loop[];
    const loop: z.ZodType<Loop> = z.lazy(() => z.union([z.string(), z.array(loop)]));
    expect(secretPointers(['a', ['b']], loop)).toEqual([]);
  });
});
