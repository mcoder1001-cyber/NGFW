import { describe, expect, it } from 'vitest';
import { exportDocument, mergeDocument } from './document.js';
import { signature, validSignature } from './transport.js';

describe('cluster document boundary', () => {
  it('omits identity and credential subtrees and restores receiver values on replacement', () => {
    const incoming = {
      system: { hostname: 'node-a', timezone: 'UTC' },
      management: { users: [{ username: 'source' }] },
      ha: { cluster: { nodeName: 'node-a' }, vrrp: { a: { priority: 200 } } },
      interfaces: { lan: { ipv4: ['192.0.2.1/24'] } },
      dataplane: { managementPci: ['a'] },
    };
    const local = {
      system: { hostname: 'node-b', timezone: 'Asia/Tehran' },
      management: { users: [{ username: 'local' }] },
      ha: { cluster: { nodeName: 'node-b' } },
      interfaces: { lan: { ipv4: ['192.0.2.2/24'] } },
      dataplane: { managementPci: ['b'] },
    };
    const source = structuredClone(incoming);
    const excluded = ['/interfaces/lan/ipv4'];
    const exported = exportDocument(incoming, excluded);
    expect(exported).not.toHaveProperty('management');
    expect(exported).not.toHaveProperty('ha.cluster');
    expect(exported).not.toHaveProperty('dataplane');
    const merged = mergeDocument(exported, local, excluded);
    expect(merged).toMatchObject({
      system: { hostname: 'node-b', timezone: 'UTC' },
      management: local.management,
      ha: { cluster: local.ha.cluster },
      interfaces: local.interfaces,
      dataplane: local.dataplane,
    });
    expect(incoming).toEqual(source);
  });
  it('does not accept peer-supplied node-local fields even if peer omits exclusions', () => {
    expect(
      mergeDocument(
        {
          management: { users: [{ username: 'attack' }] },
          ha: { cluster: { nodeName: 'attack' } },
          system: { hostname: 'attack' },
        },
        { system: { hostname: 'b' } },
        [],
      ),
    ).toEqual({ ha: {}, system: { hostname: 'b' } });
  });
  it('rejects decoded prototype pollution pointers', () => {
    expect(() => mergeDocument({}, {}, ['/constructor/prototype/evil'])).toThrow('unsafe');
    expect(() => exportDocument({}, ['/__proto__/evil'])).toThrow('unsafe');
    expect({}).not.toHaveProperty('evil');
  });
  it('authenticates exact envelope bytes and refuses wrong keys, altered payload and malformed signatures', () => {
    const key = 'NGFW_TEST_PSK_cluster_auth_fixture';
    const body = JSON.stringify({ revision: 4, document: {} }),
      mac = signature(body, key);
    expect(validSignature(body, key, mac)).toBe(true);
    expect(validSignature(body + ' ', key, mac)).toBe(false);
    expect(validSignature(body, key + 'x', mac)).toBe(false);
    expect(validSignature(body, key, 'garbage')).toBe(false);
  });
});
