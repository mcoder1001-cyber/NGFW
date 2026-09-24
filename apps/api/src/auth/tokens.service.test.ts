import { randomBytes } from 'node:crypto';
import { describe, expect, it } from 'vitest';
import { testEnv } from '../testing/fixtures.js';
import { Bus } from '../infra/bus.js';
import type { Valkey } from '../infra/valkey.js';
import { TokensService } from './tokens.service.js';

// Revocation itself (Valkey generation + Lua scripts) is covered by the e2e on the host Valkey (td2*.e2e.test.ts).
describe('TokensService access tokens carry the credential generation (D-097)', () => {
  const tokens = new TokensService(
    testEnv({ VRX_JWT_SECRET: randomBytes(24).toString('hex') }),
    {} as Valkey,
    new Bus(),
  );

  it('round-trips gen and sid; gen defaults to 0', async () => {
    const t = await tokens.signAccess({
      id: 7,
      username: 'u',
      role: 'operator',
      sid: 'fam',
      gen: 3,
    });
    expect(await tokens.verifyAccess(t)).toMatchObject({ id: 7, sid: 'fam', gen: 3 });
    const legacy = await tokens.signAccess({ id: 7, username: 'u', role: 'operator' });
    expect(await tokens.verifyAccess(legacy)).toMatchObject({ gen: 0 });
  });
});

describe('TokensService revocation order (TD-2 verify V1/V3)', () => {
  it('Valkey failing after the commit: the in-process revocation and the WebSocket close still happen', async () => {
    const bus = new Bus();
    const closed: unknown[] = [];
    bus.onSessions((e) => closed.push(e));
    const failing = {
      eval: async () => {
        throw new Error('simulated Valkey outage');
      },
    } as unknown as Valkey;
    const t = new TokensService(
      testEnv({ VRX_JWT_SECRET: randomBytes(24).toString('hex') }),
      failing,
      bus,
    );
    const old = await t.signAccess({ id: 9, username: 'v', role: 'operator', sid: 'a', gen: 4 });
    const kept = await t.signAccess({ id: 9, username: 'v', role: 'operator', sid: 'k', gen: 4 });
    expect(await t.revokeUser(9, 5, 'k')).toEqual({ persisted: false, families: 0 });
    expect(await t.verifyAccess(old)).toBeNull();
    expect(await t.verifyAccess(kept)).toMatchObject({ sid: 'k', gen: 4 });
    expect(closed).toEqual([{ userId: 9, exceptSid: 'k' }]);
  });

  it('sessionCurrent: current generation, or the session a self-service change kept — nothing else', async () => {
    const t = new TokensService(
      testEnv({ VRX_JWT_SECRET: randomBytes(24).toString('hex') }),
      { eval: async () => 1, smembers: async () => [] } as unknown as Valkey,
      new Bus(),
    );
    expect(t.sessionCurrent({ id: 3, gen: 2 }, 2)).toBe(true);
    expect(t.sessionCurrent({ id: 3, gen: 1 }, 2)).toBe(false);
    expect(t.sessionCurrent({ id: 3 }, 0)).toBe(false);
    await t.revokeUser(3, 2, 'mine');
    expect(t.sessionCurrent({ id: 3, gen: 1, sid: 'mine' }, 2)).toBe(true);
    expect(t.sessionCurrent({ id: 3, gen: 1, sid: 'other' }, 2)).toBe(false);
    // a later admin reset (no kept session) supersedes it
    await t.revokeUser(3, 3);
    expect(t.sessionCurrent({ id: 3, gen: 1, sid: 'mine' }, 3)).toBe(false);
  });
});
