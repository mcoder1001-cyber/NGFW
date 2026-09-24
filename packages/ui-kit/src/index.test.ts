import { describe, expect, it } from 'vitest';
import { createVrxTheme } from './index.js';

describe('createVrxTheme', () => {
  it('builds light and dark themes with status tokens and direction', () => {
    expect(createVrxTheme('light').vrx.status.up).toMatch(/^#/);
    expect(createVrxTheme('dark', 'rtl').direction).toBe('rtl');
  });
});
