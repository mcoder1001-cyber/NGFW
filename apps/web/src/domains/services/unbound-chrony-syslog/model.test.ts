import { describe, expect, it } from 'vitest';
import {
  fieldKey,
  ntpSchema,
  resolverSchema,
  seconds,
  severityColor,
  syslogTargetSchema,
  vppCacheSchema,
} from './model';
import { replacePatch } from './queries';

describe('F-unbound-chrony-syslog UI model', () => {
  it('replacePatch turns the old object into exactly the new one under RFC 7396', () => {
    const before = {
      listen: [{ address: '10.0.0.1', port: 53 }],
      description: 'old',
      dnssec: { enabled: true, trustAnchorAuto: true },
    };
    const after = {
      listen: [{ address: '10.0.0.2', port: 53 }],
      dnssec: { enabled: false, trustAnchorAuto: true },
    };
    expect(replacePatch(before, after)).toEqual({
      listen: [{ address: '10.0.0.2', port: 53 }],
      dnssec: { enabled: false, trustAnchorAuto: true },
      description: null,
    });
    expect(replacePatch(undefined, after)).toEqual(after);
  });

  it('extracts the sub-schemas of the one schema (with the D-086 syslog keys)', () => {
    expect(Object.keys(resolverSchema().properties ?? {})).toEqual(
      expect.arrayContaining(['listen', 'forwarders', 'localZones']),
    );
    expect(Object.keys(vppCacheSchema().properties ?? {})).toEqual(['enabled', 'upstreams']);
    expect(Object.keys(ntpSchema().properties ?? {})).toEqual(
      expect.arrayContaining(['enabled', 'servers', 'allow']),
    );
    expect(Object.keys(syslogTargetSchema().properties ?? {})).toEqual(
      expect.arrayContaining(['address', 'facilities', 'format', 'queueSize', 'tls']),
    );
  });

  it('formats labels, offsets and severities', () => {
    expect(fieldKey('resolver', 'dnssec.enabled')).toBe('field.resolver.dnssec_enabled');
    expect(seconds(0.000012)).toBe('12.0 µs');
    expect(seconds(-0.0021)).toBe('-2.100 ms');
    expect(severityColor('critical')).toBe('error');
    expect(severityColor('warning')).toBe('warning');
    expect(severityColor('debug')).toBe('default');
  });
});
