import { describe, expect, it } from 'vitest';
import { RootConfig } from '../index.js';
import { validateSemantics } from './index.js';

import { snmpFull, snmpUnknownView } from './snmp.fixtures.js';

describe('F-snmp semantic rules', () => {
  it('snmpFull (D-086 stand-ins typed) is schema-valid and semantically clean', () => {
    const cfg = RootConfig.parse(snmpFull);
    expect(cfg.services.snmp.views?.mgmt?.include).toContain('system');
    expect(cfg.services.snmp.monitors?.disks[0]?.minPercent).toBe(10);
    expect(validateSemantics(cfg)).toEqual([]);
  });

  it('an unknown view is reported with a pointer', () => {
    const pointers = validateSemantics(RootConfig.parse(snmpUnknownView)).map((i) => i.pointer);
    expect(pointers).toEqual(['/services/snmp/communities/monitoring/view']);
  });

  it('a trap receiver naming an unknown community fails the schema at its path', () => {
    const doc = structuredClone(snmpFull);
    doc.services.snmp.trapReceivers[1]!.community = 'nosuch';
    const r = RootConfig.safeParse(doc);
    expect(r.success).toBe(false);
    expect(r.error?.issues.map((i) => i.path.join('/'))).toContain(
      'services/snmp/trapReceivers/1/community',
    );
  });

  it('an enabled agent without a community or user is rejected', () => {
    const cfg = RootConfig.parse({ services: { snmp: { enabled: true } } });
    expect(validateSemantics(cfg)).toContainEqual(
      expect.objectContaining({ pointer: '/services/snmp/enabled' }),
    );
    expect(validateSemantics(RootConfig.parse({ services: { snmp: {} } }))).toEqual([]);
  });

  it('schema rejects a bad OID, the reserved view name and an unclean disk path', () => {
    const bad = (snmp: unknown): boolean => !RootConfig.safeParse({ services: { snmp } }).success;
    expect(bad({ views: { v: { include: ['1.3.x'] } } })).toBe(true);
    expect(bad({ views: { v: { include: [] } } })).toBe(true);
    expect(bad({ communities: { c: { secretRef: 'password/c', view: 'vrx_all' } } })).toBe(true);
    expect(bad({ monitors: { disks: [{ path: '/var/../etc' }] } })).toBe(true);
    expect(bad({ sysServices: 128 })).toBe(true);
    expect(bad({ monitors: { load: { max1: 1, max5: 1 } } })).toBe(true);
  });
});
