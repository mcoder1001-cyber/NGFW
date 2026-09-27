import { z } from 'zod';
import { objectName, secretRefOf, vppInterfaceName } from '../../primitives.js';
import { withUi } from '../../ui.js';
import { canonicalEntry } from './global-blocking-parse.js';

/**
 * F-global-blocking: `acl.globalBlocking` — IP block lists (IPv4/IPv6 addresses and prefixes) enforced ahead of the
 * user ACLs on chosen interfaces (VPP acl plugin) and, with `protectHost`, on traffic to the box itself (nftables).
 *
 *   lists.<name>   { enabled, description, source, interfaces | allInterfaces, direction, protectHost, log, entries[] }
 *   source         upload (entries edited/imported through the API) or url (the API downloads the list; refreshSec
 *                  re-downloads it on a schedule and applies the diff as a system change; the last good list is kept on
 *                  any failure). Credentials and a private CA are secret references, never inline.
 *   entries        canonical prefixes (an address is /32 or /128), deduplicated and collapsed by the API's import;
 *                  at most MAX_BLOCK_ENTRIES over all lists. Action is always drop.
 *
 * Decision (F-global-blocking questions Q1): under `acl` (same agent family and attachment semantics) instead of a new
 * root domain.
 */

/** Total entries over every list (the product owner's 200k). */
export const MAX_BLOCK_ENTRIES = 200_000;

/** One entry: an IPv4/IPv6 prefix in canonical form (addresses are imported as /32 or /128). */
const blockEntry = z
  .string()
  .max(49)
  .regex(/^[0-9a-fA-F:.]+\/\d{1,3}$/, 'expected a prefix such as 192.0.2.7/32 or 2001:db8::/48');

// eslint-disable-next-line no-control-regex -- refusing control characters in a URL is the purpose of this pattern
const HTTP_URL = /^https?:\/\/[^\s\u0000-\u001f\u007f]+$/;

const urlSource = z.strictObject({
  kind: z.literal('url'),
  url: withUi(
    z
      .string()
      .max(2048)
      .regex(HTTP_URL, 'expected an http(s) URL'),
    { title: 'Server URL', help: 'https by default; the box downloads the list from here' },
  ),
  refreshSec: withUi(z.number().int().min(300).max(7 * 86_400), {
    title: 'Refresh (seconds)',
    help: 'download and apply the list on this schedule; omit = only when you press Fetch now',
  }).optional(),
  verifyTls: withUi(z.boolean().default(true), {
    title: 'Verify the server certificate',
  }),
  caRef: withUi(secretRefOf('cert'), { title: 'Private CA', help: 'cert/<name> to verify the server with' }).optional(),
  authRef: withUi(secretRefOf(['token', 'password']), {
    title: 'Credentials',
    help: 'token/<name> (Authorization: Bearer) or password/<name> (user:pass for basic auth)',
  }).optional(),
});

export const GlobalBlockingListSchema = withUi(
  z.strictObject({
    enabled: withUi(z.boolean().default(true), { title: 'Enabled' }),
    description: withUi(z.string().max(255), { title: 'Description', widget: 'textarea' }).optional(),
    source: withUi(
      z.discriminatedUnion('kind', [z.strictObject({ kind: z.literal('upload') }), urlSource]),
      { title: 'Source' },
    ).default({ kind: 'upload' }),
    allInterfaces: withUi(z.boolean().default(false), {
      title: 'All interfaces',
      help: 'every L3 interface, including interfaces added later',
    }),
    interfaces: withUi(z.array(vppInterfaceName).max(1024).default([]), {
      title: 'Interfaces',
      widget: 'interface-picker',
      help: 'where the list is enforced (at least one unless All interfaces)',
    }),
    direction: withUi(z.enum(['both', 'inbound', 'outbound']).default('both'), {
      title: 'Direction',
      help: 'inbound = traffic arriving on the interface from a listed address; outbound = leaving to one',
    }),
    protectHost: withUi(z.boolean().default(true), {
      title: 'Protect the box',
      help: 'also drop traffic from listed addresses to the box itself (management plane, nftables)',
    }),
    log: withUi(z.boolean().default(false), { title: 'Log' }),
    entries: withUi(z.array(blockEntry).max(MAX_BLOCK_ENTRIES).default([]), {
      title: 'Entries',
      help: 'imported from a file or the server URL; one prefix per entry',
    }),
  }).superRefine((l, ctx) => {
    if (!l.allInterfaces && l.interfaces.length === 0) {
      ctx.addIssue({ code: 'custom', path: ['interfaces'], message: 'select at least one interface, or All interfaces' });
    }
    if (l.allInterfaces && l.interfaces.length > 0) {
      ctx.addIssue({ code: 'custom', path: ['interfaces'], message: 'All interfaces is set: leave the interface list empty' });
    }
    const seen = new Set<string>();
    l.entries.forEach((e, i) => {
      const c = canonicalEntry(e);
      if (c === undefined) ctx.addIssue({ code: 'custom', path: ['entries', i], message: `'${e}' is not an IPv4/IPv6 prefix` });
      else if (c !== e) ctx.addIssue({ code: 'custom', path: ['entries', i], message: `not canonical: write ${c}` });
      else if (seen.has(e)) ctx.addIssue({ code: 'custom', path: ['entries', i], message: `${e} is listed twice` });
      seen.add(e);
    });
  }),
  { title: 'Block list' },
);

export const GlobalBlockingSchema = withUi(
  z
    .strictObject({
      lists: withUi(z.record(objectName, GlobalBlockingListSchema).default({}), { title: 'Block lists' }),
    })
    .superRefine((g, ctx) => {
      const total = Object.values(g.lists).reduce((n, l) => n + l.entries.length, 0);
      if (total > MAX_BLOCK_ENTRIES) {
        ctx.addIssue({ code: 'custom', path: ['lists'], message: `${total} entries over all lists; at most ${MAX_BLOCK_ENTRIES}` });
      }
    }),
  { title: 'Global blocking', description: 'IP block lists enforced before the access lists.' },
);

export type GlobalBlocking = z.infer<typeof GlobalBlockingSchema>;
export type GlobalBlockingList = z.infer<typeof GlobalBlockingListSchema>;

/** The `acl.globalBlocking` field. */
export const aclGlobalBlockingField = withUi(GlobalBlockingSchema.optional(), {
  title: 'Global blocking',
  group: 'Global blocking',
  order: 0,
});
