import { Injectable } from '@nestjs/common';
import { NatSessionVariant, type NatSession } from '@ngfw/proto';
import { isPlainObject } from '@ngfw/schema';
import { AgentClient } from '../../agent/agent.client.js';
import { DatastoreService } from '../../datastore/datastore.service.js';
import type { SessionsQuery } from '../nat44-ed-sessions/dto.js';
import { filterOf, killOutcome, sessionJson } from '../nat44-ed-sessions/service.js';
import type { EiKillBody, Nat64SessionOut, Nat64SessionsQuery } from './dto.js';

type Json = Record<string, unknown>;

/** A NAT64 session row (NatSession fields mapped per docs/contracts/proto.md §11 "NAT session variants"). */
export function nat64SessionJson(s: NatSession): Nat64SessionOut {
  return {
    client: s.insideAddress,
    clientPort: s.insidePort,
    poolAddress: s.outsideAddress,
    poolPort: s.outsidePort,
    remote: s.externalAddress,
    remotePort: s.externalPort,
    remoteIpv6: s.externalNatAddress,
    protocol: s.protocol,
    vrf: s.vrf,
    tableId: s.tableId,
  };
}

/** The running configuration's NPTv6 bindings (the only source: npt66 has no dump, D-063). */
export function nptv6Json(runningNat: unknown) {
  const nptv6 = isPlainObject(runningNat) ? runningNat['nptv6'] : undefined;
  const raw = isPlainObject(nptv6) && Array.isArray(nptv6['bindings']) ? nptv6['bindings'] : [];
  const bindings = raw.filter(isPlainObject).map((b: Json) => ({
    interface: typeof b['interface'] === 'string' ? b['interface'] : '',
    internal: typeof b['internal'] === 'string' ? b['internal'] : '',
    external: typeof b['external'] === 'string' ? b['external'] : '',
    description: typeof b['description'] === 'string' ? b['description'] : null,
  }));
  return { writeOnly: true as const, bindings };
}

/**
 * `/state/nat/{ei,nat64}/sessions`, `/state/nat/nptv6` and the EI kill: the sessions come from the agent (NatSessions
 * with a NatSessionVariant, the Action stream's NatSessionKillAction with variant EI), never from VPP.
 */
@Injectable()
export class Nat44Ei6466Nptv6Service {
  constructor(
    private readonly agent: AgentClient,
    private readonly ds: DatastoreService,
  ) {}

  async eiSessions(q: SessionsQuery) {
    const r = await this.agent.natSessions({
      offset: (q.page - 1) * q.pageSize,
      limit: q.pageSize,
      filter: filterOf(q),
      variant: NatSessionVariant.NAT_SESSION_VARIANT_EI,
    });
    return {
      page: q.page,
      pageSize: q.pageSize,
      total: Number(r.totalSessions),
      totalUsers: Number(r.totalUsers),
      truncated: r.truncated,
      retrievedAt: r.retrievedAt?.toISOString(),
      items: r.sessions.slice(0, q.pageSize).map(sessionJson),
    };
  }

  async nat64Sessions(q: Nat64SessionsQuery) {
    const r = await this.agent.natSessions({
      offset: (q.page - 1) * q.pageSize,
      limit: q.pageSize,
      filter: q.protocol === undefined ? {} : { protocol: q.protocol.toLowerCase() },
      variant: NatSessionVariant.NAT_SESSION_VARIANT_NAT64,
    });
    return {
      page: q.page,
      pageSize: q.pageSize,
      total: Number(r.totalSessions),
      totalClients: Number(r.totalUsers),
      truncated: r.truncated,
      retrievedAt: r.retrievedAt?.toISOString(),
      items: r.sessions.slice(0, q.pageSize).map(nat64SessionJson),
    };
  }

  async nptv6() {
    const running = await this.ds.getRunning();
    return nptv6Json(running.doc['nat']);
  }

  async eiKill(body: EiKillBody) {
    const done = await this.agent.natSessionKill({
      protocol: body.protocol,
      insideAddress: body.insideAddress,
      insidePort: body.insidePort,
      externalAddress: body.externalAddress ?? '',
      externalPort: body.externalPort ?? 0,
      vrf: body.vrf ?? '',
      variant: NatSessionVariant.NAT_SESSION_VARIANT_EI,
    });
    return killOutcome(done);
  }
}
