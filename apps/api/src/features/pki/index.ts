import { PkiAgentFiles } from './agent-files.js';
import { PkiController } from './pki.controller.js';
import { PkiService } from './pki.service.js';

/** F-pki's API feature (wave-A-hotspots P1): one import + one spread per array in app.module.ts. */
export const pkiFeature = {
  controllers: [PkiController],
  providers: [PkiService, PkiAgentFiles],
} as const;

export { PkiService } from './pki.service.js';
