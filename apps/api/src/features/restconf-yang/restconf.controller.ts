import {
  Body,
  Controller,
  Delete,
  Get,
  Header,
  HttpCode,
  Patch,
  Post,
  Put,
  Query,
  Req,
  Res,
  UseFilters,
} from '@nestjs/common';
import { ApiExcludeController } from '@nestjs/swagger';
import { redactSecrets } from '@ngfw/schema';
import { moduleList, REVISION } from '@ngfw/yang';
import type { FastifyReply } from 'fastify';
import { getAt } from '../../common/json.js';
import { problems } from '../../common/problem.js';
import type { NgfwRequest } from '../../common/principal.js';
import { CommitService } from '../../commit/commit.service.js';
import { DatastoreService } from '../../datastore/datastore.service.js';
import { RestconfExceptionFilter } from './restconf-errors.js';
import { parseDataPath, qualify, qualifyAll, unwrapBody, type RestconfTarget } from './restconf-path.js';

const YANG_JSON = 'application/yang-data+json';

/** Everything after `/restconf/data` in the raw URL, decoded per segment, without the query string. */
function dataRest(url: string): string {
  const noQuery = url.split('?')[0] ?? '';
  const m = noQuery.match(/\/restconf\/data\/?(.*)$/);
  const rest = m ? m[1]! : '';
  return rest
    .split('/')
    .map((s) => decodeURIComponent(s))
    .join('/');
}

/**
 * F-restconf-yang: a RESTCONF (RFC 8040) compatibility layer over the existing candidate/commit engine. Reads come
 * from running (default) or candidate (`?datastore=candidate`); writes edit the candidate; `operations/ngfw:commit|
 * confirm|rollback` drive the commit engine — exactly like `/api/v1/config`, with the same auth/RBAC/audit (the global
 * guard blocks a read-only user from every non-GET route). Bodies and responses use `application/yang-data+json` with
 * module-qualified top nodes; errors are `ietf-restconf:errors` (RFC 8040 §7). Secret leaves are never returned.
 *
 * Not in the OpenAPI document (`@ApiExcludeController`, the F-restconf-yang default), so the generated api-client / CLI
 * / SDK do not grow generic RESTCONF operations. NETCONF, event streams and operational-state YANG are out of scope.
 */
@ApiExcludeController()
@UseFilters(RestconfExceptionFilter)
@Controller()
export class RestconfController {
  constructor(
    private readonly ds: DatastoreService,
    private readonly commits: CommitService,
  ) {}

  /** RFC 6415: point clients at the RESTCONF root. */
  @Get('.well-known/host-meta')
  @Header('content-type', 'application/xrd+xml')
  hostMeta(): string {
    return `<?xml version="1.0" encoding="UTF-8"?>\n<XRD xmlns="http://docs.oasis-open.org/ns/xri/xrd-1.0">\n  <Link rel="restconf" href="/restconf"/>\n</XRD>\n`;
  }

  /** RFC 8040 §3.3: the API resource. */
  @Get('restconf')
  @Header('content-type', YANG_JSON)
  root(): unknown {
    return {
      'ietf-restconf:restconf': {
        data: {},
        operations: {},
        'yang-library-version': REVISION,
      },
    };
  }

  @Get('restconf/yang-library-version')
  @Header('content-type', YANG_JSON)
  libraryVersion(): unknown {
    return { 'ietf-restconf:yang-library-version': REVISION };
  }

  /** RFC 8525 module set (the colon in `ietf-yang-library:yang-library` is a Fastify param char, so it is matched
   * inside the `restconf/data/*` handler rather than as its own route). */
  private yangLibrary(): unknown {
    return {
      'ietf-yang-library:yang-library': {
        'module-set': [
          {
            name: 'ngfw',
            module: moduleList().map((m) => ({
              name: m.name,
              namespace: m.namespace,
              revision: m.revision,
            })),
          },
        ],
        'content-id': REVISION,
      },
    };
  }

  private async doc(datastore: string | undefined): Promise<Record<string, unknown>> {
    if (datastore === 'candidate') return redactSecrets(await this.ds.getCandidate()) as Record<string, unknown>;
    if (datastore !== undefined && datastore !== 'running') {
      throw problems.badRequest(`unknown datastore '${datastore}'; use running or candidate`);
    }
    return redactSecrets((await this.ds.getRunning()).doc) as Record<string, unknown>;
  }

  @Get('restconf/data')
  @Header('content-type', YANG_JSON)
  async dataRoot(@Query('datastore') datastore?: string): Promise<unknown> {
    return qualifyAll(await this.doc(datastore));
  }

