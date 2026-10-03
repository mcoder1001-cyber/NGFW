import { describe, expect, it } from 'vitest';
import { createNgfwTheme } from './index.js';

describe('createNgfwTheme', () => {
  it('builds light and dark themes with status tokens and direction', () => {
    expect(createNgfwTheme('light').ngfw.status.up).toMatch(/^#/);
    expect(createNgfwTheme('dark', 'rtl').direction).toBe('rtl');
  });
});
