import { IpfixSflowController } from './ipfix-sflow.controller.js';

/** F-ipfix-sflow API feature: `GET /api/v1/state/ipfix` (wave-A-hotspots P1). */
export const ipfixSflowFeature = {
  controllers: [IpfixSflowController],
  providers: [],
} as const;
