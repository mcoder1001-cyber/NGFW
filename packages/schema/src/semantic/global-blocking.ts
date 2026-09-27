import { jsonPointer } from '../pointer.js';
import type { SemanticIssue, ValidatorDefinition } from './registry.js';
import { interfaceIndex } from './tunnels-common.js';

/**
 * F-global-blocking: every interface a block list selects exists (as for ACL attachments); list-internal rules
 * (canonical entries, duplicates, the 200k cap, the interface selection) are refinements of the schema
 * (`../domains/ext/global-blocking.ts`).
 */
const interfacesExist: ValidatorDefinition = {
  name: 'acl.global-blocking-interfaces-exist',
  domains: ['acl', 'interfaces', 'tunnels', 'vpn'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    const lists = config.acl.globalBlocking?.lists ?? {};
    const index = interfaceIndex(config);
    for (const [name, list] of Object.entries(lists)) {
      list.interfaces.forEach((ifName, i) => {
        if (!index.has(ifName)) {
          issues.push({
            pointer: jsonPointer('acl', 'globalBlocking', 'lists', name, 'interfaces', i),
            message: `interface '${ifName}' does not exist`,
          });
        }
      });
    }
    return issues;
  },
};

export const globalBlockingValidators: readonly ValidatorDefinition[] = [interfacesExist];
