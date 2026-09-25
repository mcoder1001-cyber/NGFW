import {
  Server,
  ServerCredentials,
  status,
  type handleServerStreamingCall,
  type handleUnaryCall,
  type sendUnaryData,
  type ServerUnaryCall,
} from '@grpc/grpc-js';
import {
  type ActionOutput,
  type ActionRequest,
  type ApplyRequest,
  type ApplyResponse,
  ApplyOperation,
  ApplyStatus,
  DataplaneService,
  type DataplaneServer,
  DesiredState,
  type DryRunRequest,
  type Event,
  EventKind,
  type HealthRequest,
  type HealthResponse,
  type InterfaceState,
  type InterfaceStateRequest,
  type InterfaceStateResponse,
  IssueSeverity,
  ObjectResultCode,
  type ObjectResult,
  type RetrieveRequest,
  type RetrieveResponse,
  type StatsBatch,
  type StreamEventsRequest,
  type StreamStatsRequest,
  type ValidationIssue,
  type ValidationReport,
} from '@ngfw/proto';
import { deepEqual, escapePointerSegment, ROOT_KEYS } from '@ngfw/schema';
import { snmpStateFake } from '../features/snmp/fake.js'; // F-snmp (unanchored import)
import { ipfixStateFake } from '../features/ipfix-sflow/fake.js'; // F-ipfix-sflow
import { EventEmitter } from 'node:events';
import { mkdirSync, rmSync } from 'node:fs';
import { dirname } from 'node:path';
import { hostStackFake } from '../features/host-stack/fake.js'; // F-host-stack (P5)
import { lispStateFake } from '../features/lisp/fake.js';

/**
 * In-process fake of the P03 `vrx.v1.Dataplane` service (P05 is not merged — TASK ENVELOPE). It follows the
 * semantics of docs/contracts/proto.md that the API depends on: owner check, D-041 subsystem selection, txn_id
 * idempotency, one pending confirm at a time with a self-revert timer (CONFIRM_REVERTED + RECONCILE events), DryRun
 * without side effects, Retrieve of the applied state, 1 Hz-style StreamStats and StreamEvents with per-stream seq.
 * Tests steer failures through `nextApply` / `dryRunIssues`. It never touches VPP.
 */
type Json = Record<string, unknown>;

// --- Action dispatch (D-134/TD-23) --------------------------------------------------------------
//
// README: features register here; never edit the dispatcher. Call registerActionHandler instead of
// editing the `action` const inside FakeAgent.impl(), and never spread a whole replacement `action`
// into impl()'s returned object. Before this seam, four wave-A branches
// each did the latter to the same `action:` property; whichever merged last silently won and the
// other three lost Action support with no error. One handler per ActionRequest oneof case, keyed by
// whichever field of the request is set — 'ping' | 'traceroute' | 'capture' today, plus whatever a
// later contract PR adds to the oneof (ActionKind tracks it automatically via `keyof ActionRequest`).
//
// Reset convention (fix round 1, F4): actionHandlers is a module-level global, shared by every
// FakeAgent in a test file (or worker) — registering under one agent's owner is visible to all of
// them. Any test file that calls registerActionHandler MUST call resetActionHandlersForTest() in its
// own afterEach, exactly as fake-agent-action.test.ts does; nothing enforces this globally (there is
// no shared Vitest setupFiles hook), so it is a convention, not a guarantee. Vitest's default
// per-file isolation keeps a forgotten reset from leaking across *files*, but not across the `it`
// blocks of one file.

/** The populated field of an ActionRequest, i.e. which oneof case it carries. */
export type ActionKind = keyof ActionRequest;

/**
 * A feature's Action implementation for one oneof case; same shape as the RPC itself. May be async
 * (fix round 1, F3): the dispatcher below awaits/catches a returned Promise the same way it catches a
 * synchronous throw, so `async (call) => { ...; throw ... }` still ends the call with a status
 * instead of hanging.
 */
export type ActionHandler = handleServerStreamingCall<ActionRequest, ActionOutput>;

const actionHandlers: Partial<Record<ActionKind, ActionHandler>> = {};

