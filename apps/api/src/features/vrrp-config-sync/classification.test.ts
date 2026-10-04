import { describe, expect, it } from 'vitest';
import { notAppliedChanges } from '../../commit/commit.service.js';
import { driftOf } from '../../state/state.controller.js';
describe('API-managed cluster configuration classification', () => {
  it('reports API cluster changes applied while retaining unapplied VRRP differences', () => {
    expect(notAppliedChanges({ ha: {} }, { ha: { cluster: { enabled: true } } }, ['ha'])).toEqual(
      [],
    );
    expect(
      notAppliedChanges(
        { ha: {} },
        { ha: { cluster: { enabled: true }, vrrp: { lan: { priority: 100 } } } },
        ['ha'],
      ),
    ).toEqual(['ha']);
  });
  it('filters only the API cluster subtree from dataplane drift', () => {
    const changes = [
      { op: 'remove' as const, pointer: '/ha/cluster', from: { enabled: true } },
      { op: 'replace' as const, pointer: '/ha/vrrp/lan/priority', from: 200, to: 100 },
    ];
    const result = driftOf(changes, [], ['ha']);
    expect(result.changes).toEqual([changes[1]]);
    expect(result.ignored).toContainEqual({ pointer: '/ha/cluster', rule: 'api.managed-field' });
  });
});
