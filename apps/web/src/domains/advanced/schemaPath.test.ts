import type { JsonSchema } from '@ngfw/ui-kit/schema-form';
import { describe, expect, it } from 'vitest';
import {
  childPropertyKeys,
  configPathTo,
  createMergePatch,
  parseConfigPath,
  resolveNode,
  valueAt,
  withoutChildProperties,
  withoutChildValues,
  wrapAtPath,
} from './schemaPath';

const ROOT_KEYS = ['interfaces', 'system'] as const;

describe('parseConfigPath', () => {
  it('splits the domain from the rest and drops empty segments', () => {
    expect(parseConfigPath('interfaces/TenGigabitEthernet0~10~10/mtu', ROOT_KEYS)).toEqual({
      domainKey: 'interfaces',
      segments: ['TenGigabitEthernet0~10~10', 'mtu'],
    });
    expect(parseConfigPath('system', ROOT_KEYS)).toEqual({ domainKey: 'system', segments: [] });
    expect(parseConfigPath('interfaces//eth0/', ROOT_KEYS)).toEqual({ domainKey: 'interfaces', segments: ['eth0'] });
  });

  it('rejects an empty splat or an unknown domain (never a hand-written domain list)', () => {
    expect(parseConfigPath(undefined, ROOT_KEYS)).toBeNull();
    expect(parseConfigPath('', ROOT_KEYS)).toBeNull();
    expect(parseConfigPath('nope/x', ROOT_KEYS)).toBeNull();
  });

  it('configPathTo is the inverse for the route splat', () => {
    expect(configPathTo('interfaces', ['eth0', 'mtu'])).toBe('config/interfaces/eth0/mtu');
    expect(configPathTo('system')).toBe('config/system');
  });
});

/** An `interfaces`-shaped domain schema: a record of named interfaces, each with a nested `subinterfaces` record. */
const SUB_ITEM: JsonSchema = { type: 'object', properties: { vlanId: { type: 'number' } }, required: ['vlanId'] };
const IFACE_ITEM: JsonSchema = {
  type: 'object',
  properties: {
    mtu: { type: 'number' },
    enabled: { type: 'boolean' },
    subinterfaces: { type: 'object', additionalProperties: { $ref: '#/$defs/sub' } },
  },
};
const INTERFACES_DOMAIN: JsonSchema = {
  type: 'object',
  additionalProperties: IFACE_ITEM,
  $defs: { sub: SUB_ITEM },
};

describe('resolveNode', () => {
  it('resolves the domain root', () => {
    const node = resolveNode(INTERFACES_DOMAIN, []);
    expect(node?.record).toBe(true);
  });

  it('descends through a record by any key, then a fixed property, then a $ref\'d nested record', () => {
    const node = resolveNode(INTERFACES_DOMAIN, ['eth0', 'subinterfaces']);
    expect(node?.record).toBe(true);
    expect(node?.schema.additionalProperties).toMatchObject({ $ref: '#/$defs/sub' });
  });

  it('resolves a $ref leaf and reports it as a non-record object', () => {
    const node = resolveNode(INTERFACES_DOMAIN, ['eth0', 'subinterfaces', '100']);
    expect(node?.record).toBe(false);
    expect(node?.schema.properties).toMatchObject({ vlanId: { type: 'number' } });
  });

  it('array segments must be a valid index into `items`', () => {
    const domain: JsonSchema = { type: 'object', properties: { list: { type: 'array', items: { type: 'string' } } } };
    expect(resolveNode(domain, ['list', '0'])?.schema).toEqual({ type: 'string' });
    expect(resolveNode(domain, ['list', 'nope'])).toBeNull();
  });

  it('a segment that names nothing in the schema is not a configuration path', () => {
    expect(resolveNode(INTERFACES_DOMAIN, ['eth0', 'nope'])).toBeNull();
  });
});

describe('childPropertyKeys / withoutChildProperties', () => {
  it('lists only the object/record properties, not scalars or arrays', () => {
    const item = resolveNode(INTERFACES_DOMAIN, ['eth0'])!.schema;
    expect(childPropertyKeys(item, INTERFACES_DOMAIN)).toEqual(['subinterfaces']);
  });

  it('a record node has no child properties of its own (its children are data keys, not schema keys)', () => {
    expect(childPropertyKeys(INTERFACES_DOMAIN, INTERFACES_DOMAIN)).toEqual([]);
  });

  it('strips exactly the child keys (and them from required) from the node\'s own form schema', () => {
    const item = resolveNode(INTERFACES_DOMAIN, ['eth0'])!.schema;
    const form = withoutChildProperties(item, ['subinterfaces']);
    expect(Object.keys(form.properties ?? {})).toEqual(['mtu', 'enabled']);
  });
});