/**
 * Registers fn as the fake agent's handler for ActionRequest's `kind` oneof case (e.g. 'ping'). Call
 * it once per kind — module load time is fine (features/<slug>/fake.ts). Registering a kind that is
 * already registered throws: two features claiming the same case is a bug to resolve explicitly at
 * rebase, not a silent last-registration-wins.
 */
export function registerActionHandler(kind: ActionKind, fn: ActionHandler): void {
  if (actionHandlers[kind] !== undefined) {
    throw new Error(`fake-agent: an action handler for '${kind}' is already registered`);
  }
  actionHandlers[kind] = fn;
}

/** Test-only: undoes registerActionHandler between test cases. Never called from product code. */
export function resetActionHandlersForTest(): void {
  for (const kind of Object.keys(actionHandlers) as ActionKind[]) {
    delete actionHandlers[kind];
  }
}

/** Which oneof case request carries, or undefined if none is set (an empty/malformed request). */
function actionKindOf(request: ActionRequest): ActionKind | undefined {
  return (Object.keys(request) as ActionKind[]).find((k) => request[k] !== undefined);
}

/** Wraps err as a gRPC-status Error, keeping its own numeric `code` if it set one. */
function asGrpcError(err: unknown, fallback: status): Error & { code: status } {
  const code = (err as { code?: unknown } | undefined)?.code;
  const resolved = typeof code === 'number' ? (code as status) : fallback;
  return err instanceof Error
    ? Object.assign(err, { code: resolved })
    : Object.assign(new Error(String(err)), { code: resolved });
}

export interface FakeAgentOptions {
  owner?: string;
  subsystems?: string[];
}

export class FakeAgent {
  readonly owner: string;
  implemented: string[];
  /** DesiredState in protobuf JSON form. */
  current: Json = {};
  confirmed: Json = {};
  pending: { txnId: string; deadline: Date; timer: NodeJS.Timeout } | undefined;
  lastTxnId = '';
  degraded = false;
  lastReconcileAt: Date | undefined;
  /** Every request received, for assertions. */
  calls: { method: string; request: unknown }[] = [];
  /** One-shot override of the next Apply outcome (after request checks, before any state change). */
  nextApply: ((req: ApplyRequest) => Partial<ApplyResponse> | undefined) | undefined;
  /** Findings of DryRun / Apply validation for a desired state (protobuf JSON). */
  dryRunIssues: ((desired: Json) => ValidationIssue[]) | undefined;
  /** gRPC status to fail every call with (simulates an unreachable/broken agent). */
  failAllWith: status | undefined;
  /** Answer Apply only after this delay — the transaction IS applied (simulates a lost/late answer). */
  applyDelayMs = 0;
  /**
   * P08 InterfaceState fidelity (review N6): extra live rows the agent does not manage (appended, `managed:false`
   * unless set), configured interfaces VPP does not have (left out of the live table), and an agent older than
   * P08 (the RPC answers UNIMPLEMENTED).
   */
  liveExtra: Partial<InterfaceState>[] = [];
  liveMissing = new Set<string>();
  interfaceStateUnimplemented = false;

  private server: Server | undefined;
  private socketPath = '';
  private readonly bus = new EventEmitter();
  private readonly txnCache = new Map<string, { key: string; res: ApplyResponse }>();
  private counters = new Map<string, number>();

  constructor(opts: FakeAgentOptions = {}) {
    this.owner = opts.owner ?? 'w1';
    this.implemented = opts.subsystems ?? [...ROOT_KEYS];
    this.bus.setMaxListeners(100);
  }

  async start(socketPath: string): Promise<void> {
    mkdirSync(dirname(socketPath), { recursive: true });
    rmSync(socketPath, { force: true });
    this.socketPath = socketPath;
    const server = new Server();
    server.addService(DataplaneService, this.impl());
    await new Promise<void>((resolve, reject) =>
      server.bindAsync(`unix:${socketPath}`, ServerCredentials.createInsecure(), (err) =>
        err ? reject(err) : resolve(),
      ),
    );
    this.server = server;
  }

  async stop(): Promise<void> {
    if (this.pending) clearTimeout(this.pending.timer);
    this.pending = undefined;
    this.bus.emit('close');
    const server = this.server;
    this.server = undefined;
    if (server) {
      await new Promise<void>((resolve) => {
        const t = setTimeout(() => {
          server.forceShutdown();
          resolve();
        }, 1000);
        server.tryShutdown(() => {
          clearTimeout(t);
          resolve();
        });
      });
    }
    if (this.socketPath) rmSync(this.socketPath, { force: true });
  }

