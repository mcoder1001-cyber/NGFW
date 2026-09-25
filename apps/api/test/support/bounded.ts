import { afterEach, beforeEach } from 'vitest';

/**
 * Bounded e2e timing (TD-2 verify2 finding T1). Vitest fails a test that runs over its timeout but does not stop its
 * body: a loop of resets or commits keeps running underneath the next tests and the file's `afterAll`.
 */

/** `p`, or a rejection naming `what` after `ms`. */
export async function within<T>(p: Promise<T>, ms: number, what: string): Promise<T> {
  let timer: NodeJS.Timeout | undefined;
  try {
    return await Promise.race([
      p,
      new Promise<never>((_, reject) => {
        timer = setTimeout(() => reject(new Error(`${what}: no answer within ${ms} ms`)), ms);
      }),
    ]);
  } finally {
    clearTimeout(timer);
  }
}

/**
 * Call inside a `describe`: registers `beforeEach`/`afterEach` there and returns `guarded`. A test body wrapped in
 * `guarded` gets its test's AbortSignal; `afterEach` aborts it and waits until the body has returned, so nothing a
 * test started outlives it. Bodies check the signal between runs and inside their loops.
 */
export function guardLongTests() {
  let ac = new AbortController();
  const running = new Set<Promise<unknown>>();
  beforeEach(() => {
    ac = new AbortController();
  });
  afterEach(async () => {
    ac.abort();
    await Promise.allSettled([...running]);
  });
  return (fn: (signal: AbortSignal) => Promise<void>) => () => {
    const p = fn(ac.signal);
    running.add(p);
    void p.then(
      () => running.delete(p),
      () => running.delete(p),
    );
    return p;
  };
}

/**
 * The API's rate limits count in fixed 60 s windows (`TokensService.hit`: bucket `floor(now / 60 s)`). A test that
 * counts to a limit calls this first: when less than `ms` is left in the current window it waits for the next one
 * (at most `ms`), so its whole count lands in one window.
 */
export async function roomInRateWindow(ms: number): Promise<void> {
  const left = 60_000 - (Date.now() % 60_000);
  if (left < ms) await new Promise((r) => setTimeout(r, left + 50));
}

/** `check` over `items` with at most `width` at a time; counts the `true` answers. */
export async function countWhere<T>(
  items: readonly T[],
  check: (item: T) => Promise<boolean>,
  width = 8,
): Promise<number> {
  let next = 0;
  let n = 0;
  const lane = async () => {
    while (next < items.length) {
      const item = items[next++]!;
      if (await check(item)) n += 1;
    }
  };
  await Promise.all(Array.from({ length: Math.min(width, items.length) }, lane));
  return n;
}
