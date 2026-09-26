import type { Provider, Type } from '@nestjs/common';
import { ObjectModelController } from './object-model.controller.js';

/** F-object-model: `/api/v1/state/objects/fqdn` and `/api/v1/state/objects/usage` (wave-A-hotspots P1). */
export const objectModelFeature: { controllers: Type[]; providers: Provider[] } = {
  controllers: [ObjectModelController],
  providers: [],
};
