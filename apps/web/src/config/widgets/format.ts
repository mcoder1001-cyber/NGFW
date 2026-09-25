import type { Formatters } from '@ngfw/ui-kit';

/** A 64-bit counter as the API sends it: a decimal string (D-039), or a number from a small table. */
export type CounterValue = string | number | bigint | null | undefined;

function toBigInt(v: CounterValue): bigint | undefined {
  if (v === null || v === undefined || v === '') return undefined;
  try {
    if (typeof v === 'bigint') return v;
    if (typeof v === 'number') return Number.isFinite(v) ? BigInt(Math.trunc(v)) : undefined;
    return /^-?[0-9]+$/.test(v.trim()) ? BigInt(v.trim()) : undefined;
  } catch {
    return undefined;
  }
}

/**
 * A counter with grouping in the UI language, exact beyond 2^53 (a `Number()` of a 64-bit decimal string is not).
 * Latin digits are formatted and then converted by `fmt.digits`, so the Persian-digits user option applies.
 */
export function formatCounter(fmt: Formatters, v: CounterValue): string {
  const b = toBigInt(v);
  if (b === undefined) return '';
  if (b <= BigInt(Number.MAX_SAFE_INTEGER) && b >= BigInt(Number.MIN_SAFE_INTEGER)) return fmt.integer(Number(b));
  const base = fmt.locale.split('-u-')[0] ?? 'en';
  return fmt.digits(new Intl.NumberFormat(`${base}-u-nu-latn`).format(b));
}

/** Sum of counters (e.g. errors + drops), exact; absent members count as 0. */
export function sumCounters(...values: CounterValue[]): bigint {
  return values.reduce<bigint>((acc, v) => acc + (toBigInt(v) ?? 0n), 0n);
}
