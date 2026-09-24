import {
  connectivityState,
  credentials,
  Metadata,
  status as GrpcStatus,
  type ServiceError,
} from '@grpc/grpc-js';
import { Inject, Injectable, type OnModuleDestroy } from '@nestjs/common';
import {
  type ApplyRequest,
  type ApplyResponse,
  DataplaneClient,
  type DesiredState,
  type DryRunRequest,
  type Event,
  type HealthResponse,
  type RetrieveResponse,
  type StatsBatch,
  type StreamEventsRequest,
  type StreamStatsRequest,
  type ValidationReport,
} from '@ngfw/proto';
import type { ClientReadableStream } from '@grpc/grpc-js';
import { ENV, type Env } from '../config.js';
import { ProblemError } from '../common/problem.js';

/**
 * The only way the API reaches the data plane (00-CONTEXT rule 1): gRPC over the agent's unix socket, stubs from
 * `@ngfw/proto` (D-005). The channel is created lazily, so building the application (OpenAPI generation, route
 * tests) never touches a socket. gRPC failures become problem+json (proto.md §2 "Outcomes").
 */
@Injectable()
export class AgentClient implements OnModuleDestroy {
  private client: DataplaneClient | undefined;

  constructor(@Inject(ENV) private readonly env: Env) {}

  get socket(): string {
    return this.env.VRX_AGENT_SOCKET;
  }

  /** Owner stated on every request (proto.md §6): a request that reaches a foreign agent fails loudly. */
  get owner(): string {
    return this.env.VRX_AGENT_OWNER;
  }

  private get c(): DataplaneClient {
    this.client ??= new DataplaneClient(
      `unix:${this.env.VRX_AGENT_SOCKET}`,
      credentials.createInsecure(),
      {
        // reconnect fast after an agent restart (the default backoff grows to 2 min)
        'grpc.max_reconnect_backoff_ms': 5000,
        'grpc.initial_reconnect_backoff_ms': 200,
      },
    );
    return this.client;
  }

  /**
   * TD-10a (ARCH-01): call `onReady` whenever the channel to the agent becomes READY after it was not — at the first
   * connect and after every agent restart — so the commit engine can compare Health.last_txn_id with running. While
   * the agent is away the channel keeps reconnecting (backoff ≤ 5 s). Returns the stop function.
   */
  watchReady(onReady: () => void): () => void {
    let stopped = false;
    const client = this.c;
    const ch = client.getChannel();
    let last = ch.getConnectivityState(true);
    const loop = (): void => {
      if (stopped || this.client !== client) return;
      ch.watchConnectivityState(last, Date.now() + 30_000, () => {
        if (stopped || this.client !== client) return;
        const now = ch.getConnectivityState(true);
        if (now === connectivityState.READY && last !== connectivityState.READY) onReady();
        last = now;
        loop();
      });
    };
    if (last === connectivityState.READY) onReady();
    loop();
    return () => {
      stopped = true;
    };
  }

  private unary<Req, Res>(
    call: (
      req: Req,
      md: Metadata,
      opts: { deadline: Date },
      cb: (err: ServiceError | null, res: Res) => void,
    ) => unknown,
    req: Req,
    timeoutMs = this.env.VRX_AGENT_TIMEOUT_MS,
  ): Promise<Res> {
    return new Promise((resolve, reject) => {
      call.call(
        this.c,
        req,
        new Metadata(),
        { deadline: new Date(Date.now() + timeoutMs) },
        (err, res) => (err ? reject(agentProblem(err)) : resolve(res)),
      );
    });
  }

  /** `timeoutMs`: the commit engine's budget for this call (TD-10a, commit/budget.ts). */
  apply(
    req: Omit<ApplyRequest, 'owner'>,
    timeoutMs = this.env.VRX_AGENT_TIMEOUT_MS,
  ): Promise<ApplyResponse> {
    return this.unary(this.c.apply, { ...req, owner: this.owner }, timeoutMs);
  }

  dryRun(
    req: Omit<DryRunRequest, 'owner'>,
    timeoutMs = this.env.VRX_AGENT_TIMEOUT_MS,
  ): Promise<ValidationReport> {
    return this.unary(this.c.dryRun, { ...req, owner: this.owner }, timeoutMs);
  }

  retrieve(subsystems: string[] = []): Promise<RetrieveResponse> {
    return this.unary(this.c.retrieve, { subsystems, owner: this.owner });
  }

  health(timeoutMs = 5000): Promise<HealthResponse> {
    return this.unary(this.c.health, {}, timeoutMs);
  }

  streamStats(req: StreamStatsRequest): ClientReadableStream<StatsBatch> {
    return this.c.streamStats(req);
  }

  streamEvents(req: StreamEventsRequest): ClientReadableStream<Event> {
    return this.c.streamEvents(req);
  }

  close(): void {
    this.client?.close();
    this.client = undefined;
  }

  onModuleDestroy(): void {
    this.close();
  }
}

/** A desired state that carries nothing: the shape of an empty document on the wire. */
export type { DesiredState };

/** gRPC status → HTTP problem (the agent's application outcomes arrive with status OK and are handled by callers). */
export function agentProblem(err: ServiceError): ProblemError {
  const detail = `agent: ${err.details || err.message}`;
  switch (err.code) {
    case GrpcStatus.UNAVAILABLE:
      return new ProblemError(503, 'agent-unavailable', 'Agent unavailable', detail, undefined, {
        grpcCode: 'UNAVAILABLE',
      });
    case GrpcStatus.DEADLINE_EXCEEDED:
      return new ProblemError(504, 'agent-timeout', 'Agent timeout', detail);
    case GrpcStatus.FAILED_PRECONDITION:
      return new ProblemError(
        409,
        'agent-precondition',
        'Agent precondition failed',
        detail,
        undefined,
        {
          grpcCode: 'FAILED_PRECONDITION',
        },
      );
    case GrpcStatus.ABORTED:
      return new ProblemError(
        409,
        'agent-aborted',
        'Agent aborted the transaction',
        detail,
        undefined,
        {
          grpcCode: 'ABORTED',
        },
      );
    case GrpcStatus.UNIMPLEMENTED:
      return new ProblemError(501, 'agent-unimplemented', 'Not implemented by the agent', detail);
    case GrpcStatus.INVALID_ARGUMENT:
      return new ProblemError(
        502,
        'agent-rejected',
        'Agent rejected the request',
        detail,
        undefined,
        {
          grpcCode: 'INVALID_ARGUMENT',
        },
      );
    default:
      return new ProblemError(502, 'agent-error', 'Agent error', detail, undefined, {
        grpcCode: GrpcStatus[err.code] ?? String(err.code),
      });
  }
}
