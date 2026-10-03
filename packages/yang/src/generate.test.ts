import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { PkiKeySpecSchema, ROOT_KEYS } from '@ngfw/schema';
import { generateModule, generateModules, moduleList, toXsdPattern } from './generate.js';

const generatedDir = new URL('../generated/', import.meta.url);

describe('YANG generator', () => {
  it('emits one module per root key', () => {
    const mods = generateModules();
    expect(Object.keys(mods).sort()).toEqual(ROOT_KEYS.map((k) => `vrx-${k}`).sort());
  });

  it('matches the checked-in generated modules (run `pnpm --filter @ngfw/yang gen` on drift)', () => {
    for (const key of ROOT_KEYS) {
      const name = `vrx-${key}`;
      const onDisk = readFileSync(new URL(`${name}.yang`, generatedDir), 'utf8');
      expect(generateModule(key), name).toBe(onDisk);
    }
  });

  it('each module is a valid-looking YANG 1.1 module with a namespace and a top container', () => {
    for (const key of ROOT_KEYS) {
      const m = generateModule(key);
      expect(m).toContain(`module vrx-${key} {`);
      expect(m).toContain('yang-version 1.1;');
      expect(m).toContain(`namespace "urn:vrx:${key}";`);
      // the top node is a container (strictObject) or a list (a record root, e.g. interfaces/vrfs)
      expect(m, key).toMatch(new RegExp(`(container|list) ${key} \\{`));
      // balanced braces, ignoring any inside quoted strings (patterns/descriptions carry literal { } )
      const code = m.replace(/"(?:[^"\\]|\\.)*"/g, '""');
      expect((code.match(/{/g) ?? []).length, key).toBe((code.match(/}/g) ?? []).length);
    }
  });

  it('maps records to key "name" lists and arrays with itemKey to their own key', () => {
    const mgmt = generateModule('management');
    expect(mgmt).toMatch(/list rules \{\s*\n\s*key "name";/); // management.alarms.rules is a record
    expect(mgmt).toMatch(/list users \{\s*\n\s*key "username";/); // management.users has itemKey
  });

  it('marks the one secret leaf write-only via NACM and never emits it as readable', () => {
    const mgmt = generateModule('management');
    expect(mgmt).toContain('import ietf-netconf-acm');
    // passwordHash (the only writeOnly leaf) carries the deny-all
    const idx = mgmt.indexOf('leaf passwordHash');
    expect(idx).toBeGreaterThan(-1);
    expect(mgmt.slice(idx, idx + 200)).toContain('nacm:default-deny-all;');
  });

  it('maps enums, integer ranges and booleans', () => {
    const sec = generateModule('security');
    expect(sec).toContain('type enumeration {');
    expect(sec).toContain('enum "webLogin";');
    expect(sec).toContain('range "1..100000";');
    expect(sec).toContain('type boolean;');
    expect(sec).toContain('default "false";');
  });

  it('preserves RSA size literal unions as exact integer ranges', () => {
    const vpn = generateModule('vpn');
    const leaves = [...vpn.matchAll(/leaf bits \{([\s\S]*?)description "RSA only \(default 2048\)";/g)];
    expect(leaves.length).toBeGreaterThan(0);
    for (const [, body] of leaves) {
      expect(body).not.toContain('decimal64');
      expect((body!.match(/type uint32/g) ?? []).length).toBe(3);
      const choices = new Set([...body!.matchAll(/range "([0-9]+)";/g)].map((m) => Number(m[1])));
      expect([...choices]).toEqual([2048, 3072, 4096]);
      for (const bits of [-1, 0, 1024, 2048, 2048.5, 3072, 4096, 8192]) {
        expect(choices.has(bits), `YANG admits ${bits}`).toBe(
          PkiKeySpecSchema.safeParse({ type: 'rsa', bits }).success,
        );
      }
    }
  });

  it('preserves the IKE version numeric literal union too', () => {
    const vpn = generateModule('vpn');
    const leaf = vpn.match(/leaf ikeVersion \{([\s\S]*?)default "2";/)?.[1];
    expect(leaf).toBeDefined();
    expect(leaf).not.toContain('decimal64');
    expect(leaf).toContain('range "1";');
    expect(leaf).toContain('range "2";');
    expect((leaf!.match(/type uint32/g) ?? []).length).toBe(2);
  });

  it('lists modules for ietf-yang-library', () => {
    const list = moduleList();
    expect(list).toHaveLength(ROOT_KEYS.length);
    expect(list[0]).toMatchObject({ name: 'vrx-system', namespace: 'urn:vrx:system' });
  });
});

describe('toXsdPattern', () => {
  it('strips JS anchors', () => {
    expect(toXsdPattern('^abc$')).toBe('abc');
  });
  it('converts non-capturing groups and unescapes slashes', () => {
    expect(toXsdPattern('^(?:a|b)\\/c$')).toBe('(a|b)/c');
  });
  it('drops patterns with lookahead', () => {
    expect(toXsdPattern('^(?=.*x)$')).toBeUndefined();
  });
  it('drops patterns with mid-pattern anchors or word boundaries', () => {
    expect(toXsdPattern('a\\bc')).toBeUndefined();
  });
});