  @Get('restconf/data/*')
  @Header('content-type', YANG_JSON)
  async dataAt(@Req() req: NgfwRequest, @Query('datastore') datastore?: string): Promise<unknown> {
    const rest = dataRest(req.url);
    if (rest.replace(/^\/+|\/+$/g, '') === 'ietf-yang-library:yang-library') return this.yangLibrary();
    const target = parseDataPath(rest);
    if (target === null) return qualifyAll(await this.doc(datastore));
    const value = getAt(await this.doc(datastore), target.pointer);
    if (value === undefined) throw problems.notFound(`no data at '${target.pointer}'`);
    return qualify(target, value);
  }

  private requireTarget(req: NgfwRequest): RestconfTarget {
    const target = parseDataPath(dataRest(req.url));
    if (target === null) throw problems.badRequest('a data path is required');
    return target;
  }

  @Put('restconf/data/*')
  @HttpCode(200)
  async putAt(@Body() body: unknown, @Req() req: NgfwRequest, @Res({ passthrough: true }) reply: FastifyReply) {
    const target = this.requireTarget(req);
    const value = unwrapBody(target, body);
    const r = await this.ds.putCandidate(req.principal!, target.pointer, value);
    req.audit = { resource: `restconf${target.pointer}`, after: { pointer: r.pointer } };
    reply.header('content-type', YANG_JSON);
    return { 'ngfw-restconf:result': 'ok' };
  }

  @Patch('restconf/data/*')
  @HttpCode(200)
  async patchAt(@Body() body: unknown, @Req() req: NgfwRequest, @Res({ passthrough: true }) reply: FastifyReply) {
    const target = this.requireTarget(req);
    const value = unwrapBody(target, body);
    const r = await this.ds.patchCandidate(req.principal!, target.pointer, value);
    req.audit = { resource: `restconf${target.pointer}`, after: { pointer: r.pointer } };
    reply.header('content-type', YANG_JSON);
    return { 'ngfw-restconf:result': 'ok' };
  }

  @Delete('restconf/data/*')
  @HttpCode(200)
  async deleteAt(@Req() req: NgfwRequest, @Res({ passthrough: true }) reply: FastifyReply) {
    const target = this.requireTarget(req);
    const r = await this.ds.deleteCandidate(req.principal!, target.pointer);
    req.audit = { resource: `restconf${target.pointer}`, after: { pointer: r.pointer } };
    reply.header('content-type', YANG_JSON);
    return { 'ngfw-restconf:result': 'ok' };
  }

  /**
   * RFC 8040 RPCs: `POST /restconf/operations/ngfw:commit|confirm|rollback`. One wildcard route dispatches on the RPC
   * name because Fastify treats the `:` in a path literal as a route parameter, which would collapse the three RPCs
   * into one duplicated route.
   */
  @Post('restconf/operations/*')
  @HttpCode(200)
  @Header('content-type', YANG_JSON)
  async operation(@Body() body: unknown, @Req() req: NgfwRequest) {
    const noQuery = req.url.split('?')[0] ?? '';
    const op = decodeURIComponent((noQuery.match(/\/restconf\/operations\/(.+)$/)?.[1] ?? '').replace(/\/+$/, ''));
    const input = (body as { input?: Record<string, unknown> } | undefined)?.input ?? {};
    if (op === 'ngfw:commit') {
      const confirm = typeof input['confirm'] === 'number' ? (input['confirm'] as number) : undefined;
      const comment = typeof input['comment'] === 'string' ? (input['comment'] as string) : undefined;
      const r = await this.commits.commit(req.principal!, {
        ...(confirm !== undefined ? { confirmSec: confirm } : {}),
        ...(comment !== undefined ? { comment } : {}),
      });
      req.audit = { resource: 'restconf/commit', after: { status: r.status, revision: r.revision?.id } };
      return { 'ngfw-restconf:output': { status: r.status, revision: r.revision?.id ?? null } };
    }
    if (op === 'ngfw:confirm') {
      const r = await this.commits.confirm(req.principal!);
      req.audit = { resource: 'restconf/confirm', after: { revision: r.revision?.id } };
      return { 'ngfw-restconf:output': { status: r.status, revision: r.revision?.id ?? null } };
    }
    if (op === 'ngfw:rollback') {
      if (typeof input['revision'] !== 'number') {
        throw problems.badRequest('rollback needs input.revision (the revision to restore)');
      }
      const r = await this.commits.rollback(req.principal!, input['revision'] as number, {});
      req.audit = { resource: 'restconf/rollback', after: { status: r.status, revision: r.revision?.id } };
      return { 'ngfw-restconf:output': { status: r.status, revision: r.revision?.id ?? null } };
    }
    throw problems.notFound(`unknown RESTCONF operation '${op}'`);
  }
}
