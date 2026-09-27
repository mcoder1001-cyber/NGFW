import type { Provider, Type } from '@nestjs/common';
import { AaaController } from './aaa.controller.js';
import { AaaService } from './aaa.service.js';

/** F-aaa (increment 1): the admin backend-test route. Login-order integration + MFA land in the next increment. */
export const aaaFeature: { controllers: Type[]; providers: Provider[] } = {
  controllers: [AaaController],
  providers: [AaaService],
};

export { AaaService } from './aaa.service.js';
