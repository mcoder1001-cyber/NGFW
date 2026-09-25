import type { Provider, Type } from '@nestjs/common';
import { QosFlatController } from './qos-flat.controller.js';

/**
 * F-qos-flat: `GET /api/v1/state/services/qos/policers`, `POST /api/v1/actions/qos/policers/{name}/reset`
 * (wave-A-hotspots P1, P3: a static route in the feature's own controller).
 */
export const qosFlatFeature: { controllers: Type[]; providers: Provider[] } = {
  controllers: [QosFlatController],
  providers: [],
};
