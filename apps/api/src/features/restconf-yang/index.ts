import type { Provider, Type } from '@nestjs/common';
import { RestconfController } from './restconf.controller.js';
import { YangController } from './yang.controller.js';

/**
 * F-restconf-yang: the RESTCONF (RFC 8040) protocol layer over the commit engine, plus a small OpenAPI-visible
 * `/api/v1/system/yang` surface for downloading the generated modules. No new providers — both controllers use the
 * existing DatastoreService / CommitService.
 */
export const restconfYangFeature: { controllers: Type[]; providers: Provider[] } = {
  controllers: [RestconfController, YangController],
  providers: [],
};
