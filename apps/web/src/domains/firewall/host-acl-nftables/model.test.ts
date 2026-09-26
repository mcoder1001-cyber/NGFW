import { describe, expect, it } from 'vitest';
import {
  attachmentSchema,
  banners,
  countersByRule,
  DEFAULT_SETTINGS,
  deleteListPatch,
  groupDigits,
  listSchema,
  localizeSchema,
  matchText,
  nextSequence,
  putRule,
  ruleKey,
  ruleSchema,
  rulesOf,
  same,
  serviceText,
  settingsOf,
  settingsSchema,
  type HostAclState,
  type HostRule,
} from './model';

const rule = (sequence: number, action: HostRule['action'] = 'accept'): HostRule => ({
  sequence,
  action,
  enabled: true,
  ipVersion: 'any',
  source: { kind: 'any' },
  destination: { kind: 'any' },
  service: { kind: 'any' },
  log: false,
});

const state = (over: Partial<HostAclState> = {}): HostAclState => ({
  table: 'vrx_w9',
  mode: 'netns',
  present: true,
  inSync: true,
  sets: [],
  chains: [],
  rules: [
    {
      list: 'mgmt',
      sequence: 10,
      pointer: '/acl/host/mgmt/rules/0',
      packets: '5',
      bytes: '300',
      nftRules: 2,
    },
  ],
  ...over,
});

