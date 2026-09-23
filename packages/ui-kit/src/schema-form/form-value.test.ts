import { describe, expect, it, vi } from 'vitest';
import { compile, compileError, findRecordKeyIssues, formPathFor, formatForPattern, fromFormValue, pointerToFormPath, toFormValue } from './form-value.js';
import { issueMessage } from './messages.js';
import { createSchemaResolver, ROOT_FIELD } from './resolver.js';
import { defaultValueFor, mergeAllOf, parsePointer, resolveRef } from './schema-utils.js';
import { WIDGET_SCHEMA, WIDGET_VALUE } from './test-schema.js';
import type { JsonSchema, Translate } from './types.js';

const S = WIDGET_SCHEMA;
const t: Translate = (key, o) => `${key}${o ? ' ' + JSON.stringify(o) : ''}`;

describe('form value ↔ JSON', () => {
  it('turns records into entry arrays (keys with / and . survive) and back', () => {
    const form = toFormValue(S, WIDGET_VALUE, S) as { subs: unknown };
    expect(form.subs).toEqual([{ key: 'Gig0/0/0.100', value: { vlanId: 100 } }]);
    expect(fromFormValue(S, form, S)).toEqual(WIDGET_VALUE);
  });

  it('maps JSON paths and RFC 6901 pointers onto the form layout', () => {
    const form = toFormValue(S, WIDGET_VALUE, S);
    expect(formPathFor(S, form, ['subs', 'Gig0/0/0.100', 'vlanId'], S)).toBe('subs.0.value.vlanId');
    expect(pointerToFormPath(S, form, '/subs/Gig0~10~10.100/vlanId', S)).toBe('subs.0.value.vlanId');
    expect(pointerToFormPath(S, form, '/auth/psk', S)).toBe('auth.psk');
    expect(pointerToFormPath(S, form, '/subs/missing/vlanId', S)).toBeNull();
    expect(pointerToFormPath(S, form, '/nope', S)).toBe('nope');
    expect(parsePointer('#/a~1b/0')).toEqual(['a/b', '0']);
  });

  it('flags empty and duplicate record keys', () => {
    const form = toFormValue(S, { ...WIDGET_VALUE, subs: { a: { vlanId: 1 }, b: { vlanId: 2 } } }, S) as {
      subs: { key: string; value: unknown }[];
    };
    form.subs[1]!.key = 'a';
    form.subs.push({ key: '', value: { vlanId: 3 } });
    expect(findRecordKeyIssues(S, form, S)).toEqual(
      expect.arrayContaining([
        { path: 'subs.1.key', type: 'duplicate' },
        { path: 'subs.0.key', type: 'duplicate' },
        { path: 'subs.2.key', type: 'empty' },
      ]),
    );
  });

  it('enforces cidr formats that Zod ignores and keeps the format-specific message', () => {
    const cidr = { type: 'string' as const, format: 'cidrv4' };
    const res = compile(cidr, cidr).safeParse('10.0.0.1/33');
    expect(res.success).toBe(false);
    const issue = res.error!.issues[0]!;
    expect(issue.code).toBe('invalid_format');
    expect(formatForPattern((issue as { pattern?: string }).pattern)).toBe('cidrv4');
    expect(issueMessage(issue as never, t)).toBe('validation.cidrv4');
    expect(compile(cidr, cidr).safeParse('10.0.0.1/24').success).toBe(true);
  });

  it('builds defaults from the schema', () => {
    expect(defaultValueFor(S, S)).toMatchObject({ enabled: true, mtu: 1500, weight: 5, rxMode: 'adaptive', subs: {}, dns: [] });
  });
});

describe('createSchemaResolver', () => {
  const resolve = createSchemaResolver(S, t);
  const opts = { fields: {}, shouldUseNativeValidation: false, criteriaMode: 'firstError' as const };

  it('returns the parsed JSON (defaults applied, records as objects) when valid', async () => {
    const res = await resolve({ [ROOT_FIELD]: toFormValue(S, WIDGET_VALUE, S) }, undefined, opts);
    expect(res.errors).toEqual({});
    expect((res.values as Record<string, unknown>)[ROOT_FIELD]).toMatchObject({
      ...WIDGET_VALUE,
      enabled: true,
      subs: { 'Gig0/0/0.100': { vlanId: 100 } },
    });
  });

  it('maps issues onto fields with translated messages', async () => {
    const form = toFormValue(S, { ...WIDGET_VALUE, mtu: 20, subs: { x: { vlanId: 0 }, y: { vlanId: 2 }, w: { vlanId: 3 } } }, S) as {
      subs: { key: string }[];
      name?: string;
    };
    form.subs[2]!.key = 'y';
    delete form.name;
    const res = await resolve({ [ROOT_FIELD]: form }, undefined, opts);
    const errors = res.errors as Record<string, Record<string, unknown>>;
    const root = errors[ROOT_FIELD]!;
    expect((root.mtu as { message: string }).message).toBe('validation.minimum {"limit":68}');
    expect((root.name as { message: string }).message).toBe('validation.required');
    const subs = root.subs as Record<string, { key?: { message: string }; value?: { vlanId?: { message: string } } }>;
    expect(subs[0]!.value!.vlanId!.message).toBe('validation.minimum {"limit":1}');
    expect(subs[1]!.key!.message).toBe('form.duplicateKey');
    expect(subs[2]!.key!.message).toBe('form.duplicateKey');
  });
});

