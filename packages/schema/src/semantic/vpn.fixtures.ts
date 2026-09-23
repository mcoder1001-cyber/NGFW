import { readFileSync } from 'node:fs';
import { RootConfig } from '../index.js';
import { haValidators } from './ha.js';
import { sortIssues, type SemanticIssue } from './registry.js';
import { servicesValidators } from './services.js';
import { tunnelsValidators } from './tunnels.js';
import { vpnValidators } from './vpn.js';

/** Test fixtures shared by the group-(c) semantic tests (`vpn`, `tunnels`, `services`, `ha`). Owner: P02c. */

const GROUP_C = [...vpnValidators, ...tunnelsValidators, ...servicesValidators, ...haValidators];

/** Interfaces/VRFs in the documented P02a shape (docs/04, P02 prompt §2), as in `examples/*.json`. */
export const BASE = {
  vrfs: { default: { id: 0 }, 'customer-a': { id: 10 } },
  interfaces: {
    'TenGigabitEthernet0/0/0': {
      ipv4: ['198.51.100.2/30'],
      ipv6: ['2001:db8:0:1::2/64'],
      vrf: 'default',
    },
    'TenGigabitEthernet0/0/1': {
      ipv4: ['192.168.10.1/24'],
      ipv6: ['2001:db8:10::1/64'],
      vrf: 'default',
    },
    'TenGigabitEthernet0/0/2': { ipv4: ['10.20.0.1/24'], vrf: 'customer-a' },
  },
};

/** Run the group-(c) validators (optionally only `name`) on a document and return the sorted issues. */
export function run(doc: unknown, name?: string): SemanticIssue[] {
  const config = RootConfig.parse(doc);
  return sortIssues(
    GROUP_C.filter((v) => name === undefined || v.name === name).flatMap((v) => v.validate(config)),
  );
}

/** Parse `packages/schema/examples/<file>`. */
export function example(file: string): unknown {
  return JSON.parse(readFileSync(new URL(`../../examples/${file}`, import.meta.url), 'utf8'));
}
