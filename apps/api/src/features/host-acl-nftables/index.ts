import type { Provider, Type } from '@nestjs/common';
import { HostAclNftablesController } from './host-acl-nftables.controller.js';

/** F-host-acl-nftables: `/api/v1/state/host-acl` (wave-A-hotspots P1). */
export const hostAclNftablesFeature: { controllers: Type[]; providers: Provider[] } = {
  controllers: [HostAclNftablesController],
  providers: [],
};
