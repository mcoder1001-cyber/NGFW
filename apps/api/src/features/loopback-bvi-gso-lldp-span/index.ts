import { APP_INTERCEPTOR } from '@nestjs/core';
import { LoopbackBviGsoLldpSpanController } from './loopback-bvi-gso-lldp-span.controller.js';
import { NsimGateInterceptor } from './nsim-gate.js';

/** F-loopback-bvi-gso-lldp-span API module (wave-A-hotspots P1): spread into AppModule's controllers / providers. */
export const loopbackBviGsoLldpSpanFeature = {
  controllers: [LoopbackBviGsoLldpSpanController],
  // the nsim lab gate (review M2): 409 for a commit / rollback that carries services.nsim while VRX_NSIM≠lab
  providers: [{ provide: APP_INTERCEPTOR, useClass: NsimGateInterceptor }],
} as const;
