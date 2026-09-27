import { Det44MapDsliteCnatController } from './controller.js';
import { Det44MapDsliteCnatService } from './service.js';

/** F-det44-map-dslite-cnat feature module: wired into app.module.ts by one spread per array (wave-A-hotspots P1). */
export const det44MapDsliteCnatFeature = {
  controllers: [Det44MapDsliteCnatController],
  providers: [Det44MapDsliteCnatService],
};
