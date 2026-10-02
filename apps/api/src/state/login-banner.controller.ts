import { Controller, Get, Header } from '@nestjs/common';
import { ApiOperation, ApiTags } from '@nestjs/swagger';
import { z } from 'zod';
import { isPlainObject } from '@ngfw/schema';
import { Public } from '../auth/decorators.js';
import { ApiOut, PublicDoc } from '../common/responses.js';
import { DatastoreService } from '../datastore/datastore.service.js';

const BannerOut = z.object({ banner: z.string().max(4096) });

/** Only explicitly configured pre-login text is public. No candidate or other
 * configuration field crosses the authentication boundary. */
@ApiTags('auth')
@Controller('api/v1/auth')
export class LoginBannerController {
  constructor(private readonly ds: DatastoreService) {}

  @Get('banner')
  @Public()
  @Header('Cache-Control', 'no-store')
  @PublicDoc(503)
  @ApiOperation({ summary: 'Configured running pre-login banner, literal text only' })
  @ApiOut(BannerOut)
  async banner() {
    const running = await this.ds.getRunning();
    const system = running.doc['system'];
    const banner = isPlainObject(system) ? system['banner'] : undefined;
    const value = isPlainObject(banner) ? banner['login'] : undefined;
    return {
      banner: typeof value === 'string' ? value.slice(0, 4096).replace(/[\uD800-\uDBFF]$/, '') : '',
    };
  }
}
