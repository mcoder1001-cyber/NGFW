import i18next from 'i18next';
import { describe, expect, it } from 'vitest';
import { fromFormValue, withDefaults } from './form-value.js';
import { acceptsNull, defaultValueFor, FORM_DEFAULTS, isAsciiOnlyPattern, isLtrString, isOptionalObject, propertyTitles, sortedProperties } from './schema-utils.js';
import { isIdentifierSchema, summarizeValue, tableColumns, type SummaryText } from './summary.js';
import { createSchemaText } from './text.js';
import type { JsonSchema } from './types.js';
import { inferStringWidget } from './fields/inputs.js';
import { defaultOffsetFor, joinDateTime, joinRange, localOffset, parseDateTime, splitRange, timeZoneNames } from './fields/widgets.js';

/** WEB-1: presence defaults, RTL-1 identifier detection, widget value helpers, summaries, per-path texts. */

const IFACE: JsonSchema = {
  type: 'object',
  properties: {
    mtu: { type: 'integer', default: 1500 },
    dhcpClient: {
      type: 'object',
      properties: { hostname: { type: 'string' }, setBroadcastFlag: { type: 'boolean', default: false } },
      additionalProperties: false,
    },
    link: { type: 'object', properties: { speed: { type: 'integer', default: 1000 } } },
    shaping: { type: 'object', default: { rate: 10 }, properties: { rate: { type: 'integer' } } },
    subs: { type: 'object', additionalProperties: { type: 'object', properties: { dhcp: { $ref: '#/$defs/dhcp' } } } },
  },
  required: ['link'],
  $defs: { dhcp: { type: 'object', properties: { on: { type: 'boolean', default: true } } } },
};

describe('presence of optional objects (P08-questions Q2)', () => {
  it('classifies optional objects: not required, plain object with members, no default', () => {
    const p = IFACE.properties!;
    expect(isOptionalObject(p.dhcpClient!, false, IFACE)).toBe(true);
    expect(isOptionalObject(p.dhcpClient!, true, IFACE)).toBe(false); // required
    expect(isOptionalObject(p.shaping!, false, IFACE)).toBe(false); // has a default: present after parse anyway
    expect(isOptionalObject(p.subs!, false, IFACE)).toBe(false); // record
    expect(isOptionalObject(p.mtu!, false, IFACE)).toBe(false);
    expect(isOptionalObject({ oneOf: [{ type: 'object', properties: { a: {} } }] }, false, IFACE)).toBe(false);
    expect(isOptionalObject({ $ref: '#/$defs/dhcp' }, false, IFACE)).toBe(true);
  });

  it('withDefaults keeps its historical fill-in without options (P08 dropPhantomOptionals relies on it)', () => {
    expect(withDefaults(IFACE, { link: {} }, IFACE)).toEqual({
      link: { speed: 1000 },
      mtu: 1500,
      dhcpClient: { setBroadcastFlag: false },
      shaping: { rate: 10 },
    });
    expect(withDefaults(IFACE.properties!.dhcpClient!, undefined, IFACE)).toEqual({ setBroadcastFlag: false });
  });

  it('with { presence: true } an absent optional object stays absent, a present one gets its defaults', () => {
    expect(withDefaults(IFACE, { link: {} }, IFACE, FORM_DEFAULTS)).toEqual({ link: { speed: 1000 }, mtu: 1500, shaping: { rate: 10 } });
    expect(withDefaults(IFACE, { link: {}, dhcpClient: { hostname: 'a' } }, IFACE, FORM_DEFAULTS)).toMatchObject({
      dhcpClient: { hostname: 'a', setBroadcastFlag: false },
    });
    // record values: an optional object inside a record value stays absent too
    expect(withDefaults(IFACE, { link: {}, subs: { x: {} } }, IFACE, FORM_DEFAULTS)).toMatchObject({ subs: { x: {} } });
    // a fresh document (value undefined) and a fresh item
    expect(defaultValueFor(IFACE, IFACE, FORM_DEFAULTS)).toEqual({ mtu: 1500, link: { speed: 1000 }, shaping: { rate: 10 }, subs: {} });
    expect(defaultValueFor(IFACE, IFACE)).toHaveProperty('dhcpClient', { setBroadcastFlag: false });
  });
});

