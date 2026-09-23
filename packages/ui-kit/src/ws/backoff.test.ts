import { describe, expect, it } from 'vitest';
import { backoffDelay, DEFAULT_BACKOFF } from './backoff.js';

describe('backoffDelay', () => {
  it('grows exponentially, is capped and jittered upwards only', () => {
    const noJitter = () => 0;
    expect(backoffDelay(0, DEFAULT_BACKOFF, noJitter)).toBe(500);
    expect(backoffDelay(1, DEFAULT_BACKOFF, noJitter)).toBe(1000);
    expect(backoffDelay(3, DEFAULT_BACKOFF, noJitter)).toBe(4000);
    expect(backoffDelay(20, DEFAULT_BACKOFF, noJitter)).toBe(30_000);
    const jittered = backoffDelay(2, DEFAULT_BACKOFF, () => 1);
    expect(jittered).toBe(2400);
  });
});
