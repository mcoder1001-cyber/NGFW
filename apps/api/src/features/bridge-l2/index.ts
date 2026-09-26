import { BridgeL2Controller } from './bridge-l2.controller.js';

/** F-bridge-l2 API module (wave-A-hotspots P1): spread into AppModule's controllers / providers. */
export const bridgeL2Feature = {
  controllers: [BridgeL2Controller],
  providers: [],
} as const;
