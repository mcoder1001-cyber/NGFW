import {
  type ArgumentsHost,
  Catch,
  type ExceptionFilter,
  HttpException,
  HttpStatus,
  Logger,
} from '@nestjs/common';
import { MergePatchError } from '@ngfw/schema';
import type { FastifyReply, FastifyRequest } from 'fastify';

/** One entry of `errors[]` in a problem document: RFC 6901 pointer into the configuration document + message. */
export interface ProblemIssue {
  pointer: string;
  message: string;
  /** Stable rule id when known (`interfaces.address-no-overlap`, agent rule ids). */
  rule?: string;
}

/**
 * RFC 9457 problem details. `errors[]` carries `{pointer, message}` for validation failures (00-CONTEXT conventions);
 * extension members go into `extra` (e.g. `lock` for 409, `results` for a failed Apply).
 */
export interface ProblemBody {
  type: string;
  title: string;
  status: number;
  detail?: string;
  instance?: string;
  errors?: ProblemIssue[];
  [extension: string]: unknown;
}

export const PROBLEM_TYPE_BASE = 'https://vrx.dev/problems/';

export class ProblemError extends HttpException {
  constructor(
    status: number,
    readonly slug: string,
    readonly title: string,
    readonly detail?: string,
    readonly errors?: ProblemIssue[],
    readonly extra: Record<string, unknown> = {},
  ) {
    super(detail ?? title, status);
  }

  body(instance?: string): ProblemBody {
    const body: ProblemBody = {
      type: PROBLEM_TYPE_BASE + this.slug,
      title: this.title,
      status: this.getStatus(),
      ...this.extra,
    };
    if (this.detail !== undefined) body.detail = this.detail;
    if (instance !== undefined) body.instance = instance;
    if (this.errors !== undefined) body.errors = this.errors;
    return body;
  }
}

export const problems = {
  validation: (errors: ProblemIssue[], detail = 'the configuration is invalid', extra = {}) =>
    new ProblemError(400, 'validation', 'Validation failed', detail, errors, extra),
  badRequest: (detail: string, errors?: ProblemIssue[]) =>
    new ProblemError(400, 'bad-request', 'Bad request', detail, errors),
  unauthorized: (detail = 'authentication required') =>
    new ProblemError(401, 'unauthorized', 'Unauthorized', detail),
  forbidden: (detail: string, errors?: ProblemIssue[]) =>
    new ProblemError(403, 'forbidden', 'Forbidden', detail, errors),
  notFound: (detail: string) => new ProblemError(404, 'not-found', 'Not found', detail),
  conflict: (slug: string, detail: string, extra: Record<string, unknown> = {}) =>
    new ProblemError(409, slug, 'Conflict', detail, undefined, extra),
  tooMany: (detail: string) => new ProblemError(429, 'rate-limited', 'Too many requests', detail),
  notImplemented: (detail: string) =>
    new ProblemError(501, 'not-implemented', 'Not implemented', detail),
  unavailable: (detail: string) =>
    new ProblemError(503, 'unavailable', 'Service unavailable', detail),
};

/**
 * Every error leaves the API as `application/problem+json`: ProblemError as built, MergePatchError → 400 with its
 * pointer (D-070), other HttpExceptions mapped by status, anything else → 500 without internals.
 */
@Catch()
export class ProblemFilter implements ExceptionFilter {
  private readonly log = new Logger('ProblemFilter');

  catch(exception: unknown, host: ArgumentsHost): void {
    const ctx = host.switchToHttp();
    const req = ctx.getRequest<FastifyRequest>();
    const reply = ctx.getResponse<FastifyReply>();
    const body = toProblem(exception, req.url.split('?')[0]);
    if (body.status >= 500 && !(exception instanceof ProblemError)) {
      this.log.error(
        `${req.method} ${req.url.split('?')[0]}: ${exception instanceof Error ? exception.stack : String(exception)}`,
      );
    }
    if (body.status === 401) void reply.header('www-authenticate', 'Bearer, ApiKey');
    void reply.status(body.status).header('content-type', 'application/problem+json').send(body);
  }
}

export function toProblem(exception: unknown, instance?: string): ProblemBody {
  if (exception instanceof ProblemError) return exception.body(instance);
  if (exception instanceof MergePatchError) {
    return problems
      .validation([{ pointer: exception.pointer, message: exception.message }], exception.message)
      .body(instance);
  }
  if (exception instanceof HttpException) {
    const status = exception.getStatus();
    const res = exception.getResponse();
    const detail =
      typeof res === 'string'
        ? res
        : typeof (res as { message?: unknown }).message === 'string'
          ? (res as { message: string }).message
          : exception.message;
    const title = HttpStatus[status]?.toString().replace(/_/g, ' ').toLowerCase() ?? 'error';
    const body: ProblemBody = {
      type: status === 404 ? PROBLEM_TYPE_BASE + 'not-found' : 'about:blank',
      title: title.charAt(0).toUpperCase() + title.slice(1),
      status,
      detail,
    };
    if (instance !== undefined) body.instance = instance;
    return body;
  }
  // Fastify errors (body too large, bad JSON) carry statusCode
  const code = (exception as { statusCode?: unknown } | null)?.statusCode;
  if (typeof code === 'number' && code >= 400 && code < 500) {
    const body: ProblemBody = {
      type: 'about:blank',
      title: 'Bad request',
      status: code,
      detail: (exception as Error).message,
    };
    if (instance !== undefined) body.instance = instance;
    return body;
  }
  const body: ProblemBody = {
    type: 'about:blank',
    title: 'Internal server error',
    status: 500,
    detail: 'internal error (see the API log)',
  };
  if (instance !== undefined) body.instance = instance;
  return body;
}
