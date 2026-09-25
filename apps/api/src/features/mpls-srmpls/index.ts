import { MplsSrmplsController } from './mpls-srmpls.controller.js';

/**
 * F-mpls-srmpls API feature (wave-A-hotspots P1): `GET /api/v1/state/routing/mpls/{fib,tunnels}` (the MPLS FIB paged by
 * the agent's MplsState, the MPLS tunnels). `routing.mpls` is configured through the generic `/api/v1/config/**`
 * pointer routes; F-mpls-ldp adds its LDP state next to these routes.
 */
export const mplsSrmplsFeature = {
  controllers: [MplsSrmplsController],
  providers: [],
};
