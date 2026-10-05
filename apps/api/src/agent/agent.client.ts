import {
  connectivityState,
  credentials,
  Metadata,
  status as GrpcStatus,
  type ServiceError,
} from '@grpc/grpc-js';
import { Inject, Injectable, Logger, Optional, type OnModuleDestroy } from '@nestjs/common';
import {
  type AutoBlockSetRequest,
  type AutoBlockSetResponse,
  type ApplyRequest,
  type ApplyResponse,
  DataplaneClient,
  type DesiredState,
  type DryRunRequest,
  type Event,
  type HealthResponse,
  type SystemIdentityStateResponse,
  type InterfaceStateResponse,
  type PppoeReconnectResponse,
  type WanStateResponse,
  type MulticastStateResponse,
  type MplsLdpStateResponse,
  type RetrieveResponse,
  type StatsBatch,
  type StreamEventsRequest,
  type StreamStatsRequest,
  type ValidationReport,
  // Feature RPC types: one import line under the feature's anchor (wave-A-hotspots P4).
  // wave-BC: F-default-vpp-nics
  type HostNicsResponse,
  // wave-BC: F-det44-map-dslite-cnat
  type CnatSessionsRequest,
  type CnatSessionsResponse,
  type Det44LookupRequest,
  type Det44LookupResponse,
  type Det44SessionCloseAction,
  type Det44SessionsRequest,
  type Det44SessionsResponse,
  // wave-BC: F-tunnels
  type TunnelStateResponse,
  // wave-BC: F-vrrp-config-sync
  type VrrpStateResponse,
  // wave-BC: F-pki
  // wave-BC: F-ikev2-native
  // wave-BC: F-ospf
  // wave-BC: F-isis-rip
  // wave-BC: F-mpls-srmpls
  type MplsStateRequest,
  type MplsStateResponse,
  // wave-BC: F-lb
  type LbFlushVipResponse,
  type LbStateResponse,
  // wave-BC: F-qos-flat
  type QosPolicerResetRequest,
  type QosPolicerResetResponse,
  type QosPolicerStateRequest,
  type QosPolicerStateResponse,
  // wave-BC: F-host-stack
  type HostStackStateResponse,
  // wave-BC: F-snmp
  type SnmpStateResponse,
  type DataplaneStartupPreviewRequest, // F-dataplane-ui (unanchored)
  type DataplaneStartupPreviewResponse, // F-dataplane-ui (unanchored)
  type DataplaneStartupStateResponse, // F-dataplane-ui (unanchored)
  // wave-BC: F-ipfix-sflow
  type IpfixStateResponse,
  // wave-BC: F-capture-trace
  type CaptureAction,
  type CaptureDeleteResponse,
  type CaptureListResponse,
  // wave-BC: F-srv6
  type Srv6StateResponse,
  // wave-BC: F-lisp
  type LispStateResponse,
  // wave-BC: F-bfd-redistribution
  // wave-BC: F-ra-vpn
  type RemoteAccessCapabilitiesResponse,
  type RemoteAccessSessionsRequest,
  type RemoteAccessSessionsResponse,
  type RemoteAccessDisconnectRequest,
  type RemoteAccessDisconnectResponse,
  // wave-BC: F-mpls-ldp
  // wave-BC: F-igmp-mfib
  // wave-BC: F-dashboard-prom-alarms
  // wave-BC: F-ha-state-sync
  // wave-A: F-bonding
  type BondStateResponse,
  // wave-A: F-bridge-l2
  type BridgeDomainMacsRequest,
  type BridgeDomainMacsResponse,
  type BridgeDomainStateResponse,
  // wave-A: F-loopback-bvi-gso-lldp-span
  type LldpNeighborsRequest,
  type LldpNeighborsResponse,
  // wave-A: F-vrf-static-ecmp
  type ActionDone,
  type ActionOutput,
  type ActionRequest,
  type ListRoutesRequest,
  type ListRoutesResponse,
  // wave-A: F-neighbors-ra
  type ActionDone as NeighborsRaActionDone,
  type ActionOutput as NeighborsRaActionOutput,
  type ArpFlushAction,
  type ListNeighborsRequest,
  type ListNeighborsResponse,
  // wave-A: F-rpf-adl-pbr
  // wave-A: F-object-model
  type FqdnObjectStateResponse,
  // wave-A: F-acl
  type AclStateRequest,
  type AclStateResponse,
  // wave-A: F-host-acl-nftables
  type HostAclStateResponse,
  // wave-A: F-nat44-ed-sessions (ActionDone/ActionOutput imported above)
  type NatSessionKillAction,
  type NatSessionsRequest,
  type NatSessionsResponse,
  type NatSummaryResponse,
  // wave-A: F-nat44-ei-64-66-nptv6
  // wave-A: P11
  // wave-A: F-wireguard
  type WireguardStateResponse,
  // wave-A: P12
  type RoutingStateRequest,
  type RoutingStateResponse,
  // wave-A: F-kea-dhcp-relay
  type DhcpLeasesRequest,
  type DhcpLeasesResponse,
  // wave-A: F-unbound-chrony-syslog (ActionOutput imported above)
  type DnsLookupAction,
  type DnsStateResponse,
  type NtpStateResponse,
  type SyslogEntriesRequest,
  type SyslogEntriesResponse,
  type SyslogStateResponse,
  type IpsecStateResponse,
} from '@ngfw/proto';
import type { ClientReadableStream } from '@grpc/grpc-js';
import { SecretDeliveryService } from '../secrets/secret-delivery.service.js';
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
  private readonly captureCalls = new Map<string, ClientReadableStream<ActionOutput>>();
  private readonly captureLog = new Logger('Captures');

  constructor(
    @Inject(ENV) private readonly env: Env,
    @Optional() private readonly secretDelivery?: SecretDeliveryService,
  ) {}

  async resolveSecrets(desired: DesiredState, versions?: Readonly<Record<string, number>>) {
    return this.secretDelivery?.resolveVersioned(desired, versions);
  }

  get socket(): string {
    return this.env.NGFW_AGENT_SOCKET;
  }

  /** Owner stated on every request (proto.md §6): a request that reaches a foreign agent fails loudly. */
  get owner(): string {
    return this.env.NGFW_AGENT_OWNER;
  }

  private get c(): DataplaneClient {
    this.client ??= new DataplaneClient(
      `unix:${this.env.NGFW_AGENT_SOCKET}`,
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
    timeoutMs = this.env.NGFW_AGENT_TIMEOUT_MS,
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
  async apply(
    req: Omit<ApplyRequest, 'owner' | 'secretBundle'> & {
      secretBundle?: ApplyRequest['secretBundle'];
    },
    timeoutMs = this.env.NGFW_AGENT_TIMEOUT_MS,
  ): Promise<ApplyResponse> {
    const secretBundle =
      req.secretBundle ??
      (req.desiredState ? await this.secretDelivery?.resolve(req.desiredState) : undefined);
    return this.unary(this.c.apply, { ...req, secretBundle, owner: this.owner }, timeoutMs);
  }

  async autoBlockSet(entries: AutoBlockSetRequest['entries']): Promise<AutoBlockSetResponse> {
    return this.unary(this.c.autoBlockSet, { entries, owner: this.owner });
  }

  async dryRun(
    req: Omit<DryRunRequest, 'owner' | 'secretBundle'> & {
      secretBundle?: DryRunRequest['secretBundle'];
    },
    timeoutMs = this.env.NGFW_AGENT_TIMEOUT_MS,
  ): Promise<ValidationReport> {
    const secretBundle =
      req.secretBundle ??
      (req.desiredState ? await this.secretDelivery?.resolve(req.desiredState) : undefined);
    return this.unary(this.c.dryRun, { ...req, secretBundle, owner: this.owner }, timeoutMs);
  }

  retrieve(subsystems: string[] = []): Promise<RetrieveResponse> {
    return this.unary(this.c.retrieve, { subsystems, owner: this.owner });
  }

  /** Live interface table (P08, proto.md §8a); an agent without the RPC answers 501. */
  interfaceState(names: string[] = []): Promise<InterfaceStateResponse> {
    return this.unary(this.c.interfaceState, { names, owner: this.owner });
  }

  /** F-multiwan: live WAN group member health; an agent without multi-WAN answers 501. */
  wanState(groups: string[] = []): Promise<WanStateResponse> {
    return this.unary(this.c.wanState, { groups, owner: this.owner });
  }

  /** F-igmp-mfib: live IGMP groups, mFIB and PIM neighbours; an agent without multicast answers 501. */
  multicastState(): Promise<MulticastStateResponse> {
    return this.unary(this.c.multicastState, { owner: this.owner });
  }

  /** F-mpls-ldp: live LDP neighbours, LIB bindings and the FRR→VPP sync status; an agent without LDP answers 501. */
  vrrpState(): Promise<VrrpStateResponse> {
    return this.unary(this.c.vrrpState, { owner: this.owner });
  }
  mplsLdpState(): Promise<MplsLdpStateResponse> {
    return this.unary(this.c.mplsLdpState, { owner: this.owner });
  }

  /** F-pppoe-client: redial a PPPoE client now; an agent without PPPoE support answers 501. */
  pppoeReconnect(iface: string): Promise<PppoeReconnectResponse> {
    return this.unary(this.c.pppoeReconnect, { interface: iface, owner: this.owner });
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
  // wave-BC: F-default-vpp-nics
  /** F-default-vpp-nics: enumerate the host's physical NICs so the API can seed the default dataplane document. */
  hostNics(): Promise<HostNicsResponse> {
    return this.unary(this.c.hostNics, { owner: this.owner });
  }

  // wave-BC: F-det44-map-dslite-cnat
  /** F-det44-map-dslite-cnat: one page of one DET44 user's sessions + the user's port block. */
  det44Sessions(req: Omit<Det44SessionsRequest, 'owner'>): Promise<Det44SessionsResponse> {
    return this.unary(this.c.det44Sessions, { ...req, owner: this.owner });
  }
  /** F-det44-map-dslite-cnat: DET44 forward / reverse lookup (read-only). */
  det44Lookup(req: Omit<Det44LookupRequest, 'owner'>): Promise<Det44LookupResponse> {
    return this.unary(this.c.det44Lookup, { ...req, owner: this.owner });
  }
  /** F-det44-map-dslite-cnat: one page of the CNAT session table. */
  cnatSessions(req: Omit<CnatSessionsRequest, 'owner'>): Promise<CnatSessionsResponse> {
    return this.unary(this.c.cnatSessions, { ...req, owner: this.owner });
  }
  /** F-det44-map-dslite-cnat: close one DET44 session (Action stream); resolves with its `done`. */
  det44SessionClose(a: Det44SessionCloseAction): Promise<ActionDone> {
    return this.cgnatDone({ det44SessionClose: a });
  }
  /** F-det44-map-dslite-cnat: purge the CNAT session table (Action stream; globals owner only). */
  cnatSessionPurge(): Promise<ActionDone> {
    return this.cgnatDone({ cnatSessionPurge: {} });
  }
  private async cgnatDone(req: ActionRequest): Promise<ActionDone> {
    const r = await this.runAction(req, this.env.NGFW_AGENT_TIMEOUT_MS);
    if (r.done === undefined)
      throw new ProblemError(
        502,
        'agent-error',
        'Agent error',
        'agent: the action ended without a result',
      );
    return r.done;
  }
  // wave-BC: F-tunnels
  tunnelState(): Promise<TunnelStateResponse> {
    return this.unary(this.c.tunnelState, { owner: this.owner });
  }

  // wave-BC: F-vrrp-config-sync
  // wave-BC: F-pki
  // wave-BC: F-ikev2-native
  // wave-BC: F-ospf
  // wave-BC: F-isis-rip
  // wave-BC: F-mpls-srmpls
  /** Live MPLS state (F-mpls-srmpls; proto.md "F-mpls-srmpls: MplsState"): the FIB paged in the agent, or the tunnels. */
  mplsState(req: Omit<MplsStateRequest, 'owner'>): Promise<MplsStateResponse> {
    return this.unary(this.c.mplsState, { ...req, owner: this.owner });
  }
  // wave-BC: F-lb
  /** F-lb: live lb state of the configured VIPs (proto.md "F-lb"); an agent without the RPC answers 501. */
  lbState(names: string[] = []): Promise<LbStateResponse> {
    return this.unary(this.c.lbState, { names, owner: this.owner });
  }
  /** F-lb: flush the sticky flow table of one configured VIP (lb_flush_vip). */
  lbFlushVip(name: string): Promise<LbFlushVipResponse> {
    return this.unary(this.c.lbFlushVip, { name, owner: this.owner });
  }
  // wave-BC: F-qos-flat
  /** F-qos-flat (proto.md §11): this owner's policers and shapers as VPP reports them, with their counters. */
  qosPolicerState(req: Omit<QosPolicerStateRequest, 'owner'>): Promise<QosPolicerStateResponse> {
    return this.unary(this.c.qosPolicerState, { ...req, owner: this.owner });
  }
  /** F-qos-flat (proto.md §11): refill one policer's token buckets (policer_reset). */
  qosPolicerReset(req: Omit<QosPolicerResetRequest, 'owner'>): Promise<QosPolicerResetResponse> {
    return this.unary(this.c.qosPolicerReset, { ...req, owner: this.owner });
  }
  // wave-BC: F-host-stack
  hostStackState(): Promise<HostStackStateResponse> {
    return this.unary(this.c.hostStackState, { owner: this.owner });
  }
  // wave-BC: F-snmp
  snmpState(): Promise<SnmpStateResponse> {
    return this.unary(this.c.snmpState, { owner: this.owner });
  }
  // wave-BC: F-ipfix-sflow
  ipfixState(): Promise<IpfixStateResponse> {
    return this.unary(this.c.ipfixState, { owner: this.owner });
  }
  // wave-BC: F-capture-trace
  /** F-capture-trace: kept pcap files, the running capture, trace/PG availability. */
  captureList(): Promise<CaptureListResponse> {
    return this.unary(this.c.captureList, { owner: this.owner });
  }
  /** F-capture-trace: remove one kept pcap file. */
  captureDelete(id: string): Promise<CaptureDeleteResponse> {
    return this.unary(this.c.captureDelete, { owner: this.owner, id });
  }
  /** F-capture-trace: one kept pcap file, collected from the CaptureRead stream (≤ the agent's byte cap). */
  captureRead(id: string, timeoutMs = 60_000): Promise<Buffer> {
    return new Promise((resolve, reject) => {
      const parts: Buffer[] = [];
      const call = this.c.captureRead({ owner: this.owner, id }, new Metadata(), {
        deadline: new Date(Date.now() + timeoutMs),
      });
      call.on('data', (c: { data: Uint8Array }) => parts.push(Buffer.from(c.data)));
      call.on('error', (e: ServiceError) => reject(agentProblem(e)));
      call.on('end', () => resolve(Buffer.concat(parts)));
    });
  }
  /**
   * F-capture-trace: start a capture (Action member 3). Resolves with the capture id from the agent's first line
   * ("capture <id> started"); the stream keeps running in the background until the capture stops (its end or error
   * after the start is only logged — the result is read back with captureList).
   */
  startCapture(req: CaptureAction, timeoutMs = 700_000): Promise<string> {
    return new Promise((resolve, reject) => {
      let started = false;
      let captureId = '';
      const call = this.c.action({ capture: req }, new Metadata(), {
        deadline: new Date(Date.now() + timeoutMs),
      });
      call.on('data', (o: ActionOutput) => {
        const m = /^capture (\S+) started$/.exec(o.line ?? '');
        if (!started && m?.[1]) {
          started = true;
          captureId = m[1];
          this.captureCalls.set(captureId, call);
          resolve(captureId);
        }
      });
      call.on('error', (e: ServiceError) => {
        this.captureCalls.delete(captureId);
        if (!started) reject(agentProblem(e));
        else if (e.code !== GrpcStatus.CANCELLED)
          this.captureLog.warn({
            captureId,
            grpcCode: e.code,
            message: 'Capture stream failed after start',
          });
      });
      call.on('end', () => {
        this.captureCalls.delete(captureId);
        if (!started)
          reject(
            new ProblemError(
              502,
              'agent-error',
              'Agent error',
              'agent: capture ended before it started',
            ),
          );
      });
    });
  }
  /** Cancel only a capture stream held by this API process. */
  async stopCapture(id: string): Promise<void> {
    const call = this.captureCalls.get(id);
    if (call) {
      this.captureCalls.delete(id);
      call.cancel();
      return;
    }
    const entry = (await this.captureList()).captures.find((capture) => capture.id === id);
    if (!entry) throw new ProblemError(404, 'not-found', 'Not found', 'no such capture');
    throw new ProblemError(
      409,
      'capture-not-running',
      'Conflict',
      'capture is not running in this API process',
    );
  }
  // wave-BC: F-srv6
  /** F-srv6: live SRv6 state (proto.md §11); callers do not poll faster than every 30 s (D-132). */
  srv6State(): Promise<Srv6StateResponse> {
    return this.unary(this.c.srv6State, { owner: this.owner });
  }
  // wave-BC: F-lisp
  /** Live LISP state (F-lisp); an agent without the RPC answers 501. */
  lispState(): Promise<LispStateResponse> {
    return this.unary(this.c.lispState, { owner: this.owner });
  }
  // wave-BC: F-bfd-redistribution
  // wave-BC: F-ra-vpn
  remoteAccessCapabilities(): Promise<RemoteAccessCapabilitiesResponse> {
    return this.unary(this.c.remoteAccessCapabilities, {});
  }
  remoteAccessSessions(request: RemoteAccessSessionsRequest): Promise<RemoteAccessSessionsResponse> {
    return this.unary(this.c.remoteAccessSessions, request);
  }
  remoteAccessDisconnect(request: RemoteAccessDisconnectRequest): Promise<RemoteAccessDisconnectResponse> {
    return this.unary(this.c.remoteAccessDisconnect, request);
  }
  // wave-BC: F-mpls-ldp
  // wave-BC: F-igmp-mfib
  // wave-BC: F-dashboard-prom-alarms
  // wave-BC: F-ha-state-sync
  // wave-A: F-bonding
  /** F-bonding: live bonds (proto.md §11); an agent without the RPC answers 501. */
  bondState(names: string[] = []): Promise<BondStateResponse> {
    return this.unary(this.c.bondState, { names, owner: this.owner });
  }

  // wave-A: F-bridge-l2
  /** F-bridge-l2: live bridge domains (proto.md §11); an agent without the RPC answers 501. */
  bridgeDomainState(ids: number[] = []): Promise<BridgeDomainStateResponse> {
    return this.unary(this.c.bridgeDomainState, { ids, owner: this.owner });
  }

  /** F-bridge-l2: one page (≤ 1000) of a bridge domain's L2 FIB. */
  bridgeDomainMacs(req: Omit<BridgeDomainMacsRequest, 'owner'>): Promise<BridgeDomainMacsResponse> {
    return this.unary(this.c.bridgeDomainMacs, { ...req, owner: this.owner });
  }

  // wave-A: F-loopback-bvi-gso-lldp-span
  /** F-loopback-bvi-gso-lldp-span: one page (≤ 1000) of the LLDP table (proto.md §11); no RPC → 501. */
  lldpNeighbors(req: Omit<LldpNeighborsRequest, 'owner'>): Promise<LldpNeighborsResponse> {
    return this.unary(this.c.lldpNeighbors, { ...req, owner: this.owner });
  }

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
  /** F-neighbors-ra: one page of the live ARP/ND table (ListNeighbors, proto.md §11). */
  listNeighbors(req: Omit<ListNeighborsRequest, 'owner'>): Promise<ListNeighborsResponse> {
    return this.unary(this.c.listNeighbors, { ...req, owner: this.owner });
  }

  /** F-neighbors-ra: run the arp_flush action (ActionRequest 4) and collect its lines and `done`. */
  arpFlush(
    req: ArpFlushAction,
    timeoutMs = this.env.NGFW_AGENT_TIMEOUT_MS,
  ): Promise<{ lines: string[]; done: NeighborsRaActionDone | undefined }> {
    return new Promise((resolve, reject) => {
      const call = this.c.action({ arpFlush: req }, new Metadata(), {
        deadline: new Date(Date.now() + timeoutMs),
      });
      const lines: string[] = [];
      let done: NeighborsRaActionDone | undefined;
      call.on('data', (o: NeighborsRaActionOutput) => {
        if (o.line !== undefined) lines.push(o.line);
        if (o.done !== undefined) done = o.done;
      });
      call.on('error', (err: ServiceError) => reject(agentProblem(err)));
      call.on('end', () => resolve({ lines, done }));
    });
  }
  // wave-A: F-rpf-adl-pbr
  // wave-A: F-object-model
  /** FQDN resolver state of the agent's FQDN address objects (F-object-model); an older agent answers 501. */
  fqdnObjectState(names: string[] = []): Promise<FqdnObjectStateResponse> {
    return this.unary(this.c.fqdnObjectState, { names, owner: this.owner });
  }
  // wave-A: F-acl
  /** ACL runtime state (F-acl, proto.md §11): list summaries, a counter page, bindings; an older agent → 501. */
  aclState(req: Omit<AclStateRequest, 'owner'>): Promise<AclStateResponse> {
    return this.unary(this.c.aclState, { ...req, owner: this.owner });
  }
  // wave-A: F-host-acl-nftables
  /** Host firewall table rendered from acl.host* with per-rule counters (F-host-acl-nftables); an older agent answers 501. */
  hostAclState(): Promise<HostAclStateResponse> {
    return this.unary(this.c.hostAclState, { owner: this.owner });
  }
  // wave-A: F-nat44-ed-sessions
  /** F-nat44-ed-sessions: one bounded page of NAT44-ED sessions (limit ≤ 1000, proto.md §11). */
  natSessions(req: Omit<NatSessionsRequest, 'owner'>): Promise<NatSessionsResponse> {
    return this.unary(this.c.natSessions, { ...req, owner: this.owner });
  }

  /** F-nat44-ed-sessions: NAT44-ED totals and per-pool usage. */
  natSummary(): Promise<NatSummaryResponse> {
    return this.unary(this.c.natSummary, { owner: this.owner });
  }

  /** F-nat44-ed-sessions: the NatSessionKillAction through the Action stream; resolves with its `done`. */
  natSessionKill(
    a: NatSessionKillAction,
    timeoutMs = this.env.NGFW_AGENT_TIMEOUT_MS,
  ): Promise<ActionDone> {
    return new Promise((resolve, reject) => {
      const call = this.c.action({ natSessionKill: a }, new Metadata(), {
        deadline: new Date(Date.now() + timeoutMs),
      });
      let done: ActionDone | undefined;
      let failed = false;
      call.on('data', (o: ActionOutput) => {
        if (o.done !== undefined) done = o.done;
      });
      call.on('error', (e: ServiceError) => {
        failed = true;
        reject(agentProblem(e));
      });
      call.on('end', () => {
        if (failed) return;
        if (done !== undefined) resolve(done);
        else
          reject(
            new ProblemError(
              502,
              'agent-error',
              'Agent error',
              'agent: the action stream ended without done',
            ),
          );
      });
    });
  }
  // wave-A: F-nat44-ei-64-66-nptv6
  // wave-A: P11
  // wave-A: F-wireguard
  /** F-wireguard: live WireGuard state (proto.md §11); callers do not poll faster than every 30 s (D-132). */
  wireguardState(interfaces: string[] = []): Promise<WireguardStateResponse> {
    return this.unary(this.c.wireguardState, { interfaces, owner: this.owner });
  }
  // wave-A: P12
  /** P12: live routing-daemon state (BGP, FRR RIB counts / lookups, linux-cp pairs) — RoutingState. */
  routingState(req: Omit<RoutingStateRequest, 'owner'>): Promise<RoutingStateResponse> {
    return this.unary(this.c.routingState, { ...req, owner: this.owner });
  }
  // wave-A: F-kea-dhcp-relay
  /** F-kea-dhcp-relay (proto.md §11): Kea lease pages + daemon status, or one interface's DHCPv4 client state. */
  dhcpLeases(req: Omit<DhcpLeasesRequest, 'owner'>): Promise<DhcpLeasesResponse> {
    return this.unary(this.c.dhcpLeases, { ...req, owner: this.owner });
  }
  // wave-A: F-unbound-chrony-syslog
  /** F-unbound-chrony-syslog: Unbound instance state (read-only). */
  dnsState(): Promise<DnsStateResponse> {
    return this.unary(this.c.dnsState, { owner: this.owner });
  }
  /** F-unbound-chrony-syslog: chronyd state (read-only). */
  systemIdentityState(): Promise<SystemIdentityStateResponse> {
    return this.unary(this.c.systemIdentityState, { owner: this.owner });
  }

  ntpState(): Promise<NtpStateResponse> {
    return this.unary(this.c.ntpState, { owner: this.owner });
  }
  /** F-unbound-chrony-syslog: remote-syslog export counters (read-only). */
  syslogState(): Promise<SyslogStateResponse> {
    return this.unary(this.c.syslogState, { owner: this.owner });
  }
  /** F-unbound-chrony-syslog: one page of the log explorer (bounded journal query). */
  syslogEntries(req: Omit<SyslogEntriesRequest, 'owner'>): Promise<SyslogEntriesResponse> {
    return this.unary(this.c.syslogEntries, { ...req, owner: this.owner });
  }
  // F-dataplane-ui (unanchored): installed VPP start-up file + host facts; startup.conf preview (read-only).
  dataplaneStartupState(): Promise<DataplaneStartupStateResponse> {
    return this.unary(this.c.dataplaneStartupState, { owner: this.owner });
  }
  dataplaneStartupPreview(
    req: Omit<DataplaneStartupPreviewRequest, 'owner'>,
  ): Promise<DataplaneStartupPreviewResponse> {
    return this.unary(this.c.dataplaneStartupPreview, { ...req, owner: this.owner });
  }
  /** F-unbound-chrony-syslog: ActionRequest.dns_lookup; collects the whole (short) output stream. */
  dnsLookup(
    req: DnsLookupAction,
    timeoutMs = this.env.NGFW_AGENT_TIMEOUT_MS,
  ): Promise<ActionOutput[]> {
    return new Promise((resolve, reject) => {
      const out: ActionOutput[] = [];
      const stream = this.c.action({ dnsLookup: req }, new Metadata(), {
        deadline: new Date(Date.now() + timeoutMs),
      });
      stream.on('data', (o: ActionOutput) => out.push(o));
      stream.on('error', (err: ServiceError) => reject(agentProblem(err)));
      stream.on('end', () => resolve(out));
    });
  }

  close(): void {
    for (const call of this.captureCalls.values()) call.cancel();
    this.captureCalls.clear();
    this.client?.close();
    this.client = undefined;
  }

  onModuleDestroy(): void {
    this.close();
  }
  ipsecState(tunnels: string[] = [], offset = 0, limit = 0): Promise<IpsecStateResponse> {
    return this.unary(this.c.ipsecState, { tunnels, owner: this.owner, offset, limit });
  }
}

/** A desired state that carries nothing: the shape of an empty document on the wire. */
export type { DesiredState };

/** gRPC status → HTTP problem (the agent's application outcomes arrive with status OK and are handled by callers). */
export function agentProblem(err: ServiceError): ProblemError {
  const detail = `agent: ${err.details || err.message}`;
  switch (err.code) {
    case GrpcStatus.PERMISSION_DENIED:
      return new ProblemError(
        403,
        'agent-permission-denied',
        'Agent permission denied',
        'The agent refused permission to perform this operation',
        undefined,
        { grpcCode: 'PERMISSION_DENIED' },
      );
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
