import { IssueSeverity } from '@ngfw/proto';
import { describe, expect, it } from 'vitest';
import { driftOf } from '../../state/state.controller.js';

describe('D-147: write-only host-stack leaves are not drift', () => {
  it('ignores agent.write-only-field pointers, keeps session-rule drift', () => {
    const changes = [
      { op: 'remove', pointer: '/services/hostStack/enabled', from: true },
      { op: 'remove', pointer: '/services/hostStack/namespaces', from: { a: { vrf: 'default' } } },
      { op: 'remove', pointer: '/services/hostStack/sessionRules/0', from: { tag: 'x' } },
    ] as Parameters<typeof driftOf>[0];
    const report = ['enabled', 'namespaces'].map((l) => ({
      pointer: `/services/hostStack/${l}`,
      rule: 'agent.write-only-field',
      message: '',
      severity: IssueSeverity.ISSUE_SEVERITY_WARNING,
    }));
    const r = driftOf(changes, report, ['services']);
    expect(r.changes.map((c) => c.pointer)).toEqual(['/services/hostStack/sessionRules/0']);
  });
});
