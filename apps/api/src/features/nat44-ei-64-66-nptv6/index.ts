import { Nat44Ei6466Nptv6Controller } from './controller.js';
import { Nat44Ei6466Nptv6Service } from './service.js';

/** F-nat44-ei-64-66-nptv6 feature module: wired into app.module.ts by one spread per array (wave-A-hotspots P1). */
export const nat44Ei6466Nptv6Feature = {
  controllers: [Nat44Ei6466Nptv6Controller],
  providers: [Nat44Ei6466Nptv6Service],
};