  /** Back to a fresh agent whose actual (and confirmed) state is `state` (protobuf JSON). */
  reset(state: Json = {}): void {
    if (this.pending) clearTimeout(this.pending.timer);
    this.pending = undefined;
    this.current = structuredClone(state);
    this.confirmed = structuredClone(state);
    this.calls = [];
    this.nextApply = undefined;
    this.dryRunIssues = undefined;
    this.failAllWith = undefined;
    this.applyDelayMs = 0;
    this.liveExtra = [];
    this.liveMissing = new Set();
    this.interfaceStateUnimplemented = false;
    this.implemented = [...ROOT_KEYS];
    this.degraded = false;
    this.txnCache.clear();
  }

  /** Emit an agent event to every StreamEvents subscriber. */
  emit(kind: EventKind, fields: Partial<Event> = {}): void {
    this.bus.emit('event', { kind, ...fields });
  }

  private record(method: string, request: unknown): void {
    this.calls.push({ method, request });
  }

  private checkCommon<T>(
    method: string,
    req: T & { owner?: string },
    cb: (err: { code: status; details: string } | null) => void,
  ): boolean {
    this.record(method, req);
    if (this.failAllWith !== undefined) {
      cb({ code: this.failAllWith, details: `fake agent: forced ${status[this.failAllWith]}` });
      return false;
    }
    if (req.owner && req.owner !== this.owner) {
      cb({
        code: status.INVALID_ARGUMENT,
        details: `owner '${req.owner}' ≠ agent owner '${this.owner}'`,
      });
      return false;
    }
    return true;
  }

  /** D-041: the domains an Apply/DryRun manages. */
  private managed(desired: Json, subsystems: readonly string[]): string[] {
    const unknown = subsystems.filter((s) => !(ROOT_KEYS as readonly string[]).includes(s));
    if (unknown.length > 0)
      throw Object.assign(new Error(`unknown subsystem ${unknown[0]}`), {
        code: status.INVALID_ARGUMENT,
      });
    const notImpl = subsystems.filter((s) => !this.implemented.includes(s));
    if (notImpl.length > 0)
      throw Object.assign(new Error(`not implemented: ${notImpl.join(',')}`), {
        code: status.UNIMPLEMENTED,
      });
    const present = Object.keys(desired);
    return subsystems.length > 0
      ? [...subsystems]
      : present.filter((k) => this.implemented.includes(k));
  }

  /** Object-level plan: one entry per interface / vrf created, updated or deleted; other domains as one object. */
  private plan(from: Json, to: Json, domains: readonly string[]): ObjectResult[] {
    const out: ObjectResult[] = [];
    for (const d of domains) {
      const a = (from[d] ?? {}) as Json;
      const b = (to[d] ?? {}) as Json;
      if (d === 'interfaces' || d === 'vrfs') {
        const desc = d === 'interfaces' ? 'interface' : 'vrf';
        for (const k of [...new Set([...Object.keys(a), ...Object.keys(b)])].sort()) {
          const op =
            a[k] === undefined
              ? ApplyOperation.APPLY_OPERATION_CREATE
              : b[k] === undefined
                ? ApplyOperation.APPLY_OPERATION_DELETE
                : deepEqual(a[k], b[k])
                  ? undefined
                  : ApplyOperation.APPLY_OPERATION_UPDATE;
          if (op !== undefined) {
            out.push({
              key: `${desc}/${k}`,
              op,
              code: ObjectResultCode.OBJECT_RESULT_CODE_OK,
              message: '',
              pointer: `/${d}/${escapePointerSegment(k)}`,
              subsystem: d,
            });
          }
        }
      } else if (!deepEqual(a, b)) {
        out.push({
          key: `${d}/config`,
          op: ApplyOperation.APPLY_OPERATION_UPDATE,
          code: ObjectResultCode.OBJECT_RESULT_CODE_OK,
          message: '',
          pointer: `/${d}`,
          subsystem: d,
        });
      }
    }
    return out;
  }

