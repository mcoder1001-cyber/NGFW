import { describe, expect, it } from 'vitest';
import { RootConfig as RootSchema } from '../index.js';
import { raVpnValidators } from './ra-vpn.js';
import { validateSemantics } from './index.js';

const profile = (enabled: boolean) => ({
  enabled, localAddr: '192.0.2.1', vrf: 'default', underlayVrf: 'default',
  certificate: 'server', proposal: 'native', pools: [{ name: 'clients', prefix: '10.19.200.0/24' }],
  users: [{ username: 'client', passwordRef: 'password/client' }],
});

describe('native remote-access capability gate', () => {
  it('rejects enabled profiles with exact escaped pointers without exposing credentials', () => {
    const config = RootSchema.parse({ vpn: { remoteAccess: { roadwarrior: profile(true) } } });
    expect(raVpnValidators[0]!.validate(config)).toEqual([
      { pointer: '/vpn/remoteAccess/roadwarrior/enabled', message: expect.stringContaining('unavailable') },
    ]);
    expect(JSON.stringify(raVpnValidators[0]!.validate(config))).not.toContain('password/client');
    expect(validateSemantics(config).some((issue) => issue.pointer === '/vpn/remoteAccess/roadwarrior/enabled')).toBe(true);
  });
  it('keeps disabled drafts parseable and does not reject unrelated VPN configurations', () => {
    const config = RootSchema.parse({ vpn: { remoteAccess: { draft: profile(false) } } });
    expect(raVpnValidators[0]!.validate(config)).toEqual([]);
    expect(raVpnValidators[0]!.validate(RootSchema.parse({}))).toEqual([]);
  });
});
