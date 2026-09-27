import type { Provider, Type } from '@nestjs/common';
import { PppoeController } from './pppoe.controller.js';

/** F-pppoe-client: the reconnect action (`POST /api/v1/actions/interfaces/{name}/pppoe/reconnect`). State is on /state/interfaces. */
export const pppoeFeature: { controllers: Type[]; providers: Provider[] } = {
  controllers: [PppoeController],
  providers: [],
};
