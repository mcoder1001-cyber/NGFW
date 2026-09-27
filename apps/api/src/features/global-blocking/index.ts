import type { Provider, Type } from '@nestjs/common';
import { GlobalBlockingController } from './global-blocking.controller.js';
import { GlobalBlockingService } from './global-blocking.service.js';

/** F-global-blocking: `/api/v1/security/global-blocking` (status, import, fetch, export) and the scheduled refresh. */
export const globalBlockingFeature: { controllers: Type[]; providers: Provider[] } = {
  controllers: [GlobalBlockingController],
  providers: [GlobalBlockingService],
};

export { GlobalBlockingService } from './global-blocking.service.js';
