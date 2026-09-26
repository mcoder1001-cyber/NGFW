import { interfaceNames } from '../domains/interfaces.js';
import type { MplsConfig, MplsPathConfig } from '../domains/ext/mpls-srmpls.js';
import { vrfExists } from '../domains/vrfs.js';
import { prefixKey } from '../ip.js';
import { jsonPointer } from '../pointer.js';
import type { RootConfig } from '../index.js';
import type { SemanticIssue, ValidatorDefinition } from './registry.js';
import { duplicateIssues } from './unique.js';

/**
 * F-mpls-srmpls rules (wave-A-hotspots C2; ids `routing.mpls-srmpls-…`) over `routing.mpls`. Single-object rules
 * (label range 16–1048575, out-label stack ≤ 16, payload only on end-of-stack routes, one address family per route) are
 * in the Zod schema (`../domains/ext/mpls-srmpls.ts`); these are the cross-object ones.
 *
 * MPLS table 0 is shared by static label routes of table 0, label ↔ IP bindings (VPP installs the bound label there)
 * and SR-MPLS binding SIDs, so a local label is used by exactly one of them.
 */

const MPLS = ['routing', 'mpls'] as const;

function mplsOf(config: RootConfig): MplsConfig | undefined {
  return config.routing.mpls;
}

/** Every path of every label route and tunnel, with its pointer segments and the tunnel it belongs to. */
function paths(m: MplsConfig) {
  const out: { path: MplsPathConfig; at: readonly (string | number)[]; tunnel?: string }[] = [];
  for (const [i, r] of m.labelRoutes.entries()) {
    for (const [j, p] of r.paths.entries())
      out.push({ path: p, at: [...MPLS, 'labelRoutes', i, 'paths', j] });
  }
  for (const [name, t] of Object.entries(m.tunnels)) {
    for (const [j, p] of t.paths.entries())
      out.push({ path: p, at: [...MPLS, 'tunnels', name, 'paths', j], tunnel: name });
  }
  return out;
}

/** Labels VPP installs in MPLS table 0 besides label routes: binding labels and binding SIDs. */
function tableZeroOwners(m: MplsConfig): Map<number, string> {
  const owners = new Map<number, string>();
  for (const bsid of Object.keys(m.sr.policies))
    owners.set(Number(bsid), `the binding SID of SR policy ${bsid}`);
  return owners;
}

