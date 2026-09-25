import type { Provider, Type } from '@nestjs/common';
import { AclController } from './acl.controller.js';
import { AclService } from './acl.service.js';

/**
 * F-acl: `/api/v1/state/acl/{lists,lists/{name}/rules,attachments}`, `/api/v1/actions/acl/{import,export.csv}` and
 * `/api/v1/actions/acl/lists/{name}/rules/bulk` (wave-A-hotspots P1).
 */
export const aclFeature: { controllers: Type[]; providers: Provider[] } = {
  controllers: [AclController],
  providers: [AclService],
};
