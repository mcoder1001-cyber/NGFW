import { afterEach, describe, expect, it } from 'vitest';
import { confirmStore, formatCountdown } from './confirm-store';

describe('confirm store (local countdown of a confirmed commit)', () => {
  afterEach(() => confirmStore.reset());

  it('formats the remaining time as m:ss and never goes negative', () => {
    expect(formatCountdown(120_000)).toBe('2:00');
    expect(formatCountdown(103_001)).toBe('1:44');
    expect(formatCountdown(59_000)).toBe('0:59');
    expect(formatCountdown(-5)).toBe('0:00');
  });

  it('only moves the local deadline earlier (the countdown errs on the short side) and survives a reload', () => {
    confirmStore.track({ txnId: 't1', deadlineMs: 100_000, kind: 'commit', trackedAt: 1 });
    confirmStore.adjustDeadline('t1', 150_000);
    expect(confirmStore.get().tracked?.deadlineMs).toBe(100_000);
    confirmStore.adjustDeadline('t1', 90_000);
    expect(confirmStore.get().tracked?.deadlineMs).toBe(90_000);
    expect(JSON.parse(sessionStorage.getItem('vrx.confirm')!).tracked.deadlineMs).toBe(90_000);
  });

  it('the first resolution wins: a late poll result does not overwrite this session\'s own confirm', () => {
    confirmStore.track({ txnId: 't1', deadlineMs: 100_000, kind: 'commit', trackedAt: 1 });
    confirmStore.resolve('confirmed', 't1', 7);
    confirmStore.resolve('confirmedElsewhere', 't1', 7);
    expect(confirmStore.get()).toEqual({ tracked: null, outcome: { kind: 'confirmed', txnId: 't1', revision: 7 } });
  });
});
