import { describe, expect, it } from 'vitest';
import { createFormatters, toPersianDigits } from './formatters.js';
import { directionFor } from './index.js';

describe('formatters', () => {
  it('formats numbers, rates and dates through Intl for en', () => {
    const f = createFormatters({ locale: 'en', timeZone: 'UTC' });
    expect(f.integer(1234567)).toBe('1,234,567');
    expect(f.rate(1.5e9, 'bps')).toBe('1.5 Gbit/s');
    expect(f.rate(950, 'pps')).toBe('950 pkt/s');
    expect(f.percent(0.1234)).toBe('12.3%');
    expect(f.date('2026-09-23T10:00:00Z')).toContain('2026');
    expect(f.number(null)).toBe('');
  });

  it('uses Persian digits and the Persian calendar for fa when asked', () => {
    const f = createFormatters({ locale: 'fa', persianDigits: true, timeZone: 'UTC' });
    expect(f.integer(1234)).toMatch(/^[۰-۹٬,]+$/);
    expect(f.date('2026-09-23T10:00:00Z')).toMatch(/۱۴۰۵/);
    expect(f.digits('eth0/1')).toBe('eth۰/۱');
    expect(toPersianDigits('2026')).toBe('۲۰۲۶');
    const latin = createFormatters({ locale: 'fa', persianDigits: false, calendar: 'gregory', timeZone: 'UTC' });
    expect(latin.integer(1234)).toMatch(/1[,٬]234/);
    expect(latin.date('2026-09-23T10:00:00Z')).toContain('2026');
  });

  it('knows which languages are RTL', () => {
    expect(directionFor('fa')).toBe('rtl');
    expect(directionFor('fa-IR')).toBe('rtl');
    expect(directionFor('en')).toBe('ltr');
  });
});
