import { sql } from 'drizzle-orm';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { appUser } from '../../src/db/schema.js';
import { startHarness, type Harness } from '../support/harness.js';

/** F-aaa-hardening 3: app_user names are unique case-insensitively (migration 0009, index on lower(username)). */
describe('F-aaa-hardening: case-insensitive username uniqueness (PostgreSQL)', () => {
  let h: Harness;
  beforeAll(async () => {
    h = await startHarness();
  });
  afterAll(async () => h?.close());

  it('a second row differing only in case is refused by the index', async () => {
    const [idx] = (
      await h.db.execute(
        sql`select indexdef from pg_indexes where indexname = 'app_user_username_lower_uq'`,
      )
    ).rows as { indexdef: string }[];
    expect(idx?.indexdef).toMatch(/UNIQUE INDEX .* \(lower\(username\)\)/);
    await h.db.insert(appUser).values({ username: 'w9case', role: 'readonly', source: 'external' });
    await expect(
      h.db.insert(appUser).values({ username: 'W9Case', role: 'readonly', source: 'external' }),
    ).rejects.toThrow();
    await h.db.delete(appUser).where(sql`lower(${appUser.username}) = 'w9case'`);
  });
});
