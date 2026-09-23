import { MergePatchError } from '@ngfw/schema';
import { describe, expect, it } from 'vitest';
import {
  canonicalJson,
  documentHash,
  getAt,
  PointerNotFoundError,
  removeAt,
  setAt,
} from './json.js';

describe('pointer helpers', () => {
  const doc = { a: { 'x/y': 1, list: [1, 2, 3] } };

  it('getAt reads escaped segments and array items, undefined when missing', () => {
    expect(getAt(doc, '/a/x~1y')).toBe(1);
    expect(getAt(doc, '/a/list/2')).toBe(3);
    expect(getAt(doc, '/a/nope/deeper')).toBeUndefined();
    expect(getAt(doc, '/a/list/-1')).toBeUndefined();
    expect(getAt(doc, '')).toBe(doc);
  });

  it('setAt replaces, creates parents, appends to arrays and never mutates', () => {
    const out = setAt(doc, '/a/new/leaf', true) as typeof doc & { a: { new: { leaf: boolean } } };
    expect(out.a.new.leaf).toBe(true);
    expect(setAt(doc, '/a/list/3', 4)).toEqual({ a: { 'x/y': 1, list: [1, 2, 3, 4] } });
    expect(doc).toEqual({ a: { 'x/y': 1, list: [1, 2, 3] } });
    expect(setAt(doc, '', 7)).toBe(7);
  });

  it('setAt rejects prototype keys and bad indexes with a pointer (D-049/D-070)', () => {
    expect(() => setAt({}, '/__proto__/x', 1)).toThrow(MergePatchError);
    expect(() => setAt(doc, '/a/list/9', 1)).toThrow(/invalid array index/);
    try {
      setAt({}, '/a/constructor', 1);
    } catch (e) {
      expect((e as MergePatchError).pointer).toBe('/a/constructor');
    }
  });

  it('removeAt deletes members and splices arrays; missing → PointerNotFoundError', () => {
    expect(removeAt(doc, '/a/x~1y')).toEqual({ a: { list: [1, 2, 3] } });
    expect(removeAt(doc, '/a/list/0')).toEqual({ a: { 'x/y': 1, list: [2, 3] } });
    expect(() => removeAt(doc, '/a/zzz')).toThrow(PointerNotFoundError);
    expect(() => removeAt(doc, '/a/list/5')).toThrow(PointerNotFoundError);
    expect(() => removeAt(doc, '')).toThrow(MergePatchError);
  });

  it('canonical JSON and hashes ignore key order', () => {
    expect(canonicalJson({ b: 1, a: { d: 2, c: [3, { f: 1, e: 0 }] } })).toBe(
      '{"a":{"c":[3,{"e":0,"f":1}],"d":2},"b":1}',
    );
    expect(documentHash({ a: 1, b: 2 })).toBe(documentHash({ b: 2, a: 1 }));
    expect(documentHash({ a: 1 })).toMatch(/^[0-9a-f]{64}$/);
  });
});
