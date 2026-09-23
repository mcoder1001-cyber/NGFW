/**
 * `/dev/*` demo routes and the "Developer" navigation group exist only in `vite` dev/test builds, or in a production build
 * made with `VITE_VRX_DEV_ROUTES=1`. A normal `pnpm build` contains neither the routes nor their chunks
 * (checked by `scripts/check-no-dev-routes.mjs`).
 */
export const DEV_ROUTES: boolean = __VRX_DEV_ROUTES__;
