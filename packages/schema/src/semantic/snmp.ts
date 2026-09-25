import { jsonPointer } from '../pointer.js';
import type { SemanticIssue, ValidatorDefinition } from './registry.js';

/**
 * F-snmp semantic rules for `services.snmp` (the P02c rules already cover: VRF exists, listen addresses exist in
 * the VRF, trap receivers name an existing community / user, authPriv needs both refs):
 * - an enabled agent needs at least one community or SNMPv3 user;
 * - a community / user `view` names a view in `services.snmp.views`.
 */

const P = (...segments: (string | number)[]): string =>
  jsonPointer('services', 'snmp', ...segments);

const credentialsWhenEnabled: ValidatorDefinition = {
  name: 'services.snmp.credentials',
  domains: ['services'],
  validate(config) {
    const s = config.services.snmp;
    if (!s.enabled) return [];
    if (Object.keys(s.communities).length + Object.keys(s.v3Users).length > 0) return [];
    return [
      {
        pointer: P('enabled'),
        message: 'an enabled SNMP agent needs at least one community or SNMPv3 user',
      },
    ];
  },
};

const viewReferences: ValidatorDefinition = {
  name: 'services.snmp.view-exists',
  domains: ['services'],
  validate(config) {
    const s = config.services.snmp;
    const views = new Set(Object.keys(s.views ?? {}));
    const issues: SemanticIssue[] = [];
    for (const [name, c] of Object.entries(s.communities)) {
      if (c.view !== undefined && !views.has(c.view))
        issues.push({
          pointer: P('communities', name, 'view'),
          message: `view '${c.view}' is not defined`,
        });
    }
    for (const [name, u] of Object.entries(s.v3Users)) {
      if (u.view !== undefined && !views.has(u.view))
        issues.push({
          pointer: P('v3Users', name, 'view'),
          message: `view '${u.view}' is not defined`,
        });
    }
    return issues;
  },
};

export const snmpValidators: readonly ValidatorDefinition[] = [
  credentialsWhenEnabled,
  viewReferences,
];
