import { describe, expect, it } from 'vitest';
import { whereUsed } from './usage.js';

const doc = {
  objects: {
    tags: { prod: { color: '#1e88e5' } },
    addresses: {
      web1: { type: 'host', address: '192.0.2.10', tags: ['prod'] },
      web2: { type: 'host', address: '192.0.2.11', tags: [] },
      // the same name in two kinds: names are unique within their kind only
      web: { type: 'fqdn', fqdn: 'www.example.com', tags: [] },
    },
    addressGroups: {
      'web-servers': { members: ['web1', 'web2'], tags: ['prod'] },
      dmz: { members: ['web-servers', 'web1'], tags: [] },
    },
    services: { web: { protocol: 'tcp', destinationPorts: ['80', '443'], sourcePorts: [], tags: [] } },
    serviceGroups: { all: { members: ['web'], tags: [] } },
    schedules: { 'office-hours': { type: 'recurring', days: ['mon'], start: '08:00', end: '18:00', tags: [] } },
    zones: { lan: { interfaces: ['host-w3l0', 'host-w3l0.100'], tags: [] } },
  },
  acl: {
    lists: {
      'web-in': {
        tags: ['prod'],
        rules: [
          {
            sequence: 10,
            action: 'permit',
            source: { kind: 'any' },
            destination: { kind: 'object', name: 'web-servers' },
            service: { kind: 'object', name: 'web' },
            schedule: 'office-hours',
          },
          {
            sequence: 20,
            action: 'permit',
            source: { kind: 'object', name: 'web1' },
            destination: { kind: 'prefix', prefix: '10.3.0.0/16' },
            service: { kind: 'inline', spec: { protocol: 'icmp' } },
          },
        ],
      },
    },
    host: {
      mgmt: {
        tags: [],
        rules: [{ sequence: 1, action: 'accept', source: { kind: 'object', name: 'web1' }, destination: { kind: 'any' }, service: { kind: 'any' } }],
      },
    },
    macip: { m: { tags: ['prod'], rules: [] } },
    attachments: [
      { list: 'web-in', target: { kind: 'zone', zone: 'lan' }, direction: 'in', sequence: 1 },
      { list: 'web-in', target: { kind: 'interface', interface: 'lan' }, direction: 'out', sequence: 1 },
    ],
  },
};

describe('whereUsed', () => {
  it('finds group members, ACL rule sides and host rules for an address', () => {
    expect(whereUsed(doc, 'web1')).toEqual({
      name: 'web1',
      definedAs: ['addresses'],
      usedBy: [
        { pointer: '/acl/host/mgmt/rules/0/source/name', container: '/acl/host/mgmt/rules/0', domain: 'acl', kind: 'acl-rule-source' },
        { pointer: '/acl/lists/web-in/rules/1/source/name', container: '/acl/lists/web-in/rules/1', domain: 'acl', kind: 'acl-rule-source' },
        { pointer: '/objects/addressGroups/dmz/members/1', container: '/objects/addressGroups/dmz', domain: 'objects', kind: 'address-group-member' },
        { pointer: '/objects/addressGroups/web-servers/members/0', container: '/objects/addressGroups/web-servers', domain: 'objects', kind: 'address-group-member' },
      ],
    });
  });

  it('reports a name defined in two kinds and every reference to either', () => {
    const u = whereUsed(doc, 'web');
    expect(u.definedAs).toEqual(['addresses', 'services']);
    expect(u.usedBy.map((r) => `${r.kind} ${r.pointer}`)).toEqual([
      'acl-rule-service /acl/lists/web-in/rules/0/service/name',
      'service-group-member /objects/serviceGroups/all/members/0',
    ]);
  });

  it('finds tags on objects and lists, schedules, zones and zone members', () => {
    expect(whereUsed(doc, 'prod').usedBy.map((r) => r.pointer)).toEqual([
      '/acl/lists/web-in/tags/0',
      '/acl/macip/m/tags/0',
      '/objects/addressGroups/web-servers/tags/0',
      '/objects/addresses/web1/tags/0',
    ]);
    expect(whereUsed(doc, 'office-hours').usedBy).toEqual([
      { pointer: '/acl/lists/web-in/rules/0/schedule', container: '/acl/lists/web-in/rules/0', domain: 'acl', kind: 'acl-rule-schedule' },
    ]);
    // the zone `lan` is attached; the interface named `lan` in attachment 1 is not a zone reference
    expect(whereUsed(doc, 'lan')).toMatchObject({
      definedAs: ['zones'],
      usedBy: [{ pointer: '/acl/attachments/0/target/zone', kind: 'acl-attachment-zone' }],
    });
    expect(whereUsed(doc, 'host-w3l0.100').usedBy).toEqual([
      { pointer: '/objects/zones/lan/interfaces/1', container: '/objects/zones/lan', domain: 'objects', kind: 'zone-interface' },
    ]);
  });

  it('is empty for an unused or unknown name and tolerates missing domains; prototype names are plain names', () => {
    expect(whereUsed(doc, 'web2').usedBy).toHaveLength(1);
    expect(whereUsed(doc, 'nosuch')).toEqual({ name: 'nosuch', definedAs: [], usedBy: [] });
    expect(whereUsed({}, 'x')).toEqual({ name: 'x', definedAs: [], usedBy: [] });
    expect(whereUsed(doc, 'constructor')).toEqual({ name: 'constructor', definedAs: [], usedBy: [] });
  });
});
