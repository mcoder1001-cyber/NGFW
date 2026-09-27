import { describe, expect, it } from 'vitest';
import { RootConfig, type RootConfigInput } from '../index.js';
import { dashboardPromAlarmsValidators } from './dashboard-prom-alarms.js';

const run = (doc: RootConfigInput) =>
  dashboardPromAlarmsValidators[0]!.validate(RootConfig.parse(doc));

describe('F-dashboard-prom-alarms semantic rules', () => {
  it('accepts rules whose targets and interface exist', () => {
    expect(
      run({
        interfaces: { wan0: { enabled: true } },
        management: {
          alarms: {
            targets: { ops: { kind: 'webhook', url: 'https://example.net/hook' } },
            rules: {
              linkdown: {
                metric: 'interface_link_down',
                threshold: 1,
                interface: 'wan0',
                targets: ['ops'],
              },
            },
          },
        },
      }),
    ).toEqual([]);
  });

  it('rejects an unknown target', () => {
    expect(
      run({
        management: {
          alarms: {
            rules: { r: { metric: 'worker_cpu_percent', threshold: 90, targets: ['nope'] } },
          },
        },
      }),
    ).toEqual([
      expect.objectContaining({
        pointer: '/management/alarms/rules/r/targets/0',
        message: expect.stringContaining("'nope' does not exist"),
      }),
    ]);
  });

  it('rejects an interface limit on a non-interface metric and an unknown interface', () => {
    const cpu = run({
      management: {
        alarms: {
          rules: { r: { metric: 'worker_cpu_percent', threshold: 90, interface: 'wan0' } },
        },
      },
    });
    expect(cpu.some((i) => i.message.includes('not per-interface'))).toBe(true);
    const missing = run({
      management: {
        alarms: { rules: { r: { metric: 'interface_rx_bps', threshold: 1, interface: 'ghost0' } } },
      },
    });
    expect(missing.some((i) => i.message.includes("'ghost0' does not exist"))).toBe(true);
  });
});
