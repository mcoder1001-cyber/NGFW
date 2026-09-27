import { describe, expect, it } from 'vitest';
import type { Db } from '../../db/db.js';
import { aaaMfa, apiKey } from '../../db/schema.js';
import { MfaService } from './mfa.service.js';

/** S-aaa-key-reset: a factor reset clears mfa_verified on the user's API keys, inside the same transaction. */
function fakeDb(hasFactor: boolean, keys: number) {
  const log: string[] = [];
  const tx = {
    delete: (t: unknown) => ({
      where: () => {
        const rows = t === aaaMfa && hasFactor ? [{ id: 7 }] : [];
        log.push(t === aaaMfa ? 'delete factor' : 'delete recovery');
        return Object.assign(Promise.resolve(rows), { returning: () => Promise.resolve(rows) });
      },
    }),
    update: (t: unknown) => ({
      set: (v: unknown) => ({
        where: () => ({
          returning: () => {
            log.push(`update ${t === apiKey ? 'api_key' : '?'} ${JSON.stringify(v)}`);
            return Promise.resolve(Array.from({ length: keys }, (_, i) => ({ id: `k${i}` })));
          },
        }),
      }),
    }),
  };
  const db = {
    transaction: async <T>(fn: (t: typeof tx) => Promise<T>) => {
      log.push('begin');
      const r = await fn(tx);
      log.push('commit');
      return r;
    },
  };
  return { db: db as unknown as Db, log };
}

describe('MfaService.reset', () => {
  it('clears mfa_verified on the owner’s keys in the factor-delete transaction', async () => {
    const { db, log } = fakeDb(true, 2);
    expect(await new MfaService(db, {} as never).reset(7)).toBe(2);
    expect(log).toEqual([
      'begin',
      'delete recovery',
      'delete factor',
      'update api_key {"mfaVerified":false}',
      'commit',
    ]);
  });
  it('no factor → null, keys untouched', async () => {
    const { db, log } = fakeDb(false, 2);
    expect(await new MfaService(db, {} as never).reset(7)).toBeNull();
    expect(log.some((l) => l.startsWith('update'))).toBe(false);
  });
});