  private summary(results: readonly ObjectResult[]) {
    const n = (op: ApplyOperation) => results.filter((r) => r.op === op).length;
    return {
      created: n(ApplyOperation.APPLY_OPERATION_CREATE),
      updated:
        n(ApplyOperation.APPLY_OPERATION_UPDATE) + n(ApplyOperation.APPLY_OPERATION_RECREATE),
      deleted: n(ApplyOperation.APPLY_OPERATION_DELETE),
      unchanged: 0,
      failed: results.filter((r) => r.code === ObjectResultCode.OBJECT_RESULT_CODE_FAILED).length,
      reverted: results.filter((r) => r.code === ObjectResultCode.OBJECT_RESULT_CODE_REVERTED)
        .length,
    };
  }

  private revert(txnId: string): void {
    this.pending = undefined;
    this.emit(EventKind.EVENT_KIND_CONFIRM_REVERTED, {
      txnId,
      message: `transaction ${txnId} not confirmed`,
    });
    this.emit(EventKind.EVENT_KIND_RECONCILE_START, { txnId: '' });
    this.current = structuredClone(this.confirmed);
    this.lastReconcileAt = new Date();
    this.emit(EventKind.EVENT_KIND_RECONCILE_DONE, { txnId: '', message: 'APPLY_STATUS_APPLIED' });
  }

  private impl(): DataplaneServer {
    const apply: handleUnaryCall<ApplyRequest, ApplyResponse> = (
      call: ServerUnaryCall<ApplyRequest, ApplyResponse>,
      cb: sendUnaryData<ApplyResponse>,
    ) => {
      const r = call.request;
      if (!this.checkCommon('Apply', r, cb)) return;
      const hasDesired = r.desiredState !== undefined;
      if (!r.confirmTxnId && !(r.txnId && hasDesired)) {
        return cb({
          code: status.INVALID_ARGUMENT,
          details: 'apply needs txn_id + desired_state or confirm_txn_id',
        });
      }
      if (r.confirmTxnId) {
        if (!this.pending || this.pending.txnId !== r.confirmTxnId) {
          return cb({
            code: status.FAILED_PRECONDITION,
            details: `transaction ${r.confirmTxnId} is not pending`,
          });
        }
        clearTimeout(this.pending.timer);
        this.pending = undefined;
        this.confirmed = structuredClone(this.current);
        if (!hasDesired) {
          return cb(null, {
            txnId: r.confirmTxnId,
            status: ApplyStatus.APPLY_STATUS_CONFIRMED,
            results: [],
            summary: this.summary([]),
            validation: undefined,
            appliedAt: new Date(),
            confirmDeadline: undefined,
            message: '',
          });
        }
      } else if (this.pending) {
        return cb({
          code: status.FAILED_PRECONDITION,
          details: `transaction ${this.pending.txnId} is pending confirmation`,
        });
      }
      const desired = DesiredState.toJSON(r.desiredState as DesiredState) as Json;
      const key = JSON.stringify([desired, r.subsystems, r.confirmTimeoutSec]);
      const cached = this.txnCache.get(r.txnId);
      if (cached) {
        return cached.key === key
          ? cb(null, cached.res)
          : cb({
              code: status.ABORTED,
              details: `txn_id ${r.txnId} reused with different content`,
            });
      }
      let domains: string[];
      try {
        domains = this.managed(desired, r.subsystems);
      } catch (e) {
        return cb({ code: (e as { code: status }).code, details: (e as Error).message });
      }
      const respond = (res: ApplyResponse) => {
        this.txnCache.set(r.txnId, { key, res });
        cb(null, res);
      };
      const base = {
        txnId: r.txnId,
        validation: undefined,
        appliedAt: new Date(),
        confirmDeadline: undefined,
        message: '',
      };
      const issues = this.dryRunIssues?.(desired) ?? [];
      if (issues.some((i) => i.severity === IssueSeverity.ISSUE_SEVERITY_ERROR)) {
        return respond({
          ...base,
          status: ApplyStatus.APPLY_STATUS_FAILED,
          results: [],
          summary: this.summary([]),
          validation: {
            txnId: r.txnId,
            ok: false,
            errors: issues,
            plan: [],
            summary: this.summary([]),
          },
        });
      }
      const injected = this.nextApply?.(r);
      this.nextApply = undefined;
      if (injected !== undefined) {
        return respond({
          ...base,
          status: ApplyStatus.APPLY_STATUS_APPLIED,
          results: [],
          summary: this.summary([]),
          ...injected,
        });
      }
      const results = this.plan(this.current, desired, domains);
      this.emit(EventKind.EVENT_KIND_RECONCILE_START, { txnId: r.txnId });
      const next = structuredClone(this.current);
      for (const d of domains) {
        if (desired[d] === undefined) delete next[d];
        else next[d] = structuredClone(desired[d]);
      }
      this.current = next;
      this.lastTxnId = r.txnId;
      this.lastReconcileAt = new Date();
      let confirmDeadline: Date | undefined;
      if (r.confirmTimeoutSec > 0) {
        confirmDeadline = new Date(Date.now() + r.confirmTimeoutSec * 1000);
        const txnId = r.txnId;
        this.pending = {
          txnId,
          deadline: confirmDeadline,
          timer: setTimeout(() => this.revert(txnId), r.confirmTimeoutSec * 1000),
        };
      } else {
        this.confirmed = structuredClone(this.current);
      }
      const summary = this.summary(results);
      this.emit(EventKind.EVENT_KIND_RECONCILE_DONE, {
        txnId: r.txnId,
        summary,
        message: 'APPLY_STATUS_APPLIED',
      });
      const res: ApplyResponse = {
        ...base,
        status: ApplyStatus.APPLY_STATUS_APPLIED,
        results,
        summary,
        confirmDeadline,
      };
      if (this.applyDelayMs > 0) setTimeout(() => respond(res), this.applyDelayMs);
      else respond(res);
    };

