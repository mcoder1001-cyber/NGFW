import { IssueSeverity, type ValidationIssue } from '@ngfw/proto';
import { diff } from '@ngfw/schema';
import { describe, expect, it } from 'vitest';
import { driftOf } from '../../state/state.controller.js';

/**
 * `agent.write-only` in the drift view's coverage rules (F-rpf-adl-pbr review M3): leaves the agent applies but VPP
 * cannot report (the ADL allow-list binding, Auto-SDL) are not drift; everything else still is.
 */
const note = (pointer: string, rule: string): ValidationIssue => ({
  pointer,
  rule,
  message: '',
  severity: IssueSeverity.ISSUE_SEVERITY_WARNING,
});

describe('driftOf with write-only leaves', () => {
  const running = {
    interfaces: {
      loop1: {
        enabled: true,
        urpf: { ipv4: 'strict', direction: 'rx' },
        adl: { ipv4: true, ipv6: false, allowVrf: 'allow', defaultAllow: true },
      },
    },
    services: {
      autoSdl: { enabled: true, threshold: 5, removeTimeoutSec: 300 },
      snmp: { enabled: false },
    },
  };
  const actual = {
    interfaces: { loop1: { enabled: true, urpf: { ipv4: 'loose', direction: 'rx' }, adl: {} } },
    services: {},
  };
  const report = [
    ...['ipv4', 'ipv6', 'allowVrf', 'defaultAllow'].map((l) =>
      note(`/interfaces/loop1/adl/${l}`, 'agent.write-only'),
    ),
    note('/services/autoSdl', 'agent.write-only'),
    note('/services/snmp', 'agent.unsupported-field'),
  ];

  it('skips the write-only leaves and keeps real drift', () => {
    const d = driftOf(diff(running, actual), report, ['interfaces', 'services']);
    expect(d.changes).toEqual([
      { op: 'replace', pointer: '/interfaces/loop1/urpf/ipv4', from: 'strict', to: 'loose' },
    ]);
    expect(d.ignored.filter((i) => i.rule === 'agent.write-only').map((i) => i.pointer)).toEqual([
      '/interfaces/loop1/adl/ipv4',
      '/interfaces/loop1/adl/ipv6',
      '/interfaces/loop1/adl/allowVrf',
      '/interfaces/loop1/adl/defaultAllow',
      '/services/autoSdl',
    ]);
  });

  it('ADL switched off in VPP is still drift (only the leaves are write-only, not the presence)', () => {
    const off = {
      interfaces: { loop1: { enabled: true, urpf: { ipv4: 'strict', direction: 'rx' } } },
      services: {},
    };
    const d = driftOf(diff(running, off), report, ['interfaces', 'services']);
    expect(d.changes.map((c) => `${c.op} ${c.pointer}`)).toEqual(['remove /interfaces/loop1/adl']);
  });

  it('an error-severity issue with the rule is never a coverage note', () => {
    const err = {
      ...note('/interfaces/loop1/adl/ipv4', 'agent.write-only'),
      severity: IssueSeverity.ISSUE_SEVERITY_ERROR,
    };
    const d = driftOf(diff(running, actual), [err], ['interfaces']);
    expect(d.changes.map((c) => c.pointer)).toContain('/interfaces/loop1/adl/ipv4');
  });
});
