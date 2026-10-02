import { Controller, Get, HttpCode, Param, Post } from '@nestjs/common';
import { ApiOkResponse, ApiOperation, ApiParam, ApiTags } from '@nestjs/swagger';
import { z } from 'zod';
import { MinRole } from '../../auth/decorators.js';
import { Protected } from '../../common/responses.js';
import { openapi, SafeParamPipe } from '../../common/zod.js';
import { NotificationsService } from './notifications.service.js';
export const NotificationsStateOut = z.object({
  queued: z.number().int(),
  busy: z.boolean(),
  error: z.string().nullable(),
  configuredChannels: z.number().int(),
  deliveries: z.array(
    z.object({
      channel: z.string(),
      rule: z.string().nullable(),
      kind: z.string(),
      at: z.string(),
      attempt: z.number().int(),
      result: z.enum(['sent', 'failed', 'discarded']),
      error: z.string().nullable(),
    }),
  ),
});
@ApiTags('notifications')
@Controller('api/v1')
export class NotificationsController {
  constructor(private readonly notifications: NotificationsService) {}
  @Get('state/management/notifications')
  @Protected()
  @ApiOperation({ summary: 'Notification queue, worker health and bounded delivery history' })
  @ApiOkResponse({ schema: openapi(NotificationsStateOut, 'output') })
  state() {
    return this.notifications.state();
  }
  @Post('actions/management/notifications/:name/test')
  @MinRole('admin')
  @HttpCode(200)
  @Protected(404, 409, 503)
  @ApiOperation({ summary: 'Queue a test through an enabled running notification channel' })
  @ApiParam({ name: 'name', type: String })
  @ApiOkResponse({ schema: openapi(z.object({ queued: z.literal(true) }), 'output') })
  test(@Param('name', new SafeParamPipe('name')) name: string) {
    return this.notifications.test(name);
  }
}
