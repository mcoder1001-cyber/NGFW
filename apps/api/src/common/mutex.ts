/** Serialises async sections in this process (commit / confirm / rollback never interleave). */
export class Mutex {
  private tail: Promise<unknown> = Promise.resolve();

  run<T>(fn: () => Promise<T>): Promise<T> {
    const p = this.tail.then(fn, fn);
    this.tail = p.catch(() => undefined);
    return p;
  }
}

/** The lock was not free within the wait the caller allowed (TD-10a, review 2.4a: no unbounded queue). */
export class LockBusyError extends Error {
  constructor(readonly holder: 'process' | 'other-process') {
    super(
      holder === 'process'
        ? 'the commit lock is held in this process'
        : 'the commit lock is held by another API process',
    );
    this.name = 'LockBusyError';
  }
}

/** Releases a held cross-process lock; never throws. */
export type Release = () => Promise<void>;

/**
 * The cross-process half of `CommitLock` (TD-10a, review 2.4b, D-111 follow-up): the product implementation is a
 * PostgreSQL session advisory lock (`commit/pg-lock.ts`).
 */
export interface ProcessLock {
  /** Wait until held. */
  acquire(): Promise<Release>;
  /** Try for up to `waitMs`; null when another process still holds it. */
  tryAcquire(waitMs: number): Promise<Release | null>;
}

/**
 * The commit-engine lock: FIFO inside this process, plus (when given) a lock that other API processes on the same
 * database respect. `run` waits (internal work: reconcile, confirm watcher, password set); `tryRun` gives up after
 * `waitMs` with `LockBusyError` (user commits, rollbacks and confirms answer 409 commit-busy instead of queueing
 * behind a commit that may take the whole server budget).
 */
export class CommitLock {
  private held = false;
  private readonly waiters: (() => void)[] = [];

  constructor(private readonly cross?: ProcessLock) {}

  /** True while a section runs (or is being handed to the next waiter). */
  get busy(): boolean {
    return this.held;
  }

  run<T>(fn: () => Promise<T>): Promise<T> {
    return this.section(fn, undefined);
  }

  tryRun<T>(fn: () => Promise<T>, waitMs: number): Promise<T> {
    return this.section(fn, waitMs);
  }

  private async section<T>(fn: () => Promise<T>, waitMs: number | undefined): Promise<T> {
    const t0 = Date.now();
    if (!(await this.acquireLocal(waitMs))) throw new LockBusyError('process');
    let release: Release | undefined;
    try {
      if (this.cross) {
        const r =
          waitMs === undefined
            ? await this.cross.acquire()
            : await this.cross.tryAcquire(Math.max(0, waitMs - (Date.now() - t0)));
        if (r === null) throw new LockBusyError('other-process');
        release = r;
      }
    } catch (e) {
      this.releaseLocal();
      throw e;
    }
    try {
      return await fn();
    } finally {
      await release?.();
      this.releaseLocal();
    }
  }

  private acquireLocal(waitMs: number | undefined): Promise<boolean> {
    if (!this.held) {
      this.held = true;
      return Promise.resolve(true);
    }
    return new Promise((resolve) => {
      let timer: NodeJS.Timeout | undefined;
      const waiter = () => {
        if (timer) clearTimeout(timer);
        resolve(true);
      };
      this.waiters.push(waiter);
      if (waitMs !== undefined) {
        timer = setTimeout(() => {
          const i = this.waiters.indexOf(waiter);
          if (i >= 0) this.waiters.splice(i, 1);
          resolve(false);
        }, waitMs);
      }
    });
  }

  /** Hand the lock to the next waiter (it stays held), or free it. */
  private releaseLocal(): void {
    const next = this.waiters.shift();
    if (next) next();
    else this.held = false;
  }
}
