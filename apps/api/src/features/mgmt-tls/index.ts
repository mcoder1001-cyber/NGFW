import { MgmtTlsController } from './mgmt-tls.controller.js';
import { MgmtTlsService } from './mgmt-tls.service.js';

export { MgmtTlsService } from './mgmt-tls.service.js';

/** F-management-ui API feature (wired in app.module.ts under the F-management-ui lines). */
export const mgmtTlsFeature = { controllers: [MgmtTlsController], providers: [MgmtTlsService] };
