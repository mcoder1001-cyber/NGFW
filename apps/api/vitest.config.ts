import swc from 'unplugin-swc';
import { defineConfig } from 'vitest/config';

// NestJS needs emitDecoratorMetadata, which esbuild cannot do — swc handles the transform.
export default defineConfig({
  test: { include: ['src/**/*.test.ts'], environment: 'node' },
  plugins: [swc.vite({ module: { type: 'es6' } })],
});
