import type { Provider, Type } from '@nestjs/common';
import { AutoBlockController } from './auto-block.controller.js';
import { AutoBlockService } from './auto-block.service.js';

/**
 * F-bruteforce-block: the auto-block detector/engine (`AutoBlockService`) and its state/admin routes. The service
 * subscribes to audited login failures and sweeps expired blocks; `AutoBlockService.start()` is called on bootstrap.
 */
export const autoBlockFeature: { controllers: Type[]; providers: Provider[] } = {
  controllers: [AutoBlockController],
  providers: [AutoBlockService],
};

export { AutoBlockService } from './auto-block.service.js';
