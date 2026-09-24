import { isPlainObject, jsonPointer } from '@ngfw/schema';

/**
 * Where-used of one name in a configuration document (F-object-model): every leaf that references it, computed from
 * the document alone (no agent). Addresses and address groups share one namespace, and so do services and service
 * groups (D-062); across those families a name may repeat (an address and a service, a zone, a tag), so `definedAs`
 * lists every kind that defines it and each reference below can only mean one of them:
 *
 * | reference                                            | kind                   | refers to                        |
 * |------------------------------------------------------|------------------------|----------------------------------|
 * | objects.addressGroups.<g>.members[i]                 | address-group-member   | address / address group          |
 * | objects.serviceGroups.<g>.members[i]                 | service-group-member   | service / service group          |
 * | objects.<kind>.<n>.tags[i], acl.<kind>.<n>.tags[i]   | tag                    | tag                              |
 * | acl.lists / acl.host  rules[i].source.name           | acl-rule-source        | address / address group          |
 * | … rules[i].destination.name                          | acl-rule-destination   | address / address group          |
 * | … rules[i].service.name                              | acl-rule-service       | service / service group          |
 * | acl.lists.<n>.rules[i].schedule                      | acl-rule-schedule      | schedule                         |
 * | acl.attachments[i].target.zone                       | acl-attachment-zone    | zone                             |
 * | objects.zones.<z>.interfaces[i]                      | zone-interface         | an interface (the name is one)   |
 *
 * NAT does not reference objects in this release (F-nat44-* add their references here when they do).
 */
export const USAGE_KINDS = [
  'address-group-member',
  'service-group-member',
  'tag',
  'acl-rule-source',
  'acl-rule-destination',
  'acl-rule-service',
  'acl-rule-schedule',
  'acl-attachment-zone',
  'zone-interface',
] as const;
export type UsageKind = (typeof USAGE_KINDS)[number];

export const OBJECT_KINDS = [
  'addresses',
  'addressGroups',
  'services',
  'serviceGroups',
  'schedules',
  'zones',
  'tags',
] as const;
export type ObjectKind = (typeof OBJECT_KINDS)[number];

export interface UsageRef {
  /** RFC 6901 pointer of the referencing leaf. */
  pointer: string;
  /** Pointer of the object that holds the reference (the group, the rule, the attachment, the tagged object). */
  container: string;
  domain: 'objects' | 'acl';
  kind: UsageKind;
}

export interface Usage {
  name: string;
  definedAs: ObjectKind[];
  usedBy: UsageRef[];
}

type Json = Record<string, unknown>;

const rec = (v: unknown): Json => (isPlainObject(v) ? v : {});
const arr = (v: unknown): unknown[] => (Array.isArray(v) ? v : []);

/** Where-used of `name` in `doc` (sorted by pointer). */
export function whereUsed(doc: Json, name: string): Usage {
  const objects = rec(doc['objects']);
  const acl = rec(doc['acl']);
  const usedBy: UsageRef[] = [];
  const add = (kind: UsageKind, domain: UsageRef['domain'], container: string, ...leaf: (string | number)[]) =>
    usedBy.push({ kind, domain, container, pointer: container + jsonPointer(...leaf) });

  const definedAs = OBJECT_KINDS.filter((k) => Object.hasOwn(rec(objects[k]), name));

  for (const [kind, member] of [
    ['addressGroups', 'address-group-member'],
    ['serviceGroups', 'service-group-member'],
  ] as const) {
    for (const [g, group] of Object.entries(rec(objects[kind]))) {
      arr(rec(group)['members']).forEach((m, i) => {
        if (m === name) add(member, 'objects', jsonPointer('objects', kind, g), 'members', i);
      });
    }
  }
  for (const kind of OBJECT_KINDS) {
    if (kind === 'tags') continue;
    for (const [n, o] of Object.entries(rec(objects[kind]))) {
      arr(rec(o)['tags']).forEach((t, i) => {
        if (t === name) add('tag', 'objects', jsonPointer('objects', kind, n), 'tags', i);
      });
    }
  }
  for (const [z, zone] of Object.entries(rec(objects['zones']))) {
    arr(rec(zone)['interfaces']).forEach((itf, i) => {
      if (itf === name) add('zone-interface', 'objects', jsonPointer('objects', 'zones', z), 'interfaces', i);
    });
  }

  for (const kind of ['lists', 'macip', 'host'] as const) {
    for (const [l, list] of Object.entries(rec(acl[kind]))) {
      const base = jsonPointer('acl', kind, l);
      arr(rec(list)['tags']).forEach((t, i) => {
        if (t === name) add('tag', 'acl', base, 'tags', i);
      });
      if (kind === 'macip') continue;
      arr(rec(list)['rules']).forEach((r, i) => {
        const rule = rec(r);
        const container = `${base}${jsonPointer('rules', i)}`;
        for (const [side, k] of [
          ['source', 'acl-rule-source'],
          ['destination', 'acl-rule-destination'],
          ['service', 'acl-rule-service'],
        ] as const) {
          const m = rec(rule[side]);
          if (m['kind'] === 'object' && m['name'] === name) add(k, 'acl', container, side, 'name');
        }
        if (rule['schedule'] === name) add('acl-rule-schedule', 'acl', container, 'schedule');
      });
    }
  }
  arr(acl['attachments']).forEach((a, i) => {
    const target = rec(rec(a)['target']);
    if (target['kind'] === 'zone' && target['zone'] === name) {
      add('acl-attachment-zone', 'acl', jsonPointer('acl', 'attachments', i), 'target', 'zone');
    }
  });

  usedBy.sort((a, b) => (a.pointer < b.pointer ? -1 : a.pointer > b.pointer ? 1 : 0));
  return { name, definedAs, usedBy };
}
