import type { JsonSchema } from '@ngfw/ui-kit/schema-form';
import { describe, expect, it } from 'vitest';
import { contrastText, itemSchema, localizeSchema, mergePatch, pickerKinds, sameEntry, summary, type AnyEntry } from './model';
import { pickerSchema } from './ObjectPicker';

describe('object model helpers', () => {
  it('pickerKinds: explicit hint, the kinds the schema help names, the property name, else addresses', () => {
    expect(pickerKinds({ objectKinds: ['zones'] }, 'x')).toEqual(['zones']);
    expect(pickerKinds({ help: 'objects.services or objects.serviceGroups' }, 'name')).toEqual(['services', 'serviceGroups']);
    expect(pickerKinds({ help: 'Names from objects.addresses or objects.addressGroups; may be empty…' }, 'members')).toEqual(['addresses', 'addressGroups']);
    expect(pickerKinds({ help: 'objects.schedules; omit = always' }, 'schedule')).toEqual(['schedules']);
    expect(pickerKinds({}, 'target.zone')).toEqual(['zones']);
    expect(pickerKinds({ help: 'Names of entries in objects.tags' }, 'tags')).toEqual(['tags']);
    expect(pickerKinds({ help: 'anything' }, 'members')).toEqual(['addresses', 'addressGroups']);
  });

  it('item schemas come from the one schema; group members and tags carry the picker widgets', () => {
    const g = itemSchema('addressGroups') as JsonSchema & { properties: Record<string, JsonSchema> };
    expect(g.properties['members']?.['x-vrx-ui']).toMatchObject({ widget: 'object-picker' });
    expect(g.properties['tags']?.['x-vrx-ui']).toMatchObject({ widget: 'tag-picker' });
    expect(itemSchema('addresses').oneOf).toHaveLength(4);
  });

  it('localizeSchema: scoped then shared field keys, variant titles, picker kinds kept from the original help', () => {
    const dict: Record<string, string> = {
      'field.addresses.start.title': 'First address',
      'field.start.title': 'Start',
      'field.members.title': 'Members',
      'field.members.help': 'translated help without the object kinds',
      'variant.host': 'Host',
      'variant.tcp|tcp-udp': 'TCP / TCP and UDP',
    };
    const t = (k: string, o?: Record<string, unknown>) => dict[k] ?? String(o?.['defaultValue'] ?? k);
    const a = localizeSchema(itemSchema('addresses'), t, 'addresses');
    const range = a.oneOf?.find((v) => v.properties?.['type']?.const === 'range');
    expect(range?.properties?.['start']?.title).toBe('First address');
    expect(a.oneOf?.[0]?.title).toBe('Host');
    expect(localizeSchema(itemSchema('schedules'), t, 'schedules').oneOf?.[0]?.properties?.['start']?.title).toBe('Start');
    expect(localizeSchema(itemSchema('services'), t, 'services').oneOf?.[0]?.title).toBe('TCP / TCP and UDP');
    const g = localizeSchema(itemSchema('addressGroups'), t, 'addressGroups');
    expect(g.properties?.['members']?.['x-vrx-ui']).toMatchObject({ help: 'translated help without the object kinds', objectKinds: ['addresses', 'addressGroups'] });
  });

  it('pickerSchema: choices of the allowed kinds with labels; arrays become a multi-select, strings a select', () => {
    const objects = {
      addresses: { web1: { type: 'host', address: '192.0.2.10', tags: [] }, cdn: { type: 'fqdn', fqdn: 'cdn.w3.test', tags: [] } },
      addressGroups: { web: { members: ['web1'], tags: [] } },
      services: { https: { protocol: 'tcp', destinationPorts: ['443'], sourcePorts: [], tags: [] } },
    } as never;
    const bare = (v: unknown) => String(v).replace(/[\u2066-\u2069]/g, '');
    const t = (k: string, o?: Record<string, unknown>) => (k === 'picker.option' ? `${bare(o?.['name'])} (${String(o?.['kind'])}: ${bare(o?.['detail'])})` : k.replace('kindOne.', ''));
    const members = pickerSchema({ schema: { type: 'array', items: { type: 'string' } }, hints: { widget: 'object-picker', dependsOn: 'x' } }, ['addresses', 'addressGroups'], objects, t);
    expect(members.schema.items?.enum).toEqual(['cdn', 'web1', 'web']);
    expect(members.hints).toMatchObject({ widget: 'multiselect', enumLabels: { web1: 'web1 (addresses: 192.0.2.10)', web: 'web (addressGroups: web1)' } });
    expect(members.hints).not.toHaveProperty('dependsOn');
    const one = pickerSchema({ schema: { type: 'string' }, hints: { widget: 'object-picker' } }, ['services'], objects, t);
    expect(one.schema.enum).toEqual(['https']);
    expect(one.hints['widget']).toBe('select');
  });

  it('summary, merge patch, pending comparison, tag text colour', () => {
    expect(summary('addresses', { type: 'range', start: '10.3.2.1', end: '10.3.2.20', tags: [] } as AnyEntry)).toBe('10.3.2.1–10.3.2.20');
    expect(summary('services', { protocol: 'tcp-udp', destinationPorts: ['53'], sourcePorts: [], tags: [] } as AnyEntry)).toBe('tcp-udp/53');
    expect(summary('services', { protocol: 'icmp', type: 8, tags: [] } as AnyEntry)).toBe('icmp type 8');
    expect(summary('schedules', { type: 'recurring', days: ['mon', 'fri'], start: '08:00', end: '18:00', tags: [] } as AnyEntry)).toBe('mon,fri 08:00–18:00');
    expect(mergePatch({ members: ['a'], description: 'x', tags: [] }, { members: ['a', 'b'], tags: [] })).toEqual({ description: null, members: ['a', 'b'] });
    expect(sameEntry({ members: ['a'] }, { members: ['a'] })).toBe(true);
    expect(sameEntry(undefined, { members: [] })).toBe(false);
    expect(contrastText('#ffeb3b')).toBe('#000000');
    expect(contrastText('#1e3a8a')).toBe('#ffffff');
    expect(contrastText(undefined)).toBeUndefined();
  });
});
