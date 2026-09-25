import { Srv6Controller } from './srv6.controller.js';

/** F-srv6's API feature (wave-A-hotspots P1): one import + one spread per array in app.module.ts. */
export const srv6Feature = {
  controllers: [Srv6Controller],
  providers: [],
} as const;
