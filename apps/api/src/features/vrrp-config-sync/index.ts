import { VrrpConfigSyncController } from './controller.js';
import { ClusterSyncService } from './service.js';
export const vrrpConfigSyncFeature = {
  controllers: [VrrpConfigSyncController],
  providers: [ClusterSyncService],
};
