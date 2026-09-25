import { LICENSING_OPTIONS, licensingOptionsFromEnv } from './licensing.config.js';
import { LicensingController } from './licensing.controller.js';
import { LicensingService } from './licensing.service.js';

export { LicensingService, licenseProblem } from './licensing.service.js';

/** F-licensing wiring for app.module.ts (wave-A-hotspots P1). */
export const licensingFeature = {
  controllers: [LicensingController],
  providers: [
    { provide: LICENSING_OPTIONS, useFactory: () => licensingOptionsFromEnv() },
    LicensingService,
  ],
};
