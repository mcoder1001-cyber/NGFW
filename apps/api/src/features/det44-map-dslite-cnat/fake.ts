import { status, type handleUnaryCall } from '@grpc/grpc-js';
import type {
  CnatSession,
  CnatSessionsRequest,
  CnatSessionsResponse,
  Det44LookupRequest,
  Det44LookupResponse,
  Det44Session,
  Det44SessionsRequest,
  Det44SessionsResponse,
} from '@ngfw/proto';
import {
  registerActionHandler,
  type ActionHandler,
  type FakeAgent,
} from '../../testing/fake-agent.js';

/**
 * The fake agent's DET44 / CNAT state RPCs and actions (F-det44-map-dslite-cnat; wired into `FakeAgent.impl()` by one
 * spread line). The deterministic mapping is computed from the applied `nat.det44.mappings` with VPP's formulas
 * (sharing ratio = 2^(outLen − inLen), ports per host = 64512 / ratio, first port = 1024 + ports × (offset mod ratio));
 * sessions are seeded by tests through `det44MapFake(fake)`. Contract as the agent's rpc_det44.go / rpc_cnat.go: owner
 * check, limit 0 = 100, > 1000 INVALID_ARGUMENT, NOT_FOUND for an unmapped user, the purge only for the globals owner
 * (owner "ngfw").
 */
export interface Det44MapFakeState {
  det44Sessions: Map<string, Det44Session[]>;
  cnatSessions: CnatSession[];
  purges: number;
}

const states = new WeakMap<FakeAgent, Det44MapFakeState>();

export function det44MapFake(fake: FakeAgent): Det44MapFakeState {
  let s = states.get(fake);
  if (s === undefined) {
    s = { det44Sessions: new Map(), cnatSessions: [], purges: 0 };
    states.set(fake, s);
  }
  return s;
}

const ip2n = (a: string): number => a.split('.').reduce((n, o) => n * 256 + Number(o), 0);
const n2ip = (n: number): string => [24, 16, 8, 0].map((s) => (n >>> s) & 255).join('.');
const isIp4 = (a: string): boolean =>
  /^(\d{1,3})(\.\d{1,3}){3}$/.test(a) && a.split('.').every((o) => Number(o) <= 255);

interface DetMap {
  inBase: number;
  inLen: number;
  outBase: number;
  outLen: number;
  ratio: number;
  ports: number;
}

function det44Maps(fake: FakeAgent): DetMap[] {
  const nat = (fake.current['nat'] ?? {}) as Record<string, unknown>;
  const det = (nat['det44'] ?? {}) as Record<string, unknown>;
  const out: DetMap[] = [];
  for (const m of (det['mappings'] as Record<string, unknown>[] | undefined) ?? []) {
    const [ia = '', il = '32'] = String(m['inside'] ?? '').split('/');
    const [oa = '', ol = '32'] = String(m['outside'] ?? '').split('/');
    const ratio = 2 ** Math.max(0, Number(ol) - Number(il));
    out.push({
      inBase: ip2n(ia),
      inLen: Number(il),
      outBase: ip2n(oa),
      outLen: Number(ol),
      ratio,
      ports: Math.floor(64512 / ratio),
    });
  }
  return out;
}

const within = (a: number, base: number, len: number): boolean =>
  len === 0 || Math.floor(a / 2 ** (32 - len)) === Math.floor(base / 2 ** (32 - len));

export function det44Forward(
  maps: DetMap[],
  inside: string,
): { outside: string; lo: number; hi: number } | undefined {
  const a = ip2n(inside);
  const m = maps.find((x) => within(a, x.inBase, x.inLen));
  if (m === undefined) return undefined;
  const off = a - m.inBase;
  const lo = 1024 + m.ports * (off % m.ratio);
  return { outside: n2ip(m.outBase + Math.floor(off / m.ratio)), lo, hi: lo + m.ports - 1 };
}

export function det44Reverse(maps: DetMap[], outside: string, port: number): string | undefined {
  const a = ip2n(outside);
  const m = maps.find((x) => within(a, x.outBase, x.outLen));
  if (m === undefined || port < 1024) return undefined;
  const po = Math.floor((port - 1024) / m.ports);
  if (po >= m.ratio) return undefined;
  return n2ip(m.inBase + (a - m.outBase) * m.ratio + po);
}

type Cb<T> = Parameters<handleUnaryCall<unknown, T>>[1];

function limitOf(l: number): number | undefined {
  if (l === 0) return 100;
  return l > 1000 ? undefined : l;
}

let actionFake: FakeAgent | undefined;
const err = (code: status, details: string) => Object.assign(new Error(details), { code, details });

const closeAction: ActionHandler = (call) => {
  const fake = actionFake;
  const a = call.request.det44SessionClose;
  if (fake === undefined || a === undefined) {
    call.emit('error', err(status.UNAVAILABLE, 'fake agent: no det44 fake'));
    return;
  }
  if (!['in', 'out'].includes(a.direction) || !isIp4(a.address) || !isIp4(a.externalAddress)) {
    call.emit('error', err(status.INVALID_ARGUMENT, 'bad det44 close request'));
    return;
  }
  const maps = det44Maps(fake);
  const user = a.direction === 'in' ? a.address : det44Reverse(maps, a.address, a.port);
  const list = user === undefined ? undefined : det44MapFake(fake).det44Sessions.get(user);
  const i =
    list?.findIndex(
      (s) =>
        (a.direction === 'in' ? s.insidePort === a.port : s.outsidePort === a.port) &&
        s.externalAddress === a.externalAddress &&
        s.externalPort === a.externalPort,
    ) ?? -1;
  if (list !== undefined && i >= 0) list.splice(i, 1);
  const what = `${a.direction} ${a.address}:${a.port} ↔ ${a.externalAddress}:${a.externalPort}`;
  call.write({
    done:
      i >= 0
        ? { summary: `closed DET44 session ${what}`, exitCode: 0, stats: {} }
        : { summary: `no DET44 session ${what}`, exitCode: 1, stats: {} },
  });
  call.end();
};

