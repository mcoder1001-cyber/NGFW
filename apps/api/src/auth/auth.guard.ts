import { type CanActivate, type ExecutionContext, Injectable } from '@nestjs/common';
import { Reflector } from '@nestjs/core';
import { AuditService } from '../audit/audit.service.js';
import { problems } from '../common/problem.js';
import { atLeast, clientKey, sourceIp, type VrxRequest } from '../common/principal.js';
import type { Role } from '../db/schema.js';
import { AuthService } from './auth.service.js';
import { PUBLIC_KEY, ROLE_KEY } from './decorators.js';

/** Default minimum role by HTTP method (P06 §6: readonly = GET only). */
export function defaultRole(method: string): Role {
  return method === 'GET' || method === 'HEAD' ? 'readonly' : 'operator';
}

/**
 * Global guard (APP_GUARD): every route is authenticated unless it is @Public(), and role-checked against
 * @MinRole() or the method default. Denied mutations are audited here (the audit interceptor runs after guards):
 * 403 per request; TD-10b (review 2.3e) 401 too — aggregated per client, route and minute (`writeAggregated`), so an
 * unauthenticated flood leaves a bounded trace instead of none (before) or one row per request.
 */
@Injectable()
export class AuthGuard implements CanActivate {
  constructor(
    private readonly reflector: Reflector,
    private readonly auth: AuthService,
    private readonly audit: AuditService,
  ) {}

  async canActivate(ctx: ExecutionContext): Promise<boolean> {
    const targets = [ctx.getHandler(), ctx.getClass()];
    if (this.reflector.getAllAndOverride<boolean>(PUBLIC_KEY, targets)) return true;
    const req = ctx.switchToHttp().getRequest<VrxRequest>();
    const principal = await this.auth.authenticate(req.headers.authorization);
    if (principal === null) {
      if (req.method !== 'GET' && req.method !== 'HEAD') {
        const ip = sourceIp(req);
        const action = `${req.method} ${req.routeOptions.url ?? req.url}`;
        const reason = req.headers.authorization ? 'invalid-credentials' : 'no-credentials';
        // not awaited: the 401 never waits for (or fails on) the audit store
        void this.audit.writeAggregated(
          {
            userId: null,
            username: null,
            sourceIp: ip,
            action,
            resource: req.url.split('?')[0] ?? null,
            after: { reason },
            result: 'failure',
            status: 401,
          },
          `${action}|${reason}|${clientKey(ip)}`,
        );
      }
      throw problems.unauthorized();
    }
    req.principal = principal;
    const required =
      this.reflector.getAllAndOverride<Role>(ROLE_KEY, targets) ?? defaultRole(req.method);
    if (!atLeast(principal.role, required)) {
      if (req.method !== 'GET' && req.method !== 'HEAD') {
        await this.audit.write({
          userId: principal.id,
          username: principal.username,
          sourceIp: sourceIp(req),
          action: `${req.method} ${req.routeOptions.url ?? req.url}`,
          resource: req.url.split('?')[0] ?? null,
          after: { reason: `role ${principal.role} < ${required}` },
          result: 'failure',
          status: 403,
        });
      }
      throw problems.forbidden(`role '${principal.role}' may not do this (needs '${required}')`);
    }
    return true;
  }
}
