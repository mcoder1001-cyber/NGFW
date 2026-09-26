import { createFormatters } from '@ngfw/ui-kit';
import type { JsonSchema } from '@ngfw/ui-kit/schema-form';
import { describe, expect, it } from 'vitest';
import { ApiError } from '../../../api-problem';
import {
  addressText,
  attachmentSchema,
  formatBytes,
  interfaceChoices,
  isAclTab,
  listMetaSchema,
  localizeSchema,
  mergePatch,
  nextSequence,
  planDrop,
  problemFor,
  ruleCounters,
  ruleFormSchema,
  ruleSchema,
  rulesQuery,
  samePlain,
  scaleBytes,
  selectedIds,
  serviceText,
  targetText,
} from './model';
import { pointerUrl } from './queries';

const t = (k: string, o?: Record<string, unknown>) =>
  k === 'unit.bytes'
    ? `${String(o?.['value'])} ${String(o?.['unit'])}`
    : k === 'target.zone'
      ? `zone ${String(o?.['zone'])}`
      : k.replace('unit.', '');

describe('ACL rule editor helpers', () => {
  it('planDrop: a free sequence before the target → PATCH the dragged rule; no room → move to the target sequence', () => {
    const dragged = { index: 7, sequence: 50 };
    // first rule of the list (predecessor 0): 10 − 0 ≥ 2 → the midpoint 5
    expect(planDrop(dragged, { sequence: 10 }, 0, true)).toEqual({
      kind: 'patch',
      index: 7,
      sequence: 5,
    });
    expect(planDrop(dragged, { sequence: 30 }, 20, true)).toEqual({
      kind: 'patch',
      index: 7,
      sequence: 25,
    });
    // neighbours 20 and 21: no free sequence → bulk move to 21 (21 and the rules after it shift up)
    expect(planDrop(dragged, { sequence: 21 }, 20, true)).toEqual({
      kind: 'move',
      sequences: [50],
      to: 21,
    });
    // unknown predecessor (first row of a later page) or a filtered page (rows are not neighbours) → move
    expect(planDrop(dragged, { sequence: 30 }, null, true)).toEqual({
      kind: 'move',
      sequences: [50],
      to: 30,
    });
    expect(planDrop(dragged, { sequence: 30 }, 20, false)).toEqual({
      kind: 'move',
      sequences: [50],
      to: 30,
    });
    // dropped on itself, or already directly before the target → nothing to do
    expect(planDrop(dragged, { sequence: 50 }, 40, true)).toEqual({ kind: 'none' });
    expect(planDrop(dragged, { sequence: 60 }, 50, true)).toEqual({ kind: 'none' });
  });

  it('rulesQuery: 1-based page, the quick filter as one `filter`, hitsOnly only when on, a page of at most 1000', () => {
    const req = { page: 0, pageSize: 100, quickFilter: [] as string[] };
    expect(rulesQuery(req, 'candidate', false)).toEqual({
      page: 1,
      pageSize: 100,
      source: 'candidate',
    });
    expect(
      rulesQuery({ page: 4, pageSize: 5000, quickFilter: ['10.3.1.0/24', 'ssh'] }, 'running', true),
    ).toEqual({
      page: 5,
      pageSize: 1000,
      source: 'running',
      filter: '10.3.1.0/24 ssh',
      hitsOnly: true,
    });
    expect(rulesQuery({ ...req, quickFilter: ['  '] }, 'candidate', false)).not.toHaveProperty(
      'filter',
    );
  });

  it('selection: include ids as they are; "all but" resolved against the rows of the page', () => {
    expect(selectedIds({ type: 'include', ids: new Set([3, 9]) }, [1, 2, 3])).toEqual([3, 9]);
    expect(selectedIds({ type: 'exclude', ids: new Set([2]) }, [1, 2, 3])).toEqual([1, 3]);
  });

  it('nextSequence: the next multiple of 10 after the last rule', () => {
    expect(nextSequence(undefined)).toBe(10);
    expect(nextSequence(10)).toBe(20);
    expect(nextSequence(25)).toBe(30);
    expect(nextSequence(2_147_483_640)).toBe(2_147_483_647);
  });

  it('counters: "—" (null) when counters are unavailable or the rule is not in VPP', () => {
    const live = { status: 'applied' as const, vppRules: 2, packets: 12, bytes: 3400 };
    expect(ruleCounters(live, true)).toEqual({ packets: 12, bytes: 3400 });
    expect(ruleCounters(live, false)).toBeNull();
    expect(ruleCounters(null, true)).toBeNull();
  });

  it('bytes are human-readable and localised (Persian digits when asked for)', () => {
    expect(scaleBytes(512)).toEqual({ value: 512, unit: 'B' });
    expect(scaleBytes(1536)).toEqual({ value: 1.5, unit: 'KiB' });
    expect(scaleBytes(5 * 1024 ** 3).unit).toBe('GiB');
    expect(formatBytes(1536, createFormatters({ locale: 'en' }), t)).toBe('1.5 KiB');
    expect(formatBytes(123_456_789, createFormatters({ locale: 'en' }), t)).toBe('118 MiB');
    expect(formatBytes(1536, createFormatters({ locale: 'fa', persianDigits: true }), t)).toBe(
      '۱٫۵ KiB',
    );
  });

  it('match texts: any → null (translated by the caller), prefixes, objects and inline services', () => {
    expect(addressText({ kind: 'any' })).toBeNull();
    expect(addressText({ kind: 'prefix', prefix: '2001:db8::/32' })).toBe('2001:db8::/32');
    expect(addressText({ kind: 'object', name: 'web-servers' })).toBe('web-servers');
    expect(
      serviceText({
        kind: 'inline',
        spec: { protocol: 'tcp', destinationPorts: ['22', '443'], sourcePorts: [] },
      }),
    ).toBe('tcp/22,443');
    expect(serviceText({ kind: 'inline', spec: { protocol: 'icmp', type: 8 } })).toBe(
      'icmp type 8',
    );
    expect(serviceText({ kind: 'inline', spec: { protocol: 'any' } })).toBeNull();
    expect(serviceText({ kind: 'object', name: 'https' })).toBe('https');
    expect(targetText({ kind: 'zone', zone: 'lan' }, t)).toBe('zone lan');
    expect(targetText({ kind: 'interface', interface: 'host-w3l0' }, t)).toBe('host-w3l0');
  });

  it('schemas come from the one schema: the rule form keeps the object pickers; a list edit has no rules; attachments pick a list', () => {
    const rule = ruleSchema() as JsonSchema & { properties: Record<string, JsonSchema> };
    expect(Object.keys(rule.properties)).toEqual(
      expect.arrayContaining([
        'sequence',
        'action',
        'source',
        'destination',
        'service',
        'schedule',
        'log',
      ]),
    );
    const objectVariant = rule.properties['source']?.oneOf?.find(
      (v) => v.properties?.['kind']?.const === 'object',
    );
    expect(objectVariant?.properties?.['name']?.['x-vrx-ui']).toMatchObject({
      widget: 'object-picker',
    });
    expect(rule.properties['schedule']?.['x-vrx-ui']).toMatchObject({ widget: 'object-picker' });
    expect(Object.keys(listMetaSchema().properties ?? {})).toEqual(['description', 'tags']);
    const att = attachmentSchema(['web-in', 'mgmt']).properties?.['list'];
    expect(att?.enum).toEqual(['web-in', 'mgmt']);
    expect(att?.['x-vrx-ui']).toMatchObject({ widget: 'select' });
  });

  it('an optional object with required members (tcpFlags) is edited as JSON text: SchemaForm would fill it when absent', () => {
    const form = ruleFormSchema();
    const inline = form.properties?.['service']?.oneOf?.find(
      (v) => v.properties?.['kind']?.const === 'inline',
    );
    const tcp = inline?.properties?.['spec']?.oneOf?.find((v) => v.properties?.['tcpFlags']);
    expect(tcp?.properties?.['tcpFlags']?.['x-vrx-ui']).toMatchObject({ widget: 'json' });
    // required objects and unions keep their own editors
    expect(form.properties?.['source']?.['x-vrx-ui']).not.toMatchObject({ widget: 'json' });
    expect(inline?.properties?.['spec']?.['x-vrx-ui']).not.toMatchObject({ widget: 'json' });
  });

  it('localizeSchema: titles, help and enum labels from the acl namespace; picker kinds survive a translated help', () => {
    const dict: Record<string, string> = {
      'field.action.title': 'کنش',
      'enum.action.permit': 'اجازه',
      'field.name.help': 'یک شیء از صفحهٔ اشیا',
      'variant.object': 'شیء',
    };
    const tr = (k: string, o?: Record<string, unknown>) =>
      dict[k] ?? String(o?.['defaultValue'] ?? k);
    const s = localizeSchema(ruleSchema(), tr);
    expect(s.properties?.['action']?.title).toBe('کنش');
    expect(s.properties?.['action']?.['x-vrx-ui']).toMatchObject({
      enumLabels: { permit: 'اجازه', deny: 'deny' },
    });
    const obj = s.properties?.['destination']?.oneOf?.find((v) => v.title === 'شیء');
    expect(obj?.properties?.['name']?.['x-vrx-ui']).toMatchObject({
      help: 'یک شیء از صفحهٔ اشیا',
      objectKinds: ['addresses', 'addressGroups'],
    });
    const svc = s.properties?.['service']?.oneOf?.find(
      (v) => v.properties?.['kind']?.const === 'object',
    );
    expect(svc?.properties?.['name']?.['x-vrx-ui']).toMatchObject({
      objectKinds: ['services', 'serviceGroups'],
    });
  });

  it('server problems are mapped onto the edited node; pointers encode one segment each', () => {
    const e = new ApiError(
      400,
      {
        title: 'Validation failed',
        errors: [{ pointer: '/acl/lists/web-in/rules/3/destination/name', message: 'empty group' }],
      },
      false,
    );
    expect(problemFor(e, '/acl/lists/web-in/rules/3')?.errors).toEqual([
      { pointer: '/destination/name', detail: 'empty group' },
    ]);
    expect(problemFor(new Error('x'), '/acl')).toBeNull();
    expect(pointerUrl('/api/v1/config/{path}', ['acl', 'lists', 'web-in', 'rules', 3])).toBe(
      '/api/v1/config/acl/lists/web-in/rules/3',
    );
    expect(pointerUrl('/api/v1/config/candidate/{path}', ['acl', 'lists', 'a b'])).toBe(
      '/api/v1/config/candidate/acl/lists/a%20b',
    );
  });

  it('merge patch, entry comparison, interface choices, tabs', () => {
    expect(mergePatch({ description: 'x', tags: ['a'] }, { tags: ['a', 'b'] })).toEqual({
      description: null,
      tags: ['a', 'b'],
    });
    expect(
      samePlain(
        { list: 'a', target: { kind: 'zone', zone: 'lan' } },
        { target: { zone: 'lan', kind: 'zone' }, list: 'a' },
      ),
    ).toBe(true);
    expect(samePlain({ list: 'a', enabled: true }, { list: 'a' })).toBe(false);
    expect(
      interfaceChoices({ 'host-w3l0': { subinterfaces: { '100': {} } }, 'host-w3w0': {} }, [
        'loop0',
      ]),
    ).toEqual(['host-w3l0', 'host-w3l0.100', 'host-w3w0', 'loop0']);
    expect(isAclTab('macip')).toBe(true);
    expect(isAclTab('adl')).toBe(false);
  });
});
