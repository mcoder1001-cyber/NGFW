import { describe, expect, it } from 'vitest';
import { contrastRatio } from './contrast.js';
import { createNgfwTheme, NGFW_STATUS_COLOURS, NGFW_STATUSES } from './createNgfwTheme.js';

describe('createNgfwTheme', () => {
  it.each(['light', 'dark'] as const)('%s status tokens keep WCAG AA text contrast on paper', (mode) => {
    const theme = createNgfwTheme(mode);
    for (const status of NGFW_STATUSES) {
      const colour = theme.ngfw.status[status];
      expect(colour).toBe(NGFW_STATUS_COLOURS[mode][status]);
      expect(contrastRatio(colour, theme.palette.background.paper)).toBeGreaterThanOrEqual(4.5);
    }
    expect(contrastRatio(theme.palette.text.primary.length === 7 ? theme.palette.text.primary : '#000000', theme.palette.background.paper)).toBeGreaterThan(1);
  });

  it('follows direction and density', () => {
    expect(createNgfwTheme('light', 'rtl').direction).toBe('rtl');
    expect(createNgfwTheme('light').ngfw.denseRowHeight).toBe(36);
    expect(createNgfwTheme('light', 'ltr', { dense: false }).ngfw.denseRowHeight).toBe(52);
    expect(createNgfwTheme('dark').palette.mode).toBe('dark');
  });
});
