import '@testing-library/jest-dom/vitest';
import { cleanup, configure } from '@testing-library/react';
import { afterEach } from 'vitest';
import { transferableAbortController } from 'node:util';

// Request/fetch remain Node's native implementation in jsdom. Its Request brand
// check needs the matching native signal, including router and query cancellation.
// Assign directly so a test's unstubAllGlobals() does not undo the runtime pairing.
const nativeAbort = transferableAbortController();
globalThis.AbortController = nativeAbort.constructor as typeof AbortController;
globalThis.AbortSignal = nativeAbort.signal.constructor as typeof AbortSignal;

// Component suites default to a desktop viewport; responsive tests override it.
// jsdom has no layout-backed matchMedia implementation.
window.matchMedia = (query: string) => ({
  matches: query.includes('min-width'), media: query, onchange: null,
  addListener: () => {}, removeListener: () => {},
  addEventListener: () => {}, removeEventListener: () => {}, dispatchEvent: () => false,
});

configure({ asyncUtilTimeout: 8000 });
afterEach(() => cleanup());

if (typeof globalThis.ResizeObserver === 'undefined') {
  class ResizeObserverStub {
    observe(): void {}
    unobserve(): void {}
    disconnect(): void {}
  }
  (globalThis as { ResizeObserver?: unknown }).ResizeObserver = ResizeObserverStub;
}
