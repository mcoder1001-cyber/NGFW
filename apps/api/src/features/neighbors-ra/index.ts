import { NeighborsRaController } from './neighbors-ra.controller.js';

/**
 * F-neighbors-ra API module (wave-A-hotspots P1): `app.module.ts` spreads `controllers` and `providers` under the
 * feature's anchors. The controller needs only the shared AgentClient.
 */
export const neighborsRaFeature = {
  controllers: [NeighborsRaController],
  providers: [],
};
