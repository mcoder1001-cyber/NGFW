import type { Provider, Type } from '@nestjs/common';
import { AlarmsService } from './alarms.service.js';
import { DashboardController } from './dashboard.controller.js';

/**
 * F-dashboard-prom-alarms: the alarm engine (`AlarmsService`) and its read/action routes. The exposition endpoint is
 * the agent's; this module is alarms + the dashboard summary. `AlarmsService.start()` is called on bootstrap.
 */
export const dashboardPromAlarmsFeature: { controllers: Type[]; providers: Provider[] } = {
  controllers: [DashboardController],
  providers: [AlarmsService],
};

export { AlarmsService } from './alarms.service.js';
