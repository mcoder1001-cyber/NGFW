import type { AlarmRule } from '@ngfw/schema';
import { describe, expect, it } from 'vitest';
import { compare, evaluate, pruneRules, stateKey, type RuleState, type Sample } from './engine.js';

const rule = (over: Partial<AlarmRule> = {}): AlarmRule => ({
  metric: 'worker_cpu_percent',
  op: 'gt',
  threshold: 90,
  forSec: 0,
  severity: 'warning',
  enabled: true,
  targets: [],
  ...over,
});

describe('compare', () => {
  it('handles every operator', () => {
    expect(compare('gt', 2, 1)).toBe(true);
    expect(compare('ge', 1, 1)).toBe(true);
    expect(compare('lt', 1, 2)).toBe(true);
    expect(compare('le', 1, 1)).toBe(true);
    expect(compare('eq', 1, 1)).toBe(true);
    expect(compare('gt', 1, 1)).toBe(false);
  });
});

describe('evaluate', () => {
  it('raises immediately with forSec 0 and clears when back under', () => {
    const state = new Map<string, RuleState>();
    const rules = { cpu: rule() };
    const hot: Sample[] = [{ metric: 'worker_cpu_percent', instance: '', value: 95 }];
    let t = evaluate(rules, hot, 1000, state);
    expect(t.raises).toEqual([expect.objectContaining({ rule: 'cpu', value: 95, threshold: 90 })]);
    // still hot: no duplicate raise
    t = evaluate(rules, hot, 2000, state);
    expect(t.raises).toEqual([]);
    // cools: clear
    t = evaluate(rules, [{ metric: 'worker_cpu_percent', instance: '', value: 10 }], 3000, state);
    expect(t.clears).toEqual([{ rule: 'cpu', instance: '' }]);
  });

  it('honours forSec hysteresis (raises only after the condition holds long enough)', () => {
    const state = new Map<string, RuleState>();
    const rules = { cpu: rule({ forSec: 5 }) };
    const hot: Sample[] = [{ metric: 'worker_cpu_percent', instance: '', value: 95 }];
    expect(evaluate(rules, hot, 100_000, state).raises).toEqual([]); // starts the timer
    expect(evaluate(rules, hot, 104_000, state).raises).toEqual([]); // 4s < 5s
    expect(evaluate(rules, hot, 105_000, state).raises.length).toBe(1); // 5s: raise
    // a dip before the window resets the timer
    const s2 = new Map<string, RuleState>();
    evaluate(rules, hot, 100_000, s2);
    evaluate(rules, [{ metric: 'worker_cpu_percent', instance: '', value: 1 }], 103_000, s2);
    expect(evaluate(rules, hot, 106_000, s2).raises).toEqual([]); // timer restarted at 106000
    expect(evaluate(rules, hot, 111_000, s2).raises.length).toBe(1);
  });

  it('is per-interface and respects the interface filter', () => {
    const state = new Map<string, RuleState>();
    const rules = {
      drops: rule({ metric: 'interface_rx_drops', op: 'gt', threshold: 0, interface: 'wan0' }),
    };
    const t = evaluate(
      rules,
      [
        { metric: 'interface_rx_drops', instance: 'wan0', value: 5 },
        { metric: 'interface_rx_drops', instance: 'lan0', value: 9 }, // filtered out
      ],
      100,
      state,
    );
    expect(t.raises).toEqual([expect.objectContaining({ instance: 'wan0' })]);
  });

  it('skips disabled rules', () => {
    const state = new Map<string, RuleState>();
    expect(
      evaluate(
        { cpu: rule({ enabled: false }) },
        [{ metric: 'worker_cpu_percent', instance: '', value: 99 }],
        1,
        state,
      ).raises,
    ).toEqual([]);
  });
});

describe('pruneRules', () => {
  it('clears and drops state for removed or disabled rules', () => {
    const state = new Map<string, RuleState>([
      [stateKey('gone', 'wan0'), { since: 1, raised: true }],
      [stateKey('cpu', ''), { since: 1, raised: true }],
    ]);
    const clears = pruneRules({ cpu: rule() }, state);
    expect(clears).toEqual([{ rule: 'gone', instance: 'wan0' }]);
    expect(state.has(stateKey('gone', 'wan0'))).toBe(false);
    expect(state.has(stateKey('cpu', ''))).toBe(true);
  });
});
