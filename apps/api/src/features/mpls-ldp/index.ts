import type { Provider, Type } from '@nestjs/common';
import { MplsLdpController } from './mpls-ldp.controller.js';

/**
 * F-mpls-ldp: the LDP state routes (live neighbours, LIB bindings, FRR→VPP sync status). Configuration is edited
 * through the generic pointer routes; the FRR ldpd section and the label sync are the host follow-up.
 */
export const mplsLdpFeature: { controllers: Type[]; providers: Provider[] } = {
  controllers: [MplsLdpController],
  providers: [],
};
