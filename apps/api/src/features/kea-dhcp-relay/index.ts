import type { Provider, Type } from '@nestjs/common';
import { KeaDhcpRelayController } from './kea-dhcp-relay.controller.js';

/**
 * F-kea-dhcp-relay: `/api/v1/state/dhcp/leases`, `/api/v1/state/dhcp/relays`,
 * `/api/v1/state/interfaces/{name}/dhcp-client` (wave-A-hotspots P1).
 */
export const keaDhcpRelayFeature: { controllers: Type[]; providers: Provider[] } = {
  controllers: [KeaDhcpRelayController],
  providers: [],
};
