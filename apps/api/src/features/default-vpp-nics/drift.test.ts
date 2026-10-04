import { IssueSeverity } from '@ngfw/proto';
import { describe, expect, it } from 'vitest';
import { driftOf } from '../../state/state.controller.js';

describe('F-default-vpp-nics: seeded NICs the data plane does not have are not drift', () => {
  it('ignores agent.nic-not-bound / agent.nic-released rows, keeps other interface drift', () => {
    const changes = [
      { op: 'remove', pointer: '/interfaces/ens161', from: { enabled: true } },
      { op: 'remove', pointer: '/interfaces/ens193', from: { enabled: true } },
      { op: 'remove', pointer: '/interfaces/loop0', from: { enabled: true } },
    ] as Parameters<typeof driftOf>[0];
    const report = [
      {
        pointer: '/interfaces/ens161',
        rule: 'agent.nic-not-bound',
        message: '',
        severity: IssueSeverity.ISSUE_SEVERITY_WARNING,
      },
      {
        pointer: '/interfaces/ens193',
        rule: 'agent.nic-released',
        message: '',
        severity: IssueSeverity.ISSUE_SEVERITY_INFO,
      },
    ];
    const r = driftOf(changes, report, ['interfaces']);
    expect(r.changes.map((c) => c.pointer)).toEqual(['/interfaces/loop0']);
  });
});
