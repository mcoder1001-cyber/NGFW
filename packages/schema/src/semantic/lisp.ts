import type { RootConfig } from '../index.js';
import type { LispConfig } from '../domains/ext/lisp.js';
import { canonicalIp, canonicalPrefix, ipFamily } from '../ip.js';
import { jsonPointer } from '../pointer.js';
import type { SemanticIssue, ValidatorDefinition } from './registry.js';
import { interfaceIndex, knownVrfs } from './tunnels-common.js';

/**
 * Semantic validators for `tunnels.lisp` (F-lisp; rule ids `tunnels.lisp-…`). Kept thin (T3): canonical EIDs, EIDs
 * unique per VNI, references resolve (locator sets, interfaces, VRFs, EID-table maps, adjacency endpoints), RLOC
 * families consistent, and the global switches cover what is configured (`gpe` and every object need `enabled`).
 */

const MAC = /^[0-9a-fA-F]{2}(?::[0-9a-fA-F]{2}){5}$/;

const P = (...rest: (string | number)[]): string => jsonPointer('tunnels', 'lisp', ...rest);

function lispOf(config: RootConfig): LispConfig | undefined {
  return config.tunnels.lisp;
}

/** The canonical text of an EID, or undefined when it is not one. */
export function canonicalEid(eid: string): string | undefined {
  if (MAC.test(eid)) return eid.toLowerCase();
  return canonicalPrefix(eid);
}

function isMacEid(eid: string): boolean {
  return MAC.test(eid);
}

/** Family of an EID: 'mac', 4 or 6 (undefined when malformed). */
function eidFamily(eid: string): 'mac' | 4 | 6 | undefined {
  if (isMacEid(eid)) return 'mac';
  const slash = eid.indexOf('/');
  return slash < 0 ? undefined : ipFamily(eid.slice(0, slash));
}

const eidKey = (vni: number, eid: string): string => `${vni}/${canonicalEid(eid) ?? eid}`;

function hasObjects(l: LispConfig): boolean {
  return (
    Object.keys(l.locatorSets).length > 0 ||
    l.localEids.length > 0 ||
    l.remoteMappings.length > 0 ||
    l.adjacencies.length > 0 ||
    Object.keys(l.eidTables).length > 0 ||
    l.gpeEntries.length > 0 ||
    l.mapResolvers.length > 0 ||
    l.mapServers.length > 0 ||
    l.pitr !== undefined
  );
}

const eidCanonical: ValidatorDefinition = {
  name: 'tunnels.lisp-eid-canonical',
  domains: ['tunnels'],
  validate(config) {
    const l = lispOf(config);
    if (!l) return [];
    const issues: SemanticIssue[] = [];
    const check = (eid: string, ...path: (string | number)[]) => {
      const c = canonicalEid(eid);
      if (c === undefined)
        issues.push({
          pointer: P(...path),
          message: `'${eid}' is not an IP prefix or MAC address`,
        });
      else if (c !== eid)
        issues.push({
          pointer: P(...path),
          message: `EID '${eid}' is not canonical; write '${c}'`,
        });
    };
    l.localEids.forEach((e, i) => check(e.eid, 'localEids', i, 'eid'));
    l.remoteMappings.forEach((m, i) => check(m.eid, 'remoteMappings', i, 'eid'));
    l.adjacencies.forEach((a, i) => {
      check(a.reid, 'adjacencies', i, 'reid');
      check(a.leid, 'adjacencies', i, 'leid');
    });
    l.gpeEntries.forEach((g, i) => {
      check(g.reid, 'gpeEntries', i, 'reid');
      check(g.leid, 'gpeEntries', i, 'leid');
    });
    return issues;
  },
};

