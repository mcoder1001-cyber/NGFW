import { z } from 'zod';
import { ipv4Address, objectName, portNumber, vppInterfaceName } from '../../primitives.js';
import { withUi } from '../../ui.js';

/**
 * `nat.pnat` — policy 1:1 NAT (VPP `pnat` plugin, F-det44-map-dslite-cnat, WBS D4.6). IPv4 only (the VPP API is
 * IPv4-only). A binding matches a tuple and rewrites one; an attachment applies a binding on an interface's input or
 * output path. The agent projects them onto DF-3's `pnat.binding` / `pnat.attachment` (docs/agent/descriptors/pnat.md).
 *
 * Intra-`pnat` rules are refinements here (pointers inside `nat.pnat`): a non-empty match and rewrite, ports only
 * with protocol tcp/udp, unique binding names and match tuples (the VPP binding id IS the match tuple), attachments
 * naming an existing binding, no duplicate attachment, and one match mask per (interface, point) — VPP rejects a
 * second binding with a different mask there (-2). Interface existence is the semantic rule
 * `nat.det44-map-dslite-cnat-pnat-interfaces`.
 */

export const PNAT_PROTOCOLS = ['tcp', 'udp', 'icmp'] as const;
export const PNAT_POINTS = ['input', 'output'] as const;

export const PnatMatchSchema = withUi(
  z.strictObject({
    proto: withUi(z.enum(PNAT_PROTOCOLS), { title: 'Protocol' }).optional(),
    src: withUi(ipv4Address, { title: 'Source address' }).optional(),
    sport: withUi(portNumber, { title: 'Source port' }).optional(),
    dst: withUi(ipv4Address, { title: 'Destination address' }).optional(),
    dport: withUi(portNumber, { title: 'Destination port' }).optional(),
  }),
  { title: 'Match', help: 'Unset fields are wildcards' },
);

export const PnatRewriteSchema = withUi(
  z.strictObject({
    src: withUi(ipv4Address, { title: 'New source address' }).optional(),
    sport: withUi(portNumber, { title: 'New source port' }).optional(),
    dst: withUi(ipv4Address, { title: 'New destination address' }).optional(),
    dport: withUi(portNumber, { title: 'New destination port' }).optional(),
  }),
  { title: 'Rewrite', help: 'Unset fields are left unchanged' },
);

export const PnatBindingSchema = withUi(
  z.strictObject({
    name: withUi(objectName, { title: 'Name' }),
    match: PnatMatchSchema,
    rewrite: PnatRewriteSchema,
  }),
  { title: 'PNAT binding' },
);

export const PnatAttachmentSchema = withUi(
  z.strictObject({
    binding: withUi(objectName, { title: 'Binding' }),
    interface: withUi(vppInterfaceName, { title: 'Interface', widget: 'interface-picker' }),
    point: withUi(z.enum(PNAT_POINTS), { title: 'Point', help: 'input = before routing, output = after' }),
  }),
  { title: 'PNAT attachment' },
);

type PnatMatch = z.infer<typeof PnatMatchSchema>;

/** The canonical match tuple — the agent's binding id (`<proto>/<src>/<sport>/<dst>/<dport>`, `any` = wildcard). */
export function pnatMatchId(m: PnatMatch): string {
  const p = (v: string | number | undefined): string => (v === undefined ? 'any' : String(v));
  return [p(m.proto), p(m.src), p(m.sport), p(m.dst), p(m.dport)].join('/');
}

/** The match mask (which fields are set); VPP requires one mask per interface and point. */
export function pnatMatchMask(m: PnatMatch): string {
  return (['proto', 'src', 'sport', 'dst', 'dport'] as const).filter((k) => m[k] !== undefined).join('+');
}

export const PnatSchema = withUi(
  z
    .strictObject({
      bindings: withUi(z.array(PnatBindingSchema).max(4096).default([]), { title: 'Bindings' }),
      attachments: withUi(z.array(PnatAttachmentSchema).max(8192).default([]), {
        title: 'Attachments',
      }),
    })
    .superRefine((v, ctx) => {
      const add = (path: (string | number)[], message: string): void => {
        ctx.addIssue({ code: 'custom', path, message });
      };
      const names = new Map<string, number>();
      const ids = new Map<string, string>();
      v.bindings.forEach((b, i) => {
        if (names.has(b.name)) add(['bindings', i, 'name'], `duplicate binding name '${b.name}'`);
        names.set(b.name, i);
        const m = b.match;
        const r = b.rewrite;
        if (pnatMatchMask(m) === '') add(['bindings', i, 'match'], 'a match needs at least one field');
        if ([r.src, r.sport, r.dst, r.dport].every((x) => x === undefined)) {
          add(['bindings', i, 'rewrite'], 'a rewrite needs at least one field');
        }
        const l4 = m.proto === 'tcp' || m.proto === 'udp';
        if (!l4 && (m.sport !== undefined || m.dport !== undefined)) {
          add(['bindings', i, 'match', 'proto'], 'match ports need protocol tcp or udp');
        }
        if (!l4 && (r.sport !== undefined || r.dport !== undefined)) {
          add(['bindings', i, 'match', 'proto'], 'rewriting ports needs a match on protocol tcp or udp');
        }
        const id = pnatMatchId(m);
        const other = ids.get(id);
        if (other !== undefined) {
          add(['bindings', i, 'match'], `binding '${other}' has the same match (the match is the binding's identity)`);
        } else {
          ids.set(id, b.name);
        }
      });
      const seen = new Set<string>();
      const masks = new Map<string, string>();
      v.attachments.forEach((a, i) => {
        const bi = names.get(a.binding);
        if (bi === undefined) {
          add(['attachments', i, 'binding'], `binding '${a.binding}' does not exist`);
          return;
        }
        const key = `${a.interface}|${a.point}|${a.binding}`;
        if (seen.has(key)) add(['attachments', i], 'duplicate attachment');
        seen.add(key);
        const mask = pnatMatchMask(v.bindings[bi]!.match);
        const at = `${a.interface}|${a.point}`;
        const prev = masks.get(at);
        if (prev !== undefined && prev !== mask) {
          add(
            ['attachments', i, 'binding'],
            `bindings on ${a.interface} ${a.point} must match the same fields (${prev} vs ${mask}); VPP rejects mixed masks`,
          );
        } else if (prev === undefined) {
          masks.set(at, mask);
        }
      });
    }),
  { title: 'PNAT (policy 1:1 NAT)' },
);

export type PnatConfig = z.infer<typeof PnatSchema>;
export type PnatBinding = z.infer<typeof PnatBindingSchema>;
export type PnatAttachment = z.infer<typeof PnatAttachmentSchema>;
