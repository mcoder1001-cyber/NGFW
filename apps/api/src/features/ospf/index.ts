import { OspfController } from './ospf.controller.js';

/** Read-only OSPFv2 observations through existing RoutingState; no daemon/configuration ownership. */
export const ospfFeature = { controllers: [OspfController], providers: [] };
