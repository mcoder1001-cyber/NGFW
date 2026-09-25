import { describe, expect, it } from 'vitest';
import { Mutex } from '../common/mutex.js';
import { ProblemError } from '../common/problem.js';
import { withinCommitLock } from './commit-busy.js';

/** TD-10b (manager addendum, TD-10a review M2): a password set waits ≤ 1 s for the commit lock, then 409. */
const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

describe('withinCommitLock', () => {
  it('lock held longer than the wait → 409 commit-busy after ~the wait, and the queued section never runs', async () => {
    const m = new Mutex();
    const holder = m.run(() => sleep(600));
    let ran = false;
    const t0 = Date.now();
    const err = await withinCommitLock(
      (f) => m.run(f),
      async () => {
        ran = true;
        return 'changed';
      },
      200,
    ).catch((e: unknown) => e);
    const waited = Date.now() - t0;
    expect(err).toBeInstanceOf(ProblemError);
    expect((err as ProblemError).getStatus()).toBe(409);
    expect((err as ProblemError).body()).toMatchObject({
      type: 'https://vrx.dev/problems/commit-busy',
      retryAfterSec: 2,
    });
    expect(waited).toBeGreaterThanOrEqual(190);
    expect(waited).toBeLessThan(550);
    await holder;
    await m.run(async () => undefined); // the abandoned section had its turn before this one
    expect(ran).toBe(false);
  });

  it('a free lock (or one freed within the wait) runs the section and returns its result', async () => {
    const m = new Mutex();
    expect(
      await withinCommitLock(
        (f) => m.run(f),
        async () => 1,
        200,
      ),
    ).toBe(1);
    const holder = m.run(() => sleep(50));
    expect(
      await withinCommitLock(
        (f) => m.run(f),
        async () => 2,
        500,
      ),
    ).toBe(2);
    await holder;
  });

  it('a section that started before the deadline is awaited to its end (never abandoned half-way)', async () => {
    const m = new Mutex();
    const r = await withinCommitLock(
      (f) => m.run(f),
      async () => {
        await sleep(300);
        return 'done';
      },
      100,
    );
    expect(r).toBe('done');
  });

  it('the section’s own error is the answer', async () => {
    const m = new Mutex();
    await expect(
      withinCommitLock(
        (f) => m.run(f),
        async () => {
          throw new Error('boom');
        },
        100,
      ),
    ).rejects.toThrow('boom');
  });
});
