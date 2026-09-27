import { Injectable } from '@nestjs/common';
import type { ActionDone, CnatSession, Det44Session } from '@ngfw/proto';
import { AgentClient } from '../../agent/agent.client.js';
import { ProblemError, problems } from '../../common/problem.js';
import type {
  CnatSessionsQuery,
  Det44CloseBody,
  Det44LookupBody,
  Det44SessionsQuery,
} from './dto.js';

/**
 * DET44 ports per host for an inside/outside prefix pair (VPP det44_add_del_map): sharing ratio = 2^(outLen − inLen)
 * inside hosts per outside address, 64 512 ports (1024–65535) split evenly. Mirrors the agent's Det44PortsPerHost.
 */
export function det44PortsPerHost(insideLen: number, outsideLen: number): number {
  const ratio = 2 ** Math.max(0, outsideLen - insideLen);
  return Math.floor(64_512 / ratio);
}

/** The agent's NOT_FOUND / PERMISSION_DENIED (mapped to 502 by agentProblem) → 404 / 403 for these routes. */
function remap(e: unknown): never {
  if (e instanceof ProblemError) {
    const code = e.body()['grpcCode'];
    if (code === 'NOT_FOUND') throw problems.notFound(String(e.body()['detail'] ?? 'not found'));
    if (code === 'PERMISSION_DENIED')
      throw new ProblemError(403, 'forbidden', 'Forbidden', String(e.body()['detail'] ?? ''));
  }
  throw e;
}

/** An action's `done` → HTTP: 0 ok, 1 not found (404), anything else 502. */
export function doneOutcome(done: ActionDone) {
  if (done.exitCode === 0) return { summary: done.summary };
  if (done.exitCode === 1) throw problems.notFound(`agent: ${done.summary}`);
  throw new ProblemError(502, 'agent-error', 'Agent error', `agent: ${done.summary}`);
}

export const det44SessionJson = (s: Det44Session) => ({
  insidePort: s.insidePort,
  outsidePort: s.outsidePort,
  externalAddress: s.externalAddress,
  externalPort: s.externalPort,
  state: s.state,
  expire: s.expire,
});

export const cnatSessionJson = (s: CnatSession) => ({
  dstAddress: s.dstAddress,
  dstPort: s.dstPort,
  srcAddress: s.srcAddress,
  srcPort: s.srcPort,
  protocol: s.protocol,
  translationIndex: s.translationIndex,
  flags: s.flags,
});

/** `/state/nat/det44|cnat/**` and the two actions: everything through the agent, never VPP. */
@Injectable()
export class Det44MapDsliteCnatService {
  constructor(private readonly agent: AgentClient) {}

  async det44Sessions(q: Det44SessionsQuery) {
    const r = await this.agent
      .det44Sessions({ user: q.user, offset: (q.page - 1) * q.pageSize, limit: q.pageSize })
      .catch(remap);
    return {
      user: q.user,
      outsideAddress: r.outsideAddress,
      portLo: r.portLo,
      portHi: r.portHi,
      page: q.page,
      pageSize: q.pageSize,
      total: Number(r.totalSessions),
      retrievedAt: r.retrievedAt?.toISOString(),
      items: r.sessions.slice(0, q.pageSize).map(det44SessionJson),
    };
  }

  async det44Lookup(b: Det44LookupBody) {
    const req =
      'inside' in b
        ? { insideAddress: b.inside }
        : { outsideAddress: b.outside, outsidePort: b.port };
    const r = await this.agent.det44Lookup(req).catch(remap);
    const fwd = 'inside' in b;
    return {
      inside: r.insideAddress,
      outside: r.outsideAddress,
      portLo: fwd ? r.portLo : null,
      portHi: fwd ? r.portHi : null,
    };
  }

  async det44Close(b: Det44CloseBody) {
    const done = await this.agent.det44SessionClose(b).catch(remap);
    return doneOutcome(done);
  }

  async cnatSessions(q: CnatSessionsQuery) {
    const r = await this.agent
      .cnatSessions({ offset: (q.page - 1) * q.pageSize, limit: q.pageSize })
      .catch(remap);
    return {
      page: q.page,
      pageSize: q.pageSize,
      total: Number(r.totalSessions),
      truncated: r.truncated,
      retrievedAt: r.retrievedAt?.toISOString(),
      items: r.sessions.slice(0, q.pageSize).map(cnatSessionJson),
    };
  }

  async cnatPurge() {
    const done = await this.agent.cnatSessionPurge().catch(remap);
    return doneOutcome(done);
  }
}
