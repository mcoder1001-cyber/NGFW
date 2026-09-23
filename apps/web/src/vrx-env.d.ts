/**
 * Build-time flags injected by `vite.config.ts` (`define`). They are literals in the output, so dead branches — and the
 * dynamic imports inside them — are removed from production bundles.
 */
declare const __VRX_DEV_ROUTES__: boolean;
