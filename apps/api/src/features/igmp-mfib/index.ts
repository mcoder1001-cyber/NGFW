import type { Provider, Type } from '@nestjs/common';
import { IgmpMfibController } from './igmp-mfib.controller.js';

/**
 * F-igmp-mfib: the multicast state routes (live IGMP groups, mFIB, PIM neighbours). Configuration is edited through the
 * generic pointer routes; the data-plane (VPP mFIB / FRR pimd) is the agent's, delivered in the host follow-up.
 */
export const igmpMfibFeature: { controllers: Type[]; providers: Provider[] } = {
  controllers: [IgmpMfibController],
  providers: [],
};
