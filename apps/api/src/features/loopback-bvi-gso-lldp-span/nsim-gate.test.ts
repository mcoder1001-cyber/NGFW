import type { CallHandler, ExecutionContext } from '@nestjs/common';
import { lastValueFrom, of } from 'rxjs';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { AuditEntry, AuditService } from '../../audit/audit.service.js';
import type { DatastoreService } from '../../datastore/datastore.service.js';
import { NSIM_ENV, NSIM_LAB, NsimGateInterceptor } from './nsim-gate.js';

const NSIM = { services: { nsim: { delayMs: 20, bandwidthMbps: 100 } } };

function setup(doc: unknown, method: string, url: string, route: string) {
  const writes: AuditEntry[] = [];
  const audit = { write: vi.fn(async (e: AuditEntry) => void writes.push(e)) };
  const ds = {
    getCandidate: vi.fn(async () => doc),
    getRevision: vi.fn(async () => ({ payload: doc })),
  };
  const req = {
    method,
    url,
    ip: '192.0.2.7',
    routeOptions: { url: route },
    principal: { id: 3, username: 'op1', role: 'operator', via: 'jwt' },
  };
  const ctx = {
    getType: () => 'http',
    switchToHttp: () => ({ getRequest: () => req }),
  } as unknown as ExecutionContext;
  const next = { handle: vi.fn(() => of('handled')) } satisfies CallHandler;
  const gate = new NsimGateInterceptor(
    ds as unknown as DatastoreService,
    audit as unknown as AuditService,
  );
  return { run: () => lastValueFrom(gate.intercept(ctx, next)), writes, next, ds };
}

/** Review M2 gate + TD-10b 2.3e: a refused config mutation leaves exactly one audit row with its reason. */
describe('NsimGateInterceptor', () => {
  afterEach(() => {
    delete process.env[NSIM_ENV];
  });

  it('a refused commit is a 409 and leaves one audit row (reason nsim-disabled); the handler never runs', async () => {
    const t = setup(NSIM, 'POST', '/api/v1/config/commit', '/api/v1/config/commit');
    await expect(t.run()).rejects.toMatchObject({ status: 409 });
    expect(t.next.handle).not.toHaveBeenCalled();
    expect(t.writes).toEqual([
      {
        userId: 3,
        username: 'op1',
        sourceIp: '192.0.2.7',
        action: 'POST /api/v1/config/commit',
        resource: '/api/v1/config/commit',
        after: { reason: 'nsim-disabled' },
        result: 'failure',
        status: 409,
      },
    ]);
  });

  it('a refused rollback is audited under its route, the revision it names as the resource', async () => {
    const t = setup(NSIM, 'POST', '/api/v1/config/rollback/12', '/api/v1/config/rollback/:rev');
    await expect(t.run()).rejects.toMatchObject({ status: 409 });
    expect(t.ds.getRevision).toHaveBeenCalledWith(12);
    expect(t.writes).toEqual([
      expect.objectContaining({
        action: 'POST /api/v1/config/rollback/:rev',
        resource: '/api/v1/config/rollback/12',
        result: 'failure',
        status: 409,
        after: { reason: 'nsim-disabled' },
      }),
    ]);
  });

  it('passes a commit without nsim, and any commit with VRX_NSIM=lab, without writing (the audit interceptor does)', async () => {
    const plain = setup({ services: {} }, 'POST', '/api/v1/config/commit', '/api/v1/config/commit');
    await expect(plain.run()).resolves.toBe('handled');
    expect(plain.writes).toEqual([]);
    process.env[NSIM_ENV] = NSIM_LAB;
    const lab = setup(NSIM, 'POST', '/api/v1/config/commit', '/api/v1/config/commit');
    await expect(lab.run()).resolves.toBe('handled');
    expect(lab.writes).toEqual([]);
    expect(lab.ds.getCandidate).not.toHaveBeenCalled();
  });
});