describe('cleared fields (null in the form)', () => {
  it('fromFormValue: null means absent, unless the schema allows null', () => {
    const S2: JsonSchema = {
      type: 'object',
      properties: { mtu: { type: 'integer' }, gw: { anyOf: [{ type: 'string' }, { type: 'null' }] }, note: { type: ['string', 'null'] } },
    };
    expect(fromFormValue(S2, { mtu: null, gw: null, note: null }, S2)).toEqual({ gw: null, note: null });
    expect(acceptsNull(S2.properties!.mtu!, S2)).toBe(false);
    expect(acceptsNull({ enum: ['a', null] }, S2)).toBe(true);
  });
});

describe('RTL-1: identifier values render LTR', () => {
  it('recognises ASCII-only patterns conservatively', () => {
    expect(isAsciiOnlyPattern('^[A-Za-z0-9][A-Za-z0-9_.-]*$')).toBe(true); // objectName
    expect(isAsciiOnlyPattern('^[a-z_][a-z0-9_-]{0,31}$')).toBe(true); // username
    expect(isAsciiOnlyPattern('^[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?(?:\\.[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*$')).toBe(true); // hostname
    expect(isAsciiOnlyPattern('^[\\x21-\\x7e]+$')).toBe(true);
    expect(isAsciiOnlyPattern('^[^\\u0000-\\u0008\\u000b-\\u001f\\u007f-\\u009f]*$')).toBe(false); // printable text: prose
    expect(isAsciiOnlyPattern('^.+$')).toBe(false);
    expect(isAsciiOnlyPattern('^\\S+$')).toBe(false);
    expect(isAsciiOnlyPattern('^\\p{L}+$')).toBe(false);
    expect(isAsciiOnlyPattern('^[a-z\\u00e9]+$')).toBe(false);
    expect(isAsciiOnlyPattern('^سلام$')).toBe(false);
    expect(isAsciiOnlyPattern(undefined)).toBe(false);
  });

  it('isLtrString: identifier widget, identifier format, or ASCII-only pattern', () => {
    expect(isLtrString({ type: 'string' }, 'vrf-picker')).toBe(true);
    expect(isLtrString({ type: 'string' }, 'port-range')).toBe(true);
    expect(isLtrString({ type: 'string', format: 'hostname' }, undefined)).toBe(true);
    expect(isLtrString({ type: 'string', pattern: '^[a-z]+$' }, 'text')).toBe(true);
    expect(isLtrString({ type: 'string', maxLength: 63 }, 'text')).toBe(false); // free text follows the page
    expect(isLtrString({ type: 'string', format: 'idn-hostname' }, undefined)).toBe(false);
  });

  it('a long identifier stays a one-line input; long prose becomes a textarea', () => {
    expect(inferStringWidget({ type: 'string', maxLength: 253, pattern: '^[A-Za-z0-9.-]+$' })).toBe('text'); // hostname
    expect(inferStringWidget({ type: 'string', maxLength: 253, format: 'hostname' })).toBe('text');
    expect(inferStringWidget({ type: 'string', maxLength: 255 })).toBe('textarea');
  });
});

describe('widget value helpers', () => {
  it('ranges split at the first dash and join back (single value when the end is empty)', () => {
    expect(splitRange('8000-8080')).toEqual(['8000', '8080']);
    expect(splitRange('443')).toEqual(['443', '']);
    expect(splitRange('-8080')).toEqual(['', '8080']);
    expect(joinRange('10.0.0.10', '10.0.0.20')).toBe('10.0.0.10-10.0.0.20');
    expect(joinRange('443', '')).toBe('443');
    expect(joinRange('', '')).toBeUndefined();
    for (const v of ['8000-8080', '443', '-8080']) expect(joinRange(...splitRange(v))).toBe(v);
  });

  it('RFC 3339 date-times keep their offset; seconds are added for datetime-local values', () => {
    expect(parseDateTime('2026-09-24T18:00:00+03:30')).toEqual({ local: '2026-09-24T18:00', frac: '', offset: '+03:30' });
    expect(parseDateTime('2026-09-24T18:00:30.25Z')).toEqual({ local: '2026-09-24T18:00:30', frac: '.25', offset: 'Z' });
    const p = parseDateTime('2026-09-24T18:00:00.5+03:30')!; // fractions survive an offset edit (review L3)
    expect(joinDateTime(p.local, '+04:00', p.frac)).toBe('2026-09-24T18:00:00.5+04:00');
    expect(parseDateTime('24/09/2026')).toBeUndefined();
    expect(joinDateTime('2026-09-24T18:00', '+03:30')).toBe('2026-09-24T18:00:00+03:30');
    expect(joinDateTime('2026-09-24T18:00:30', 'Z')).toBe('2026-09-24T18:00:30Z');
    expect(joinDateTime('', '+03:30')).toBeUndefined();
    expect(localOffset(new Date())).toMatch(/^[+-]\d{2}:\d{2}$/);
  });

  it('a new date-time takes the offset of the chosen date, not of today (daylight saving, review L3)', () => {
    const saved = process.env.TZ;
    try {
      process.env.TZ = 'Europe/Berlin';
      expect(defaultOffsetFor('2026-12-01T10:00')).toBe('+01:00');
      expect(defaultOffsetFor('2026-07-01T10:00')).toBe('+02:00');
      process.env.TZ = 'Asia/Tehran';
      expect(defaultOffsetFor('2026-12-01T10:00')).toBe('+03:30');
      expect(defaultOffsetFor('')).toMatch(/^[+-]\d{2}:\d{2}$/); // no date yet: now
    } finally {
      if (saved === undefined) delete process.env.TZ;
      else process.env.TZ = saved;
    }
  });

  it('time zones: UTC first, then the runtime ICU list', () => {
    const zones = timeZoneNames();
    expect(zones[0]).toBe('UTC');
    expect(zones).toContain('Asia/Tehran');
    expect(zones.filter((z) => z === 'UTC')).toHaveLength(1);
  });
});

