import { describe, expect, it } from 'vitest';
import { RootConfig } from '../index.js';
import { SEMANTIC_VALIDATORS, validateSemantics } from './index.js';
import { qosFlatValidators } from './qos-flat.js';
import { BASE } from './vpn.fixtures.js';

const IF0 = 'TenGigabitEthernet0/0/0';
const IF1 = 'TenGigabitEthernet0/0/1';

const doc = (interfaces: Record<string, unknown>, extra: Record<string, unknown> = {}) => ({
  ...BASE,
  services: {
    qos: {
      policers: { gold: { cir: 10000, cb: 12500 } },
      shapers: { wan: { rateKbps: 50000 } },
      maps: { remark: { rows: { ip: [{ from: 46, to: 34 }] } } },
      interfaces,
      ...extra,
    },
  },
});

const qosIssues = (d: unknown) =>
  validateSemantics(RootConfig.parse(d)).filter((i) => i.pointer.startsWith('/services/qos'));

describe('F-qos-flat semantic rules', () => {
  it('are registered once, under services.qos-flat-*', () => {
    for (const v of qosFlatValidators) {
      expect(v.name.startsWith('services.qos-flat-')).toBe(true);
      expect(v.domains).toContain('services');
      expect(SEMANTIC_VALIDATORS.filter((x) => x.name === v.name)).toHaveLength(1);
    }
  });

  it('a complete flat QoS configuration is clean', () => {
    expect(
      qosIssues(
        doc({
          [IF0]: { policer: { input: 'gold' }, shaper: 'wan', record: 'ip', mark: { map: 'remark', output: 'ip' } },
          [IF1]: { policer: { output: 'gold' }, store: { source: 'ip', value: 46 } },
        }),
      ),
    ).toEqual([]);
  });

  it.each(['vlan', 'mpls', 'ext'])('store.source %s → services.qos-flat-store-source at store/source', (source) => {
    expect(qosIssues(doc({ [IF1]: { store: { source, value: 3 } } }))).toEqual([
      {
        pointer: '/services/qos/interfaces/TenGigabitEthernet0~10~11/store/source',
        message: `VPP 26.06 stores a QoS value for the ip source only (qos store ${source} is not implemented); use record for ${source}`,
      },
    ]);
  });
});

/**
 * The two other F-qos-flat rules of the prompt are enforced by the schema tier (domains/services.ts), which runs first:
 * a semantic rule would never see these documents. Pinned here so a schema change that drops them fails loudly.
 */
describe('F-qos-flat rules enforced by the schema tier', () => {
  const issues = (d: unknown) =>
    RootConfig.safeParse(d).error?.issues.map((i) => '/' + i.path.join('/')) ?? [];

  it('mark requires map', () => {
    expect(issues(doc({ [IF0]: { mark: { output: 'ip' } } }))).toEqual([
      `/services/qos/interfaces/${IF0}/mark/map`,
    ]);
  });

  it('shaper and policer.output are exclusive (one egress policer per interface)', () => {
    expect(issues(doc({ [IF0]: { shaper: 'wan', policer: { output: 'gold' } } }))).toEqual([
      `/services/qos/interfaces/${IF0}/shaper`,
    ]);
  });
});
