import { describe, expect, it } from 'vitest';
import { compile, findRecordKeyIssues, formPathFor, formatForPattern, fromFormValue, pointerToFormPath, toFormValue } from './form-value.js';
import { issueMessage } from './messages.js';
import { createSchemaResolver, ROOT_FIELD } from './resolver.js';
import { defaultValueFor, parsePointer } from './schema-utils.js';
import { WIDGET_SCHEMA, WIDGET_VALUE } from './test-schema.js';
import type { Translate } from './types.js';

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
