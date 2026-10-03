import { Controller, Get, Param } from '@nestjs/common';
import { ApiOkResponse, ApiOperation, ApiParam, ApiTags } from '@nestjs/swagger';
import { generateModules, moduleList } from '@ngfw/yang';
import { z } from 'zod';
import { problems } from '../../common/problem.js';
import { Protected } from '../../common/responses.js';
import { openapi } from '../../common/zod.js';

const ModulesOut = z.object({
  modules: z.array(z.object({ name: z.string(), namespace: z.string(), revision: z.string() })),
});
const ModuleOut = z.object({ name: z.string(), yang: z.string() });

/**
 * F-restconf-yang: a small, OpenAPI-visible read surface so the web "download YANG" card (and curl) can list and fetch
 * the generated modules with the typed client. The RESTCONF protocol layer itself is `RestconfController` (excluded
 * from OpenAPI). Read-only; guarded like everything else.
 */
@ApiTags('yang')
@Controller('api/v1/system/yang')
export class YangController {
  @Get()
  @Protected()
  @ApiOperation({ summary: 'List the YANG modules generated from the configuration schema' })
  @ApiOkResponse({ schema: openapi(ModulesOut, 'output') })
  list() {
    return { modules: moduleList() };
  }

  @Get(':name')
  @Protected(404)
  @ApiParam({ name: 'name', schema: { type: 'string' }, description: 'module name, e.g. ngfw-interfaces' })
  @ApiOperation({ summary: 'The text of one generated YANG module' })
  @ApiOkResponse({ schema: openapi(ModuleOut, 'output') })
  one(@Param('name') name: string) {
    const modules = generateModules();
    const text = modules[name];
    if (text === undefined) throw problems.notFound(`no YANG module '${name}'`);
    return { name, yang: text };
  }
}
