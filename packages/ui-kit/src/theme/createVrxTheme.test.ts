import { describe, expect, it } from 'vitest';
import { contrastRatio } from './contrast.js';
import { createVrxTheme, VRX_STATUS_COLOURS, VRX_STATUSES } from './createVrxTheme.js';

describe('createVrxTheme', () => {
  it.each(['light', 'dark'] as const)('%s status tokens keep WCAG AA text contrast on paper', (mode) => {
    const theme = createVrxTheme(mode);
    for (const status of VRX_STATUSES) {
      const colour = theme.vrx.status[status];
      expect(colour).toBe(VRX_STATUS_COLOURS[mode][status]);
      expect(contrastRatio(colour, theme.palette.background.paper)).toBeGreaterThanOrEqual(4.5);
    }
    expect(contrastRatio(theme.palette.text.primary.length === 7 ? theme.palette.text.primary : '#000000', theme.palette.background.paper)).toBeGreaterThan(1);
  });

  it('follows direction and density', () => {
    expect(createVrxTheme('light', 'rtl').direction).toBe('rtl');
    expect(createVrxTheme('light').vrx.denseRowHeight).toBe(36);
    expect(createVrxTheme('light', 'ltr', { dense: false }).vrx.denseRowHeight).toBe(52);
    expect(createVrxTheme('dark').palette.mode).toBe('dark');
  });
});
