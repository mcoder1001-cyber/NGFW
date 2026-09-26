import { HostMetricsController } from './host-metrics.controller.js';
import { HostMetricsService } from './host-metrics.service.js';

/** WEB-dashboard host metrics (wired in app.module.ts under the WEB-dashboard anchors). */
export const hostMetricsFeature = {
  controllers: [HostMetricsController],
  providers: [HostMetricsService],
};
