import {
  type CallHandler,
  type ExecutionContext,
  Injectable,
  type NestInterceptor,
} from '@nestjs/common';
import { isPlainObject } from '@ngfw/schema';
import { from, mergeMap, type Observable } from 'rxjs';
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

export function nsimOff(): ProblemError {
  return new ProblemError(
    409,
    'nsim-disabled',
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
 */
@Injectable()
export class NsimGateInterceptor implements NestInterceptor {
  constructor(private readonly ds: DatastoreService) {}

  intercept(ctx: ExecutionContext, next: CallHandler): Observable<unknown> {
    if (ctx.getType() !== 'http' || nsimEnabled()) return next.handle();
    const req = ctx.switchToHttp().getRequest<{ method: string; url: string }>();
    if (req.method !== 'POST') return next.handle();
    const path = req.url.split('?')[0] ?? '';
    const rb = ROLLBACK.exec(path);
    if (path !== '/api/v1/config/commit' && rb === null) return next.handle();
    const target = async (): Promise<unknown> =>
      rb === null ? this.ds.getCandidate() : (await this.ds.getRevision(Number(rb[1]))).payload;
    return from(target()).pipe(
      mergeMap((doc) => {
        if (hasNsim(doc)) throw nsimOff();
        return next.handle();
      }),
    );
  }
}