describe('host ACL model (F-host-acl-nftables)', () => {
  it('takes its forms from the generated acl schema (lists without rules, rules, attachments, settings)', () => {
    expect(Object.keys(listSchema().properties ?? {})).toEqual(['description', 'tags']);
    expect(Object.keys(ruleSchema().properties ?? {})).toEqual(
      expect.arrayContaining([
        'sequence',
        'action',
        'ipVersion',
        'source',
        'destination',
        'service',
        'interface',
        'log',
        'enabled',
      ]),
    );
    expect(Object.keys(attachmentSchema().properties ?? {})).toEqual([
      'list',
      'chain',
      'priority',
      'enabled',
      'description',
    ]);
    expect(Object.keys(settingsSchema().properties ?? {})).toEqual([
      'defaultInput',
      'allowIcmp',
      'antiLockout',
    ]);
  });

  it('localizes titles and help, scoped keys first, and keeps the picker kinds of object fields', () => {
    const dict: Record<string, string> = {
      'field.antiLockout.enabled.title': 'Anti-lockout rule (scoped)',
      'field.enabled.title': 'Enabled (shared)',
      'field.enabled.help': '',
      'field.name.help': '',
    };
    const t = (k: string, o?: Record<string, unknown>) =>
      dict[k] ?? String(o?.['defaultValue'] ?? k);
    const s = localizeSchema(settingsSchema(), t);
    const lock = s.properties?.['antiLockout'];
    expect(lock?.properties?.['enabled']?.title).toBe('Anti-lockout rule (scoped)');
    const r = localizeSchema(ruleSchema(), t);
    expect(r.properties?.['enabled']?.title).toBe('Enabled (shared)');
    const variants = r.properties?.['source']?.anyOf ?? r.properties?.['source']?.oneOf ?? [];
    const objectVariant = variants.find((v) => v.properties?.['kind']?.const === 'object');
    const hints = objectVariant?.properties?.['name']?.['x-vrx-ui'] as Record<string, unknown>;
    expect(hints['objectKinds']).toEqual(['addresses', 'addressGroups']);
    expect(hints['help']).toBeUndefined();
  });

  it('keeps rules in sequence order and finds the next sequence', () => {
    const rules = [rule(20), rule(10)];
    expect(rulesOf({ rules, tags: [] }).map((x) => [x.rule.sequence, x.index])).toEqual([
      [10, 1],
      [20, 0],
    ]);
    expect(putRule(rules, undefined, rule(15)).map((r) => r.sequence)).toEqual([10, 15, 20]);
    expect(putRule(rules, 0, rule(5, 'drop')).map((r) => [r.sequence, r.action])).toEqual([
      [5, 'drop'],
      [10, 'accept'],
    ]);
    expect(nextSequence(rules)).toBe(30);
    expect(nextSequence([])).toBe(10);
  });

  it('deleting a list also detaches it', () => {
    const acl = {
      host: { a: { rules: [], tags: [] } },
      hostAttachments: [
        { list: 'a', chain: 'input' as const, priority: 0, enabled: true },
        { list: 'b', chain: 'input' as const, priority: 0, enabled: true },
      ],
    };
    expect(deleteListPatch(acl, 'a')).toEqual({
      host: { a: null },
      hostAttachments: [acl.hostAttachments[1]],
    });
    expect(deleteListPatch(acl, 'c')).toEqual({ host: { c: null } });
  });

  it('renders matches and services as language-neutral text', () => {
    expect(matchText(undefined)).toBe('*');
    expect(matchText({ kind: 'prefix', prefix: '10.9.0.0/24' })).toBe('10.9.0.0/24');
    expect(matchText({ kind: 'object', name: 'admins' })).toBe('admins');
    expect(
      serviceText({
        kind: 'inline',
        spec: { protocol: 'tcp', destinationPorts: ['22', '443'], sourcePorts: [] },
      }),
    ).toBe('tcp/22,443');
    expect(serviceText({ kind: 'inline', spec: { protocol: 'icmp', type: 8 } })).toBe(
      'icmp type 8',
    );
    expect(serviceText({ kind: 'object', name: 'ssh' })).toBe('ssh');
  });

  it('joins counters by list and sequence and groups uint64 digits exactly', () => {
    expect(countersByRule(state()).get(ruleKey('mgmt', 10))?.packets).toBe('5');
    expect(countersByRule(undefined).size).toBe(0);
    expect(groupDigits('18446744073709551615')).toBe('18,446,744,073,709,551,615');
    expect(groupDigits('999')).toBe('999');
  });

  it('fills absent settings with the schema defaults', () => {
    expect(settingsOf(undefined)).toEqual(DEFAULT_SETTINGS);
    expect(
      settingsOf({
        hostSettings: {
          defaultInput: 'drop',
          allowIcmp: true,
          antiLockout: { ...DEFAULT_SETTINGS.antiLockout, enabled: false },
        },
      }).antiLockout.enabled,
    ).toBe(false);
  });

  it('banner: info while anti-lockout is on, warning while off, error on drift or a missing table', () => {
    const running = {
      hostAttachments: [{ list: 'mgmt', chain: 'input' as const, priority: 0, enabled: true }],
    };
    expect(banners(DEFAULT_SETTINGS, state(), running).map((b) => [b.severity, b.key])).toEqual([
      ['info', 'banner.enabledAnySourceAnyIface'],
    ]);
    const narrowed = {
      ...DEFAULT_SETTINGS,
      antiLockout: {
        ...DEFAULT_SETTINGS.antiLockout,
        sources: ['10.9.0.0/24'],
        interfaces: ['w9l0'],
      },
    };
    expect(banners(narrowed, state(), running)[0]).toEqual({
      severity: 'info',
      key: 'banner.enabledSourcesIfaces',
      params: { ports: '22, 443', sources: '10.9.0.0/24', interfaces: 'w9l0' },
    });
    const off = {
      ...DEFAULT_SETTINGS,
      antiLockout: { ...DEFAULT_SETTINGS.antiLockout, enabled: false },
    };
    expect(banners(off, state({ inSync: false }), running).map((b) => [b.severity, b.key])).toEqual(
      [
        ['warning', 'banner.disabled'],
        ['error', 'banner.drift'],
      ],
    );
    expect(
      banners(DEFAULT_SETTINGS, state({ present: false }), running).map((b) => b.key),
    ).toContain('banner.missing');
    // check mode never loads the table: not an error
    expect(
      banners(DEFAULT_SETTINGS, state({ present: false, mode: 'check' }), running).map((b) => [
        b.severity,
        b.key,
      ]),
    ).toEqual([
      ['info', 'banner.enabledAnySourceAnyIface'],
      ['info', 'banner.checkMode'],
    ]);
  });

  it('compares configuration regardless of key order', () => {
    expect(same({ a: 1, b: [1, 2] }, { b: [1, 2], a: 1 })).toBe(true);
    expect(same({ a: 1 }, { a: 2 })).toBe(false);
    expect(same(undefined, {})).toBe(false);
  });
});
