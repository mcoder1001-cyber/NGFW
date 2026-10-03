import type { Provider, Type } from '@nestjs/common';
import { NotificationsController } from './notifications.controller.js';
import { NotificationsService } from './notifications.service.js';
export const notificationsFeature: { controllers: Type[]; providers: Provider[] } = {
  controllers: [NotificationsController],
  providers: [NotificationsService],
};
