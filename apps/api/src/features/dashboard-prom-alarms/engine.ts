import type { AlarmMetric, AlarmOp, AlarmRule } from '@ngfw/schema';

/**
 * F-dashboard-prom-alarms: the pure alarm evaluation (no I/O, no clock beyond the `now` passed in), so the hysteresis
 * and raise/clear logic are unit-testable without a database or streams. `AlarmsService` feeds it samples derived from
 * the agent's StreamStats/StreamEvents and persists what it returns.
 */

/** One evaluated metric value for a (rule, instance). instance is "" for a device-wide metric. */
export interface Sample {
  metric: AlarmMetric;
  instance: string;
  value: number;
}

/** Per-(rule,instance) memory the engine keeps between evaluations. */
export interface RuleState {
  /** epoch ms the condition first held continuously; 0 = not currently in violation. */
  since: number;
  /** an alarm is currently raised for this key. */
  raised: boolean;
}

export interface Raise {
  rule: string;
  instance: string;
  metric: AlarmMetric;
  severity: string;
  value: number;
  threshold: number;
}

export interface Transition {
  raises: Raise[];
  clears: { rule: string; instance: string }[];
}

const KEY_SEP = '\u0000';
export function stateKey(rule: string, instance: string): string {
  return rule + KEY_SEP + instance;
}

export function compare(op: AlarmOp, value: number, threshold: number): boolean {
  switch (op) {
    case 'gt':
      return value > threshold;
    case 'ge':
      return value >= threshold;
    case 'lt':
      return value < threshold;
    case 'le':
      return value <= threshold;
    case 'eq':
      return value === threshold;
    default:
      return false;
  }
}

/** A rule matches a sample when the metric matches and, for a per-interface rule, the interface matches. */
function matches(rule: AlarmRule, s: Sample): boolean {
  if (rule.metric !== s.metric) return false;
  if (rule.interface !== undefined && rule.interface !== '' && rule.interface !== s.instance)
    return false;
  return true;
}

/**
 * evaluate advances the alarm state for one batch of samples at time `now` (epoch ms). It returns the raises and
 * clears to persist and mutates `state` in place. A rule with `forSec` raises only once the condition has held that
 * long; when the condition stops holding the alarm clears. Samples not present in this batch for a currently-raised
 * key are left as-is (absence is not "cleared" — the caller re-samples on a fixed cadence and passes a 0 for a metric
 * that is genuinely zero, e.g. link_down).
 */
export function evaluate(
  rules: Record<string, AlarmRule>,
  samples: Sample[],
  now: number,
  state: Map<string, RuleState>,
): Transition {
  const out: Transition = { raises: [], clears: [] };
  for (const [name, rule] of Object.entries(rules)) {
    if (rule.enabled === false) continue;
    for (const s of samples) {
      if (!matches(rule, s)) continue;
      const key = stateKey(name, s.instance);
      const st = state.get(key) ?? { since: 0, raised: false };
      const violating = compare(rule.op, s.value, rule.threshold);
      if (violating) {
        if (st.since === 0) st.since = now;
        const heldMs = now - st.since;
        if (!st.raised && heldMs >= rule.forSec * 1000) {
          st.raised = true;
          out.raises.push({
            rule: name,
            instance: s.instance,
            metric: rule.metric,
            severity: rule.severity,
            value: s.value,
            threshold: rule.threshold,
          });
        }
      } else {
        if (st.raised) out.clears.push({ rule: name, instance: s.instance });
        st.since = 0;
        st.raised = false;
      }
      state.set(key, st);
    }
  }
  return out;
}

/** Drop state for rules that no longer exist (called when the running config changes). Returns keys to clear. */
export function pruneRules(
  rules: Record<string, AlarmRule>,
  state: Map<string, RuleState>,
): { rule: string; instance: string }[] {
  const clears: { rule: string; instance: string }[] = [];
  for (const [key, st] of state) {
    const rule = key.slice(0, key.indexOf(KEY_SEP));
    const instance = key.slice(key.indexOf(KEY_SEP) + 1);
    if (rules[rule] === undefined || rules[rule].enabled === false) {
      if (st.raised) clears.push({ rule, instance });
      state.delete(key);
    }
  }
  return clears;
}