    const dryRun: handleUnaryCall<DryRunRequest, ValidationReport> = (call, cb) => {
      const r = call.request;
      if (!this.checkCommon('DryRun', r, cb)) return;
      const desired = DesiredState.toJSON(r.desiredState ?? DesiredState.fromPartial({})) as Json;
      let domains: string[];
      try {
        domains = this.managed(desired, r.subsystems);
      } catch (e) {
        return cb({ code: (e as { code: status }).code, details: (e as Error).message });
      }
      const errors = this.dryRunIssues?.(desired) ?? [];
      const ok = !errors.some((i) => i.severity === IssueSeverity.ISSUE_SEVERITY_ERROR);
      const plan = ok
        ? this.plan(this.current, desired, domains).map((p) => ({
            ...p,
            code: ObjectResultCode.OBJECT_RESULT_CODE_UNSPECIFIED,
          }))
        : [];
      cb(null, { txnId: r.txnId, ok, errors, plan, summary: this.summary(plan) });
    };

    const retrieve: handleUnaryCall<RetrieveRequest, RetrieveResponse> = (call, cb) => {
      const r = call.request;
      if (!this.checkCommon('Retrieve', r, cb)) return;
      const subs = r.subsystems.length > 0 ? r.subsystems : this.implemented;
      const picked: Json = {};
      for (const s of subs) if (this.current[s] !== undefined) picked[s] = this.current[s];
      cb(null, {
        desiredState: DesiredState.fromJSON(picked),
        subsystems: subs,
        owner: this.owner,
        retrievedAt: new Date(),
      });
    };

    const health: handleUnaryCall<HealthRequest, HealthResponse> = (call, cb) => {
      if (!this.checkCommon('Health', call.request, cb)) return;
      cb(null, {
        agentVersion: 'fake-0.0.0',
        vppConnected: true,
        vppVersion: 'fake',
        owner: this.owner,
        subsystems: this.implemented,
        lastTxnId: this.lastTxnId,
        pendingConfirmTxnId: this.pending?.txnId ?? '',
        confirmDeadline: this.pending?.deadline,
        degraded: this.degraded,
        lastReconcileAt: this.lastReconcileAt,
        reconcileInProgress: false,
      });
    };

