import { DesiredState, IssueSeverity, type ValidationIssue } from '@ngfw/proto';
import { diff, RootConfig } from '@ngfw/schema';
import { describe, expect, it } from 'vitest';
import { canonicalDriftDocument, driftOf } from './state.controller.js';

const canonical = (doc: unknown) => canonicalDriftDocument(DesiredState.fromJSON(doc));
const note = (pointer: string): ValidationIssue => ({
  pointer,
  rule: 'agent.unsupported-field',
  message: '',
  severity: IssueSeverity.ISSUE_SEVERITY_WARNING,
});

describe('sparse Retrieve drift defaults', () => {
  it('treats a default document and a sparse empty Retrieve as equivalent', () => {
    expect(diff(canonical(RootConfig.parse({})), canonical({}))).toEqual([]);
  });

  it('filters API-owned management leaves after filling absent defaults', () => {
    const running = RootConfig.parse({
      management: { users: [{ username: 'admin', role: 'admin' }] },
    });
    const result = driftOf(
      diff(canonical(running), canonical({})),
      [note('/management/users')],
      ['management'],
    );
    expect(result.changes).toEqual([]);
  });

  it('keeps enabled SNMP disappearing and changed NAT settings visible', () => {
    const running = canonical({ services: { snmp: { enabled: true } }, nat: { forwarding: true } });
    const changes = driftOf(diff(running, canonical({})), [], ['nat', 'services']).changes;
    expect(changes.map((c) => c.pointer)).toContain('/services/snmp/enabled');
    expect(changes.map((c) => c.pointer)).toContain('/nat/forwarding');
  });

  it('does not replace an invalid retrieved document with an empty default', () => {
    const invalid = DesiredState.fromJSON({ nat: { mode: 'invalid' } });
    expect(canonicalDriftDocument(invalid)).toEqual(DesiredState.toJSON(invalid));
  });
});