const RULE: JsonSchema = {
  type: 'object',
  properties: {
    sequence: { type: 'integer', title: 'Sequence' },
    description: { type: 'string', title: 'Description', 'x-vrx-ui': { widget: 'textarea' } },
    action: { type: 'string', enum: ['permit', 'deny'], title: 'Action' },
    source: {
      title: 'Source',
      oneOf: [
        { type: 'object', properties: { kind: { const: 'any' } }, required: ['kind'], additionalProperties: false },
        { type: 'object', properties: { kind: { const: 'prefix' }, prefix: { type: 'string' } }, required: ['kind', 'prefix'], additionalProperties: false },
      ],
    },
    ports: { type: 'array', items: { type: 'integer' }, title: 'Ports' },
    log: { type: 'boolean', title: 'Log' },
    psk: { type: 'string', writeOnly: true, title: 'PSK' },
    nested: { type: 'array', items: { type: 'object', properties: { a: { type: 'string' } } } },
    opts: { type: 'object', additionalProperties: { type: 'string' } },
    name: { type: 'string', title: 'Name' },
  },
};

const plainText: SummaryText = {
  title: (_p, fallback) => fallback,
  enumLabels: (p) => (p === 'rules.action' ? { permit: 'Allow' } : p === 'rules.source' ? undefined : undefined),
  variant: (p, key, fallback) => (p === 'rules.source' && key === 'any' ? 'Anything' : fallback),
  yes: 'Yes',
  no: 'No',
};

describe('summaries and rule-editor columns', () => {
  it('only identifier columns/keys are shown LTR; words follow the page (review M2)', () => {
    expect(isIdentifierSchema({ type: 'string', 'x-vrx-ui': { widget: 'cidr' } }, RULE)).toBe(true);
    expect(isIdentifierSchema({ type: 'string', pattern: '^[a-z_][a-z0-9_-]{0,31}$' }, RULE)).toBe(true); // user name
    expect(isIdentifierSchema({ anyOf: [{ type: 'string', format: 'ipv4' }, { type: 'string', format: 'ipv6' }] }, RULE)).toBe(true);
    expect(isIdentifierSchema({ type: 'array', items: { type: 'string', format: 'cidrv6' } }, RULE)).toBe(true);
    expect(isIdentifierSchema(RULE.properties!.action!, RULE)).toBe(false); // enum → translated words
    expect(isIdentifierSchema(RULE.properties!.source!, RULE)).toBe(false); // union with a variant word
    expect(isIdentifierSchema(RULE.properties!.log!, RULE)).toBe(false);
    expect(isIdentifierSchema({ type: 'string', maxLength: 63 }, RULE)).toBe(false); // free text
  });

  const s = (prop: string, v: unknown) => summarizeValue(RULE.properties![prop]!, v, RULE, `rules.${prop}`, plainText);

  it('summarises enums (translated), unions, lists, booleans, records; never secrets', () => {
    expect(s('action', 'permit')).toBe('Allow');
    expect(s('action', 'deny')).toBe('deny');
    expect(s('source', { kind: 'any' })).toBe('Anything');
    expect(s('source', { kind: 'prefix', prefix: '10.0.0.0/8' })).toBe('prefix 10.0.0.0/8');
    expect(s('ports', [22, 80, 443, 8080, 8443])).toBe('22, 80, 443 +2');
    expect(s('log', true)).toBe('Yes');
    expect(s('log', false)).toBe('No');
    expect(s('psk', 'VRX_TEST_PSK_web1')).toBe('');
    expect(s('opts', { a: 'x', b: 'y' })).toBe('a, b');
    expect(s('name', undefined)).toBe('');
    // inside an object, a true boolean shows its title, a false one nothing
    expect(summarizeValue(RULE, { action: 'deny', log: true, sequence: 5 }, RULE, 'rules', plainText)).toBe('5 deny Log');
  });

  it('table columns: itemKey members first, then column-worthy members in order', () => {
    expect(tableColumns(RULE, {}, RULE).map((c) => c.key)).toEqual(['sequence', 'action', 'source', 'ports', 'log', 'name']);
    expect(tableColumns(RULE, { itemKey: ['name', 'sequence'] }, RULE).map((c) => c.key)).toEqual([
      'name',
      'sequence',
      'action',
      'source',
      'ports',
      'log',
    ]);
    expect(tableColumns({ oneOf: [{ type: 'string' }] }, {}, RULE)).toEqual([{ key: '', schema: { oneOf: [{ type: 'string' }] }, title: '' }]);
  });

  it('siblings sharing a title (shared sub-schema, e.g. ACL source/destination) fall back to their humanized keys', () => {
    const match: JsonSchema = { title: 'Address match', type: 'string' };
    const rule: JsonSchema = { type: 'object', properties: { source: match, destination: match, action: { type: 'string', title: 'Action' }, ipVersion: { type: 'string' } } };
    expect(propertyTitles(sortedProperties(rule, rule))).toEqual({ source: 'Source', destination: 'Destination', action: 'Action', ipVersion: 'Ip Version' });
    expect(tableColumns(rule, {}, rule).map((c) => c.title)).toEqual(['Source', 'Destination', 'Action', 'Ip Version']);
  });
});

