import type { Provider, Type } from '@nestjs/common';
import { MultiwanController } from './multiwan.controller.js';

/** F-multiwan: the live WAN state route (`GET /api/v1/state/wan`). Config is via the generic pointer routes. */
export const multiwanFeature: { controllers: Type[]; providers: Provider[] } = {
  controllers: [MultiwanController],
  providers: [],
};
