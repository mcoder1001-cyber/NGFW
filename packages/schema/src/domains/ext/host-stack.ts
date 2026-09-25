import { z } from 'zod';
import { canonicalIp, canonicalPrefix, ipFamily } from '../../ip.js';
import { ipAddress, objectName, vppInterfaceName } from '../../primitives.js';
import { withUi } from '../../ui.js';
import { ipv4OrIpv6Cidr, secretRefOf, transportPort, u32Int, vrfRef } from '../_shared/primitives.js';

/**
 * `services.hostStack` — the API-configurable part of VPP's host stack (F-host-stack, WBS D7.10, scoped down by
 * D-085): session-layer enable, application namespaces, session rules (the host-stack "firewall"), a TCP source
 * address pool and — opt-in on the agent — the `http_static` server. Everything startup.conf-only (buffer sizes,
 * congestion control, event-queue segments) belongs to the start-up generator (F-startup-gen), not here.
 *
 * Secrets rule (D-051): a namespace secret is only a `key/<name>` reference. D-049: tags and namespace ids carry no
 * `..`, no control characters; `wwwRootPath` lies under `/var/lib/vrx/www/`. Cross-domain rules (VRFs and
 * interfaces exist, rule namespaces exist, session rules vs. Auto-SDL) live in `semantic/host-stack.ts`.
 */

type Path = (string | number)[];
const add = (ctx: z.RefinementCtx, path: Path, message: string): void => {
  ctx.addIssue({ code: 'custom', path, message });
};

/** Namespace ids and rule tags: an `objectName` without `..`; ≤ 40 characters (the agent prefixes its owner). */
export const hostStackId = withUi(
  objectName.max(40).refine((s) => !s.includes('..'), 'must not contain `..`'),
  { title: 'Id' },
);

/** Root of every http_static `www_root` (D-049, DF-8 review M5). */
export const HOST_STACK_WWW_ROOT = '/var/lib/vrx/www/';

export const hostStackWwwRootPath = withUi(
  z
    .string()
    .min(HOST_STACK_WWW_ROOT.length + 1)
    .max(255)
    .refine((s) => s.startsWith(HOST_STACK_WWW_ROOT), `must be under ${HOST_STACK_WWW_ROOT}`)
    .refine((s) => !s.split('/').includes('..'), 'must not contain `..`')
    // eslint-disable-next-line no-control-regex
    .refine((s) => !/[\u0000-\u001f\u007f]/.test(s), 'must not contain control characters'),
  { title: 'Web root', help: `Directory under ${HOST_STACK_WWW_ROOT}` },
);

export const HostStackNamespaceSchema = z.strictObject({
  secretRef: withUi(secretRefOf('key').optional(), {
    title: 'Secret',
    help: 'key/<name>; not applied until the API→agent secret channel exists',
  }),
  interface: withUi(vppInterfaceName.optional(), { title: 'Interface', widget: 'interface-picker' }),
  vrf: vrfRef,
});

const canonicalCidr = ipv4OrIpv6Cidr.refine(
  (s) => canonicalPrefix(s) === s,
  'prefix must be canonical (host bits zero, lower-case, compressed)',
);

export const HostStackSessionRuleSchema = z
  .strictObject({
    tag: hostStackId,
    scope: withUi(z.enum(['global', 'local']).default('global'), { title: 'Scope' }),
    transport: withUi(z.enum(['tcp', 'udp']), { title: 'Transport' }),
    local: withUi(canonicalCidr, { title: 'Local prefix' }),
    localPort: withUi(transportPort.optional(), { title: 'Local port', help: 'unset = any' }),
    remote: withUi(canonicalCidr, { title: 'Remote prefix' }),
    remotePort: withUi(transportPort.optional(), { title: 'Remote port', help: 'unset = any' }),
    action: withUi(z.enum(['allow', 'deny', 'redirect']), { title: 'Action' }),
    redirectAppIndex: withUi(u32Int.optional(), {
      title: 'Redirect to app index',
      help: 'required with action redirect',
    }),
    appNamespace: withUi(hostStackId.optional(), { title: 'App namespace', help: 'unset = default' }),
  })
  .superRefine((r, ctx) => {
    if (ipFamily(r.local.split('/')[0] ?? '') !== ipFamily(r.remote.split('/')[0] ?? ''))
      add(ctx, ['remote'], 'local and remote prefixes must be of the same address family');
    if (r.action === 'redirect' && r.redirectAppIndex === undefined)
      add(ctx, ['redirectAppIndex'], 'action redirect needs redirectAppIndex');
    if (r.action !== 'redirect' && r.redirectAppIndex !== undefined)
      add(ctx, ['redirectAppIndex'], 'redirectAppIndex is only valid with action redirect');
  });

export const HostStackTcpSourceSchema = z
  .strictObject({
    first: withUi(ipAddress, { title: 'First address' }),
    last: withUi(ipAddress, { title: 'Last address' }),
    vrf: vrfRef,
  })
  .superRefine((t, ctx) => {
    if (canonicalIp(t.first) !== t.first) add(ctx, ['first'], 'address must be canonical');
    if (canonicalIp(t.last) !== t.last) add(ctx, ['last'], 'address must be canonical');
    if (ipFamily(t.first) !== ipFamily(t.last))
      add(ctx, ['last'], 'first and last must be of the same address family');
  });

export const HostStackHttpStaticSchema = z.strictObject({
  enabled: withUi(z.boolean().default(false), { title: 'Enabled' }),
  wwwRootPath: hostStackWwwRootPath,
  uri: withUi(
    z
      .string()
      .min(1)
      .max(255)
      .regex(/^(tcp|tls):\/\/[0-9A-Fa-f.:]+\/[0-9]{1,5}$/, 'tcp://<address>/<port> or tls://<address>/<port>'),
    { title: 'URI', help: 'e.g. tcp://0.0.0.0/80' },
  ),
  cacheSizeMb: withUi(z.int().min(1).max(4095).default(10), { title: 'Cache size (MiB)' }),
});

export const HostStackSchema = withUi(
  z
    .strictObject({
      enabled: withUi(z.boolean().default(false), {
        title: 'Session layer',
        help: 'Required state of the VPP session layer (rule-table engine); set only by the globals owner',
      }),
      namespaces: withUi(z.record(hostStackId, HostStackNamespaceSchema).default({}), {
        title: 'App namespaces',
        widget: 'record',
      }),
      sessionRules: withUi(z.array(HostStackSessionRuleSchema).default([]), { title: 'Session rules' }),
      tcpSourceAddresses: HostStackTcpSourceSchema.optional(),
      httpStatic: HostStackHttpStaticSchema.optional(),
    })
    .superRefine((h, ctx) => {
      const seen = new Map<string, number>();
      h.sessionRules.forEach((r, i) => {
        const prev = seen.get(r.tag);
        if (prev !== undefined) add(ctx, ['sessionRules', i, 'tag'], `tag '${r.tag}' is already used by rule ${prev}`);
        else seen.set(r.tag, i);
      });
      if ((h.sessionRules.length > 0 || Object.keys(h.namespaces).length > 0) && !h.enabled)
        add(ctx, ['enabled'], 'namespaces and session rules need the session layer (enabled: true)');
    }),
  {
    title: 'Host stack',
    description: 'VPP host stack (advanced, T3): session layer, app namespaces, session rules, TCP source addresses',
  },
);

export type HostStackConfig = z.infer<typeof HostStackSchema>;
export type HostStackSessionRule = z.infer<typeof HostStackSessionRuleSchema>;
export type HostStackNamespace = z.infer<typeof HostStackNamespaceSchema>;