    const streamStats: handleServerStreamingCall<StreamStatsRequest, StatsBatch> = (call) => {
      this.record('StreamStats', call.request);
      const interval = call.request.intervalMs || 1000;
      if (interval < 50 || interval > 60000) {
        call.destroy(
          Object.assign(new Error('interval out of range'), { code: status.INVALID_ARGUMENT }),
        );
        return;
      }
      let seq = 0;
      const timer = setInterval(() => {
        const names = Object.keys((this.current['interfaces'] ?? {}) as Json).sort();
        const want = call.request.interfaces;
        const counters = names
          .filter((n) => want.length === 0 || want.includes(n))
          .map((name, i) => {
            const v = (this.counters.get(name) ?? 0) + 10;
            this.counters.set(name, v);
            return {
              name,
              swIfIndex: i + 1,
              rxPackets: String(v),
              rxBytes: String(v * 100),
              txPackets: String(v),
              txBytes: String(v * 100),
              drops: '0',
              errors: '0',
              punts: '0',
              rxMisses: '0',
            };
          });
        seq += 1;
        call.write({
          ts: new Date(),
          seq: String(seq),
          interfaceCounters: counters,
          workerCpu: call.request.includeWorkerCpu
            ? [
                {
                  worker: 0,
                  name: 'vpp_main',
                  utilizationPct: 1.5,
                  vectorsPerCall: 1,
                  calls: '10',
                  vectors: '10',
                },
              ]
            : [],
          intervalMs: interval,
        });
      }, interval);
      const end = () => {
        clearInterval(timer);
        this.bus.off('close', close);
      };
      const close = () => {
        end();
        call.end();
      };
      this.bus.on('close', close);
      call.on('cancelled', end);
      call.on('close', end);
    };

    const streamEvents: handleServerStreamingCall<StreamEventsRequest, Event> = (call) => {
      this.record('StreamEvents', call.request);
      let seq = 0;
      const kinds = call.request.kinds;
      const onEvent = (e: Partial<Event> & { kind: EventKind }) => {
        if (kinds.length > 0 && !kinds.includes(e.kind)) return;
        seq += 1;
        call.write({
          ts: new Date(),
          seq: String(seq),
          interface: e.interface,
          message: e.message ?? '',
          txnId: e.txnId ?? '',
          summary: e.summary,
          attributes: e.attributes ?? {},
          kind: e.kind,
        });
      };
      const end = () => {
        this.bus.off('event', onEvent);
        this.bus.off('close', close);
      };
      const close = () => {
        end();
        call.end();
      };
      this.bus.on('event', onEvent);
      this.bus.on('close', close);
      call.on('cancelled', end);
      call.on('close', end);
    };

    // P08: the live table of the applied interfaces (admin state = `enabled`, link up with it).
    const interfaceState: handleUnaryCall<InterfaceStateRequest, InterfaceStateResponse> = (
      call,
      cb,
    ) => {
      const r = call.request;
      if (!this.checkCommon('InterfaceState', r, cb)) return;
      if (this.interfaceStateUnimplemented) {
        return cb({ code: status.UNIMPLEMENTED, details: 'unknown method InterfaceState' });
      }
      const ifs = (this.current['interfaces'] ?? {}) as Record<string, Json>;
      const out: InterfaceState[] = [];
      let idx = 1;
      const row = (name: string, c: Json, parent: string, vlanId: number): InterfaceState => ({
        name,
        vppName: name,
        swIfIndex: idx++,
        type: parent ? 'sub-interface' : name.startsWith('loop') ? 'loopback' : 'af-packet',
        adminUp: c['enabled'] === true,
        linkUp: c['enabled'] === true,
        mtu: typeof c['mtu'] === 'number' ? c['mtu'] : 9000,
        linkMtu: 9000,
        mac: '02:fe:00:00:00:01',
        ipv4: (c['ipv4'] as string[] | undefined) ?? [],
        ipv6: (c['ipv6'] as string[] | undefined) ?? [],
        vrf: typeof c['vrf'] === 'string' ? c['vrf'] : 'default',
        tableId: 0,
        parent,
        vlanId,
        innerVlanId: 0,
        managed: true,
        linkSpeedKbps: '0',
        rxMode: 'interrupt',
        description: typeof c['description'] === 'string' ? c['description'] : '',
      });
      for (const name of Object.keys(ifs).sort()) {
        const c = ifs[name]!;
        out.push(row(name, c, '', 0));
        const subs = (c['subinterfaces'] ?? {}) as Record<string, Json>;
        for (const id of Object.keys(subs).sort()) {
          const s = subs[id]!;
          out.push(
            row(`${name}.${id}`, s, name, typeof s['vlanId'] === 'number' ? s['vlanId'] : 0),
          );
        }
      }
      for (const x of this.liveExtra) {
        const name = x.name ?? `extra${idx}`;
        out.push({ ...row(name, {}, x.parent ?? '', x.vlanId ?? 0), managed: false, ...x });
      }
      const want = r.names;
      cb(null, {
        interfaces: out.filter(
          (i) => !this.liveMissing.has(i.name) && (want.length === 0 || want.includes(i.name)),
        ),
        owner: this.owner,
        retrievedAt: new Date(),
      });
    };

