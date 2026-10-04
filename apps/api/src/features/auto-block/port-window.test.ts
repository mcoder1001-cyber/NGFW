import { describe, expect, it } from 'vitest';
import { DistinctPortWindows } from './engine.js';

describe('distinct-port sliding window', () => {
  it('refreshes a repeated port without counting it twice, then trips on a different port', () => {
    const windows = new DistinctPortWindows();
    const observe = (port: number, at: number) =>
      windows.observe('192.0.2.7', port, at, 10, 2, 100);
    expect(observe(22, 0)).toEqual({ count: 1, tripped: false });
    expect(observe(22, 9000)).toEqual({ count: 1, tripped: false });
    expect(observe(443, 11000)).toEqual({ count: 2, tripped: true });
    windows.clear('192.0.2.7');
    expect(observe(443, 12000)).toEqual({ count: 1, tripped: false });
  });
  it('expires the exact boundary and isolates sources', () => {
    const w = new DistinctPortWindows();
    w.observe('192.0.2.7', 22, 0, 10, 2, 100);
    w.observe('192.0.2.8', 22, 9000, 10, 2, 100);
    expect(w.observe('192.0.2.7', 443, 10000, 10, 2, 100)).toEqual({ count: 1, tripped: false });
    expect(w.observe('192.0.2.8', 443, 10000, 10, 2, 100)).toEqual({ count: 2, tripped: true });
  });
  it('bounds sources while reclaiming expired windows for a new source', () => {
    const w = new DistinctPortWindows();
    w.observe('192.0.2.7', 22, 0, 10, 2, 1);
    expect(w.observe('192.0.2.8', 22, 9000, 10, 2, 1)).toEqual({ count: 0, tripped: false });
    expect(w.size).toBe(1);
    expect(w.observe('192.0.2.8', 22, 10000, 10, 2, 1)).toEqual({ count: 1, tripped: false });
    w.prune(10000);
    expect(w.size).toBe(0);
  });
});

it('refuses unachievable thresholds and caps source tracking independently of configured maxEntries', () => {
  const w = new DistinctPortWindows();
  expect(w.observe('192.0.2.7', 22, 0, 10, 100_000, 1_000_000)).toEqual({
    count: 0,
    tripped: false,
  });
  expect(w.size).toBe(0);
  for (let i = 0; i < 10_000; i++) w.observe(`source-${i}`, 22, 0, 10, 2, 1_000_000);
  expect(w.observe('overflow', 443, 0, 10, 2, 1_000_000)).toEqual({ count: 0, tripped: false });
  expect(w.size).toBe(10_000);
  expect(w.observationCount).toBe(10_000);
  w.prune(0);
  expect(w.observationCount).toBe(0);
});

it('caps aggregate observations and frees capacity without inventing threshold hits', () => {
  const w = new DistinctPortWindows();
  for (let source = 0; source < 1000; source++) {
    for (let port = 1; port <= 100; port++) {
      w.observe(`source-${source}`, port, 0, 10, 4096, 1_000_000);
    }
  }
  expect(w.observationCount).toBe(100_000);
  expect(w.observe('overflow', 443, 0, 10, 4096, 1_000_000)).toEqual({ count: 0, tripped: false });
  expect(w.observe('source-0', 101, 0, 10, 4096, 1_000_000)).toEqual({
    count: 100,
    tripped: false,
  });
  expect(w.observe('source-0', 22, 9000, 10, 4096, 1_000_000)).toEqual({
    count: 100,
    tripped: false,
  });
  expect(w.observationCount).toBe(100_000);
  w.clear('source-1');
  expect(w.observationCount).toBe(99_900);
  expect(w.observe('overflow', 443, 9000, 10, 4096, 1_000_000)).toEqual({
    count: 1,
    tripped: false,
  });
  w.prune(9000);
  expect(w.observationCount).toBe(0);
});
