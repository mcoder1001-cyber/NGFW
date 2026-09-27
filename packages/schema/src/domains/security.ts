import { z } from 'zod';
import { withUi } from '../ui.js';
import { ipAddress } from '../primitives.js';
import { descriptionField, enabledFlag, ipv4OrIpv6Cidr } from './_shared/primitives.js';

/**
 * `security` — the box's own defensive policy for its management and control planes. Today it carries `autoBlock`
 * (F-bruteforce-block): brute-force and port-scan detection that temporarily blocks offending source IPs by feeding
 * them into the Global Blocking engine (`acl.globalBlocking`) as a system-owned, TTL'd list — no config commit per
 * block. IPS signatures (BL-SEC-08) and honeypots (BL-SEC-19) are reserved for later fields here.
 *
 * The detectors that count failures live where the events are: web/API logins and VPN/EAP auth in the API, SSH
 * failures and port scans on the host (the agent). The thresholds, allow-list and caps below are the shared contract
 * both read; the *blocked entries* are runtime state, never part of the committed document.
 *
 * A source that matches the allow-list (plus the always-implicit loopback) is never blocked — this is how an
 * operator keeps their management networks, and their own address, out of any lock-out.
 */

const GROUP = 'auto-block';

/** What is being guarded — one detector each. */
export const AutoBlockSourceKind = z.enum(['webLogin', 'ssh', 'vpnAuth', 'portScan']);
export type AutoBlockSourceKind = z.infer<typeof AutoBlockSourceKind>;

/** An allow-list entry: a bare address (treated as /32 or /128) or a CIDR prefix. */
export const autoBlockAllowEntry = withUi(z.union([ipAddress, ipv4OrIpv6Cidr]), {
  title: 'Allowed source',
  help: 'a management address or network that is never auto-blocked; a bare address means that host only',
  widget: 'cidr',
});

export const AutoBlockRuleSchema = z.strictObject({
  source: withUi(AutoBlockSourceKind, {
    title: 'Detector',
    widget: 'select',
    help: 'webLogin (web/API sign-in), ssh, vpnAuth (IKE/EAP), or portScan (local-in probes)',
    order: 1,
  }),
  enabled: withUi(z.boolean().default(true), { title: 'Enabled', order: 2 }),
  threshold: withUi(z.number().int().min(1).max(100_000).default(5), {
    title: 'Threshold',
    help: 'failures (portScan: distinct ports) from one source within the window before it is blocked',
    order: 3,
  }),
  windowSec: withUi(z.number().int().min(1).max(86_400).default(60), {
    title: 'Window (seconds)',
    help: 'the sliding window the threshold is counted over',
    order: 4,
  }),
  blockSec: withUi(z.number().int().min(1).max(30 * 86_400).default(900), {
    title: 'Block (seconds)',
    help: 'how long a first offence is blocked',
    order: 5,
  }),
  escalate: withUi(z.boolean().default(true), {
    title: 'Escalate on repeat',
    help: 'double the block time on each repeat offence within the memory window, up to the cap',
    order: 6,
  }),
  maxBlockSec: withUi(z.number().int().min(1).max(30 * 86_400).default(86_400), {
    title: 'Maximum block (seconds)',
    help: 'the escalation cap; a single block never lasts longer than this',
    order: 7,
  }),
});
export type AutoBlockRule = z.infer<typeof AutoBlockRuleSchema>;

export const AutoBlockSchema = withUi(
  z
    .strictObject({
      enabled: withUi(z.boolean().default(false), {
        title: 'Enabled',
        help: 'master switch; when off no source is auto-blocked',
        order: 1,
      }),
      rules: withUi(z.array(AutoBlockRuleSchema).max(16).default([]), {
        title: 'Detectors',
        help: 'one rule per source; a source with no rule is not watched',
        order: 2,
      }),
      allowlist: withUi(z.array(autoBlockAllowEntry).max(256).default([]), {
        title: 'Allow list',
        help: 'addresses/networks that are never blocked (your management networks and your own address)',
        order: 3,
      }),
      maxEntries: withUi(z.number().int().min(1).max(1_000_000).default(10_000), {
        title: 'Maximum blocked entries',
        help: 'cap on the live auto-block set; the oldest expiring entry is dropped when it is reached',
        order: 4,
      }),
      description: descriptionField.optional(),
    })
    .superRefine((a, ctx) => {
      const seen = new Set<string>();
      a.rules.forEach((r, i) => {
        if (seen.has(r.source)) {
          ctx.addIssue({
            code: 'custom',
            path: ['rules', i, 'source'],
            message: `there is already a rule for '${r.source}'`,
          });
        }
        seen.add(r.source);
        if (r.escalate && r.maxBlockSec < r.blockSec) {
          ctx.addIssue({
            code: 'custom',
            path: ['rules', i, 'maxBlockSec'],
            message: 'the maximum block must be at least the first-offence block',
          });
        }
      });
    }),
  {
    title: 'Auto-block',
    description: 'brute-force / scan detection with automatic temporary blocking via Global Blocking',
    order: 1,
  },
);
export type AutoBlock = z.infer<typeof AutoBlockSchema>;

export const SecuritySchema = withUi(
  z.strictObject({
    autoBlock: withUi(AutoBlockSchema.prefault({}), { group: GROUP, order: 1 }),
  }),
  {
    title: 'Security',
    description: 'defensive policy for the box’s own management and control planes',
    order: 140,
  },
);
export type SecurityConfig = z.infer<typeof SecuritySchema>;
