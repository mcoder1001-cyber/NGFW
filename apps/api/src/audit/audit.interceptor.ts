import {
  type CallHandler,
  type ExecutionContext,
  HttpException,
  Injectable,
  type NestInterceptor,
} from '@nestjs/common';
import { Reflector } from '@nestjs/core';
import { ApiResponse } from '@nestjs/swagger';
import { MergePatchError } from '@ngfw/schema';
import { catchError, from, mergeMap, type Observable, throwError } from 'rxjs';
import { NO_AUDIT_KEY } from '../auth/decorators.js';
import { sourceIp, type VrxRequest } from '../common/principal.js';
import { ProblemError } from '../common/problem.js';
import { ref } from '../common/zod.js';
import { AuditService, type AuditEntry } from './audit.service.js';

const MUTATING = new Set(['POST', 'PUT', 'PATCH', 'DELETE']);

/**
 * TD-10b (review 2.3e, fail closed): routes that change who can do what — passwords, API keys, secrets. Their audit
 * row is written BEFORE the action (`AuditService.begin`); when it cannot be written the action does not run
 * (503 `audit-unavailable`). Every other mutation stays fail-open (the row is written after; a failure is counted and
 * reported as a system_event). Route patterns as Fastify registers them; the route-guard test checks each exists.
 */
export const PRIVILEGED_ROUTES: ReadonlySet<string> = new Set([
  'POST /api/v1/users/:name/password',
  'POST /api/v1/auth/password',
  'POST /api/v1/auth/api-keys',
  'DELETE /api/v1/auth/api-keys/:id',
  'POST /api/v1/secrets',
  'DELETE /api/v1/secrets/:kind/:name',
]);

/** 503 `audit-unavailable`: a privileged change refused because its audit row could not be written first. */
export function auditUnavailable(): ProblemError {
  return new ProblemError(
    503,
    'audit-unavailable',
    'Audit log unavailable',
    'the audit log cannot be written; changes to passwords, API keys and secrets are refused until it can (nothing was changed)',
  );
}

/** OpenAPI: the 503 of a privileged route (its handlers carry it; the list above decides the behaviour). */
export const AuditUnavailableDoc = () =>
  ApiResponse({
    status: 503,
    description:
      '`audit-unavailable` (TD-10b): the audit row could not be written before the change, so nothing was changed; also `unavailable` when the database or agent is down',
    content: { 'application/problem+json': { schema: ref('Problem') } },
  });

function statusOf(err: unknown): number {
  if (err instanceof HttpException) return err.getStatus();
  if (err instanceof MergePatchError) return 400;
  return 500;
}

/**
 * Global interceptor: one `audit_log` row per mutation (P06 §7) — user, source IP, route, resource, the handler's
 * redacted before/after (`req.audit`), result and HTTP status. Failures are audited too. Privileged routes write
 * their row first and fail closed (TD-10b, `PRIVILEGED_ROUTES`).
 */
@Injectable()
export class AuditInterceptor implements NestInterceptor {
  constructor(
    private readonly reflector: Reflector,
    private readonly audit: AuditService,
  ) {}

  intercept(ctx: ExecutionContext, next: CallHandler): Observable<unknown> {
    const req = ctx.switchToHttp().getRequest<VrxRequest>();
    if (
      !MUTATING.has(req.method) ||
      this.reflector.getAllAndOverride<boolean>(NO_AUDIT_KEY, [ctx.getHandler(), ctx.getClass()])
    ) {
      return next.handle();
    }
    const action = `${req.method} ${req.routeOptions.url ?? req.url}`;
    const entry = (result: 'success' | 'failure', status: number): AuditEntry => ({
      userId: req.principal?.id ?? null,
      username: req.principal?.username ?? null,
      sourceIp: sourceIp(req),
      action,
      resource: req.audit?.resource ?? req.url.split('?')[0] ?? null,
      before: req.audit?.before,
      after: req.audit?.after,
      result,
      status,
    });
    const write = (result: 'success' | 'failure', status: number) =>
      this.audit.write(entry(result, status));
    // Nest sets the status after the interceptors run: read @HttpCode (Nest's '__httpCode__' metadata) or its default
    const status =
      this.reflector.get<number | undefined>('__httpCode__', ctx.getHandler()) ??
      (req.method === 'POST' ? 201 : 200);
    if (PRIVILEGED_ROUTES.has(action)) {
      return from(this.audit.begin(entry('failure', 0))).pipe(
        // only the write-ahead row's failure lands here: the handler has not run
        catchError(() => throwError(() => auditUnavailable())),
        mergeMap((id) =>
          next.handle().pipe(
            mergeMap((value) =>
              from(this.audit.finish(id, entry('success', status)).then(() => value)),
            ),
            catchError((err: unknown) =>
              from(this.audit.finish(id, entry('failure', statusOf(err)))).pipe(
                mergeMap(() => throwError(() => err)),
              ),
            ),
          ),
        ),
      );
    }
    return next.handle().pipe(
      mergeMap((value) => from(write('success', status).then(() => value))),
      catchError((err: unknown) =>
        from(write('failure', statusOf(err))).pipe(mergeMap(() => throwError(() => err))),
      ),
    );
  }
}