const eidUnique: ValidatorDefinition = {
  name: 'tunnels.lisp-eid-unique',
  domains: ['tunnels'],
  validate(config) {
    const l = lispOf(config);
    if (!l) return [];
    const issues: SemanticIssue[] = [];
    // VPP keeps one EID-table entry per (VNI, EID): local and remote share the space.
    const seen = new Map<string, string>();
    const claim = (vni: number, eid: string, where: string, ...path: (string | number)[]) => {
      const k = eidKey(vni, eid);
      const prev = seen.get(k);
      if (prev !== undefined) {
        issues.push({
          pointer: P(...path),
          message: `EID ${eid} in VNI ${vni} is already configured at ${prev}`,
        });
      } else seen.set(k, where);
    };
    l.localEids.forEach((e, i) => claim(e.vni, e.eid, P('localEids', i), 'localEids', i, 'eid'));
    l.remoteMappings.forEach((m, i) =>
      claim(m.vni, m.eid, P('remoteMappings', i), 'remoteMappings', i, 'eid'),
    );
    const once = (list: { vni: number; reid: string; leid: string }[], field: string) => {
      const s = new Set<string>();
      list.forEach((a, i) => {
        const k = `${eidKey(a.vni, a.reid)}|${canonicalEid(a.leid) ?? a.leid}`;
        if (s.has(k))
          issues.push({
            pointer: P(field, i),
            message: `duplicate ${field} entry ${a.vni}/${a.reid}/${a.leid}`,
          });
        s.add(k);
      });
    };
    once(l.adjacencies, 'adjacencies');
    once(l.gpeEntries, 'gpeEntries');
    const dup = (list: string[], field: string) => {
      const s = new Set<string>();
      list.forEach((a, i) => {
        const k = canonicalIp(a) ?? a;
        if (s.has(k)) issues.push({ pointer: P(field, i), message: `duplicate address ${a}` });
        s.add(k);
      });
    };
    dup(l.mapResolvers, 'mapResolvers');
    dup(l.mapServers, 'mapServers');
    return issues;
  },
};

const refsExist: ValidatorDefinition = {
  name: 'tunnels.lisp-references',
  domains: ['tunnels', 'interfaces', 'vrfs'],
  validate(config) {
    const l = lispOf(config);
    if (!l) return [];
    const issues: SemanticIssue[] = [];
    const sets = new Set(Object.keys(l.locatorSets));
    const ifs = interfaceIndex(config);
    const vrfs = knownVrfs(config);
    for (const [name, set] of Object.entries(l.locatorSets)) {
      const seen = new Set<string>();
      set.locators.forEach((loc, i) => {
        if (!ifs.has(loc.interface)) {
          issues.push({
            pointer: P('locatorSets', name, 'locators', i, 'interface'),
            message: `interface '${loc.interface}' does not exist`,
          });
        }
        if (seen.has(loc.interface)) {
          issues.push({
            pointer: P('locatorSets', name, 'locators', i, 'interface'),
            message: `interface '${loc.interface}' is already a locator of '${name}'`,
          });
        }
        seen.add(loc.interface);
      });
    }
    l.localEids.forEach((e, i) => {
      if (!sets.has(e.locatorSet))
        issues.push({
          pointer: P('localEids', i, 'locatorSet'),
          message: `locator set '${e.locatorSet}' does not exist`,
        });
    });
    if (l.pitr !== undefined && !sets.has(l.pitr)) {
      issues.push({ pointer: P('pitr'), message: `locator set '${l.pitr}' does not exist` });
    }
    for (const [vni, t] of Object.entries(l.eidTables)) {
      if ((t.vrf === undefined) === (t.bridgeDomain === undefined)) {
        issues.push({
          pointer: P('eidTables', vni),
          message: `VNI ${vni} needs exactly one of vrf (L3) or bridgeDomain (L2)`,
        });
      } else if (t.vrf !== undefined && !vrfs.has(t.vrf)) {
        issues.push({
          pointer: P('eidTables', vni, 'vrf'),
          message: `VRF '${t.vrf}' does not exist`,
        });
      }
      if (vni === '0')
        issues.push({
          pointer: P('eidTables', vni),
          message:
            'VNI 0 is the default instance and maps to the default table; it cannot be remapped',
        });
    }
    l.gpeEntries.forEach((g, i) => {
      if (!vrfs.has(g.vrf))
        issues.push({
          pointer: P('gpeEntries', i, 'vrf'),
          message: `VRF '${g.vrf}' does not exist`,
        });
    });
    // Adjacencies bind a configured remote mapping to a configured local EID of the same VNI.
    const local = new Set(l.localEids.map((e) => eidKey(e.vni, e.eid)));
    const remote = new Set(l.remoteMappings.map((m) => eidKey(m.vni, m.eid)));
    l.adjacencies.forEach((a, i) => {
      if (!remote.has(eidKey(a.vni, a.reid)))
        issues.push({
          pointer: P('adjacencies', i, 'reid'),
          message: `no remote mapping for ${a.reid} in VNI ${a.vni}`,
        });
      if (!local.has(eidKey(a.vni, a.leid)))
        issues.push({
          pointer: P('adjacencies', i, 'leid'),
          message: `no local EID ${a.leid} in VNI ${a.vni}`,
        });
    });
    return issues;
  },
};