export const mplsSrmplsValidators: readonly ValidatorDefinition[] = [
  {
    // MPLS is enabled on interfaces of the interfaces domain (not on MPLS tunnels: they are MPLS already)
    name: 'routing.mpls-srmpls-interface-exists',
    domains: ['routing', 'interfaces'],
    validate: (config) => {
      const m = mplsOf(config);
      if (!m) return [];
      const names = interfaceNames(config.interfaces);
      return m.interfaces.flatMap((name, i) =>
        names.has(name)
          ? []
          : [
              {
                pointer: jsonPointer(...MPLS, 'interfaces', i),
                message: `interface '${name}' does not exist`,
              },
            ],
      );
    },
  },
  {
    name: 'routing.mpls-srmpls-interface-unique',
    domains: ['routing'],
    validate: (config) => {
      const m = mplsOf(config);
      if (!m) return [];
      return duplicateIssues(
        m.interfaces.map((name, i) => ({ name, i })),
        ({ name }) => name,
        ({ i }) => [...MPLS, 'interfaces', i],
        ({ name }) => `MPLS is already enabled on interface '${name}'`,
      );
    },
  },
  {
    // a label route lives in the default MPLS table 0 or in a table declared under `tables`
    name: 'routing.mpls-srmpls-table-exists',
    domains: ['routing'],
    validate: (config) => {
      const m = mplsOf(config);
      if (!m) return [];
      return m.labelRoutes.flatMap((r, i) =>
        r.table === 0 || Object.hasOwn(m.tables, String(r.table))
          ? []
          : [
              {
                pointer: jsonPointer(...MPLS, 'labelRoutes', i, 'table'),
                message: `MPLS table ${r.table} is not declared under routing.mpls.tables`,
              },
            ],
      );
    },
  },
  {
    // VPP keeps one entry per (table, label, end-of-stack)
    name: 'routing.mpls-srmpls-label-route-unique',
    domains: ['routing'],
    validate: (config) => {
      const m = mplsOf(config);
      if (!m) return [];
      return duplicateIssues(
        m.labelRoutes.map((r, i) => ({ r, i })),
        ({ r }) => `${r.table}/${r.label}/${r.eos ? 'eos' : 'neos'}`,
        ({ i }) => [...MPLS, 'labelRoutes', i, 'label'],
        ({ r }) =>
          `label ${r.label} (${r.eos ? 'end of stack' : 'not end of stack'}) already has a route in MPLS table ${r.table}`,
      );
    },
  },
  {
    // a path's interface is an interface of the interfaces domain or an MPLS tunnel (never the tunnel itself)
    name: 'routing.mpls-srmpls-path-interface-exists',
    domains: ['routing', 'interfaces'],
    validate: (config) => {
      const m = mplsOf(config);
      if (!m) return [];
      const names = interfaceNames(config.interfaces);
      const issues: SemanticIssue[] = [];
      for (const { path, at, tunnel } of paths(m)) {
        if (path.interface === undefined) continue;
        const pointer = jsonPointer(...at, 'interface');
        if (tunnel !== undefined && path.interface === tunnel) {
          issues.push({
            pointer,
            message: `MPLS tunnel '${tunnel}' cannot send its packets into itself`,
          });
        } else if (!names.has(path.interface) && !Object.hasOwn(m.tunnels, path.interface)) {
          issues.push({
            pointer,
            message: `interface or MPLS tunnel '${path.interface}' does not exist`,
          });
        }
      }
      return issues;
    },
  },
  {
    name: 'routing.mpls-srmpls-vrf-exists',
    domains: ['routing', 'vrfs'],
    validate: (config) => {
      const m = mplsOf(config);
      if (!m) return [];
      const issues: SemanticIssue[] = [];
      const check = (vrf: string | undefined, at: readonly (string | number)[]) => {
        if (vrf !== undefined && !vrfExists(config.vrfs, vrf))
          issues.push({ pointer: jsonPointer(...at), message: `VRF '${vrf}' does not exist` });
      };
      for (const { path, at } of paths(m)) check(path.vrf, [...at, 'vrf']);
      for (const [i, b] of m.ipBindings.entries()) check(b.vrf, [...MPLS, 'ipBindings', i, 'vrf']);
      for (const [i, s] of m.sr.steering.entries())
        check(s.vrf, [...MPLS, 'sr', 'steering', i, 'vrf']);
      return issues;
    },
  },
  {
    // a tunnel's name is the logical name of its VPP interface (D-069): one interface name space
    name: 'routing.mpls-srmpls-tunnel-name',
    domains: ['routing', 'interfaces'],
    validate: (config) => {
      const m = mplsOf(config);
      if (!m) return [];
      const names = interfaceNames(config.interfaces);
      return Object.keys(m.tunnels)
        .filter((name) => names.has(name))
        .map((name) => ({
          pointer: jsonPointer(...MPLS, 'tunnels', name),
          message: `'${name}' is already an interface; an MPLS tunnel needs a name of its own`,
        }));
    },
  },
  {
    // "a BSID is not used as a static label route": VPP installs every binding SID as a local label in MPLS table 0
    name: 'routing.mpls-srmpls-bsid-not-label-route',
    domains: ['routing'],
    validate: (config) => {
      const m = mplsOf(config);
      if (!m) return [];
      const owners = tableZeroOwners(m);
      return m.labelRoutes.flatMap((r, i) => {
        const owner = r.table === 0 ? owners.get(r.label) : undefined;
        return owner === undefined
          ? []
          : [
              {
                pointer: jsonPointer(...MPLS, 'labelRoutes', i, 'label'),
                message: `label ${r.label} is ${owner} (MPLS table 0)`,
              },
            ];
      });
    },
  },
  {
    // a bound label is installed in MPLS table 0: it is not a label route of table 0, a binding SID or another binding's
    // label; one label per (VRF, prefix) (VPP keeps one local label per prefix)
    name: 'routing.mpls-srmpls-binding-unique',
    domains: ['routing'],
    validate: (config) => {
      const m = mplsOf(config);
      if (!m) return [];
      const owners = tableZeroOwners(m);
      for (const r of m.labelRoutes) {
        if (r.table === 0 && !owners.has(r.label))
          owners.set(r.label, 'a label route of MPLS table 0');
      }
      const issues: SemanticIssue[] = [];
      const bound = new Map<number, number>();
      for (const [i, b] of m.ipBindings.entries()) {
        const pointer = jsonPointer(...MPLS, 'ipBindings', i, 'label');
        const owner = owners.get(b.label);
        const first = bound.get(b.label);
        if (owner !== undefined)
          issues.push({ pointer, message: `label ${b.label} is already ${owner}` });
        else if (first !== undefined)
          issues.push({
            pointer,
            message: `label ${b.label} is already bound (first at ${jsonPointer(...MPLS, 'ipBindings', first)})`,
          });
        else bound.set(b.label, i);
      }
      issues.push(
        ...duplicateIssues(
          m.ipBindings.map((b, i) => ({ b, i })),
          ({ b }) => `${b.vrf} ${prefixKey(b.prefix)}`,
          ({ i }) => [...MPLS, 'ipBindings', i, 'prefix'],
          ({ b }) => `prefix ${b.prefix} of VRF '${b.vrf}' already has a bound label`,
        ),
      );
      return issues;
    },
  },
  {
    name: 'routing.mpls-srmpls-steering-policy-exists',
    domains: ['routing'],
    validate: (config) => {
      const m = mplsOf(config);
      if (!m) return [];
      return m.sr.steering.flatMap((s, i) =>
        Object.hasOwn(m.sr.policies, String(s.bsid))
          ? []
          : [
              {
                pointer: jsonPointer(...MPLS, 'sr', 'steering', i, 'bsid'),
                message: `SR policy ${s.bsid} does not exist (routing.mpls.sr.policies)`,
              },
            ],
      );
    },
  },
  {
    // VPP steers one (table, prefix) into one policy
    name: 'routing.mpls-srmpls-steering-unique',
    domains: ['routing'],
    validate: (config) => {
      const m = mplsOf(config);
      if (!m) return [];
      return duplicateIssues(
        m.sr.steering.map((s, i) => ({ s, i })),
        ({ s }) => `${s.vrf} ${prefixKey(s.prefix)}`,
        ({ i }) => [...MPLS, 'sr', 'steering', i, 'prefix'],
        ({ s }) => `prefix ${s.prefix} of VRF '${s.vrf}' is already steered`,
      );
    },
  },
  {
    // VPP merges identical segment lists of one policy into one path: each list is given once
    name: 'routing.mpls-srmpls-segment-list-unique',
    domains: ['routing'],
    validate: (config) => {
      const m = mplsOf(config);
      if (!m) return [];
      return Object.entries(m.sr.policies).flatMap(([bsid, p]) =>
        duplicateIssues(
          p.segmentLists.map((sl, j) => ({ sl, j })),
          ({ sl }) => sl.labels.join('/'),
          ({ j }) => [...MPLS, 'sr', 'policies', bsid, 'segmentLists', j, 'labels'],
          ({ sl }) => `segment list ${sl.labels.join(' ')} is already a list of SR policy ${bsid}`,
        ),
      );
    },
  },
];
