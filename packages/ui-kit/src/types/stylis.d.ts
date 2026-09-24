// `stylis` ships no type declarations; only `prefixer` is used (as an Emotion stylis plugin).
declare module 'stylis' {
  import type { StylisPlugin } from '@emotion/cache';
  export const prefixer: StylisPlugin;
}
