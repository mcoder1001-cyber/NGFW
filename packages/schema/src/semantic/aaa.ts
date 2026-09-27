import { jsonPointer } from '../pointer.js';
import type { SemanticIssue, ValidatorDefinition } from './registry.js';

/**
 * F-aaa cross-object rules (shapes in `../domains/ext/aaa.ts` and `../domains/management.ts`): an LDAP server must not
 * bind in clear text (ldaps:// or StartTLS), and role-map groups are unique. The "method in order has a backend" rules
 * are schema refines; secret-ref existence is the secret tier's.
 */
const aaaRules: ValidatorDefinition = {
  name: 'management.aaa',
  domains: ['management'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    const aaa = config.management.aaa;
    if (!aaa) return issues;
    (aaa.ldap?.servers ?? []).forEach((srv, i) => {
      if (!srv.url.startsWith('ldaps://') && !srv.startTls) {
        issues.push({
          pointer: jsonPointer('management', 'aaa', 'ldap', 'servers', i, 'url'),
          message: 'clear-text LDAP bind is refused: use ldaps:// or enable StartTLS',
        });
      }
    });
    const seen = new Set<string>();
    (aaa.roleMap ?? []).forEach((m, i) => {
      if (seen.has(m.group)) {
        issues.push({
          pointer: jsonPointer('management', 'aaa', 'roleMap', i, 'group'),
          message: `group '${m.group}' is mapped twice`,
        });
      }
      seen.add(m.group);
    });
    return issues;
  },
};

export const aaaValidators: readonly ValidatorDefinition[] = [aaaRules];