describe('withoutChildValues (the diff base for a node\'s own form, review: child containers must never be nulled)', () => {
  it('drops exactly the child-container members, keeping the rest', () => {
    const value = { mtu: 1500, enabled: true, subinterfaces: { '100': { vlanId: 100 } } };
    expect(withoutChildValues(value, ['subinterfaces'])).toEqual({ mtu: 1500, enabled: true });
  });

  it('a merge patch of the projected base against the form value never nulls the child container', () => {
    const base = { hostname: 'vrx', timezone: 'UTC', banner: { login: 'hi' }, dns: { servers: [] } };
    const formValue = { hostname: 'vrx-a', timezone: 'UTC' }; // the form never carries banner/dns (they are child nodes)
    const naive = createMergePatch(base, formValue);
    expect(naive).toEqual({ hostname: 'vrx-a', banner: null, dns: null }); // the bug this projection avoids
    const patch = createMergePatch(withoutChildValues(base, ['banner', 'dns']), formValue);
    expect(patch).toEqual({ hostname: 'vrx-a' });
  });

  it('passes non-object values and an empty child-key list through unchanged', () => {
    expect(withoutChildValues(undefined, ['a'])).toBeUndefined();
    expect(withoutChildValues({ a: 1 }, [])).toEqual({ a: 1 });
  });
});

describe('valueAt', () => {
  const doc = { eth0: { mtu: 1500, subinterfaces: { '100': { vlanId: 100 } } }, list: [1, 2, 3] };
  it('walks objects and arrays', () => {
    expect(valueAt(doc, ['eth0', 'mtu'])).toBe(1500);
    expect(valueAt(doc, ['eth0', 'subinterfaces', '100', 'vlanId'])).toBe(100);
    expect(valueAt(doc, ['list', '1'])).toBe(2);
  });
  it('is undefined off the end of the document', () => {
    expect(valueAt(doc, ['eth1'])).toBeUndefined();
    expect(valueAt(doc, ['eth0', 'mtu', 'nope'])).toBeUndefined();
    expect(valueAt(doc, ['list', 'nope'])).toBeUndefined();
  });
  it('the empty path is the whole document', () => {
    expect(valueAt(doc, [])).toBe(doc);
  });
});

describe('wrapAtPath (subtree PATCH/DELETE as a merge patch of the domain root)', () => {
  it('nests the body under every segment', () => {
    expect(wrapAtPath(['eth0', 'mtu'], 1500)).toEqual({ eth0: { mtu: 1500 } });
  });
  it('an empty path returns the body itself (editing the domain root)', () => {
    expect(wrapAtPath([], { mtu: 1500 })).toEqual({ mtu: 1500 });
  });
  it('DELETE is the same wrap with a null body (RFC 7386 remove)', () => {
    expect(wrapAtPath(['eth0'], null)).toEqual({ eth0: null });
    expect(wrapAtPath(['eth0', 'subinterfaces', '100'], null)).toEqual({ eth0: { subinterfaces: { '100': null } } });
  });
});

describe('createMergePatch', () => {
  it('removed members become null, unchanged members are omitted, changed members are replaced', () => {
    expect(createMergePatch({ a: 1, b: 2, c: 3 }, { a: 1, b: 20 })).toEqual({ b: 20, c: null });
  });
  it('recurses into nested objects and drops an empty inner patch', () => {
    expect(createMergePatch({ n: { x: 1 } }, { n: { x: 1 } })).toEqual({});
    expect(createMergePatch({ n: { x: 1 } }, { n: { x: 2 } })).toEqual({ n: { x: 2 } });
  });
  it('arrays are replaced wholesale, never merged (D-021)', () => {
    expect(createMergePatch({ a: [1, 2] }, { a: [1] })).toEqual({ a: [1] });
  });
  it('a brand-new node (no base) patches to the value itself', () => {
    expect(createMergePatch(undefined, { mtu: 1500 })).toEqual({ mtu: 1500 });
  });
});
