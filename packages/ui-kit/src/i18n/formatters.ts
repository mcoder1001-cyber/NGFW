/**
 * Locale-aware formatting through `Intl` only (no date library). Persian digits are a user option:
 * when on, numbers and dates use the `arabext` numbering system and `digits()` converts Latin digits in
 * arbitrary strings (pre-formatted counters, interface names are left alone by callers).
 */

export type VrxCalendar = 'gregory' | 'persian';

export interface FormatterOptions {
  /** BCP 47 tag, e.g. 'en' or 'fa'. */
  locale: string;
  /** Render digits as Persian (۰–۹). Only meaningful for fa but honoured for any locale. */
  persianDigits?: boolean;
  /** Calendar for dates; defaults to persian for `fa`, gregory otherwise. */
  calendar?: VrxCalendar;
  /** IANA time zone; defaults to the browser's. */
  timeZone?: string;
}

export interface Formatters {
  readonly locale: string;
  /** Formats a number; `undefined`/`null`/NaN → '' . */
  number(value: number | null | undefined, opts?: Intl.NumberFormatOptions): string;
  /** Integer with grouping (counters). */
  integer(value: number | null | undefined): string;
  /** 0..1 → percent. */
  percent(ratio: number | null | undefined, fractionDigits?: number): string;
  /** Bytes/s or bits/s with SI prefixes, e.g. "1.2 Gbit/s". */
  rate(value: number | null | undefined, unit: 'bps' | 'pps'): string;
  date(value: Date | number | string | null | undefined, opts?: Intl.DateTimeFormatOptions): string;
  dateTime(value: Date | number | string | null | undefined): string;
  time(value: Date | number | string | null | undefined): string;
  /** Relative time ("3 minutes ago"), from `Intl.RelativeTimeFormat`. */
  relative(value: Date | number | string | null | undefined, now?: Date): string;
  /** Converts ASCII digits in a string to Persian digits when the option is on; identity otherwise. */
  digits(text: string): string;
}

const PERSIAN_DIGITS = ['۰', '۱', '۲', '۳', '۴', '۵', '۶', '۷', '۸', '۹'] as const;

export function toPersianDigits(text: string): string {
  return text.replace(/[0-9]/g, (d) => PERSIAN_DIGITS[Number(d)]!);
}

function toDate(v: Date | number | string): Date {
  return v instanceof Date ? v : new Date(v);
}

function withExtensions(locale: string, persianDigits: boolean, calendar?: VrxCalendar): string {
  const base = locale.split('-u-')[0]!;
  const ext: string[] = [];
  ext.push(`nu-${persianDigits ? 'arabext' : 'latn'}`);
  if (calendar) ext.push(`ca-${calendar}`);
  return `${base}-u-${ext.join('-')}`;
}

const SI = [
  { factor: 1e12, prefix: 'T' },
  { factor: 1e9, prefix: 'G' },
  { factor: 1e6, prefix: 'M' },
  { factor: 1e3, prefix: 'k' },
] as const;

export function createFormatters({
  locale,
  persianDigits = false,
  calendar,
  timeZone,
}: FormatterOptions): Formatters {
  const lang = locale.split('-')[0]!;
  const cal: VrxCalendar = calendar ?? (lang === 'fa' ? 'persian' : 'gregory');
  const numLocale = withExtensions(locale, persianDigits);
  const dateLocale = withExtensions(locale, persianDigits, cal);
  const tz = timeZone === undefined ? {} : { timeZone };
  const numberCache = new Map<string, Intl.NumberFormat>();
  const dateCache = new Map<string, Intl.DateTimeFormat>();
  const nf = (opts: Intl.NumberFormatOptions = {}) => {
    const k = JSON.stringify(opts);
    let f = numberCache.get(k);
    if (!f) numberCache.set(k, (f = new Intl.NumberFormat(numLocale, opts)));
    return f;
  };
  const df = (opts: Intl.DateTimeFormatOptions = {}) => {
    const k = JSON.stringify(opts);
    let f = dateCache.get(k);
    if (!f) dateCache.set(k, (f = new Intl.DateTimeFormat(dateLocale, { ...tz, ...opts })));
    return f;
  };
  const rtf = new Intl.RelativeTimeFormat(numLocale, { numeric: 'auto' });
  const digits = (text: string) => (persianDigits ? toPersianDigits(text) : text);

  const number: Formatters['number'] = (v, opts) =>
    v === null || v === undefined || Number.isNaN(v) ? '' : nf(opts).format(v);

  return {
    locale,
    number,
    integer: (v) => number(v, { maximumFractionDigits: 0 }),
    percent: (r, fd = 1) =>
      r === null || r === undefined || Number.isNaN(r)
        ? ''
        : nf({ style: 'percent', maximumFractionDigits: fd }).format(r),
    rate: (v, unit) => {
      if (v === null || v === undefined || Number.isNaN(v)) return '';
      const suffix = unit === 'bps' ? 'bit/s' : 'pkt/s';
      const abs = Math.abs(v);
      const si = SI.find((s) => abs >= s.factor);
      const scaled = si ? v / si.factor : v;
      return `${number(scaled, { maximumFractionDigits: si ? 2 : 0 })} ${si?.prefix ?? ''}${suffix}`;
    },
    date: (v, opts) => (v === null || v === undefined ? '' : df(opts ?? { dateStyle: 'medium' }).format(toDate(v))),
    dateTime: (v) =>
      v === null || v === undefined ? '' : df({ dateStyle: 'medium', timeStyle: 'medium' }).format(toDate(v)),
    time: (v) => (v === null || v === undefined ? '' : df({ timeStyle: 'medium' }).format(toDate(v))),
    relative: (v, now = new Date()) => {
      if (v === null || v === undefined) return '';
      const diff = (toDate(v).getTime() - now.getTime()) / 1000;
      const abs = Math.abs(diff);
      if (abs < 60) return rtf.format(Math.round(diff), 'second');
      if (abs < 3600) return rtf.format(Math.round(diff / 60), 'minute');
      if (abs < 86400) return rtf.format(Math.round(diff / 3600), 'hour');
      return rtf.format(Math.round(diff / 86400), 'day');
    },
    digits,
  };
}