const eidTableMapped: ValidatorDefinition = {
  name: 'tunnels.lisp-eid-table',
  domains: ['tunnels'],
  validate(config) {
    const l = lispOf(config);
    if (!l) return [];
    const issues: SemanticIssue[] = [];
    const check = (vni: number, eid: string, ...path: (string | number)[]) => {
      if (vni === 0) return; // the default instance lives in the default table / no bridge domain map
      const t = l.eidTables[String(vni)];
      const fam = eidFamily(eid);
      if (t === undefined) {
        issues.push({
          pointer: P(...path),
          message: `VNI ${vni} has no eidTables mapping (VRF or bridge domain)`,
        });
      } else if (fam === 'mac' && t.bridgeDomain === undefined) {
        issues.push({
          pointer: P(...path),
          message: `MAC EID ${eid} needs VNI ${vni} mapped to a bridge domain`,
        });
      } else if (fam !== 'mac' && fam !== undefined && t.vrf === undefined) {
        issues.push({
          pointer: P(...path),
          message: `IP EID ${eid} needs VNI ${vni} mapped to a VRF`,
        });
      }
    };
    l.localEids.forEach((e, i) => check(e.vni, e.eid, 'localEids', i, 'vni'));
    l.remoteMappings.forEach((m, i) => check(m.vni, m.eid, 'remoteMappings', i, 'vni'));
    return issues;
  },
};

const families: ValidatorDefinition = {
  name: 'tunnels.lisp-rloc-family',
  domains: ['tunnels'],
  validate(config) {
    const l = lispOf(config);
    if (!l) return [];
    const issues: SemanticIssue[] = [];
    l.remoteMappings.forEach((m, i) => {
      const fams = new Set(m.rlocs.map((r) => ipFamily(r.address)));
      if (fams.size > 1)
        issues.push({
          pointer: P('remoteMappings', i, 'rlocs'),
          message: 'all RLOCs of a mapping must be of one address family',
        });
      if (m.rlocs.length > 0 && m.action !== 'no-action') {
        issues.push({
          pointer: P('remoteMappings', i, 'action'),
          message: 'action applies to negative mappings (no RLOCs) only',
        });
      }
    });
    l.gpeEntries.forEach((g, i) => {
      g.pairs.forEach((p, j) => {
        if (ipFamily(p.local) !== ipFamily(p.remote)) {
          issues.push({
            pointer: P('gpeEntries', i, 'pairs', j),
            message: 'local and remote RLOC must be of one address family',
          });
        }
      });
      if (g.pairs.length > 0 && g.action !== 'no-action') {
        issues.push({
          pointer: P('gpeEntries', i, 'action'),
          message: 'action applies to negative entries (no pairs) only',
        });
      }
      if (eidFamily(g.reid) !== eidFamily(g.leid)) {
        issues.push({
          pointer: P('gpeEntries', i, 'leid'),
          message: 'local and remote EID must be of one family',
        });
      }
    });
    l.adjacencies.forEach((a, i) => {
      if (eidFamily(a.reid) !== eidFamily(a.leid)) {
        issues.push({
          pointer: P('adjacencies', i, 'leid'),
          message: 'local and remote EID must be of one family',
        });
      }
    });
    return issues;
  },
};

const switches: ValidatorDefinition = {
  name: 'tunnels.lisp-enabled',
  domains: ['tunnels'],
  validate(config) {
    const l = lispOf(config);
    if (!l || l.enabled) return [];
    const issues: SemanticIssue[] = [];
    if (l.gpe)
      issues.push({ pointer: P('gpe'), message: 'LISP-GPE requires LISP (enabled: true)' });
    if (hasObjects(l))
      issues.push({
        pointer: P('enabled'),
        message: 'LISP objects are configured but LISP is not enabled',
      });
    return issues;
  },
};

const gpeNeeded: ValidatorDefinition = {
  name: 'tunnels.lisp-gpe-entries',
  domains: ['tunnels'],
  validate(config) {
    const l = lispOf(config);
    if (!l || l.gpe || l.gpeEntries.length === 0) return [];
    return [{ pointer: P('gpe'), message: 'GPE forwarding entries need LISP-GPE (gpe: true)' }];
  },
};

export const lispValidators: readonly ValidatorDefinition[] = [
  eidCanonical,
  eidUnique,
  refsExist,
  eidTableMapped,
  families,
  switches,
  gpeNeeded,
];
