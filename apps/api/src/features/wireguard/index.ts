import { WireguardController } from './wireguard.controller.js';

/** F-wireguard's API feature (wave-A-hotspots P1): one import + one spread per array in app.module.ts. */
export const wireguardFeature = {
  controllers: [WireguardController],
  providers: [],
} as const;
