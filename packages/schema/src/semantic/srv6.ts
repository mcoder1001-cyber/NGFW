import { isPlainObject } from '../json.js';
import {
  SRV6_INTERFACE_BEHAVIORS,
  SRV6_LOOKUP_BEHAVIORS,
  SRV6_NEXT_HOP_BEHAVIORS,
  SRV6_PSP_BEHAVIORS,
  type Srv6Config,
} from '../domains/ext/srv6.js';
import { canonicalIp, canonicalPrefix, ipFamily, parseCidr, parseIpv6 } from '../ip.js';
import { jsonPointer } from '../pointer.js';
import type { RootConfig } from '../index.js';
import { interfaceExists, lookupInterface, vrfExists } from './objects.js';
import type { SemanticIssue, ValidatorDefinition } from './registry.js';
import { duplicateIssues } from './unique.js';

/**
 * F-srv6 rules for `routing.srv6` (tier (b)). Rule ids:
 *
 * - `routing.srv6-canonical`: every address and prefix is written in canonical form (lower case, shortest `::`
 *   form) — the local SID and binding SID record keys included. The agent identifies SIDs by their canonical text and
 *   Retrieve reports them so; another spelling would never compare equal (D-049).
 * - `routing.srv6-sid-address`: local SIDs, binding SIDs, segments and encapsulation sources are unicast IPv6
 *   addresses (not `::`, `::1` or multicast).
 * - `routing.srv6-sid-unique`: a local SID is not also a policy's binding SID (both are /128 FIB entries).
 * - `routing.srv6-behavior-fields`: the fields a behaviour needs are set and the others are not — `interface` for
 *   end.x/end.dx2/end.dx4/end.dx6, `nextHop` for end.x/end.dx6 (IPv6) and end.dx4 (IPv4), `lookupVrf` for
 *   end.t/end.dt4/end.dt6, `psp` only on end/end.x/end.t.
 * - `routing.srv6-vrf-exists`, `routing.srv6-interface-exists`: referenced VRFs and interfaces are configured.
 * - `routing.srv6-encap-source` (D-074): an encapsulating policy has an outer source — its own `encapSource` or
 *   `routing.srv6.encapSource`; an insert policy has none of its own (there is no outer header).
 * - `routing.srv6-steering-bsid`: a steering entry points at a configured policy.
 * - `routing.srv6-steering-encap`: L2 steering and IPv4 steering need an encapsulating policy (VPP refuses them on an
 *   insert policy).
 * - `routing.srv6-steering-unique`: one entry per (VRF, prefix) and per L2 interface.
 * - `routing.srv6-l2-interface`: an L2-steered interface carries no IP address (VPP switches it to L2 cross-connect
 *   mode). The same interface as the end.dx2 target of a local SID is the usual L2VPN (VPWS) pair and is allowed.
 */

type Path = readonly (string | number)[];
const P = (...segments: Path): string => jsonPointer('routing', 'srv6', ...segments);

function srv6Of(config: RootConfig): Srv6Config | undefined {
  return config.routing.srv6;
}

/** True for a unicast IPv6 address: not ::, ::1 or ff00::/8. */
function isUnicast6(text: string): boolean {
  const v = parseIpv6(text);
  if (v === undefined) return false;
  if (v === 0n || v === 1n) return false;
  return v >> 120n !== 0xffn;
}

/** Every IPv6 address of the SRv6 section that names a SID or a source, with its pointer segments. */
function sidAddresses(s: Srv6Config): { path: Path; text: string; what: string }[] {
  const out: { path: Path; text: string; what: string }[] = [];
  for (const sid of Object.keys(s.localSids)) out.push({ path: ['localSids', sid], text: sid, what: 'local SID' });
  for (const [bsid, p] of Object.entries(s.policies)) {
    out.push({ path: ['policies', bsid], text: bsid, what: 'binding SID' });
    p.sidLists.forEach((l, i) =>
      l.sids.forEach((seg, j) =>
        out.push({ path: ['policies', bsid, 'sidLists', i, 'sids', j], text: seg, what: 'segment' }),
      ),
    );
    if (p.encapSource !== undefined)
      out.push({ path: ['policies', bsid, 'encapSource'], text: p.encapSource, what: 'encapsulation source' });
  }
  if (s.encapSource !== undefined) out.push({ path: ['encapSource'], text: s.encapSource, what: 'encapsulation source' });
  s.steering.forEach((st, i) => out.push({ path: ['steering', i, 'bsid'], text: st.bsid, what: 'binding SID' }));
  return out;
}

