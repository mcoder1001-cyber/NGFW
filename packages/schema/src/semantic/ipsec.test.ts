import { describe, expect, it } from 'vitest';
import { RootConfig } from '../index.js';
import { ipsecValidators } from './ipsec.js';
import { sortIssues } from './registry.js';
import { BASE } from './vpn.fixtures.js';

const proposal = {
  ike: { encr: 'aes256gcm16', prf: 'prfsha256', dh: 'curve25519' },
  esp: { encr: 'aes256gcm16' },
};
const tunnel = (remoteAddr: string, extra: Record<string, unknown> = {}) => ({
  localAddr: '198.51.100.2',
  remoteAddr,
  localId: '@local.example',
  remoteId: `@peer-${remoteAddr}.example`,
  routeBased: { ipipInterface: 'ipip0' },
  auth: { method: 'psk', secretRef: `psk/${remoteAddr}` },
  proposal: 'p1',
  localTs: ['10.1.0.0/16'],
  remoteTs: ['10.2.0.0/16'],
  ...extra,
});
const doc = (
  tunnels: Record<string, unknown>,
  proposals: Record<string, unknown> = { p1: proposal },
) => ({
  ...BASE,
  vpn: { ipsec: { proposals, tunnels } },
});
const run = (d: unknown) =>
  sortIssues(ipsecValidators.flatMap((v) => v.validate(RootConfig.parse(d))));

describe('P11 semantic rule vpn.ipsec-psk-identity-unique', () => {
  it('is the only P11 rule, prefixed vpn. and reading vpn', () => {
    expect(ipsecValidators.map((v) => v.name)).toEqual(['vpn.ipsec-psk-identity-unique']);
    expect(ipsecValidators[0]?.domains).toContain('vpn');
  });

  it('accepts PSK tunnels with distinct identity pairs', () => {
    expect(run(doc({ a: tunnel('203.0.113.1'), b: tunnel('203.0.113.2') }))).toEqual([]);
  });

  it('reports the second tunnel of a duplicate identity pair at its secretRef', () => {
    const issues = run(
      doc({
        a: tunnel('203.0.113.1'),
        b: tunnel('203.0.113.9', { remoteId: '@peer-203.0.113.1.example' }),
      }),
    );
    expect(issues).toHaveLength(1);
    expect(issues[0]?.pointer).toBe('/vpn/ipsec/tunnels/b/auth/secretRef');
  });

  it('ignores disabled and certificate tunnels', () => {
    expect(
      run(
        doc({
          a: tunnel('203.0.113.1'),
          b: tunnel('203.0.113.1', { enabled: false }),
          d: tunnel('203.0.113.1', { auth: { method: 'cert', certificate: 'c1' } }),
        }),
      ),
    ).toEqual([]);
  });
});

describe('P11 name caps (D-089)', () => {
  const name = (n: number) => 'x'.repeat(n);
  it('caps tunnel names at 60 characters', () => {
    expect(RootConfig.safeParse(doc({ [name(60)]: tunnel('203.0.113.1') })).success).toBe(true);
    const r = RootConfig.safeParse(doc({ [name(61)]: tunnel('203.0.113.1') }));
    expect(r.success).toBe(false);
  });
  it('keeps proposal names at objectName (63): they never become charon sections', () => {
    const t = tunnel('203.0.113.1', { proposal: name(63) });
    expect(RootConfig.safeParse(doc({ a: t }, { [name(63)]: proposal })).success).toBe(true);
    expect(RootConfig.safeParse(doc({ a: t }, { [name(64)]: proposal })).success).toBe(false);
  });
});
