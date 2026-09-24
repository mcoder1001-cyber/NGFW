import { status, type handleUnaryCall } from '@grpc/grpc-js';
import {
  NatSessionVariant,
  type NatSession,
  type NatSessionsRequest,
  type NatSessionsResponse,
} from '@ngfw/proto';
import type { FakeAgent } from '../../testing/fake-agent.js';
import { fakeSession, nat44EdSessionsFake } from '../nat44-ed-sessions/fake.js';

/**
 * The fake agent's NatSessions for the EI and NAT64 variants (wave-A-hotspots P5: one spread line in
 * `FakeAgent.impl()`, after F-nat44-ed-sessions' so this handler wins). It follows the agent's contract
 * (proto.md §11 "NAT session variants"): owner check, limit 0 = 100, > 1000 INVALID_ARGUMENT, the variant echoed;
 * EI: the ED filter and order over `natVariantsFake(fake).ei`; NAT64: only `filter.protocol` (anything else
 * INVALID_ARGUMENT) over `natVariantsFake(fake).nat64`; an unset / ED variant is handed to F-nat44-ed-sessions' fake
 * unchanged. The EI kill runs through the fake's generic Action handler (UNIMPLEMENTED, F-nat44-ed-sessions Q5).
 */
export interface NatVariantsFakeState {
  ei: NatSession[];
  nat64: NatSession[];
}

const states = new WeakMap<FakeAgent, NatVariantsFakeState>();

/** The EI / NAT64 session state of one fake agent (created on first use, empty). */
export function natVariantsFake(fake: FakeAgent): NatVariantsFakeState {
  let s = states.get(fake);
  if (s === undefined) {
    s = { ei: [], nat64: [] };
    states.set(fake, s);
  }
  return s;
}

/** A NAT64 session row (IPv6 client, IPv4 pool, IPv4 remote, the remote's IPv6 form). */
export function fakeNat64Session(p: Partial<NatSession>): NatSession {
  return fakeSession({
    insideAddress: 'fd00:1::10',
    insidePort: 40000,
    outsideAddress: '10.1.64.1',
    outsidePort: 1024,
    externalAddress: '10.1.2.2',
    externalPort: 80,
    externalNatAddress: 'fd00:1:64::a01:202',
    externalNatPort: 80,
    idleSeconds: '0',
    bytes: '0',
    packets: '0',
    ...p,
  });
}

const ipNum = (a: string): number => a.split('.').reduce((n, o) => n * 256 + Number(o), 0);

export function nat44Ei6466Nptv6Fake(fake: FakeAgent): {
  natSessions: handleUnaryCall<NatSessionsRequest, NatSessionsResponse>;
} {
  const ed = nat44EdSessionsFake(fake).natSessions;
  return {
    natSessions: (call, cb) => {
      const r = call.request;
      const variant = r.variant ?? NatSessionVariant.NAT_SESSION_VARIANT_UNSPECIFIED;
      if (
        variant !== NatSessionVariant.NAT_SESSION_VARIANT_EI &&
        variant !== NatSessionVariant.NAT_SESSION_VARIANT_NAT64
      ) {
        ed(call, cb);
        return;
      }
      fake.calls.push({ method: 'NatSessions', request: r });
      if (r.owner && r.owner !== fake.owner)
        return cb({
          code: status.INVALID_ARGUMENT,
          details: `owner '${r.owner}' ≠ agent owner '${fake.owner}'`,
        });
      const limit = r.limit === 0 ? 100 : r.limit;
      if (limit > 1000)
        return cb({ code: status.INVALID_ARGUMENT, details: `limit ${limit} > 1000` });
      const f = r.filter ?? {};
      const st = natVariantsFake(fake);
      let rows: NatSession[];
      if (variant === NatSessionVariant.NAT_SESSION_VARIANT_NAT64) {
        if (
          f.insideAddress !== undefined ||
          f.outsideAddress !== undefined ||
          f.externalAddress !== undefined ||
          f.port !== undefined ||
          f.vrf !== undefined
        )
          return cb({
            code: status.INVALID_ARGUMENT,
            details: 'NAT64 sessions accept only filter.protocol',
          });
        rows = st.nat64.filter((s) => f.protocol === undefined || s.protocol === f.protocol);
      } else {
        rows = [...st.ei]
          .sort((a, b) => a.tableId - b.tableId || ipNum(a.insideAddress) - ipNum(b.insideAddress))
          .filter(
            (s) =>
              (f.insideAddress === undefined || s.insideAddress === f.insideAddress) &&
              (f.outsideAddress === undefined || s.outsideAddress === f.outsideAddress) &&
              (f.externalAddress === undefined || s.externalAddress === f.externalAddress) &&
              (f.protocol === undefined || s.protocol === f.protocol) &&
              (f.vrf === undefined || s.vrf === f.vrf) &&
              (f.port === undefined ||
                [s.insidePort, s.outsidePort, s.externalPort].includes(f.port)),
          );
      }
      const page = rows.slice(r.offset, r.offset + limit);
      const end = r.offset + page.length;
      cb(null, {
        sessions: page,
        ...(end < rows.length ? { nextOffset: end } : {}),
        totalUsers: String(new Set(rows.map((s) => `${s.tableId}|${s.insideAddress}`)).size),
        totalSessions: String(rows.length),
        truncated: false,
        owner: fake.owner,
        retrievedAt: new Date(),
        variant,
      });
    },
  };
}