const canonical: ValidatorDefinition = {
  name: 'routing.srv6-canonical',
  domains: ['routing'],
  validate(config) {
    const s = srv6Of(config);
    if (s === undefined) return [];
    const issues: SemanticIssue[] = [];
    const check = (text: string, want: string | undefined, path: Path) => {
      if (want !== undefined && want !== text)
        issues.push({ pointer: P(...path), message: `write ${text} in canonical form: ${want}` });
    };
    for (const a of sidAddresses(s)) check(a.text, canonicalIp(a.text), a.path);
    for (const [sid, l] of Object.entries(s.localSids))
      if (l.nextHop !== undefined) check(l.nextHop, canonicalIp(l.nextHop), ['localSids', sid, 'nextHop']);
    s.steering.forEach((st, i) => {
      if (st.type === 'l3') check(st.prefix, canonicalPrefix(st.prefix), ['steering', i, 'prefix']);
    });
    return issues;
  },
};

const sidAddress: ValidatorDefinition = {
  name: 'routing.srv6-sid-address',
  domains: ['routing'],
  validate(config) {
    const s = srv6Of(config);
    if (s === undefined) return [];
    return sidAddresses(s)
      .filter((a) => !isUnicast6(a.text))
      .map((a) => ({
        pointer: P(...a.path),
        message: `${a.what} ${a.text} must be a unicast IPv6 address (not ::, ::1 or multicast)`,
      }));
  },
};

const sidUnique: ValidatorDefinition = {
  name: 'routing.srv6-sid-unique',
  domains: ['routing'],
  validate(config) {
    const s = srv6Of(config);
    if (s === undefined) return [];
    const bsids = new Map(Object.keys(s.policies).map((b) => [canonicalIp(b) ?? b, b]));
    const issues: SemanticIssue[] = [];
    for (const sid of Object.keys(s.localSids)) {
      const b = bsids.get(canonicalIp(sid) ?? sid);
      if (b !== undefined)
        issues.push({
          pointer: P('localSids', sid),
          message: `${sid} is also the binding SID of policy ${b}; a SID is either a local SID or a binding SID`,
        });
    }
    return issues;
  },
};

const behaviorFields: ValidatorDefinition = {
  name: 'routing.srv6-behavior-fields',
  domains: ['routing'],
  validate(config) {
    const s = srv6Of(config);
    if (s === undefined) return [];
    const issues: SemanticIssue[] = [];
    for (const [sid, l] of Object.entries(s.localSids)) {
      const b = l.behavior;
      const at = (field: string, message: string) =>
        issues.push({ pointer: P('localSids', sid, field), message });
      const needsIf = SRV6_INTERFACE_BEHAVIORS.includes(b);
      if (needsIf && l.interface === undefined) at('interface', `${b} needs an interface to cross-connect to`);
      if (!needsIf && l.interface !== undefined) at('interface', `${b} takes no interface (only end.x, end.dx2, end.dx4, end.dx6)`);
      const needsNh = SRV6_NEXT_HOP_BEHAVIORS.includes(b);
      if (needsNh && l.nextHop === undefined) at('nextHop', `${b} needs a next hop`);
      if (!needsNh && l.nextHop !== undefined) at('nextHop', `${b} takes no next hop (only end.x, end.dx4, end.dx6)`);
      if (needsNh && l.nextHop !== undefined) {
        const want = b === 'end.dx4' ? 4 : 6;
        if (ipFamily(l.nextHop) !== want) at('nextHop', `${b} needs an IPv${want} next hop`);
      }
      const needsLookup = SRV6_LOOKUP_BEHAVIORS.includes(b);
      if (needsLookup && l.lookupVrf === undefined) at('lookupVrf', `${b} needs a lookup VRF`);
      if (!needsLookup && l.lookupVrf !== undefined) at('lookupVrf', `${b} takes no lookup VRF (only end.t, end.dt4, end.dt6)`);
      if (l.psp && !SRV6_PSP_BEHAVIORS.includes(b)) at('psp', `${b} does not support PSP (only end, end.x, end.t)`);
    }
    return issues;
  },
};

const vrfsExist: ValidatorDefinition = {
  name: 'routing.srv6-vrf-exists',
  domains: ['routing', 'vrfs'],
  validate(config) {
    const s = srv6Of(config);
    if (s === undefined) return [];
    const issues: SemanticIssue[] = [];
    const check = (name: string | undefined, path: Path) => {
      if (name !== undefined && !vrfExists(config, name))
        issues.push({ pointer: P(...path), message: `VRF '${name}' does not exist` });
    };
    for (const [sid, l] of Object.entries(s.localSids)) {
      check(l.vrf, ['localSids', sid, 'vrf']);
      check(l.lookupVrf, ['localSids', sid, 'lookupVrf']);
    }
    for (const [bsid, p] of Object.entries(s.policies)) check(p.vrf, ['policies', bsid, 'vrf']);
    s.steering.forEach((st, i) => {
      if (st.type === 'l3') check(st.vrf, ['steering', i, 'vrf']);
    });
    return issues;
  },
};

const interfacesExist: ValidatorDefinition = {
  name: 'routing.srv6-interface-exists',
  domains: ['routing', 'interfaces'],
  validate(config) {
    const s = srv6Of(config);
    if (s === undefined) return [];
    const issues: SemanticIssue[] = [];
    const check = (name: string | undefined, path: Path) => {
      if (name !== undefined && !interfaceExists(config, name))
        issues.push({ pointer: P(...path), message: `interface '${name}' is not configured` });
    };
    for (const [sid, l] of Object.entries(s.localSids)) check(l.interface, ['localSids', sid, 'interface']);
    s.steering.forEach((st, i) => {
      if (st.type === 'l2') check(st.interface, ['steering', i, 'interface']);
    });
    return issues;
  },
};

