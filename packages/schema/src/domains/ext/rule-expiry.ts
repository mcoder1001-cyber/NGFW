import { z } from 'zod';
import { withUi } from '../../ui.js';

/**
 * F-rule-expiry: common rule metadata on ACL rules (`acl.lists.<n>.rules[]`), host rules (`acl.host.<n>.rules[]`) and
 * NAT44-ED static mappings / port forwards (`nat.staticMappings[]`). The existing `description` is the comment.
 *
 *   expiresAt  RFC 3339 with offset. At that instant the agent removes the rule from the data plane by itself (no
 *              commit); the configuration keeps it, marked expired, so it can be extended or deleted. A NEW rule (or a
 *              changed expiry) in the past is refused at commit (`rule.expires-in-past`); an unchanged expired rule
 *              stays valid.
 *   owner      who asked for the rule (free text, e.g. a user or team)
 *   ticket     the change/ticket reference (free text, e.g. CHG-1234)
 */

/** One line of printable text: no C0/C1 control characters and no bidi overrides (D-049). */
// eslint-disable-next-line no-control-regex -- matching control characters is the purpose of this pattern
const ONE_PRINTABLE_LINE = /^[^\u0000-\u001f\u007f-\u009f\u202a-\u202e\u2066-\u2069]*$/;

const metaText = (title: string, help: string) =>
  withUi(z.string().min(1).max(64).regex(ONE_PRINTABLE_LINE, 'one line of printable text'), {
    title,
    help,
    group: 'metadata',
  });

export const ruleExpiryFields = {
  expiresAt: withUi(z.iso.datetime({ offset: true }), {
    title: 'Expires at',
    widget: 'datetime',
    help: 'RFC 3339; the rule stops matching at this time without a commit and stays in the configuration marked expired',
    group: 'metadata',
  }).optional(),
  owner: metaText('Owner', 'who asked for this rule').optional(),
  ticket: metaText('Ticket', 'change or ticket reference, e.g. CHG-1234').optional(),
};

/** Whether `expiresAt` (RFC 3339) is at or before `now`; false when unset or unparsable. */
export function ruleExpired(expiresAt: string | undefined, now: Date): boolean {
  if (expiresAt === undefined) return false;
  const t = Date.parse(expiresAt);
  return Number.isFinite(t) && t <= now.getTime();
}