const purgeAction: ActionHandler = (call) => {
  const fake = actionFake;
  if (fake === undefined) {
    call.emit('error', err(status.UNAVAILABLE, 'fake agent: no cnat fake'));
    return;
  }
  if (fake.owner !== 'ngfw') {
    call.emit(
      'error',
      err(
        status.PERMISSION_DENIED,
        'the CNAT session table is a VPP global: only the globals owner may purge it',
      ),
    );
    return;
  }
  const st = det44MapFake(fake);
  st.cnatSessions = [];
  st.purges++;
  call.write({ done: { summary: 'purged the CNAT session table', exitCode: 0, stats: {} } });
  call.end();
};

export function det44MapDsliteCnatFake(fake: FakeAgent): {
  det44Sessions: handleUnaryCall<Det44SessionsRequest, Det44SessionsResponse>;
  det44Lookup: handleUnaryCall<Det44LookupRequest, Det44LookupResponse>;
  cnatSessions: handleUnaryCall<CnatSessionsRequest, CnatSessionsResponse>;
} {
  actionFake = fake;
  registerActionHandler('det44SessionClose', closeAction);
  registerActionHandler('cnatSessionPurge', purgeAction);
  const pre = (method: string, req: { owner: string }, cb: Cb<never>): boolean => {
    fake.calls.push({ method, request: req });
    if (fake.failAllWith !== undefined) {
      cb(err(fake.failAllWith, 'fake agent: failing every call'), null);
      return false;
    }
    if (req.owner && req.owner !== fake.owner) {
      cb(err(status.INVALID_ARGUMENT, `owner '${req.owner}' ≠ agent owner '${fake.owner}'`), null);
      return false;
    }
    return true;
  };
  return {
    det44Sessions: (call, cb) => {
      const r = call.request;
      if (!pre('Det44Sessions', r, cb as Cb<never>)) return;
      const limit = limitOf(r.limit);
      if (limit === undefined)
        return cb(err(status.INVALID_ARGUMENT, `limit ${r.limit} > 1000`), null);
      if (!isIp4(r.user)) return cb(err(status.INVALID_ARGUMENT, `user ${r.user}`), null);
      const fw = det44Forward(det44Maps(fake), r.user);
      if (fw === undefined)
        return cb(err(status.NOT_FOUND, `no DET44 mapping for ${r.user}`), null);
      const all = det44MapFake(fake).det44Sessions.get(r.user) ?? [];
      const page = all.slice(r.offset, r.offset + limit);
      const end = r.offset + page.length;
      cb(null, {
        sessions: page,
        ...(end < all.length ? { nextOffset: end } : {}),
        totalSessions: String(all.length),
        outsideAddress: fw.outside,
        portLo: fw.lo,
        portHi: fw.hi,
        owner: fake.owner,
        retrievedAt: new Date(),
      });
    },
    det44Lookup: (call, cb) => {
      const r = call.request;
      if (!pre('Det44Lookup', r, cb as Cb<never>)) return;
      const maps = det44Maps(fake);
      if (r.insideAddress !== undefined && r.outsideAddress === undefined) {
        const fw = det44Forward(maps, r.insideAddress);
        if (fw === undefined)
          return cb(err(status.NOT_FOUND, `no DET44 mapping for ${r.insideAddress}`), null);
        return cb(null, {
          insideAddress: r.insideAddress,
          outsideAddress: fw.outside,
          portLo: fw.lo,
          portHi: fw.hi,
        });
      }
      if (r.outsideAddress !== undefined && r.insideAddress === undefined) {
        if (r.outsidePort === undefined || r.outsidePort < 1024)
          return cb(err(status.INVALID_ARGUMENT, 'outside_port ≥ 1024 is required'), null);
        const inside = det44Reverse(maps, r.outsideAddress, r.outsidePort);
        if (inside === undefined)
          return cb(err(status.NOT_FOUND, `no DET44 mapping for ${r.outsideAddress}`), null);
        return cb(null, {
          insideAddress: inside,
          outsideAddress: r.outsideAddress,
          portLo: 0,
          portHi: 0,
        });
      }
      cb(
        err(status.INVALID_ARGUMENT, 'set exactly one of inside_address or outside_address'),
        null,
      );
    },
    cnatSessions: (call, cb) => {
      const r = call.request;
      if (!pre('CnatSessions', r, cb as Cb<never>)) return;
      const limit = limitOf(r.limit);
      if (limit === undefined)
        return cb(err(status.INVALID_ARGUMENT, `limit ${r.limit} > 1000`), null);
      const all = det44MapFake(fake).cnatSessions;
      const page = all.slice(r.offset, r.offset + limit);
      const end = r.offset + page.length;
      cb(null, {
        sessions: page,
        ...(end < all.length ? { nextOffset: end } : {}),
        totalSessions: String(all.length),
        truncated: false,
        owner: fake.owner,
        retrievedAt: new Date(),
      });
    },
  };
}
