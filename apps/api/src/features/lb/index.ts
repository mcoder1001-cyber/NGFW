import { LbController } from './lb.controller.js';

/** F-lb API module (wave-A-hotspots P1): spread into AppModule's controllers / providers. */
export const lbFeature = {
  controllers: [LbController],
  providers: [],
} as const;
