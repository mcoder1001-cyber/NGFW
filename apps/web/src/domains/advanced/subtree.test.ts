import { describe, expect, it } from 'vitest';
import { absolutePointer, inSubtree, notAppliedForDomain, pointerInSubtree } from './subtree';

describe('pointerInSubtree', () => {
  it('matches the pointer itself, a descendant, and an ancestor', () => {
    expect(pointerInSubtree('/interfaces/eth0', '/interfaces/eth0')).toBe(true);
    expect(pointerInSubtree('/interfaces/eth0/mtu', '/interfaces/eth0')).toBe(true);
    expect(pointerInSubtree('/interfaces', '/interfaces/eth0')).toBe(true);
  });
  it('does not match an unrelated pointer, including a sibling with a shared prefix', () => {
    expect(pointerInSubtree('/routing', '/interfaces/eth0')).toBe(false);
    expect(pointerInSubtree('/interfaces/eth0x', '/interfaces/eth0')).toBe(false);
  });
});

describe('absolutePointer', () => {
  it('escapes `/` and `~` in a segment (D-049)', () => {
    expect(absolutePointer('interfaces', ['TenGigabitEthernet0/0/0', 'mtu'])).toBe('/interfaces/TenGigabitEthernet0~10~10/mtu');
  });
});

describe('notAppliedForDomain', () => {
  it('is true only when the domain is named in the summary', () => {
    expect(notAppliedForDomain(['routing', 'ha'], 'routing')).toBe(true);
    expect(notAppliedForDomain(['routing'], 'interfaces')).toBe(false);
    expect(notAppliedForDomain([], 'interfaces')).toBe(false);
  });
});

describe('inSubtree (agent.unsupported-field warnings and diff changes scoped to a subtree)', () => {
  const items = [
    { pointer: '/interfaces/eth0/mtu', rule: 'agent.unsupported-field' },
    { pointer: '/routing', rule: 'agent.unimplemented-domain' },
    { pointer: '/interfaces', rule: 'note' },
  ];
  it('keeps items at, under or above the base pointer', () => {
    expect(inSubtree(items, '/interfaces/eth0')).toEqual([
      { pointer: '/interfaces/eth0/mtu', rule: 'agent.unsupported-field' },
      { pointer: '/interfaces', rule: 'note' },
    ]);
    expect(inSubtree(items, '/routing')).toEqual([{ pointer: '/routing', rule: 'agent.unimplemented-domain' }]);
  });
});
