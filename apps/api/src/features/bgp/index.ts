import { BgpController } from './bgp.controller.js';

/**
 * P12 API feature (wave-A-hotspots P1): `GET /api/v1/state/bgp` (live BGP / FRR / linux-cp state via the agent's
 * RoutingState RPC) and the FRR annotation + `proto` filter of F-vrf-static-ecmp's `/state/routes` (routes.ts, one shared
 * hunk there). BGP, prefix lists, route maps and `interfaces.<n>.lcp` are configured through the generic
 * `/api/v1/config/**` pointer routes; neighbour and RIB changes stream on the `routing.events` WebSocket topic.
 */
export const bgpFeature = {
  controllers: [BgpController],
  providers: [],
};