const encapSource: ValidatorDefinition = {
  name: 'routing.srv6-encap-source',
  domains: ['routing'],
  validate(config) {
    const s = srv6Of(config);
    if (s === undefined) return [];
    const issues: SemanticIssue[] = [];
    for (const [bsid, p] of Object.entries(s.policies)) {
      if (p.encap && p.encapSource === undefined && s.encapSource === undefined)
        issues.push({
          pointer: P('policies', bsid, 'encapSource'),
          message:
            'an encapsulating policy needs an outer source address: set encapSource here or routing.srv6.encapSource (VPP’s global default cannot be read back, D-074)',
        });
      if (!p.encap && p.encapSource !== undefined)
        issues.push({
          pointer: P('policies', bsid, 'encapSource'),
          message: 'an insert policy (encap off) has no outer header: remove encapSource',
        });
    }
    return issues;
  },
};

/** The policy a steering BSID names (canonical compare), or undefined. */
function policyOf(s: Srv6Config, bsid: string) {
  const want = canonicalIp(bsid) ?? bsid;
  for (const [k, p] of Object.entries(s.policies)) if ((canonicalIp(k) ?? k) === want) return p;
  return undefined;
}

const steeringBsid: ValidatorDefinition = {
  name: 'routing.srv6-steering-bsid',
  domains: ['routing'],
  validate(config) {
    const s = srv6Of(config);
    if (s === undefined) return [];
    const issues: SemanticIssue[] = [];
    s.steering.forEach((st, i) => {
      if (policyOf(s, st.bsid) === undefined)
        issues.push({ pointer: P('steering', i, 'bsid'), message: `no policy with binding SID ${st.bsid}` });
    });
    return issues;
  },
};

const steeringEncap: ValidatorDefinition = {
  name: 'routing.srv6-steering-encap',
  domains: ['routing'],
  validate(config) {
    const s = srv6Of(config);
    if (s === undefined) return [];
    const issues: SemanticIssue[] = [];
    s.steering.forEach((st, i) => {
      const p = policyOf(s, st.bsid);
      if (p === undefined || p.encap) return;
      if (st.type === 'l2')
        issues.push({
          pointer: P('steering', i, 'bsid'),
          message: `L2 steering needs an encapsulating policy; ${st.bsid} inserts (encap off)`,
        });
      else if (parseCidr(st.prefix)?.family === 4)
        issues.push({
          pointer: P('steering', i, 'bsid'),
          message: `IPv4 steering needs an encapsulating policy; ${st.bsid} inserts (encap off)`,
        });
    });
    return issues;
  },
};

const steeringUnique: ValidatorDefinition = {
  name: 'routing.srv6-steering-unique',
  domains: ['routing'],
  validate(config) {
    const s = srv6Of(config);
    if (s === undefined) return [];
    const indexed = s.steering.map((st, i) => ({ st, i }));
    return duplicateIssues(
      indexed,
      ({ st }) => (st.type === 'l2' ? `l2|${st.interface}` : `l3|${st.vrf}|${canonicalPrefix(st.prefix) ?? st.prefix}`),
      ({ i }) => ['routing', 'srv6', 'steering', i],
      ({ st }) =>
        st.type === 'l2'
          ? `interface ${st.interface} is steered twice`
          : `prefix ${st.prefix} of VRF '${st.vrf}' is steered twice`,
    );
  },
};

const l2Interface: ValidatorDefinition = {
  name: 'routing.srv6-l2-interface',
  domains: ['routing', 'interfaces'],
  validate(config) {
    const s = srv6Of(config);
    if (s === undefined) return [];
    const issues: SemanticIssue[] = [];
    s.steering.forEach((st, i) => {
      if (st.type !== 'l2') return;
      const entry = lookupInterface(config, st.interface);
      if (isPlainObject(entry)) {
        const v4 = Array.isArray(entry.ipv4) ? entry.ipv4.length : 0;
        const v6 = Array.isArray(entry.ipv6) ? entry.ipv6.length : 0;
        if (v4 + v6 > 0)
          issues.push({
            pointer: P('steering', i, 'interface'),
            message: `interface ${st.interface} has IP addresses; L2 steering switches it to L2 mode — remove them first`,
          });
      }
    });
    return issues;
  },
};

/** Every F-srv6 rule (one spread line in `semantic/index.ts`). */
export const srv6Validators: readonly ValidatorDefinition[] = [
  canonical,
  sidAddress,
  sidUnique,
  behaviorFields,
  vrfsExist,
  interfacesExist,
  encapSource,
  steeringBsid,
  steeringEncap,
  steeringUnique,
  l2Interface,
];