    // Dispatcher — never edit this to add a feature's case; call registerActionHandler instead (see
    // the README comment near ActionKind above). It owns ending the call with a status on every path:
    // no handler for the request's oneof case answers UNIMPLEMENTED, and a handler that throws —
    // synchronously, or asynchronously by rejecting (fix round 1, F3: ActionHandler's type says
    // `void`, but TS lets an `async` function satisfy that, so a handler can return a Promise at
    // runtime even though the type doesn't say so; `result instanceof Promise` catches it either
    // way) — answers with its own `code` or INTERNAL. Every path ends the call through 'error', never
    // destroy() (D-134: a destroyed stream never reaches the client with a status, so the caller
    // hangs to its deadline).
    const action: handleServerStreamingCall<ActionRequest, ActionOutput> = (call) => {
      this.record('Action', call.request);
      const kind = actionKindOf(call.request);
      const handler = kind === undefined ? undefined : actionHandlers[kind];
      if (!handler) {
        call.emit(
          'error',
          Object.assign(new Error(`action '${kind ?? '(unset)'}' is not implemented`), {
            code: status.UNIMPLEMENTED,
          }),
        );
        return;
      }
      try {
        const result: unknown = handler(call);
        if (result instanceof Promise) {
          result.catch((err: unknown) => {
            call.emit('error', asGrpcError(err, status.INTERNAL));
          });
        }
      } catch (err) {
        call.emit('error', asGrpcError(err, status.INTERNAL));
      }
    };

    return {
      apply,
      dryRun,
      retrieve,
      health,
      interfaceState,
      streamStats,
      streamEvents,
      action,
      // Feature RPCs: one handler line under the feature's anchor (the contract commit's UNIMPLEMENTED stub;
      // real fake behaviour lives in features/<slug>/fake.ts, wired by the same line — wave-A-hotspots P5).
      // wave-BC: F-det44-map-dslite-cnat
      // wave-BC: F-tunnels
      // wave-BC: F-vrrp-config-sync
      // wave-BC: F-pki
      // wave-BC: F-ikev2-native
      // wave-BC: F-ospf
      // wave-BC: F-isis-rip
      // wave-BC: F-mpls-srmpls
      // wave-BC: F-lb
      // wave-BC: F-qos-flat
      // wave-BC: F-host-stack
      hostStackState: hostStackFake(this.owner, () => this.current),
      // wave-BC: F-snmp
      snmpState: snmpStateFake(() => this.current),
      // wave-BC: F-ipfix-sflow
      ipfixState: ipfixStateFake(this),
      // wave-BC: F-capture-trace
      // wave-BC: F-srv6
      // wave-BC: F-lisp
      lispState: lispStateFake({ owner: this.owner, current: () => this.current, record: (m, r) => this.record(m, r), failWith: () => this.failAllWith }),
      // wave-BC: F-bfd-redistribution
      // wave-BC: F-ra-vpn
      // wave-BC: F-mpls-ldp
      // wave-BC: F-igmp-mfib
      // wave-BC: F-dashboard-prom-alarms
      // wave-BC: F-ha-state-sync
      // wave-A: F-bonding
      // wave-A: F-bridge-l2
      // wave-A: F-loopback-bvi-gso-lldp-span
      // wave-A: F-vrf-static-ecmp
      // wave-A: F-neighbors-ra
      // wave-A: F-rpf-adl-pbr
      // wave-A: F-object-model
      // wave-A: F-acl
      // wave-A: F-host-acl-nftables
      // wave-A: F-nat44-ed-sessions
      // wave-A: F-nat44-ei-64-66-nptv6
      // wave-A: P11
      // wave-A: F-wireguard
      // wave-A: P12
      // wave-A: F-kea-dhcp-relay
      // wave-A: F-unbound-chrony-syslog
    };
  }
}
