import {
  type CallHandler,
  type ExecutionContext,
  Injectable,
  type NestInterceptor,
} from '@nestjs/common';
import { isPlainObject } from '@ngfw/schema';
import { from, mergeMap, type Observable } from 'rxjs';
import { AuditService } from '../../audit/audit.service.js';
import { sourceIp, type VrxRequest } from '../../common/principal.js';
import { ProblemError } from '../../common/problem.js';
import { DatastoreService } from '../../datastore/datastore.service.js';

type Json = Record<string, unknown>;

/** The lab gate of the delay simulator (review M2): `VRX_NSIM=lab` in the API's (and the agent's) environment. */
export const NSIM_ENV = 'VRX_NSIM';
export const NSIM_LAB = 'lab';

/** Whether the nsim lab tool is enabled for this API process (read per request; off by default). */
export function nsimEnabled(env: NodeJS.ProcessEnv = process.env): boolean {
  return env[NSIM_ENV] === NSIM_LAB;
}

/** Whether a configuration document carries `services.nsim`. */
export function hasNsim(doc: unknown): boolean {
  if (!isPlainObject(doc)) return false;
  const services = (doc as Json)['services'];
  return isPlainObject(services) && (services as Json)['nsim'] !== undefined;
}

/** The problem type, and the audit reason, of a refusal by the lab gate. */
export const NSIM_DISABLED = 'nsim-disabled';

export function nsimOff(): ProblemError {
  return new ProblemError(
    409,
    NSIM_DISABLED,
    'Conflict',
    `services.nsim is a lab tool and is off: set ${NSIM_ENV}=${NSIM_LAB} for the API and the agent (globals owner) to apply it, or remove services.nsim (configuring nsim keeps VPP's main thread polling until VPP restarts)`,
    [
      {
        pointer: '/services/nsim',
        message: 'the network delay simulator (lab tool) is not enabled on this system',
      },
    ],
  );
}

const ROLLBACK = /^\/api\/v1\/config\/rollback\/(\d+)$/;

/**
 * Refuses (409 problem+json, pointer `/services/nsim`) a commit of a candidate — or a rollback to a revision — that carries
 * `services.nsim` while the lab gate is off (review M2: nsim can crash a VPP with worker threads and keeps the main thread
 * polling until VPP restarts). Everything else passes through unchanged. Registered as a global interceptor by this
 * feature's providers; the agent enforces the same gate (it applies nsim only as the globals owner with VRX_NSIM=lab).
 *
 * The refusal is audited here (TD-10b 2.3e: every config mutation attempt leaves a row, refusals included): this
 * interceptor is registered before the global AuditInterceptor, so it is the outer one and the audit interceptor
 * never sees a request refused here. One `failure` row, status 409, `after.reason = 'nsim-disabled'` — the pattern of
 * the AuthGuard's 403 row. The row is written before the 409 is answered.
 */
@Injectable()
export class NsimGateInterceptor implements NestInterceptor {
  constructor(
    private readonly ds: DatastoreService,
    private readonly audit: AuditService,
  ) {}

  intercept(ctx: ExecutionContext, next: CallHandler): Observable<unknown> {
    if (ctx.getType() !== 'http' || nsimEnabled()) return next.handle();
    const req = ctx.switchToHttp().getRequest<VrxRequest>();
    if (req.method !== 'POST') return next.handle();
    const path = req.url.split('?')[0] ?? '';
    const rb = ROLLBACK.exec(path);
    if (path !== '/api/v1/config/commit' && rb === null) return next.handle();
    const target = async (): Promise<unknown> =>
      rb === null ? this.ds.getCandidate() : (await this.ds.getRevision(Number(rb[1]))).payload;
    return from(target()).pipe(
      mergeMap((doc) => {
        if (!hasNsim(doc)) return next.handle();
        return from(this.refused(req)).pipe(
          mergeMap(() => {
            throw nsimOff();
          }),
        );
      }),
    );
  }

  /** The audit row of a refused commit or rollback (AuditService.write never throws). */
  private refused(req: VrxRequest): Promise<void> {
    return this.audit.write({
      userId: req.principal?.id ?? null,
      username: req.principal?.username ?? null,
      sourceIp: sourceIp(req),
      action: `${req.method} ${req.routeOptions.url ?? req.url}`,
      resource: req.url.split('?')[0] ?? null,
      after: { reason: NSIM_DISABLED },
      result: 'failure',
      status: 409,
    });
  }
}
