import { credentials, Metadata, status as GrpcStatus, type ServiceError } from '@grpc/grpc-js';
import { Inject, Injectable, type OnModuleDestroy } from '@nestjs/common';
import {
  type ApplyRequest,
  type ApplyResponse,
  DataplaneClient,
  type DesiredState,
  type DryRunRequest,
  type Event,
  type HealthResponse,
  type InterfaceStateResponse,
  type RetrieveResponse,
  type StatsBatch,
  type StreamEventsRequest,
  type StreamStatsRequest,
  type ValidationReport,
  // Feature RPC types: one import line under the feature's anchor (wave-A-hotspots P4).
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
  // wave-BC: F-snmp
  // wave-BC: F-ipfix-sflow
  // wave-BC: F-capture-trace
  // wave-BC: F-srv6
  // wave-BC: F-lisp
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
  type ActionDone,
  type ActionOutput,
  type ActionRequest,
  type ListRoutesRequest,
  type ListRoutesResponse,
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
  type RoutingStateRequest,
  type RoutingStateResponse,
  // wave-A: F-kea-dhcp-relay
  // wave-A: F-unbound-chrony-syslog
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

  apply(req: Omit<ApplyRequest, 'owner'>): Promise<ApplyResponse> {
    return this.unary(this.c.apply, { ...req, owner: this.owner });
  }

  dryRun(req: Omit<DryRunRequest, 'owner'>): Promise<ValidationReport> {
    return this.unary(this.c.dryRun, { ...req, owner: this.owner });
  }

  retrieve(subsystems: string[] = []): Promise<RetrieveResponse> {
    return this.unary(this.c.retrieve, { subsystems, owner: this.owner });
  }

  /** Live interface table (P08, proto.md §8a); an agent without the RPC answers 501. */
  interfaceState(names: string[] = []): Promise<InterfaceStateResponse> {
    return this.unary(this.c.interfaceState, { names, owner: this.owner });
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

  // Feature RPCs: one method per RPC under the feature's anchor, e.g.
  // `natSessions(req: …): Promise<…> { return this.unary(this.c.natSessions, { ...req, owner: this.owner }); }`
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
  // wave-BC: F-snmp
  // wave-BC: F-ipfix-sflow
  // wave-BC: F-capture-trace
  // wave-BC: F-srv6
  // wave-BC: F-lisp
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
  /** One page of one VRF's live FIB (F-vrf-static-ecmp; proto.md §11): paging and filtering happen in the agent. */
  listRoutes(req: Omit<ListRoutesRequest, 'owner'>): Promise<ListRoutesResponse> {
    return this.unary(this.c.listRoutes, { ...req, owner: this.owner });
  }

  /**
   * Runs one diagnostic (the Action RPC) to its end and collects the stream: every `line`, the number of pcap bytes and
   * the terminal `done`. gRPC failures become problems (agentProblem); the generic `/actions/:action` bridge and the
   * features that serve their own action routes (F-neighbors-ra, F-nat44-ed-sessions) share it.
   */
  runAction(
    req: ActionRequest,
    timeoutMs = 60_000,
  ): Promise<{ lines: string[]; pcapBytes: number; done: ActionDone | undefined }> {
    return new Promise((resolve, reject) => {
      const lines: string[] = [];
      let pcapBytes = 0;
      let done: ActionDone | undefined;
      const call = this.c.action(req, new Metadata(), {
        deadline: new Date(Date.now() + timeoutMs),
      });
      call.on('data', (o: ActionOutput) => {
        if (o.line !== undefined) lines.push(o.line);
        if (o.pcapChunk !== undefined) pcapBytes += o.pcapChunk.length;
        if (o.done !== undefined) done = o.done;
      });
      call.on('error', (e: ServiceError) => reject(agentProblem(e)));
      call.on('end', () => resolve({ lines, pcapBytes, done }));
    });
  }
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
  /** P12: live routing-daemon state (BGP, FRR RIB counts / lookups, linux-cp pairs) — RoutingState. */
  routingState(req: Omit<RoutingStateRequest, 'owner'>): Promise<RoutingStateResponse> {
    return this.unary(this.c.routingState, { ...req, owner: this.owner });
  }
  // wave-A: F-kea-dhcp-relay
  // wave-A: F-unbound-chrony-syslog

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
