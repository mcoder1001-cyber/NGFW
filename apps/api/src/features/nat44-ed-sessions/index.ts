import { Nat44EdSessionsController } from './controller.js';
import { Nat44EdSessionsService } from './service.js';

/** F-nat44-ed-sessions feature module: wired into app.module.ts by one spread per array (wave-A-hotspots P1). */
export const nat44EdSessionsFeature = {
  controllers: [Nat44EdSessionsController],
  providers: [Nat44EdSessionsService],
};
