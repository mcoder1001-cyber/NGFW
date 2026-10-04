import { DataplaneApplyController } from './apply.controller.js';
import { DataplaneApplyService } from './apply.service.js';
import { DataplaneController } from './dataplane.controller.js';

/** F-dataplane-ui API feature (wired in app.module.ts under the F-dataplane-ui lines). */
export const dataplaneFeature = {
  controllers: [DataplaneController, DataplaneApplyController],
  providers: [DataplaneApplyService],
};
