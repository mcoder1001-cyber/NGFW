import {
  type CallHandler,
  type ExecutionContext,
  HttpException,
  Injectable,
  type NestInterceptor,
} from '@nestjs/common';
import { Reflector } from '@nestjs/core';
import { MergePatchError } from '@ngfw/schema';
import { catchError, from, mergeMap, type Observable, throwError } from 'rxjs';
import { NO_AUDIT_KEY } from '../auth/decorators.js';
import { sourceIp, type VrxRequest } from '../common/principal.js';
import { AuditService } from './audit.service.js';

const MUTATING = new Set(['POST', 'PUT', 'PATCH', 'DELETE']);

function statusOf(err: unknown): number {
  if (err instanceof HttpException) return err.getStatus();
  if (err instanceof MergePatchError) return 400;
  return 500;
}

/**
 * Global interceptor: one `audit_log` row per mutation (P06 §7) — user, source IP, route, resource, the handler's
 * redacted before/after (`req.audit`), result and HTTP status. Failures are audited too.
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
    const write = (result: 'success' | 'failure', status: number) =>
      this.audit.write({
        userId: req.principal?.id ?? null,
        username: req.principal?.username ?? null,
        sourceIp: sourceIp(req),
        action: `${req.method} ${req.routeOptions.url ?? req.url}`,
        resource: req.audit?.resource ?? req.url.split('?')[0] ?? null,
        before: req.audit?.before,
        after: req.audit?.after,
        result,
        status,
      });
    // Nest sets the status after the interceptors run: read @HttpCode (Nest's '__httpCode__' metadata) or its default
    const status =
      this.reflector.get<number | undefined>('__httpCode__', ctx.getHandler()) ??
      (req.method === 'POST' ? 201 : 200);
    return next.handle().pipe(
      mergeMap((value) => from(write('success', status).then(() => value))),
      catchError((err: unknown) =>
        from(write('failure', statusOf(err))).pipe(mergeMap(() => throwError(() => err))),
      ),
    );
  }
}
