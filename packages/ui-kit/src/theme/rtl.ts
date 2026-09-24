import createCache, { type EmotionCache } from '@emotion/cache';
import type { Direction } from '@mui/material/styles';
import { prefixer } from 'stylis';
import rtlPlugin from 'stylis-plugin-rtl';

/**
 * Emotion cache per direction. The RTL cache runs `stylis-plugin-rtl`, which mirrors physical CSS
 * inside MUI's own styles; our code uses logical properties and is not affected.
 */
export function createVrxEmotionCache(direction: Direction): EmotionCache {
  return createCache({
    key: direction === 'rtl' ? 'vrx-rtl' : 'vrx',
    stylisPlugins: direction === 'rtl' ? [prefixer, rtlPlugin] : [prefixer],
  });
}
