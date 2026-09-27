import { jsonPointer } from '../pointer.js';
import type { SemanticIssue, ValidatorDefinition } from './registry.js';
import { ipFamily, parseCidr, parseIp } from './tunnels-common.js';

/**
 * F-bruteforce-block: cross-checks for `security.autoBlock` beyond what the schema refines (unique rule per source,
 * escalation cap ordering). Here: allow-list entries are well-formed and not listed twice (by canonical range), and
 * an enabled auto-block that watches nothing is called out so it is not mistaken for protection.
 */

/** Canonical `first/len` key of an allow-list entry (address → /32 or /128), or undefined when unparsable. */
function canonicalRange(entry: string): string | undefined {
  const withLen = entry.includes('/') ? entry : `${entry}/${entry.includes(':') ? 128 : 32}`;
  const c = parseCidr(withLen);
  if (c === undefined) {
    // a bare address with no prefix
    const ip = parseIp(entry);
    if (ip === undefined) return undefined;
    return `${ipFamily(entry)}:${ip}/${entry.includes(':') ? 128 : 32}`;
  }
  return `${c.family}:${c.first}/${c.prefixLength}`;
}

const autoBlock: ValidatorDefinition = {
  name: 'security.auto-block',
  domains: ['security'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    const ab = config.security.autoBlock;
    if (ab === undefined) return issues;

    const seen = new Map<string, number>();
    ab.allowlist.forEach((entry, i) => {
      const key = canonicalRange(entry);
      if (key === undefined) {
        issues.push({
          pointer: jsonPointer('security', 'autoBlock', 'allowlist', i),
          message: `'${entry}' is not an IP address or prefix`,
        });
        return;
      }
      const first = seen.get(key);
      if (first !== undefined) {
        issues.push({
          pointer: jsonPointer('security', 'autoBlock', 'allowlist', i),
          message: `'${entry}' covers the same range as allow-list entry ${first}`,
        });
      } else {
        seen.set(key, i);
      }
    });

    if (ab.enabled && !ab.rules.some((r) => r.enabled)) {
      issues.push({
        pointer: jsonPointer('security', 'autoBlock', 'enabled'),
        message: 'auto-block is enabled but no detector rule is enabled; nothing would be blocked',
      });
    }

    return issues;
  },
};

export const autoBlockValidators: readonly ValidatorDefinition[] = [autoBlock];
