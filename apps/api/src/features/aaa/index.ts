import type { Provider, Type } from '@nestjs/common';
import { AaaController } from './aaa.controller.js';
import { AaaService } from './aaa.service.js';
import { MfaService } from './mfa.service.js';

/**
 * F-aaa: the admin backend-test route (increment 1) plus, since F-aaa-login (increment 2), the AAA policy/backend
 * service the login path walks and the per-user MFA service. `AuthService` injects both.
 */
export const aaaFeature: { controllers: Type[]; providers: Provider[] } = {
  controllers: [AaaController],
  providers: [AaaService, MfaService],
};

export { AaaService } from './aaa.service.js';
export { MfaService } from './mfa.service.js';
