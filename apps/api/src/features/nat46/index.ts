import { Nat46Controller } from './controller.js';
import { Nat46Service } from './service.js';

/** F-nat46 feature module: wired into app.module.ts by one spread per array (wave-A-hotspots P1; unanchored). */
export const nat46Feature = {
  controllers: [Nat46Controller],
  providers: [Nat46Service],
};
