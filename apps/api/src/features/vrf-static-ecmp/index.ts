import { VrfStaticEcmpController } from './vrf-static-ecmp.controller.js';

/**
 * F-vrf-static-ecmp API feature (wave-A-hotspots P1): `GET /api/v1/state/routes` (the FIB browser, agent-side paging via
 * ListRoutes). VRFs and static routes are configured through the generic `/api/v1/config/**` pointer routes; the ping /
 * traceroute actions go through the generic Action bridge in `../../actions/` (P3, owned by this feature this wave).
 */
export const vrfStaticEcmpFeature = {
  controllers: [VrfStaticEcmpController],
  providers: [],
};
