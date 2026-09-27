import type { Provider, Type } from '@nestjs/common';
import { AaaController } from './aaa.controller.js';
import { AaaService } from './aaa.service.js';
import { MfaCommitGuard } from './mfa-guard.js';
import { MfaController } from './mfa.controller.js';
import { MfaService } from './mfa.service.js';
import { OidcController } from './oidc.controller.js';

/** F-aaa: the admin backend-test route; F-aaa-login: the TOTP second factor (login step, set-up, admin reset). */
export const aaaFeature: { controllers: Type[]; providers: Provider[] } = {
  controllers: [AaaController, MfaController, OidcController],
  providers: [AaaService, MfaService, MfaCommitGuard], // F-aaa-mfa-lockout: MfaCommitGuard
};

export { AaaService } from './aaa.service.js';
export { MfaService } from './mfa.service.js';
export { MfaCommitGuard } from './mfa-guard.js';
