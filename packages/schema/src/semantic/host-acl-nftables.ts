import { prefixKey } from '../ip.js';
import { jsonPointer } from '../pointer.js';
import { duplicates } from './nat.js';
import type { SemanticIssue, ValidatorDefinition } from './registry.js';

/**
 * Semantic validators of `acl.hostSettings` (F-host-acl-nftables, tier b). Pure functions of the schema-valid
 * document, `{ pointer, message }[]`.
 *
 *   acl.host-settings   the same prefix, interface or port twice in `antiLockout.sources` / `.interfaces` / `.ports`
 *                       (prefixes compare in canonical form: 10.0.0.1/24 = 10.0.0.0/24)
 *
 * Whether the rules would drop management traffic when the anti-lockout rule is off (rule acl.host-anti-lockout)
 * needs the expanded address/service objects and the FQDN answers, so the agent decides it in its DryRun (tier c).
 */
export const hostAclNftablesValidators: readonly ValidatorDefinition[] = [
  {
    name: 'acl.host-settings',
    domains: ['acl'],
    validate: ({ acl }) => {
      const issues: SemanticIssue[] = [];
      const lockout = acl.hostSettings?.antiLockout;
      if (lockout === undefined) return issues;
      const at = (...s: (string | number)[]): string =>
        jsonPointer('acl', 'hostSettings', 'antiLockout', ...s);
      for (const { index, first } of duplicates(lockout.sources, prefixKey)) {
        issues.push({
          pointer: at('sources', index),
          message: `the same prefix as sources[${first}]`,
        });
      }
      for (const { index, first } of duplicates(lockout.interfaces, (i) => i)) {
        issues.push({
          pointer: at('interfaces', index),
          message: `the same interface as interfaces[${first}]`,
        });
      }
      for (const { index, first } of duplicates(lockout.ports, (p) => String(p))) {
        issues.push({ pointer: at('ports', index), message: `the same port as ports[${first}]` });
      }
      return issues;
    },
  },
];