describe('dependsOn pruning and compile failures (review M3, M4)', () => {
  const GATED: JsonSchema = {
    type: 'object',
    properties: {
      mode: { type: 'string', enum: ['static', 'dhcp'] },
      nested: {
        type: 'object',
        properties: {
          on: { type: 'boolean' },
          sibling: { type: 'string', minLength: 3, 'x-vrx-ui': { dependsOn: 'on' } },
          absolute: { type: 'string', minLength: 3, 'x-vrx-ui': { dependsOn: { field: '/mode', value: 'static' } } },
        },
      },
    },
  };

  it('drops fields whose sibling or absolute dependsOn is not met, keeps them when it is', () => {
    const hidden = { mode: 'dhcp', nested: { on: false, sibling: 'x', absolute: 'y' } };
    expect(fromFormValue(GATED, hidden, GATED, hidden)).toEqual({ mode: 'dhcp', nested: { on: false } });
    const shown = { mode: 'static', nested: { on: true, sibling: 'abc', absolute: 'def' } };
    expect(fromFormValue(GATED, shown, GATED, shown)).toEqual(shown);
    // without formRoot (variant matching helpers) nothing is pruned
    expect(fromFormValue(GATED, hidden, GATED)).toEqual(hidden);
  });

  it('the resolver accepts a form whose only invalid values are hidden, and returns them stripped', async () => {
    const resolve = createSchemaResolver(GATED, t);
    const form = { mode: 'dhcp', nested: { on: false, sibling: 'x', absolute: 'y' } };
    const r = await resolve({ [ROOT_FIELD]: form }, undefined, { fields: {}, shouldUseNativeValidation: false });
    expect(r.errors).toEqual({});
    expect(r.values).toEqual({ [ROOT_FIELD]: { mode: 'dhcp', nested: { on: false } } });
    const shown = { mode: 'static', nested: { on: true, sibling: 'x', absolute: 'y' } };
    const r2 = await resolve({ [ROOT_FIELD]: shown }, undefined, { fields: {}, shouldUseNativeValidation: false });
    expect(Object.keys((r2.errors as Record<string, Record<string, unknown>>)[ROOT_FIELD]!.nested as object).sort()).toEqual(['absolute', 'sibling']);
  });

  it('a schema Zod cannot compile rejects everything and reports why', async () => {
    const error = vi.spyOn(console, 'error').mockImplementation(() => {});
    const bad: JsonSchema = { type: 'object', properties: { a: { $ref: '#/$defs/missing' } } };
    expect(compileError(bad, bad)?.message).toMatch(/Reference not found/);
    expect(compile(bad, bad).safeParse({}).success).toBe(false);
    const r = await createSchemaResolver(bad, t)({ [ROOT_FIELD]: {} }, undefined, { fields: {}, shouldUseNativeValidation: false });
    expect(r.values).toEqual({});
    expect(JSON.stringify(r.errors)).toContain('form.validationUnavailable');
    expect(compileError(S, S)).toBeUndefined();
    error.mockRestore();
  });
});

describe('schema memoisation (review L1)', () => {
  it('resolveRef/mergeAllOf return the same object for the same input, so compile() hits its cache', () => {
    const root: JsonSchema = {
      $defs: { name: { type: 'string', minLength: 1 }, base: { type: 'object', properties: { a: { type: 'string' } } } },
      type: 'object',
      properties: {
        n: { $ref: '#/$defs/name', title: 'N' },
        m: { allOf: [{ $ref: '#/$defs/base' }, { properties: { b: { type: 'integer' } } }] },
      },
    };
    const n = root.properties!.n!;
    const m = root.properties!.m!;
    expect(resolveRef(n, root)).toBe(resolveRef(n, root));
    expect(resolveRef(n, root)).toMatchObject({ type: 'string', minLength: 1, title: 'N' });
    expect(mergeAllOf(m, root)).toBe(mergeAllOf(m, root));
    expect(Object.keys(mergeAllOf(m, root).properties!)).toEqual(['a', 'b']);
    expect(compile(resolveRef(n, root), root)).toBe(compile(resolveRef(n, root), root));
  });
});

