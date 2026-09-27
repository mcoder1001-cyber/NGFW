import { jsonPointer } from '../pointer.js';
import type { SemanticIssue, ValidatorDefinition } from './registry.js';

/**
 * F-dashboard-prom-alarms cross-object rules (shapes are in `../domains/ext/dashboard-prom-alarms.ts`):
 *  - a rule's `targets` name existing `management.alarms.targets`;
 *  - a rule with an interface metric restricted to an interface names one that exists.
 * Webhook token references are checked by the secret tier at commit.
 */
const INTERFACE_METRICS = new Set([
  'interface_rx_bps',
  'interface_tx_bps',
  'interface_rx_drops',
  'interface_tx_drops',
  'interface_link_down',
]);

const alarmsRules: ValidatorDefinition = {
  name: 'management.alarms',
  domains: ['management', 'interfaces'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    const alarms = config.management.alarms;
    if (!alarms) return issues;
    const targets = new Set(Object.keys(alarms.targets ?? {}));
    const ifaces = new Set(Object.keys(config.interfaces));
    for (const [name, rule] of Object.entries(alarms.rules ?? {})) {
      rule.targets.forEach((tgt, i) => {
        if (!targets.has(tgt)) {
          issues.push({
            pointer: jsonPointer('management', 'alarms', 'rules', name, 'targets', i),
            message: `notification target '${tgt}' does not exist in management.alarms.targets`,
          });
        }
      });
      if (rule.interface !== undefined && rule.interface !== '' && !ifaces.has(rule.interface)) {
        issues.push({
          pointer: jsonPointer('management', 'alarms', 'rules', name, 'interface'),
          message: `interface '${rule.interface}' does not exist`,
        });
      }
      if (
        rule.interface !== undefined &&
        rule.interface !== '' &&
        !INTERFACE_METRICS.has(rule.metric)
      ) {
        issues.push({
          pointer: jsonPointer('management', 'alarms', 'rules', name, 'interface'),
          message: `metric '${rule.metric}' is not per-interface, so it cannot be limited to an interface`,
        });
      }
    }
    return issues;
  },
};

export const dashboardPromAlarmsValidators: readonly ValidatorDefinition[] = [alarmsRules];