describe('per-path texts (I18N-1)', () => {
  const i18n = i18next.createInstance();
  void i18n.init({
    lng: 'fa',
    fallbackLng: 'en',
    initImmediate: false,
    resources: {
      fa: {
        demo: {
          field: {
            role: { title: 'نقش', help: 'راهنما', enum: { admin: 'مدیر', '802.1q': 'دات‌وان‌کیو' } },
            auth: { variant: { psk: 'کلید' } },
            group: { Security: 'امنیت' },
            nested: { group: { Security: 'امنیت تو در تو' }, itemTitle: 'مورد', keyTitle: 'کلید رکورد' },
          },
        },
        common: { legacyHelp: 'راهنمای قدیمی' },
      },
    },
    ns: ['demo', 'common'],
    defaultNS: 'common',
  });

  it('prefix keys win; the schema texts are the fallback; maps keep dotted keys', () => {
    const text = createSchemaText(i18n, 'demo:field');
    expect(text.title('role', 'Role')).toBe('نقش');
    expect(text.title('scope', 'Scope')).toBe('Scope');
    expect(text.title('', 'Root')).toBe('Root');
    expect(text.help('role', 'hint', 'desc')).toBe('راهنما');
    expect(text.help('scope', undefined, 'desc')).toBe('desc');
    expect(text.help('scope', 'legacyHelp', 'desc')).toBe('راهنمای قدیمی'); // historical: a help hint that is a key
    expect(text.enumLabels('role')).toEqual({ admin: 'مدیر', '802.1q': 'دات‌وان‌کیو' });
    expect(text.enumLabels('scope')).toBeUndefined();
    expect(text.variant('auth', 'psk', 'PSK')).toBe('کلید');
    expect(text.variant('auth', 'cert', 'Certificate')).toBe('Certificate');
    expect(text.group('', 'Security')).toBe('امنیت');
    expect(text.group('nested', 'Security')).toBe('امنیت تو در تو');
    expect(text.group('other', 'Security')).toBe('امنیت'); // prefix-wide fallback
    expect(text.group('other', 'Dataplane')).toBe('Dataplane');
    expect(text.itemTitle('nested')).toBe('مورد');
    expect(text.keyTitle('nested')).toBe('کلید رکورد');
    expect(text.title('nested', 'Nested')).toBe('Nested'); // an object key is never returned as a label
  });

  it('without a prefix every text is the schema’s own', () => {
    const text = createSchemaText(i18n, undefined);
    expect(text.title('role', 'Role')).toBe('Role');
    expect(text.enumLabels('role')).toBeUndefined();
    expect(text.group('', 'Security')).toBe('Security');
    expect(text.help('x', 'legacyHelp', undefined)).toBe('راهنمای قدیمی');
  });
});
