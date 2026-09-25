import { BondingController } from './bonding.controller.js';

/** F-bonding API module (wave-A-hotspots P1): spread into AppModule's controllers / providers. */
export const bondingFeature = {
  controllers: [BondingController],
  providers: [],
} as const;
